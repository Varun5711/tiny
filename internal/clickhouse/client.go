// Package clickhouse provides a client for writing click-analytics events to
// ClickHouse and querying pre-aggregated analytics data. ClickHouse is used
// instead of (or alongside) Elasticsearch for analytics because its columnar
// storage engine and materialized views make aggregation queries over billions
// of rows orders of magnitude faster than a general-purpose search engine.
//
// The analytics schema follows a common ClickHouse pattern:
//
//  1. Raw events land in the analytics.click_events table (MergeTree engine).
//  2. Materialized views (clicks_by_country, clicks_by_device, hourly_clicks,
//     daily_clicks_by_url) automatically pre-aggregate incoming rows at insert
//     time using the SummingMergeTree engine. These views act as persistent,
//     incrementally-updated rollups.
//  3. Read queries hit the materialized views instead of scanning raw events,
//     reducing query latency from seconds to milliseconds for dashboard panels.
//
// Batch inserts are the primary write path because ClickHouse is optimized
// for large, infrequent inserts rather than row-at-a-time writes.
package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/Varun5711/shorternit/internal/config"
)

// Client wraps a ClickHouse connection and provides domain-specific methods
// for inserting click events and querying analytics data.
type Client struct {
	conn driver.Conn
}

// NewClient opens a native-protocol connection to ClickHouse, pings the
// server to verify connectivity, and returns the wrapped client. Connection
// pool settings (MaxOpenConns, MaxIdleConns, ConnMaxLifetime) are derived
// from the application config. ConnOpenInOrder is used so connections are
// opened to the first healthy address, which is appropriate for single-node
// deployments or when a load balancer sits in front of the cluster.
func NewClient(cfg config.ClickHouseConfig) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			// Guard against runaway queries consuming server resources.
			"max_execution_time": 60,
		},
		DialTimeout:  time.Second * 30,
		MaxOpenConns: cfg.MaxConns,
		// Keep half the pool idle to balance resource usage with connection reuse.
		MaxIdleConns:     cfg.MaxConns / 2,
		ConnMaxLifetime:  time.Hour,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to clickhouse: %w", err)
	}

	// Ping verifies the connection is alive before returning the client,
	// so callers get an immediate error instead of a deferred failure.
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	return &Client{conn: conn}, nil
}

// Close releases the underlying ClickHouse connection pool.
func (c *Client) Close() error {
	return c.conn.Close()
}

// ClickEvent represents a single URL redirect event with full geo-IP and
// user-agent metadata. This struct maps 1:1 to the analytics.click_events
// table columns. Boolean-like fields (IsMobile, IsTablet, etc.) use uint8
// because ClickHouse's UInt8 type is the idiomatic way to store booleans.
type ClickEvent struct {
	EventID     string
	ShortCode   string
	OriginalURL string
	ClickedAt   time.Time

	// Geo-IP fields, populated by the redirect handler's IP lookup.
	IPAddress   string
	Country     string
	CountryCode string
	Region      string
	City        string
	Latitude    float64
	Longitude   float64
	Timezone    string

	// User-agent fields, parsed from the request's User-Agent header.
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

// InsertClickEvents writes a batch of click events to the analytics.click_events
// table using ClickHouse's native batch insert protocol. Batch inserts are
// critical for ClickHouse performance -- the engine is designed for large,
// infrequent inserts (ideally thousands of rows per batch) rather than
// row-at-a-time writes, because each insert creates a new data part that
// must later be merged in the background.
//
// When rows land in click_events, ClickHouse's materialized views
// (clicks_by_country, clicks_by_device, hourly_clicks, daily_clicks_by_url)
// automatically process the new data and update their pre-aggregated
// SummingMergeTree tables. This happens at insert time with no extra code
// needed here.
func (c *Client) InsertClickEvents(ctx context.Context, events []ClickEvent) error {
	if len(events) == 0 {
		return nil
	}

	// PrepareBatch opens a native-protocol batch insert. Rows are appended
	// in memory and sent to ClickHouse in a single network round trip on Send().
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

	// Send transmits the entire batch to ClickHouse in one network call.
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// Exec runs an arbitrary DDL or DML statement (e.g. CREATE TABLE, ALTER)
// against the ClickHouse connection. It is used for schema migrations and
// administrative operations that do not return result sets.
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.conn.Exec(ctx, query, args...)
}

// Query runs an arbitrary SELECT and returns the raw row iterator. Callers
// are responsible for scanning and closing the rows. This is useful for
// ad-hoc or dynamically constructed queries that do not have a dedicated
// method on the Client.
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.conn.Query(ctx, query, args...)
}

// GetClickEvents retrieves raw click events for a specific short code from the
// analytics.click_events table, ordered by most recent first. This queries the
// raw event table (not a materialized view) because callers need individual
// event details rather than aggregated counts.
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
	defer func() { _ = rows.Close() }()

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

// GetAllClickEvents retrieves the most recent raw click events across all
// short codes. This is used for admin-level analytics views that show
// system-wide activity. For per-URL queries, use GetClickEvents instead.
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
	defer func() { _ = rows.Close() }()

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
