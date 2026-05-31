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

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("url-service")
}

func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

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

func provideIDGenerator(cfg *config.Config) (*idgen.Generator, error) {
	return idgen.NewGenerator(cfg.Snowflake.DatacenterID, cfg.Snowflake.WorkerID)
}

func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

func provideCache(cfg *config.Config, rc *redislib.Client) *cache.Cache {
	return cache.NewMultiTierCache(cfg.Cache.L1Capacity, rc, cfg.Cache.L2TTL)
}

func provideStorage(db *database.DBManager) *storage.PostgresStorage {
	return storage.NewPostgresStorage(db)
}

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

func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "url-service",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

func provideGRPCServer() *grpc.Server {
	return grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
}

func provideListener() (net.Listener, error) {
	return net.Listen("tcp", ":50051")
}

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
			tracing.ShutdownTracer(ctx, tp)
			redisClient.Close()
			dbManager.Close()
			return nil
		},
	})
}

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
