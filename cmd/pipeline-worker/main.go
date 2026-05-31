package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Varun5711/shorternit/internal/clickhouse"
	"github.com/Varun5711/shorternit/internal/config"
	es "github.com/Varun5711/shorternit/internal/elasticsearch"
	"github.com/Varun5711/shorternit/internal/enrichment"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
)

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("pipeline-worker")
}

func provideRedisClient(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	return client, nil
}

func provideClickHouseClient(cfg *config.Config) (*clickhouse.Client, error) {
	return clickhouse.NewClient(cfg.ClickHouse)
}

func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "pipeline-worker",
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
		log.Warn("Elasticsearch unavailable, running without indexing: %v", err)
		return nil
	}
	return client
}

func provideGeoEnricher() *enrichment.GeoIPEnricher {
	return enrichment.NewGeoIPEnricher()
}

func providePipelineWorker(
	redisClient *redis.Client,
	chClient *clickhouse.Client,
	esClient *es.Client,
	geoEnricher *enrichment.GeoIPEnricher,
	cfg *config.Config,
) *PipelineWorker {
	return &PipelineWorker{
		redisClient:   redisClient,
		chClient:      chClient,
		esClient:      esClient,
		geoEnricher:   geoEnricher,
		streamName:    cfg.Redis.StreamName,
		consumerGroup: cfg.Analytics.ConsumerGroup,
		consumerName:  cfg.Analytics.ConsumerName,
		batchSize:     cfg.Analytics.BatchSize,
		pollInterval:  cfg.Analytics.PollInterval,
		blockTime:     cfg.Analytics.BlockTime,
	}
}

func registerLifecycle(
	lc fx.Lifecycle,
	worker *PipelineWorker,
	tp *sdktrace.TracerProvider,
	redisClient *redis.Client,
	chClient *clickhouse.Client,
	geoEnricher *enrichment.GeoIPEnricher,
	cfg *config.Config,
	log *logger.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			err := redisClient.XGroupCreateMkStream(ctx, cfg.Redis.StreamName, cfg.Analytics.ConsumerGroup, "0").Err()
			if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
				return err
			}

			workerCtx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				worker.Start(workerCtx, log)
			}()

			log.Info("Pipeline worker started")

			lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					log.Info("Shutting down pipeline worker...")
					cancel()
					wg.Wait()
					_ = tracing.ShutdownTracer(ctx, tp)
					_ = geoEnricher.Close()
					_ = chClient.Close()
					_ = redisClient.Close()
					return nil
				},
			})
			return nil
		},
	})
}

type PipelineWorker struct {
	redisClient   *redis.Client
	chClient      *clickhouse.Client
	esClient      *es.Client
	geoEnricher   *enrichment.GeoIPEnricher
	streamName    string
	consumerGroup string
	consumerName  string
	batchSize     int
	pollInterval  time.Duration
	blockTime     time.Duration
}

func (w *PipelineWorker) Start(ctx context.Context, log *logger.Logger) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processBatch(ctx, log); err != nil {
				log.Error("Failed to process batch: %v", err)
			}
		}
	}
}

