// Package main implements the API gateway for the Tiny URL shortener.
//
// The API gateway is the single public-facing HTTP entry point for all client
// requests. It routes REST API calls to the appropriate backend gRPC services
// (url-service for CRUD, user-service for authentication) and serves analytics
// data from ClickHouse. The gateway also applies cross-cutting concerns like
// CORS, rate limiting, distributed tracing, request IDs, and panic recovery
// through a layered middleware stack.
//
// Dependency injection is managed by Uber FX, which wires infrastructure
// clients, gRPC stubs, HTTP handlers, and middleware into a single lifecycle.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/analytics"
	"github.com/Varun5711/shorternit/internal/clickhouse"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	es "github.com/Varun5711/shorternit/internal/elasticsearch"
	"github.com/Varun5711/shorternit/internal/handlers"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/middleware"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/tracing"
	userpb "github.com/Varun5711/shorternit/proto/user"
	redislib "github.com/redis/go-redis/v9"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ---------------------------------------------------------------------------
// Provider functions — infrastructure
// ---------------------------------------------------------------------------

// provideConfig loads the unified application configuration from environment
// variables and config files. Every other provider that needs tunable values
// depends on this.
func provideConfig() (*config.Config, error) {
	return config.Load()
}

// provideLogger creates the structured logger tagged with "api-gateway" so
// log output from this service is easily distinguishable in aggregated logs.
func provideLogger() *logger.Logger {
	return logger.New("api-gateway")
}

// provideRedisClient establishes a connection to the shared Redis instance.
// Redis is used by the gateway for rate limiting (via the raw client) and
// health-check pings.
func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

// provideDBManager sets up a PostgreSQL connection pool with primary/replica
// topology. The gateway needs direct DB access for the analytics service,
// which queries click-count aggregates stored in PostgreSQL.
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

// provideClickHouseClient connects to ClickHouse for high-volume click
// analytics queries (timeline, geo, device breakdown). ClickHouse is the
// OLAP store that the pipeline-worker writes enriched events into.
func provideClickHouseClient(cfg *config.Config) (*clickhouse.Client, error) {
	return clickhouse.NewClient(cfg.ClickHouse)
}

// provideUserGRPCConn dials the user-service gRPC endpoint. The address is
// read from USER_SERVICE_ADDR and defaults to localhost:50052 for local
// development. Insecure credentials are used because services communicate
// over an internal network (TLS termination happens at the edge).
func provideUserGRPCConn() (*grpc.ClientConn, error) {
	addr := os.Getenv("USER_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:50052"
	}
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// provideRawRedisClient unwraps the internal RedisClient to expose the
// underlying go-redis *Client. The rate limiter middleware needs the raw
// client for its sliding-window Lua script.
func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

// ---------------------------------------------------------------------------
// Provider functions — gRPC clients
// ---------------------------------------------------------------------------

// provideUserServiceClient creates a typed gRPC stub for the user-service.
// This client is injected into the auth handler and auth middleware so they
// can validate JWTs and fetch user profiles without direct DB access.
func provideUserServiceClient(conn *grpc.ClientConn) userpb.UserServiceClient {
	return userpb.NewUserServiceClient(conn)
}

// ---------------------------------------------------------------------------
// Provider functions — handlers & middleware
// ---------------------------------------------------------------------------

// provideHTTPHandler creates the URL CRUD handler that proxies requests to
// the url-service over gRPC. It also receives the Elasticsearch client for
// URL search functionality; if ES is nil, search endpoints return 501.
func provideHTTPHandler(cfg *config.Config, esClient *es.Client) (*handlers.HTTPHandler, error) {
	return handlers.NewHTTPHandler(cfg.Services.URLServiceAddr, cfg.Services.BaseURL, esClient)
}

// provideAuthHandler creates the handler for /api/auth/* endpoints
// (register, login, profile). It delegates all authentication logic to the
// user-service via gRPC, keeping the gateway stateless.
func provideAuthHandler(userClient userpb.UserServiceClient) *handlers.AuthHandler {
	return handlers.NewAuthHandler(userClient)
}

// provideAnalyticsService creates the PostgreSQL-backed analytics service
// that reads pre-aggregated click counts. This complements the ClickHouse
// analytics with lightweight summary queries.
func provideAnalyticsService(db *database.DBManager) *analytics.Service {
	return analytics.NewService(db)
}

// provideAnalyticsHandler wires together the PostgreSQL analytics service
// and ClickHouse client into a single handler that serves all
// /api/analytics/* endpoints (stats, timeline, geo, devices, referrers).
func provideAnalyticsHandler(svc *analytics.Service, ch *clickhouse.Client) *handlers.AnalyticsHandler {
	return handlers.NewAnalyticsHandler(svc, ch)
}

// provideAuthMiddleware creates JWT-validation middleware that calls the
// user-service to verify tokens. Protected routes wrap their handlers with
// RequireAuth, which populates the request context with the authenticated
// user ID.
func provideAuthMiddleware(userClient userpb.UserServiceClient) *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(userClient)
}

