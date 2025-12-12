package main

import (
	"context"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/storage"
)

func main() {
	log := logger.New("cleanup-worker")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	ctx := context.Background()

	redisClient, err := redis.NewRedisClient(ctx, redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatal("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	urlCache := cache.NewMultiTierCache(
		cfg.Cache.L1Capacity,
		redisClient.GetClient(),
		cfg.Cache.L2TTL,
	)

	dbConfig := database.Config{
		PrimaryDSN:      cfg.Database.PrimaryDSN,
		ReplicaDSNs:     cfg.Database.ReplicaDSNs,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	}

	dbManager, err := database.NewDBManager(ctx, dbConfig)
	if err != nil {
		log.Fatal("Failed to connect to database: %v", err)
	}
	defer dbManager.Close()

	store := storage.NewPostgresStorage(dbManager)

	log.Info("Cleanup worker started. Running every 24 hours...")

	runCleanup(ctx, store, urlCache, log)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runCleanup(ctx, store, urlCache, log)
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

		log.Info("Cache will be cleared by TTL naturally")
	} else {
		log.Info("No expired URLs found")
	}

	log.Info("Cleanup completed successfully")
}
