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
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("cleanup-worker")
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

func provideCache(cfg *config.Config, rc *redislib.Client) *cache.Cache {
	return cache.NewMultiTierCache(cfg.Cache.L1Capacity, rc, cfg.Cache.L2TTL)
}

func provideStorage(db *database.DBManager) *storage.PostgresStorage {
	return storage.NewPostgresStorage(db)
}

func registerLifecycle(
	lc fx.Lifecycle,
	store *storage.PostgresStorage,
	urlCache *cache.Cache,
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
					redisClient.Close()
					dbManager.Close()
					return nil
				},
			})
			return nil
		},
	})
}

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

func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
			provideRedisClient,
			provideRawRedisClient,
			provideDBManager,
			provideCache,
			provideStorage,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