// provideRateLimiter builds a Redis-backed sliding-window rate limiter.
// Limits are configured per-IP and enforced globally across gateway
// instances because state is stored in Redis, not in-process memory.
func provideRateLimiter(cfg *config.Config, rc *redislib.Client) *middleware.RateLimiter {
	return middleware.NewRateLimiter(rc, cfg.RateLimit.Requests, cfg.RateLimit.Window)
}

// provideTracerProvider initializes OpenTelemetry distributed tracing and
// exports spans to Jaeger. Tracing propagates across service boundaries so
// a single user request can be followed through the gateway, url-service,
// and user-service.
func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "api-gateway",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

// provideESClient optionally connects to Elasticsearch for full-text URL
// search. The client is nil when ES is disabled or unreachable, which
// gracefully degrades search to unavailable rather than crashing the gateway.
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
		log.Warn("Elasticsearch unavailable, search disabled: %v", err)
		return nil
	}
	return client
}

// provideSwaggerHandler serves the OpenAPI spec and Swagger UI, enabling
// interactive API exploration at /swagger/*.
func provideSwaggerHandler() *handlers.SwaggerHandler {
	return handlers.NewSwaggerHandler("api/openapi/api-gateway.yaml")
}

// ---------------------------------------------------------------------------
// Provider functions — HTTP mux and server
// ---------------------------------------------------------------------------

