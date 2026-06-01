// Package analytics provides read-only query methods for click-event data
// stored in PostgreSQL. It serves the analytics API endpoints and the TUI
// analytics view by aggregating raw click rows into meaningful summaries:
// time-series click counts, geographic breakdowns, device-type distributions,
// and top referrers.
//
// All queries use the DBManager's read replica connection (s.db.Read()) to
// avoid placing analytical load on the primary write database, which keeps
// URL creation and redirect latency unaffected by expensive COUNT/GROUP BY
// queries.
package analytics

import (
	"context"
	"time"

	"github.com/Varun5711/shorternit/internal/database"
)

// Service is the analytics query layer. It holds a reference to the database
// manager and exposes one method per analytics dimension. Each method runs a
// single, focused SQL query rather than pulling all data and filtering in Go,
// because the database is better at aggregation and the click table can grow
// very large.
type Service struct {
	db *database.DBManager
}

// NewService creates an analytics Service backed by the given DBManager.
func NewService(db *database.DBManager) *Service {
	return &Service{db: db}
}

// URLStats holds the high-level click metrics for a single short URL.
// The rolling time windows (24h, 7d, 30d) let the dashboard show trend
// sparklines without requiring a full timeline query.
type URLStats struct {
	ShortCode      string
	TotalClicks    int64
	UniqueVisitors int64
	Last24Hours    int64
	Last7Days      int64
	Last30Days     int64
}

// GetURLStats computes aggregate click metrics for a short code.
// Total clicks and unique visitors come from one query; the three time-window
// counts are fetched separately. If a time-window query fails (e.g., on a
// fresh database with no clicks), that counter defaults to zero rather than
// failing the entire call, because the total/unique data is still valuable.
func (s *Service) GetURLStats(ctx context.Context, shortCode string) (*URLStats, error) {
	conn := s.db.Read()

	var stats URLStats
	stats.ShortCode = shortCode

	err := conn.QueryRow(ctx, `
		SELECT
			COUNT(*) as total_clicks,
			COUNT(DISTINCT ip_address) as unique_visitors
		FROM clicks
		WHERE short_code = $1
	`, shortCode).Scan(&stats.TotalClicks, &stats.UniqueVisitors)

	if err != nil {
		return nil, err
	}

	now := time.Now()

	// Each time-window query is independent; a failure in one should not
	// prevent the others from populating, so errors are swallowed and the
	// count defaults to zero.
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM clicks
		WHERE short_code = $1 AND clicked_at > $2
	`, shortCode, now.Add(-24*time.Hour)).Scan(&stats.Last24Hours)
	if err != nil {
		stats.Last24Hours = 0
	}

	err = conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM clicks
		WHERE short_code = $1 AND clicked_at > $2
	`, shortCode, now.Add(-7*24*time.Hour)).Scan(&stats.Last7Days)
	if err != nil {
		stats.Last7Days = 0
	}

	err = conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM clicks
		WHERE short_code = $1 AND clicked_at > $2
	`, shortCode, now.Add(-30*24*time.Hour)).Scan(&stats.Last30Days)
	if err != nil {
		stats.Last30Days = 0
	}

	return &stats, nil
}

// TimelinePoint represents a single data point in the click timeline chart,
// bucketed by day.
type TimelinePoint struct {
	Timestamp time.Time
	Clicks    int64
}

// GetClickTimeline returns daily click counts for the given short code over
// the last N days. It uses PostgreSQL's time_bucket function (from the
// TimescaleDB extension if available, or a compatible shim) to aggregate
// clicks into 1-day buckets. Results are ordered chronologically so the
// frontend can render them directly as a time-series chart.
func (s *Service) GetClickTimeline(ctx context.Context, shortCode string, days int) ([]TimelinePoint, error) {
	conn := s.db.Read()

	startDate := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	rows, err := conn.Query(ctx, `
		SELECT
			time_bucket('1 day', clicked_at) as bucket,
			COUNT(*) as clicks
		FROM clicks
		WHERE short_code = $1 AND clicked_at > $2
		GROUP BY bucket
		ORDER BY bucket ASC
	`, shortCode, startDate)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var timeline []TimelinePoint
	for rows.Next() {
		var point TimelinePoint
		if err := rows.Scan(&point.Timestamp, &point.Clicks); err != nil {
			return nil, err
		}
		timeline = append(timeline, point)
	}

	return timeline, rows.Err()
}

// GeoStat pairs a country name with its click count for geographic
// breakdown charts.
type GeoStat struct {
	Country string
	Clicks  int64
}

// GetGeoStats returns the top 10 countries by click count for a short code.
// The LIMIT 10 keeps the response compact for dashboard rendering; NULL
// countries (clicks with no geo enrichment) are excluded to avoid a confusing
// blank entry in the chart.
func (s *Service) GetGeoStats(ctx context.Context, shortCode string) ([]GeoStat, error) {
	conn := s.db.Read()

	rows, err := conn.Query(ctx, `
		SELECT
			country,
			COUNT(*) as clicks
		FROM clicks
		WHERE short_code = $1 AND country IS NOT NULL
		GROUP BY country
		ORDER BY clicks DESC
		LIMIT 10
	`, shortCode)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []GeoStat
	for rows.Next() {
		var stat GeoStat
		if err := rows.Scan(&stat.Country, &stat.Clicks); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// DeviceStats aggregates click counts by device category. The three
// categories (Desktop, Mobile, Bot) mirror the device types produced
// by the enrichment.ParseUserAgent function. Total is the sum across
// all categories, including any unknown device types that do not match
// the switch cases.
type DeviceStats struct {
	Desktop int64
	Mobile  int64
	Bot     int64
	Total   int64
}

// GetDeviceStats returns the device-type breakdown for a short code.
// The query groups by the device_type column populated during click
// enrichment. Unknown device types still contribute to Total even though
// they are not individually surfaced, ensuring Total always matches the
// sum of all clicks.
func (s *Service) GetDeviceStats(ctx context.Context, shortCode string) (*DeviceStats, error) {
	conn := s.db.Read()

	rows, err := conn.Query(ctx, `
		SELECT
			device_type,
			COUNT(*) as clicks
		FROM clicks
		WHERE short_code = $1
		GROUP BY device_type
	`, shortCode)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &DeviceStats{}
	for rows.Next() {
		var deviceType string
		var clicks int64
		if err := rows.Scan(&deviceType, &clicks); err != nil {
			return nil, err
		}

		switch deviceType {
		case "desktop":
			stats.Desktop = clicks
		case "mobile":
			stats.Mobile = clicks
		case "bot":
			stats.Bot = clicks
		}
		stats.Total += clicks
	}

	return stats, rows.Err()
}

// RefererStat pairs a referrer URL (or "direct" for null referrers)
// with its click count for the top-referrers leaderboard.
type RefererStat struct {
	Referer string
	Clicks  int64
}

// GetTopReferrers returns the highest-traffic referrer sources for a short
// code, limited to the specified count. NULL referrers are coalesced to the
// string "direct" so the frontend always has a displayable label. The result
// is ordered by click count descending.
func (s *Service) GetTopReferrers(ctx context.Context, shortCode string, limit int) ([]RefererStat, error) {
	conn := s.db.Read()

	rows, err := conn.Query(ctx, `
		SELECT
			COALESCE(referer, 'direct') as referer,
			COUNT(*) as clicks
		FROM clicks
		WHERE short_code = $1
		GROUP BY referer
		ORDER BY clicks DESC
		LIMIT $2
	`, shortCode, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []RefererStat
	for rows.Next() {
		var stat RefererStat
		if err := rows.Scan(&stat.Referer, &stat.Clicks); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}
