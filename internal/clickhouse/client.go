package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/Varun5711/shorternit/internal/config"
)

type Client struct {
	conn driver.Conn
}

func NewClient(cfg config.ClickHouseConfig) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:      time.Second * 30,
		MaxOpenConns:     cfg.MaxConns,
		MaxIdleConns:     cfg.MaxConns / 2,
		ConnMaxLifetime:  time.Hour,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to clickhouse: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

type ClickEvent struct {
	EventID     string
	ShortCode   string
	OriginalURL string
	ClickedAt   time.Time

	IPAddress   string
	Country     string
	CountryCode string
	Region      string
	City        string
	Latitude    float64
	Longitude   float64
	Timezone    string

	UserAgent      string
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string
	DeviceBrand    string
	DeviceModel    string
	IsMobile       uint8
	IsTablet       uint8
	IsDesktop      uint8
	IsBot          uint8

	Referer     string
	QueryParams string
}

func (c *Client) InsertClickEvents(ctx context.Context, events []ClickEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO analytics.click_events (
		event_id, short_code, original_url, clicked_at,
		ip_address, country, country_code, region, city, latitude, longitude, timezone,
		user_agent, browser, browser_version, os, os_version,
		device_type, device_brand, device_model,
		is_mobile, is_tablet, is_desktop, is_bot,
		referer, query_params
	)`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, event := range events {
		err := batch.Append(
			event.EventID,
			event.ShortCode,
			event.OriginalURL,
			event.ClickedAt,
			event.IPAddress,
			event.Country,
			event.CountryCode,
			event.Region,
			event.City,
			event.Latitude,
			event.Longitude,
			event.Timezone,
			event.UserAgent,
			event.Browser,
			event.BrowserVersion,
			event.OS,
			event.OSVersion,
			event.DeviceType,
			event.DeviceBrand,
			event.DeviceModel,
			event.IsMobile,
			event.IsTablet,
			event.IsDesktop,
			event.IsBot,
			event.Referer,
			event.QueryParams,
		)
		if err != nil {
			return fmt.Errorf("failed to append event: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.conn.Exec(ctx, query, args...)
}

func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.conn.Query(ctx, query, args...)
}

// GetClickEvents retrieves click events for a specific short code
func (c *Client) GetClickEvents(ctx context.Context, shortCode string, limit int) ([]ClickEvent, error) {
	query := `
		SELECT
			event_id, short_code, original_url, clicked_at,
			ip_address, country, country_code, region, city,
			browser, browser_version, os, os_version, device_type,
			referer
		FROM analytics.click_events
		WHERE short_code = ?
		ORDER BY clicked_at DESC
		LIMIT ?
	`

	rows, err := c.conn.Query(ctx, query, shortCode, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query click events: %w", err)
	}
	defer rows.Close()

	var events []ClickEvent
	for rows.Next() {
		var event ClickEvent
		err := rows.Scan(
			&event.EventID,
			&event.ShortCode,
			&event.OriginalURL,
			&event.ClickedAt,
			&event.IPAddress,
			&event.Country,
			&event.CountryCode,
			&event.Region,
			&event.City,
			&event.Browser,
			&event.BrowserVersion,
			&event.OS,
			&event.OSVersion,
			&event.DeviceType,
			&event.Referer,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan click event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return events, nil
}

// GetAllClickEvents retrieves all click events for analytics
func (c *Client) GetAllClickEvents(ctx context.Context, limit int) ([]ClickEvent, error) {
	query := `
		SELECT
			event_id, short_code, original_url, clicked_at,
			ip_address, country, country_code, region, city,
			browser, browser_version, os, os_version, device_type,
			referer
		FROM analytics.click_events
		ORDER BY clicked_at DESC
		LIMIT ?
	`

	rows, err := c.conn.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query click events: %w", err)
	}
	defer rows.Close()

	var events []ClickEvent
	for rows.Next() {
		var event ClickEvent
		err := rows.Scan(
			&event.EventID,
			&event.ShortCode,
			&event.OriginalURL,
			&event.ClickedAt,
			&event.IPAddress,
			&event.Country,
			&event.CountryCode,
			&event.Region,
			&event.City,
			&event.Browser,
			&event.BrowserVersion,
			&event.OS,
			&event.OSVersion,
			&event.DeviceType,
			&event.Referer,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan click event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return events, nil
}