func (w *PipelineWorker) processBatch(ctx context.Context, log *logger.Logger) error {
	streams, err := w.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  []string{w.streamName, ">"},
		Count:    int64(w.batchSize),
		Block:    w.blockTime,
	}).Result()

	if err != nil {
		if err == redis.Nil || ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("failed to read from stream: %w", err)
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil
	}

	messages := streams[0].Messages
	log.Info("Processing batch of %d events", len(messages))

	var clickEvents []clickhouse.ClickEvent
	var messageIDs []string

	for _, msg := range messages {
		event, err := w.enrichEvent(msg.Values)
		if err != nil {
			log.Error("Failed to enrich event %s: %v", msg.ID, err)
			continue
		}
		clickEvents = append(clickEvents, *event)
		messageIDs = append(messageIDs, msg.ID)
	}

	if len(clickEvents) == 0 {
		return nil
	}

	if err := w.chClient.InsertClickEvents(ctx, clickEvents); err != nil {
		return fmt.Errorf("failed to insert events to ClickHouse: %w", err)
	}

	if w.esClient != nil {
		esDocs := make([]es.ClickEventDocument, len(clickEvents))
		for i, ce := range clickEvents {
			esDocs[i] = es.ClickEventDocument{
				EventID:     ce.EventID,
				ShortCode:   ce.ShortCode,
				OriginalURL: ce.OriginalURL,
				ClickedAt:   ce.ClickedAt,
				IPAddress:   ce.IPAddress,
				Country:     ce.Country,
				CountryCode: ce.CountryCode,
				Region:      ce.Region,
				City:        ce.City,
				Latitude:    ce.Latitude,
				Longitude:   ce.Longitude,
				UserAgent:   ce.UserAgent,
				Browser:     ce.Browser,
				OS:          ce.OS,
				DeviceType:  ce.DeviceType,
				Referer:     ce.Referer,
			}
		}
		if err := w.esClient.IndexClickEventsBulk(ctx, esDocs); err != nil {
			log.Error("Failed to index click events to ES: %v", err)
		}
	}

	for _, msgID := range messageIDs {
		if err := w.redisClient.XAck(ctx, w.streamName, w.consumerGroup, msgID).Err(); err != nil {
			log.Error("Failed to ack message %s: %v", msgID, err)
		}
	}

	log.Info("Successfully processed %d events", len(clickEvents))
	return nil
}

func (w *PipelineWorker) enrichEvent(fields map[string]interface{}) (*clickhouse.ClickEvent, error) {
	shortCode, _ := fields["short_code"].(string)
	timestamp, _ := fields["timestamp"].(string)
	ipAddress, _ := fields["ip"].(string)
	userAgent, _ := fields["user_agent"].(string)
	originalURL, _ := fields["original_url"].(string)
	referer, _ := fields["referer"].(string)
	queryParams, _ := fields["query_params"].(string)

	var clickedAt time.Time
	if timestamp != "" {
		var ts int64
		if err := json.Unmarshal([]byte(timestamp), &ts); err == nil {
			clickedAt = time.Unix(ts, 0)
		} else {
			clickedAt = time.Now()
		}
	} else {
		clickedAt = time.Now()
	}

	geoInfo := w.geoEnricher.Lookup(ipAddress)
	uaInfo := enrichment.ParseUserAgent(userAgent)

	var isMobile, isTablet, isDesktop, isBot uint8
	switch uaInfo.DeviceType {
	case "mobile":
		isMobile = 1
	case "tablet":
		isTablet = 1
	case "bot":
		isBot = 1
	default:
		isDesktop = 1
	}

	if uaInfo.IsTablet {
		isTablet = 1
	}

	return &clickhouse.ClickEvent{
		EventID:        uuid.New().String(),
		ShortCode:      shortCode,
		OriginalURL:    originalURL,
		ClickedAt:      clickedAt,
		IPAddress:      ipAddress,
		Country:        geoInfo.Country,
		CountryCode:    geoInfo.CountryCode,
		Region:         geoInfo.Region,
		City:           geoInfo.City,
		Latitude:       geoInfo.Latitude,
		Longitude:      geoInfo.Longitude,
		Timezone:       geoInfo.Timezone,
		UserAgent:      userAgent,
		Browser:        uaInfo.Browser,
		BrowserVersion: uaInfo.BrowserVersion,
		OS:             uaInfo.OS,
		OSVersion:      uaInfo.OSVersion,
		DeviceType:     uaInfo.DeviceType,
		DeviceBrand:    uaInfo.DeviceBrand,
		DeviceModel:    uaInfo.DeviceModel,
		IsMobile:       isMobile,
		IsTablet:       isTablet,
		IsDesktop:      isDesktop,
		IsBot:          isBot,
		Referer:        referer,
		QueryParams:    queryParams,
	}, nil
}

func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
			provideTracerProvider,
			provideRedisClient,
			provideClickHouseClient,
			provideESClient,
			provideGeoEnricher,
			providePipelineWorker,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