// provideMux assembles the HTTP routing table. Routes are grouped into:
//   - /api/auth/*     -- authentication (register, login, profile)
//   - /api/urls/*     -- URL CRUD (create, list, custom aliases)
//   - /api/search     -- full-text URL search via Elasticsearch
//   - /api/analytics/* -- click analytics (stats, timeline, geo, devices)
//   - /health         -- liveness probe that pings both Postgres and Redis
//   - /swagger/*      -- interactive API documentation
func provideMux(
	cfg *config.Config,
	dbManager *database.DBManager,
	redisClient *redis.RedisClient,
	httpHandler *handlers.HTTPHandler,
	authHandler *handlers.AuthHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	authMiddleware *middleware.AuthMiddleware,
	rateLimiter *middleware.RateLimiter,
	swaggerHandler *handlers.SwaggerHandler,
	log *logger.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()

	// Auth routes
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.HandleFunc("/api/auth/profile", authMiddleware.RequireAuth(authHandler.GetProfile))

	// URL routes
	mux.HandleFunc("/api/urls", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authMiddleware.RequireAuth(httpHandler.CreateURL)(w, r)
		case http.MethodGet:
			authMiddleware.RequireAuth(httpHandler.ListURLs)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/urls/custom", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authMiddleware.RequireAuth(httpHandler.CreateCustomURL)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Health check — pings both DB and Redis
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if err := dbManager.Primary().Ping(ctx); err != nil {
			log.Error("health: DB ping failed: %v", err)
			http.Error(w, fmt.Sprintf("DB unavailable: %v", err), http.StatusServiceUnavailable)
			return
		}

		if err := redisClient.GetClient().Ping(ctx).Err(); err != nil {
			log.Error("health: Redis ping failed: %v", err)
			http.Error(w, fmt.Sprintf("Redis unavailable: %v", err), http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Search
	mux.HandleFunc("/api/search", httpHandler.SearchURLs)

	// Swagger
	swaggerHandler.RegisterRoutes(mux)

	// Analytics routes
	mux.HandleFunc("/api/analytics/clicks", authMiddleware.RequireAuth(analyticsHandler.GetClickEvents))

	mux.HandleFunc("/api/analytics/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/stats"):
			analyticsHandler.GetStats(w, r)
		case strings.HasSuffix(path, "/timeline"):
			analyticsHandler.GetTimeline(w, r)
		case strings.HasSuffix(path, "/geo"):
			analyticsHandler.GetGeoStats(w, r)
		case strings.HasSuffix(path, "/devices"):
			analyticsHandler.GetDeviceStats(w, r)
		case strings.HasSuffix(path, "/referrers"):
			analyticsHandler.GetReferrers(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	return mux
}

// provideHTTPServer wraps the mux in the middleware stack and configures
// server timeouts. Middleware is applied in reverse order (outermost runs
// first):
//   - Rate limiter -- rejects excess requests before any work is done
//   - Recovery -- catches panics and returns 500 instead of crashing
//   - Request ID -- attaches a unique ID for correlation in logs/traces
//   - Tracing -- creates an OpenTelemetry span for each HTTP request
//   - CORS -- adds cross-origin headers for browser clients
//
// Conservative read/write timeouts protect against slow clients.
func provideHTTPServer(
	cfg *config.Config,
	mux *http.ServeMux,
	rateLimiter *middleware.RateLimiter,
	log *logger.Logger,
) *http.Server {
	handler := middleware.CORS(cfg.CORS.AllowedOrigins)(mux)
	handler = middleware.Tracing("api-gateway")(handler)
	handler = middleware.RequestID(handler)
	handler = middleware.Recovery(log)(handler)
	handler = rateLimiter.Middleware(handler)

	return &http.Server{
		Addr:         ":" + cfg.Services.APIGatewayPort,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// ---------------------------------------------------------------------------
// Lifecycle registration
// ---------------------------------------------------------------------------

// registerLifecycle hooks the HTTP server and all infrastructure clients
// into the FX lifecycle. On start, the server begins accepting requests in
// a background goroutine. On stop, it performs an orderly shutdown:
//
//  1. Gracefully drain in-flight HTTP requests (respects the context deadline).
//  2. Flush and shut down the OpenTelemetry tracer.
//  3. Close Redis, PostgreSQL, ClickHouse, and Elasticsearch connections.
//  4. Close the gRPC connection to the user-service.
//
// This ordering ensures that no new requests arrive while connections are
// being torn down.
func registerLifecycle(
	lc fx.Lifecycle,
	server *http.Server,
	tp *sdktrace.TracerProvider,
	redisClient *redis.RedisClient,
	dbManager *database.DBManager,
	clickhouseClient *clickhouse.Client,
	esClient *es.Client,
	userConn *grpc.ClientConn,
	log *logger.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Listening on %s", server.Addr)
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Error("Server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Shutting down server...")

			if err := server.Shutdown(ctx); err != nil {
				log.Error("Server shutdown error: %v", err)
			}

			_ = tracing.ShutdownTracer(ctx, tp)
			_ = redisClient.Close()
			dbManager.Close()
			_ = clickhouseClient.Close()
			if esClient != nil {
				_ = esClient.Close()
			}
			_ = userConn.Close()

			return nil
		},
	})
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

// main assembles the complete FX dependency graph for the API gateway.
//
// The graph is organized in four layers:
//   - Infrastructure: config, logger, tracing, Redis, Postgres, ClickHouse,
//     Elasticsearch, and the gRPC connection to user-service.
//   - gRPC clients: typed stubs generated from proto/user.
//   - Handlers and middleware: HTTP handlers for auth, URLs, analytics, and
//     search; JWT auth middleware; Redis-backed rate limiter; Swagger docs.
//   - HTTP server: the mux that routes requests and the http.Server that
//     listens on the configured port.
//
// fx.Invoke(registerLifecycle) triggers construction of the entire graph
// and hooks start/stop behavior. app.Run() blocks until a termination
// signal (SIGINT/SIGTERM) is received.
func main() {
	app := fx.New(
		// Infrastructure providers
		fx.Provide(
			provideConfig,
			provideLogger,
			provideTracerProvider,
			provideRedisClient,
			provideDBManager,
			provideClickHouseClient,
			provideESClient,
			provideUserGRPCConn,
			provideRawRedisClient,
		),

		// gRPC client providers
		fx.Provide(
			provideUserServiceClient,
		),

		// Handler & middleware providers
		fx.Provide(
			provideHTTPHandler,
			provideAuthHandler,
			provideAnalyticsService,
			provideAnalyticsHandler,
			provideAuthMiddleware,
			provideRateLimiter,
			provideSwaggerHandler,
		),

		// HTTP server providers
		fx.Provide(
			provideMux,
			provideHTTPServer,
		),

		// Register lifecycle hooks
		fx.Invoke(registerLifecycle),
	)

	app.Run()
}
