// Package main implements the cleanup worker for the Tiny URL shortener.
//
// The cleanup worker is a background job that periodically deletes expired
// URLs from PostgreSQL. URLs can have an optional TTL set at creation time;
// once expired, they should no longer resolve and their storage can be
// reclaimed. The worker runs a single cleanup pass immediately on startup,
// then repeats every 24 hours.
//
// A multi-tier cache (in-process LRU + Redis) is injected so that future
// enhancements can invalidate cached entries for deleted URLs. Currently
// the cache is available but cache eviction on cleanup is not yet
// implemented -- stale entries will expire naturally via their TTL.
//
// Dependency injection is managed by Uber FX.
package main

import (
	"context"
	"sync"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/storage"
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

// provideLogger creates a structured logger tagged with "cleanup-worker"
// so log output is identifiable in centralized logging.
func provideLogger() *logger.Logger {
	return logger.New("cleanup-worker")
}

// provideRedisClient connects to the shared Redis instance. Redis is needed
// here to back the L2 cache tier; the cleanup worker may evict cached
// entries for URLs it deletes.
func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

// provideRawRedisClient unwraps the internal RedisClient to expose the
// underlying go-redis *Client needed by the multi-tier cache.
func provideRawRedisClient(rc *redis.RedisClient) *redislib.Client {
	return rc.GetClient()
}

// provideDBManager sets up a PostgreSQL connection pool. The cleanup worker
// writes to the primary: it runs DELETE statements against expired rows in
// the urls table.
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

// provideCache builds a two-tier cache (in-process LRU + Redis) so deleted
// URLs can be evicted from the cache in future enhancements.
func provideCache(cfg *config.Config, rc *redislib.Client) *cache.Cache {
	return cache.NewMultiTierCache(cfg.Cache.L1Capacity, rc, cfg.Cache.L2TTL)
}

// provideTracerProvider initializes OpenTelemetry distributed tracing and
// exports spans to Jaeger, giving visibility into cleanup batch duration
// and database DELETE performance.
func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "cleanup-worker",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

// provideStorage creates the PostgreSQL-backed URL storage layer, which
// exposes the DeleteExpiredURLs method used by the cleanup loop.
func provideStorage(db *database.DBManager) *storage.PostgresStorage {
	return storage.NewPostgresStorage(db)
}

// registerLifecycle wires the cleanup loop into the FX lifecycle. On start,
// it launches a goroutine that runs an immediate cleanup pass followed by
// a 24-hour ticker loop. On stop, it cancels the context and waits for the
// goroutine to finish, then closes tracing, Redis, and database connections.
func registerLifecycle(
	lc fx.Lifecycle,
	store *storage.PostgresStorage,
	urlCache *cache.Cache,
	tp *sdktrace.TracerProvider,
	redisClient *redis.RedisClient,
	dbManager *database.DBManager,
	log *logger.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			workerCtx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runCleanupLoop(workerCtx, store, urlCache, log)
			}()

			log.Info("Cleanup worker started, running every 24 hours")

			lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					log.Info("Shutting down cleanup-worker...")
					cancel()
					wg.Wait()
					_ = tracing.ShutdownTracer(ctx, tp)
					_ = redisClient.Close()
					dbManager.Close()
					return nil
				},
			})
			return nil
		},
	})
}

// runCleanupLoop runs an immediate cleanup pass on startup, then repeats
// every 24 hours. The immediate pass ensures newly deployed instances
// catch up on any backlog of expired URLs without waiting a full day.
func runCleanupLoop(ctx context.Context, store *storage.PostgresStorage, urlCache *cache.Cache, log *logger.Logger) {
	runCleanup(ctx, store, urlCache, log)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCleanup(ctx, store, urlCache, log)
		}
	}
}

// runCleanup performs a single cleanup pass: it deletes all URLs whose
// expires_at timestamp is in the past and logs how many were removed.
// Errors are logged but do not crash the worker -- the next tick will
// retry automatically.
func runCleanup(ctx context.Context, store *storage.PostgresStorage, urlCache *cache.Cache, log *logger.Logger) {
	log.Info("Starting cleanup of expired URLs...")

	deletedCount, err := store.DeleteExpiredURLs(ctx)
	if err != nil {
		log.Error("Failed to delete expired URLs: %v", err)
		return
	}

	if deletedCount > 0 {
		log.Info("Deleted %d expired URLs from database", deletedCount)
	} else {
		log.Info("No expired URLs found")
	}
}

// main assembles the complete FX dependency graph for the cleanup worker.
//
// The graph connects config, logging, tracing, Redis (for the cache tier),
// Postgres (for URL deletion), and the storage layer. There is no HTTP or
// gRPC server -- the worker is a pure background job on a 24-hour timer.
// fx.Invoke(registerLifecycle) triggers graph construction and starts the
// cleanup loop. Run() blocks until a termination signal is received.
func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
			provideTracerProvider,
			provideRedisClient,
			provideRawRedisClient,
			provideDBManager,
			provideCache,
			provideStorage,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
