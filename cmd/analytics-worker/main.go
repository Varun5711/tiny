package main

import (
	"context"
	"sync"
	"time"

	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/tracing"
	redislib "github.com/redis/go-redis/v9"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
)

type WorkerParams struct {
	StreamName    string
	ConsumerGroup string
	ConsumerName  string
	BatchSize     int
	PollInterval  time.Duration
	BlockTime     time.Duration
}

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("analytics-worker")
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

func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "analytics-worker",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

func provideWorkerParams(cfg *config.Config) WorkerParams {
	return WorkerParams{
		StreamName:    cfg.Redis.StreamName,
		ConsumerGroup: cfg.Analytics.ConsumerGroup,
		ConsumerName:  cfg.Analytics.ConsumerName,
		BatchSize:     cfg.Analytics.BatchSize,
		PollInterval:  cfg.Analytics.PollInterval,
		BlockTime:     cfg.Analytics.BlockTime,
	}
}

func registerLifecycle(
	lc fx.Lifecycle,
	redisClient *redis.RedisClient,
	tp *sdktrace.TracerProvider,
	dbManager *database.DBManager,
	params WorkerParams,
	log *logger.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			client := redisClient.GetClient()
			err := client.XGroupCreateMkStream(ctx, params.StreamName, params.ConsumerGroup, "0").Err()
			if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
				return err
			}

			workerCtx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				processEvents(workerCtx, client, dbManager, params, log)
			}()

			log.Info("Processing click events")

			lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					log.Info("Shutting down analytics-worker...")
					cancel()
					wg.Wait()
					tracing.ShutdownTracer(ctx, tp)
					redisClient.Close()
					dbManager.Close()
					return nil
				},
			})
			return nil
		},
	})
}

func processEvents(ctx context.Context, client *redislib.Client, dbManager *database.DBManager, params WorkerParams, log *logger.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := client.XReadGroup(ctx, &redislib.XReadGroupArgs{
			Group:    params.ConsumerGroup,
			Consumer: params.ConsumerName,
			Streams:  []string{params.StreamName, ">"},
			Count:    int64(params.BatchSize),
			Block:    params.BlockTime,
		}).Result()

		if err != nil {
			if err == redislib.Nil || ctx.Err() != nil {
				continue
			}
			log.Error("Failed to read from stream: %v", err)
			time.Sleep(params.PollInterval)
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
				if err := client.XAck(ctx, params.StreamName, params.ConsumerGroup, messageIDs...).Err(); err != nil {
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
		_, err := tx.Exec(ctx, `
			UPDATE urls
			SET clicks = clicks + $1, updated_at = NOW()
			WHERE short_code = $2
		`, count, shortCode)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
			provideTracerProvider,
			provideRedisClient,
			provideDBManager,
			provideWorkerParams,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
