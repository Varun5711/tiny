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

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("api-gateway")
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

func provideClickHouseClient(cfg *config.Config) (*clickhouse.Client, error) {
	return clickhouse.NewClient(cfg.ClickHouse)
}

func provideUserGRPCConn() (*grpc.ClientConn, error) {
	addr := os.Getenv("USER_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:50052"
	}
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

// ---------------------------------------------------------------------------
// Provider functions — gRPC clients
// ---------------------------------------------------------------------------

func provideUserServiceClient(conn *grpc.ClientConn) userpb.UserServiceClient {
	return userpb.NewUserServiceClient(conn)
}

// ---------------------------------------------------------------------------
// Provider functions — handlers & middleware
// ---------------------------------------------------------------------------

func provideHTTPHandler(cfg *config.Config, esClient *es.Client) (*handlers.HTTPHandler, error) {
	return handlers.NewHTTPHandler(cfg.Services.URLServiceAddr, cfg.Services.BaseURL, esClient)
}

func provideAuthHandler(userClient userpb.UserServiceClient) *handlers.AuthHandler {
	return handlers.NewAuthHandler(userClient)
}

func provideAnalyticsService(db *database.DBManager) *analytics.Service {
	return analytics.NewService(db)
}

func provideAnalyticsHandler(svc *analytics.Service, ch *clickhouse.Client) *handlers.AnalyticsHandler {
	return handlers.NewAnalyticsHandler(svc, ch)
}

func provideAuthMiddleware(userClient userpb.UserServiceClient) *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(userClient)
}

func provideRateLimiter(cfg *config.Config, rc *redislib.Client) *middleware.RateLimiter {
	return middleware.NewRateLimiter(rc, cfg.RateLimit.Requests, cfg.RateLimit.Window)
}

func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "api-gateway",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
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
		log.Warn("Elasticsearch unavailable, search disabled: %v", err)
		return nil
	}
	return client
}

func provideSwaggerHandler() *handlers.SwaggerHandler {
	return handlers.NewSwaggerHandler("api/openapi/api-gateway.yaml")
}

// ---------------------------------------------------------------------------
// Provider functions — HTTP mux and server
// ---------------------------------------------------------------------------

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
