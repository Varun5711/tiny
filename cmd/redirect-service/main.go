// Package main implements the redirect service for the Tiny URL shortener.
//
// The redirect service handles the public-facing short-link resolution:
// when a user visits a short URL (e.g. https://tiny.example/abc123), this
// service resolves the short code to the original URL and issues an HTTP
// 301/302 redirect. It is intentionally separate from the API gateway so
// redirect traffic (high volume, latency-sensitive) can scale independently
// of the CRUD API traffic.
//
// Resolution is accelerated by a two-tier cache (in-process LRU + Redis).
// On every successful redirect, a click event is published to a Redis
// Stream, which the analytics-worker and pipeline-worker consume
// asynchronously -- keeping redirect latency minimal.
//
// Dependency injection is managed by Uber FX.
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

// provideConfig loads the unified application configuration from environment
// variables and config files.
func provideConfig() (*config.Config, error) {
	return config.Load()
}

// provideLogger creates a structured logger tagged with "redirect-service"
// so log output is identifiable in centralized logging.
func provideLogger() *logger.Logger {
	return logger.New("redirect-service")
}

// provideRedisClient connects to the shared Redis instance. Redis serves
// as the L2 cache tier for URL lookups and as the transport for click
// event publishing via Streams.
func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

// provideRawRedisClient unwraps the internal RedisClient to expose the
// underlying go-redis *Client needed by the cache and click producer.
func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

// provideCache builds a two-tier URL lookup cache (in-process LRU + Redis).
// This is the primary optimization that keeps redirect latency low: most
// popular short codes are resolved without any network round-trip.
func provideCache(cfg *config.Config, rc *redislib.Client) *cache.Cache {
	return cache.NewMultiTierCache(cfg.Cache.L1Capacity, rc, cfg.Cache.L2TTL)
}

// provideClickProducer creates an event producer that writes click events
// to a Redis Stream. The stream is consumed by the analytics-worker (for
// aggregate click counts in Postgres) and the pipeline-worker (for
// enriched events in ClickHouse/Elasticsearch).
func provideClickProducer(rc *redislib.Client, cfg *config.Config) *events.ClickProducer {
	return events.NewClickProducer(rc, cfg.Redis.StreamName)
}

// provideRedirectHandler creates the HTTP handler that resolves short codes
// and issues redirects. It first checks the multi-tier cache, then falls
// back to the url-service via gRPC if the code is not cached. On every
// hit it fires a click event asynchronously.
func provideRedirectHandler(cfg *config.Config, producer *events.ClickProducer, urlCache *cache.Cache) (*handlers.RedirectHandler, error) {
	return handlers.NewRedirectHandler(cfg.Services.URLServiceAddr, producer, urlCache)
}

// provideTracerProvider initializes OpenTelemetry distributed tracing and
// exports spans to Jaeger. Redirect latency is a key SLI, so tracing helps
// diagnose slow lookups across cache / gRPC / DB layers.
func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "redirect-service",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

// provideRateLimiter builds a Redis-backed sliding-window rate limiter.
// Rate limiting on the redirect path prevents abuse (link-bombing) and
// protects the url-service from thundering-herd cache misses.
func provideRateLimiter(cfg *config.Config, rc *redislib.Client) *middleware.RateLimiter {
	return middleware.NewRateLimiter(rc, cfg.RateLimit.Requests, cfg.RateLimit.Window)
}

// provideHTTPServer assembles the HTTP server with its routing table and
// middleware stack. The mux has two routes:
//   - "/" -- the redirect handler (catch-all for short code resolution)
//   - "/health" -- liveness probe that pings Redis
//
// Middleware is layered in reverse order: rate limiting runs first (outermost),
// then panic recovery, then distributed tracing (innermost before the handler).
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
		_, _ = w.Write([]byte("OK"))
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

// registerLifecycle hooks the HTTP server and Redis client into the FX
// lifecycle. On start, the server begins accepting redirect requests in a
// background goroutine. On stop, it drains in-flight requests, flushes
// the tracer, and closes the Redis connection.
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
			_ = tracing.ShutdownTracer(ctx, tp)
			_ = redisClient.Close()
			return nil
		},
	})
}

// main assembles the complete FX dependency graph for the redirect service.
//
// The graph flows from infrastructure (config, logging, tracing, Redis)
// through the cache and click-event producer up to the redirect handler
// and HTTP server. This service is intentionally lightweight -- it has no
// direct database dependency. URL resolution is handled via the multi-tier
// cache and a gRPC fallback to the url-service.
// fx.Invoke(registerLifecycle) triggers graph construction and starts the
// HTTP server. Run() blocks until a termination signal is received.
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
