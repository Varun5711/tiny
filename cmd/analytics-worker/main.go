// Package main implements the analytics worker for the Tiny URL shortener.
//
// The analytics worker is a background consumer that reads click events
// from a Redis Stream (published by the redirect-service) and increments
// per-URL click counters in PostgreSQL. It runs as part of a Redis consumer
// group, which means multiple instances can share the workload and pick up
// where a crashed peer left off.
//
// This worker handles the lightweight "aggregate counts" path. For the
// richer enrichment pipeline (GeoIP lookup, user-agent parsing, ClickHouse
// and Elasticsearch storage), see cmd/pipeline-worker.
//
// Dependency injection is managed by Uber FX.
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

// WorkerParams bundles the Redis Stream consumer configuration into a
// single injectable value. Extracting these from *config.Config avoids
// passing the entire config to the event-processing loop.
type WorkerParams struct {
	StreamName    string
	ConsumerGroup string
	ConsumerName  string
	BatchSize     int
	PollInterval  time.Duration
	BlockTime     time.Duration
}

// provideConfig loads the unified application configuration from environment
// variables and config files.
func provideConfig() (*config.Config, error) {
	return config.Load()
}

// provideLogger creates a structured logger tagged with "analytics-worker"
// so log output is identifiable in centralized logging.
func provideLogger() *logger.Logger {
	return logger.New("analytics-worker")
}

// provideRedisClient connects to the shared Redis instance. Redis is the
// source of click events (via Streams) that this worker consumes.
func provideRedisClient(cfg *config.Config) (*redis.RedisClient, error) {
	return redis.NewRedisClient(context.Background(), redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

// provideDBManager sets up a PostgreSQL connection pool. The analytics
// worker only writes to the primary: it increments click counters in the
// urls table via batch UPDATE statements.
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

// provideTracerProvider initializes OpenTelemetry distributed tracing and
// exports spans to Jaeger, enabling visibility into batch processing
// latency and database write performance.
func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "analytics-worker",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

// provideWorkerParams extracts Redis Stream consumer settings from the
// config into a dedicated struct. BatchSize and BlockTime control the
// trade-off between latency (smaller batches) and throughput (larger
// batches with longer blocking reads).
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

// registerLifecycle wires the worker into the FX lifecycle. On start, it
// ensures the Redis consumer group exists (creating the stream if needed),
// then launches the event processing loop in a background goroutine. On
// stop, it cancels the worker context and waits for the goroutine to drain,
// ensuring no events are lost mid-batch before closing infrastructure
// connections.
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

// processEvents is the main event loop. It performs a blocking XREADGROUP
// on the Redis Stream, batches messages by short code to minimize database
// round-trips, updates click counts in a single transaction, and
// acknowledges consumed messages. On transient errors it backs off by
// PollInterval before retrying.
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

// updateClickCounts applies batched click increments to the urls table
// inside a single transaction. Grouping by short code avoids issuing one
// UPDATE per message, which would be prohibitively slow under high traffic.
func updateClickCounts(ctx context.Context, dbManager *database.DBManager, clickCounts map[string]int) error {
	tx, err := dbManager.Write().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

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

// main assembles the complete FX dependency graph for the analytics worker.
//
// The graph is intentionally small: config, logging, tracing, Redis (event
// source), Postgres (click count sink), and worker parameters. There is no
// HTTP or gRPC server -- the worker is a pure consumer.
// fx.Invoke(registerLifecycle) triggers graph construction and starts the
// event loop. Run() blocks until a termination signal is received.
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
