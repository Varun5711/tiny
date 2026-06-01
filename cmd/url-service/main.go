// Package main implements the URL service for the Tiny URL shortener.
//
// The URL service is a gRPC microservice responsible for all URL CRUD
// operations: creating short links (with auto-generated or custom aliases),
// resolving short codes to original URLs, listing a user's URLs, and
// deleting them. Short codes are derived from Twitter-style Snowflake IDs
// to guarantee global uniqueness without coordination.
//
// Lookups are accelerated by a two-tier cache (in-process LRU + Redis) so
// the hot path for redirect resolution avoids hitting PostgreSQL. When
// Elasticsearch is available, newly created URLs are indexed for full-text
// search. Click events are published to a Redis Stream by the
// redirect-service; this service only owns the URL metadata.
//
// Dependency injection is managed by Uber FX.
package main

import (
	"context"
	"net"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	es "github.com/Varun5711/shorternit/internal/elasticsearch"
	"github.com/Varun5711/shorternit/internal/idgen"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/service"
	"github.com/Varun5711/shorternit/internal/storage"
	"github.com/Varun5711/shorternit/internal/tracing"
	pb "github.com/Varun5711/shorternit/proto/url"
	redislib "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

// provideConfig loads the unified application configuration from environment
// variables and config files.
func provideConfig() (*config.Config, error) {
	return config.Load()
}

// provideLogger creates a structured logger tagged with "url-service" so log
// output is identifiable in centralized logging.
func provideLogger() *logger.Logger {
	return logger.New("url-service")
}

// provideRedisClient connects to Redis, which serves double duty here: as
// the L2 tier of the URL lookup cache and as the backing store for the raw
// client used by the URL service to publish events.
func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

// provideDBManager sets up a PostgreSQL connection pool with primary/replica
// topology. The primary handles writes (create, delete); replicas serve
// read-heavy operations like listing a user's URLs.
func provideDBManager(cfg *config.Config) (*database.DBManager, error) {
	return database.NewDBManager(context.Background(), database.Config{
		PrimaryDSN:      cfg.Database.PrimaryDSN,
		ReplicaDSNs:     cfg.Database.ReplicaDSNs,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	})
}

// provideIDGenerator creates a Snowflake ID generator configured with a
// unique datacenter/worker pair. Snowflake IDs are base62-encoded to produce
// short, URL-safe codes without requiring a centralized sequence counter.
func provideIDGenerator(cfg *config.Config) (*idgen.Generator, error) {
	return idgen.NewGenerator(cfg.Snowflake.DatacenterID, cfg.Snowflake.WorkerID)
}

// provideRawRedisClient unwraps the internal RedisClient to expose the
// underlying go-redis *Client. The URL service needs the raw client to
// publish click events to Redis Streams.
func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

// provideCache builds a two-tier URL lookup cache: an in-process LRU (L1)
// for sub-microsecond hits, backed by Redis (L2) for cross-instance
// consistency. This keeps redirect latency low even under heavy traffic.
func provideCache(cfg *config.Config, rc *redislib.Client) *cache.Cache {
	return cache.NewMultiTierCache(cfg.Cache.L1Capacity, rc, cfg.Cache.L2TTL)
}

// provideStorage creates the PostgreSQL-backed URL storage layer. All SQL
// queries for URL CRUD are encapsulated here, keeping the service layer
// free of database concerns.
func provideStorage(db *database.DBManager) *storage.PostgresStorage {
	return storage.NewPostgresStorage(db)
}

// provideESClient optionally connects to Elasticsearch for indexing new
// URLs so they are searchable via the API gateway. Returns nil when ES is
// disabled or unreachable, which gracefully disables search indexing.
func provideESClient(cfg *config.Config, log *logger.Logger) *es.Client {
	if !cfg.Elasticsearch.Enabled {
		return nil
	}
	client, err := es.NewClient(es.Config{
		Addresses:   cfg.Elasticsearch.Addresses,
		Username:    cfg.Elasticsearch.Username,
		Password:    cfg.Elasticsearch.Password,
		IndexPrefix: cfg.Elasticsearch.IndexPrefix,
	})
	if err != nil {
		log.Warn("Elasticsearch unavailable, running without search: %v", err)
		return nil
	}
	return client
}

// provideURLService assembles the core business logic layer. It combines
// storage, ID generation, caching, Redis Streams (for click event
// publishing), and Elasticsearch indexing into a single gRPC-compatible
// service implementation.
func provideURLService(
	store *storage.PostgresStorage,
	idGen *idgen.Generator,
	urlCache *cache.Cache,
	rc *redislib.Client,
	esClient *es.Client,
	cfg *config.Config,
) *service.URLService {
	return service.NewURLService(store, idGen, urlCache, rc, esClient, cfg.Services.BaseURL, cfg.Services.DefaultURLTTL)
}

// provideTracerProvider initializes OpenTelemetry distributed tracing and
// exports spans to Jaeger. Traces propagate through gRPC metadata so a
// request can be followed from the API gateway into this service.
func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "url-service",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

// provideGRPCServer creates a gRPC server with OpenTelemetry instrumentation.
// The otelgrpc stats handler automatically creates spans for every inbound
// RPC and propagates trace context from the caller.
func provideGRPCServer() *grpc.Server {
	return grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
}

// provideListener binds a TCP listener on port 50051, the well-known port
// for the URL service in this architecture. Other services (api-gateway,
// redirect-service) connect to this port via gRPC.
func provideListener() (net.Listener, error) {
	return net.Listen("tcp", ":50051")
}

// registerLifecycle wires the gRPC server into the FX lifecycle. On start,
// it registers the URLService implementation and begins serving RPCs in a
// background goroutine. On stop, it performs a graceful shutdown: the gRPC
// server drains in-flight requests, then the tracer, Redis, and database
// connections are closed in order.
func registerLifecycle(
	lc fx.Lifecycle,
	grpcServer *grpc.Server,
	urlService *service.URLService,
	listener net.Listener,
	tp *sdktrace.TracerProvider,
	redisClient *redis.RedisClient,
	dbManager *database.DBManager,
	log *logger.Logger,
) {
	pb.RegisterURLServiceServer(grpcServer, urlService)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Listening on :50051")
			go func() {
				if err := grpcServer.Serve(listener); err != nil {
					log.Error("Server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Shutting down url-service...")
			grpcServer.GracefulStop()
			_ = tracing.ShutdownTracer(ctx, tp)
			_ = redisClient.Close()
			dbManager.Close()
			return nil
		},
	})
}

// main assembles the complete FX dependency graph for the URL service.
//
// The graph flows from infrastructure (config, logging, tracing, Redis,
// Postgres) through domain components (Snowflake ID generator, multi-tier
// cache, storage, Elasticsearch) up to the URLService that implements the
// gRPC proto/url interface. fx.Invoke(registerLifecycle) triggers graph
// construction and starts the gRPC server. Run() blocks until a
// termination signal is received.
func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
			provideTracerProvider,
			provideRedisClient,
			provideDBManager,
			provideIDGenerator,
			provideRawRedisClient,
			provideCache,
			provideStorage,
			provideESClient,
			provideURLService,
			provideGRPCServer,
			provideListener,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
