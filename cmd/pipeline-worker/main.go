package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Varun5711/shorternit/internal/cassandra"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/enrichment"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/gocql/gocql"
)

var log *logger.Logger

func main() {
	log = logger.New("pipeline-worker")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	ctx := context.Background()
	cassandraClient, err := cassandra.NewCassandraClient(cassandra.Config{
		Hosts:       cfg.Cassandra.Hosts,
		Keyspace:    cfg.Cassandra.Keyspace,
		Consistency: cfg.Cassandra.Consistency,
	})
	if err != nil {
		log.Fatal("Failed to connect to Cassandra: %v", err)
	}
	defer cassandraClient.Close()

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

	geoEnricher := enrichment.NewGeoIPEnricher()
	defer geoEnricher.Close()

	log.Info("Pipeline worker started, running every hour")

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	processPipeline(ctx, cassandraClient, dbManager, geoEnricher)

	for range ticker.C {
		processPipeline(ctx, cassandraClient, dbManager, geoEnricher)
	}
}

func processPipeline(ctx context.Context, cassClient *cassandra.CassandraClient, dbManager *database.DBManager, geoEnricher *enrichment.GeoIPEnricher) {
	log.Info("starting pipeline run")
	startTime := time.Now()

	lastHour := time.Now().Add(-1 * time.Hour)
	query := `
  		SELECT short_code, clicked_at, click_id, ip_address, user_agent, referer
  		FROM recent_clicks
  		WHERE clicked_at > ?
  		ALLOW FILTERING
  	`

	iter := cassClient.GetSession().Query(query, lastHour).Iter()

	var clicks []ClickData
	var click ClickData

	for iter.Scan(
		&click.ShortCode,
		&click.ClickedAt,
		&click.ClickID,
		&click.IPAddress,
		&click.UserAgent,
		&click.Referer,
	) {
		clicks = append(clicks, click)
		click = ClickData{}
	}

	if err := iter.Close(); err != nil {
		log.Error("Failed to read from Cassandra : %v", err)
		return
	}

	if len(clicks) == 0 {
		log.Info("No new clicks to process")
		return
	}

	log.Info("Processing %d clicks", len(clicks))

	enrichedClicks := enrichClicks(clicks, geoEnricher)

	if err := writeToTimescaleDB(ctx, dbManager, enrichedClicks); err != nil {
		log.Error("Failed to write to TimescaleDB: %v", err)
		return
	}

	if err := updateClickCounters(ctx, dbManager, enrichedClicks); err != nil {
		log.Error("Failed to update counters: %v", err)
		return
	}

	duration := time.Since(startTime)
	log.Info("Pipeline run completed: %d clicks processed in %v", len(clicks), duration)
}

type ClickData struct {
	ShortCode string
	ClickedAt time.Time
	ClickID   gocql.UUID
	IPAddress string
	UserAgent string
	Referer   string
}

type EnrichedClick struct {
	ClickData
	Country    string
	City       string
	Browser    string
	OS         string
	DeviceType string
}

func enrichClicks(clicks []ClickData, geoEnricher *enrichment.GeoIPEnricher) []EnrichedClick {
	enriched := make([]EnrichedClick, len(clicks))

	for i, click := range clicks {
		geoInfo := geoEnricher.Lookup(click.IPAddress)
		uaInfo := enrichment.ParseUserAgent(click.UserAgent)

		enriched[i] = EnrichedClick{
			ClickData:  click,
			Country:    geoInfo.Country,
			City:       geoInfo.City,
			Browser:    uaInfo.Browser,
			OS:         uaInfo.OS,
			DeviceType: uaInfo.DeviceType,
		}
	}

	return enriched
}

func writeToTimescaleDB(ctx context.Context, dbManager *database.DBManager, clicks []EnrichedClick) error {
	conn := dbManager.Primary()

	query := `
		INSERT INTO clicks (
			short_code, clicked_at, click_id, ip_address, user_agent, referer,
			country, city, device_type, browser, os
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	for _, click := range clicks {
		_, err := conn.Exec(ctx, query,
			click.ShortCode,
			click.ClickedAt,
			click.ClickID.String(),
			click.IPAddress,
			click.UserAgent,
			click.Referer,
			click.Country,
			click.City,
			click.DeviceType,
			click.Browser,
			click.OS,
		)
		if err != nil {
			return fmt.Errorf("failed to insert click: %w", err)
		}
	}

	return nil
}

func updateClickCounters(ctx context.Context, dbManager *database.DBManager, clicks []EnrichedClick) error {
	conn := dbManager.Primary()

	clickCounts := make(map[string]int)
	for _, click := range clicks {
		clickCounts[click.ShortCode]++
	}

	query := `UPDATE urls SET clicks = clicks + $1 WHERE short_code = $2`

	for shortCode, count := range clickCounts {
		_, err := conn.Exec(ctx, query, count, shortCode)
		if err != nil {
			log.Warn("Failed to update counter for %s: %v", shortCode, err)
		}
	}

	return nil
}
