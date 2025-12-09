package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/redis"
	redislib "github.com/redis/go-redis/v9"
)

var (
	log           *logger.Logger
	streamName    string
	consumerGroup string
	consumerName  string
	batchSize     int
	pollInterval  time.Duration
	blockTime     time.Duration
)

func main() {
	log = logger.New("analytics-worker")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	streamName = cfg.Redis.StreamName
	consumerGroup = cfg.Analytics.ConsumerGroup
	consumerName = cfg.Analytics.ConsumerName
	batchSize = cfg.Analytics.BatchSize
	pollInterval = cfg.Analytics.PollInterval
	blockTime = cfg.Analytics.BlockTime

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

	err = redisClient.GetClient().XGroupCreateMkStream(ctx, streamName, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Fatal("Failed to create consumer group: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info("Processing click events")
	go processEvents(ctx, redisClient.GetClient(), dbManager)

	<-sigChan
	log.Info("Shutting down")
}

func processEvents(ctx context.Context, client *redislib.Client, dbManager *database.DBManager) {
	for {
		messages, err := client.XReadGroup(ctx, &redislib.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{streamName, ">"},
			Count:    int64(batchSize),
			Block:    blockTime,
		}).Result()

		if err != nil {
			if err == redislib.Nil {
				continue
			}
			log.Error("Failed to read from stream: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		for _, stream := range messages {
			if len(stream.Messages) == 0 {
				continue
			}

			clickCounts := make(map[string]int)
			messageIDs := make([]string, 0, len(stream.Messages))

			for _, msg := range stream.Messages {
				shortCode, ok := msg.Values["short_code"].(string)
				if !ok {
					log.Warn("Invalid message format: %v", msg.ID)
					continue
				}

				clickCounts[shortCode]++
				messageIDs = append(messageIDs, msg.ID)
			}

			if len(clickCounts) > 0 {
				if err := updateClickCounts(ctx, dbManager, clickCounts); err != nil {
					log.Error("Failed to update database: %v", err)
					continue
				}
				log.Debug("Processed %d events for %d URLs", len(messageIDs), len(clickCounts))
			}

			if len(messageIDs) > 0 {
				if err := acknowledgeMessages(ctx, client, messageIDs); err != nil {
					log.Error("Failed to acknowledge messages: %v", err)
				}
			}
		}
	}
}

func updateClickCounts(ctx context.Context, dbManager *database.DBManager, clickCounts map[string]int) error {
	tx, err := dbManager.Write().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for shortCode, count := range clickCounts {
		query := `
			UPDATE urls
			SET clicks = clicks + $1,
			    updated_at = NOW()
			WHERE short_code = $2
		`
		_, err := tx.Exec(ctx, query, count, shortCode)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func acknowledgeMessages(ctx context.Context, client *redislib.Client, messageIDs []string) error {
	return client.XAck(ctx, streamName, consumerGroup, messageIDs...).Err()
}
