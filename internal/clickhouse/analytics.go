package clickhouse

import (
	"context"
	"fmt"
	"time"
)

// CountryStats holds aggregated click counts broken down by country.
// Percentage is computed server-side as a proportion of total clicks for
// the requested short code and date range.
type CountryStats struct {
	Country     string
	CountryCode string
	ClickCount  uint64
	Percentage  float64
}

// DeviceStats holds aggregated click counts broken down by device type,
// browser, and operating system. This powers the "Device Breakdown" panel
// on the analytics dashboard.
type DeviceStats struct {
	DeviceType string
	Browser    string
	OS         string
	ClickCount uint64
	Percentage float64
}

// TimeSeriesPoint represents a single data point in a time-series chart,
// containing both total clicks and unique visitors (by IP) for the bucket.
type TimeSeriesPoint struct {
	Timestamp      time.Time
	ClickCount     uint64
	UniqueVisitors uint64
}

// URLStats contains overall lifetime statistics for a single shortened URL.
type URLStats struct {
	ShortCode      string
	TotalClicks    uint64
	UniqueVisitors uint64
	LastClickedAt  time.Time
}

// GetCountryStats returns click counts grouped by country for a specific URL
// and date range. It queries the analytics.clicks_by_country materialized view,
// which is backed by a SummingMergeTree engine.
//
// Materialized views in ClickHouse act as continuously-updated rollup tables:
// when rows are inserted into the raw click_events table, the materialized
// view's SELECT runs automatically and inserts pre-aggregated rows into the
// destination table. SummingMergeTree then collapses rows with the same
// sorting key by summing numeric columns during background merges. This
// means reads against the view scan far fewer rows than the raw table --
// typically one row per (short_code, country, date) combination instead of
// one row per click event.
//
// The percentage subquery computes each country's share of total clicks
// within the same date range, avoiding a separate round trip.
func (c *Client) GetCountryStats(ctx context.Context, shortCode string, startDate, endDate time.Time) ([]CountryStats, error) {
	query := `
  		SELECT
  			country,
  			country_code,
  			sum(click_count) AS total_clicks,
  			(total_clicks * 100.0 / (SELECT sum(click_count) FROM analytics.clicks_by_country
  				WHERE short_code = ? AND clicked_date BETWEEN ? AND ?)) AS percentage
  		FROM analytics.clicks_by_country
  		WHERE short_code = ?
  			AND clicked_date BETWEEN ? AND ?
  			AND country != ''
  		GROUP BY country, country_code
  		ORDER BY total_clicks DESC
  		LIMIT 20
  	`

	rows, err := c.conn.Query(ctx, query, shortCode, startDate, endDate, shortCode, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query country stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []CountryStats
	for rows.Next() {
		var s CountryStats
		if err := rows.Scan(&s.Country, &s.CountryCode, &s.ClickCount, &s.Percentage); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetDeviceStats returns click counts grouped by device type, browser, and OS
// for a specific URL and date range. It queries the analytics.clicks_by_device
// materialized view (SummingMergeTree), which pre-aggregates per-click rows
// into per-(device, browser, os, date) summaries at insert time. This avoids
// scanning potentially millions of raw events for each dashboard load.
//
// The percentage is computed inline via a scalar subquery against the same
// materialized view, so the entire result set (counts + percentages) comes
// back in a single query.
func (c *Client) GetDeviceStats(ctx context.Context, shortCode string, startDate, endDate time.Time) ([]DeviceStats, error) {
	query := `
  		SELECT
  			device_type,
  			browser,
  			os,
  			sum(click_count) AS total_clicks,
  			(total_clicks * 100.0 / (SELECT sum(click_count) FROM analytics.clicks_by_device
  				WHERE short_code = ? AND clicked_date BETWEEN ? AND ?)) AS percentage
  		FROM analytics.clicks_by_device
  		WHERE short_code = ?
  			AND clicked_date BETWEEN ? AND ?
  		GROUP BY device_type, browser, os
  		ORDER BY total_clicks DESC
  		LIMIT 50
  	`

	rows, err := c.conn.Query(ctx, query, shortCode, startDate, endDate, shortCode, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query device stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []DeviceStats
	for rows.Next() {
		var s DeviceStats
		if err := rows.Scan(&s.DeviceType, &s.Browser, &s.OS, &s.ClickCount, &s.Percentage); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetHourlyTimeSeries returns hourly click counts and unique visitor counts
// for a specific URL within a time range. It reads from the
// analytics.hourly_clicks materialized view, where each row represents one
// (short_code, hour) bucket pre-aggregated by SummingMergeTree.
//
// Because SummingMergeTree may not have fully collapsed all parts yet, the
// query uses sum() to ensure correctness even when multiple partial rows
// exist for the same hour. Results are ordered chronologically for direct
// use in time-series charts.
func (c *Client) GetHourlyTimeSeries(ctx context.Context, shortCode string, startDate time.Time, endDate time.Time) ([]TimeSeriesPoint, error) {
	query := `
  		SELECT
  			clicked_hour,
  			sum(click_count) AS total_clicks,
  			sum(unique_visitors) AS unique_visitors
  		FROM analytics.hourly_clicks
  		WHERE short_code = ?
  			AND clicked_hour BETWEEN ? AND ?
  		GROUP BY clicked_hour
  		ORDER BY clicked_hour ASC
  	`

	rows, err := c.conn.Query(ctx, query, shortCode, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query time series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Timestamp, &p.ClickCount, &p.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		points = append(points, p)
	}

	return points, nil
}

// GetDailyTimeSeries returns daily click counts and unique visitor counts
// for a specific URL within a date range. It reads from the
// analytics.daily_clicks_by_url materialized view. This is the preferred
// query for "last 30 days" or "last 90 days" dashboard panels, where hourly
// granularity would produce too many data points.
//
// Like all SummingMergeTree-backed views, the sum() aggregation in the query
// is necessary to handle not-yet-merged parts correctly.
func (c *Client) GetDailyTimeSeries(ctx context.Context, shortCode string, startDate, endDate time.Time) ([]TimeSeriesPoint, error) {
	query := `
  		SELECT
  			clicked_date,
  			sum(click_count) AS total_clicks,
  			sum(unique_visitors) AS unique_visitors
  		FROM analytics.daily_clicks_by_url
  		WHERE short_code = ?
  			AND clicked_date BETWEEN ? AND ?
  		GROUP BY clicked_date
  		ORDER BY clicked_date ASC
  	`

	rows, err := c.conn.Query(ctx, query, shortCode, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily time series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		var date time.Time
		if err := rows.Scan(&date, &p.ClickCount, &p.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		p.Timestamp = date
		points = append(points, p)
	}

	return points, nil
}

// GetURLStats returns lifetime aggregate statistics for a single short code
// by scanning the raw analytics.click_events table. Unlike the time-series
// methods that read materialized views, this queries the raw table directly
// because it needs the uniq(ip_address) HyperLogLog approximation for unique
// visitors, which is not available in the SummingMergeTree views (they store
// pre-summed counts, not distinct sets). For URLs with very high traffic,
// consider adding a dedicated materialized view with AggregatingMergeTree
// and an uniqState column to avoid full-table scans.
func (c *Client) GetURLStats(ctx context.Context, shortCode string) (*URLStats, error) {
	query := `
  		SELECT
  			short_code,
  			count() AS total_clicks,
  			uniq(ip_address) AS unique_visitors,
  			max(clicked_at) AS last_clicked
  		FROM analytics.click_events
  		WHERE short_code = ?
  		GROUP BY short_code
  	`

	row := c.conn.QueryRow(ctx, query, shortCode)

	var stats URLStats
	if err := row.Scan(&stats.ShortCode, &stats.TotalClicks, &stats.UniqueVisitors, &stats.LastClickedAt); err != nil {
		return nil, fmt.Errorf("failed to get url stats: %w", err)
	}

	return &stats, nil
}

// GetTopReferrers returns the most common referrer URLs for a given short code
// and date range, ordered by click count descending. This helps URL owners
// understand which websites or campaigns are driving traffic to their links.
//
// The query scans the raw click_events table (filtered by clicked_date for
// partition pruning) because referrer cardinality is too high and too
// unpredictable for a practical materialized view. Empty referrers (direct
// traffic) are excluded from results.
func (c *Client) GetTopReferrers(ctx context.Context, shortCode string, startDate, endDate time.Time,
	limit int) ([]struct {
	Referer    string
	ClickCount uint64
}, error) {
	query := `
  		SELECT
  			referer,
  			count() AS click_count
  		FROM analytics.click_events
  		WHERE short_code = ?
  			AND clicked_date BETWEEN ? AND ?
  			AND referer != ''
  		GROUP BY referer
  		ORDER BY click_count DESC
  		LIMIT ?
  	`

	rows, err := c.conn.Query(ctx, query, shortCode, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query referrers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var referrers []struct {
		Referer    string
		ClickCount uint64
	}

	for rows.Next() {
		var r struct {
			Referer    string
			ClickCount uint64
		}
		if err := rows.Scan(&r.Referer, &r.ClickCount); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		referrers = append(referrers, r)
	}

	return referrers, nil
}
