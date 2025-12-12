package analytics

import (
	"context"
	"time"

	"github.com/Varun5711/shorternit/internal/database"
)

type Service struct {
	db *database.DBManager
}

func NewService(db *database.DBManager) *Service {
	return &Service{db: db}
}

type URLStats struct {
	ShortCode      string
	TotalClicks    int64
	UniqueVisitors int64
	Last24Hours    int64
	Last7Days      int64
	Last30Days     int64
}

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

type TimelinePoint struct {
	Timestamp time.Time
	Clicks    int64
}

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

type GeoStat struct {
	Country string
	Clicks  int64
}

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

type DeviceStats struct {
	Desktop int64
	Mobile  int64
	Bot     int64
	Total   int64
}

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

type RefererStat struct {
	Referer string
	Clicks  int64
}

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
