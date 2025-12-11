package clickhouse

import (
	"context"
	"fmt"
	"time"
)

type CountryStats struct {
	Country     string
	CountryCode string
	ClickCount  uint64
	Percentage  float64
}

type DeviceStats struct {
	DeviceType string
	Browser    string
	OS         string
	ClickCount uint64
	Percentage float64
}

type TimeSeriesPoint struct {
	Timestamp      time.Time
	ClickCount     uint64
	UniqueVisitors uint64
}

type URLStats struct {
	ShortCode      string
	TotalClicks    uint64
	UniqueVisitors uint64
	LastClickedAt  time.Time
}

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
	defer rows.Close()

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
	defer rows.Close()

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
	defer rows.Close()

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
	defer rows.Close()

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
	defer rows.Close()

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
