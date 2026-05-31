package main

import (
	"context"
	"net/http"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/events"
	"github.com/Varun5711/shorternit/internal/handlers"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/middleware"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/tracing"
	redislib "github.com/redis/go-redis/v9"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
)

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("redirect-service")
}

func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

func provideCache(cfg *config.Config, rc *redislib.Client) *cache.Cache {
	return cache.NewMultiTierCache(cfg.Cache.L1Capacity, rc, cfg.Cache.L2TTL)
}

func provideClickProducer(rc *redislib.Client, cfg *config.Config) *events.ClickProducer {
	return events.NewClickProducer(rc, cfg.Redis.StreamName)
}

func provideRedirectHandler(cfg *config.Config, producer *events.ClickProducer, urlCache *cache.Cache) (*handlers.RedirectHandler, error) {
	return handlers.NewRedirectHandler(cfg.Services.URLServiceAddr, producer, urlCache)
}

func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "redirect-service",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

func provideRateLimiter(cfg *config.Config, rc *redislib.Client) *middleware.RateLimiter {
	return middleware.NewRateLimiter(rc, cfg.RateLimit.Requests, cfg.RateLimit.Window)
}

func provideHTTPServer(
	cfg *config.Config,
	redirectHandler *handlers.RedirectHandler,
	rateLimiter *middleware.RateLimiter,
	redisClient *redis.RedisClient,
	log *logger.Logger,
) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", redirectHandler.HandleRedirect)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := redisClient.Ping(r.Context()); err != nil {
			http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	handler := middleware.Tracing("redirect-service")(mux)
	handler = middleware.Recovery(log)(handler)
	handler = rateLimiter.Middleware(handler)

	return &http.Server{
		Addr:         ":" + cfg.Services.RedirectServicePort,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func registerLifecycle(
	lc fx.Lifecycle,
	server *http.Server,
	tp *sdktrace.TracerProvider,
	redisClient *redis.RedisClient,
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
			log.Info("Shutting down redirect-service...")
			if err := server.Shutdown(ctx); err != nil {
				log.Error("Shutdown error: %v", err)
			}
			tracing.ShutdownTracer(ctx, tp)
			redisClient.Close()
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
			provideRawRedisClient,
			provideCache,
			provideClickProducer,
			provideRedirectHandler,
			provideRateLimiter,
			provideHTTPServer,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
