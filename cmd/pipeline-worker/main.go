package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Varun5711/shorternit/internal/clickhouse"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/enrichment"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var log *logger.Logger

func main() {
	log = logger.New("pipeline-worker")
	log.Info("Starting pipeline worker...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis: %v", err)
	}
	log.Info("Connected to Redis at %s", cfg.Redis.Addr)

	chClient, err := clickhouse.NewClient(cfg.ClickHouse)
	if err != nil {
		log.Fatal("Failed to connect to ClickHouse: %v", err)
	}
	defer chClient.Close()
	log.Info("Connected to ClickHouse at %s", cfg.ClickHouse.Addr)

	geoEnricher := enrichment.NewGeoIPEnricher()
	defer geoEnricher.Close()

	err = redisClient.XGroupCreateMkStream(ctx, cfg.Redis.StreamName, cfg.Analytics.ConsumerGroup,
		"0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Fatal("Failed to create consumer group: %v", err)
	}
	log.Info("Consumer group '%s' ready", cfg.Analytics.ConsumerGroup)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	worker := &PipelineWorker{
		redisClient:   redisClient,
		chClient:      chClient,
		geoEnricher:   geoEnricher,
		streamName:    cfg.Redis.StreamName,
		consumerGroup: cfg.Analytics.ConsumerGroup,
		consumerName:  cfg.Analytics.ConsumerName,
		batchSize:     cfg.Analytics.BatchSize,
		pollInterval:  cfg.Analytics.PollInterval,
		blockTime:     cfg.Analytics.BlockTime,
	}

	go worker.Start(ctx)

	<-sigChan
	log.Info("Shutting down pipeline worker...")
	cancel()
	time.Sleep(2 * time.Second)
	log.Info("Pipeline worker stopped")
}

type PipelineWorker struct {
	redisClient   *redis.Client
	chClient      *clickhouse.Client
	geoEnricher   *enrichment.GeoIPEnricher
	streamName    string
	consumerGroup string
	consumerName  string
	batchSize     int
	pollInterval  time.Duration
	blockTime     time.Duration
}

func (w *PipelineWorker) Start(ctx context.Context) {
	log.Info("Pipeline worker started (batch size: %d)", w.batchSize)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processBatch(ctx); err != nil {
				log.Error("Failed to process batch: %v", err)
			}
		}
	}
}

func (w *PipelineWorker) processBatch(ctx context.Context) error {
	streams, err := w.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  []string{w.streamName, ">"},
		Count:    int64(w.batchSize),
		Block:    w.blockTime,
	}).Result()

	if err != nil {
		if err == redis.Nil {
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
		timestampInt, _ := parseTimestamp(timestamp)
		clickedAt = time.Unix(timestampInt, 0)
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

func parseTimestamp(s string) (int64, error) {
	var ts int64
	if err := json.Unmarshal([]byte(s), &ts); err != nil {
		return 0, err
	}
	return ts, nil
}
