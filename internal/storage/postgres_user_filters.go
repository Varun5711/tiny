package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Varun5711/shorternit/internal/models"
)

func (s *PostgresStorage) ListByUserID(userID string) ([]*models.URL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT short_code, long_url, clicks, created_at, expires_at, COALESCE(qr_code, ''), COALESCE(user_id, '')
		FROM urls
		WHERE user_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
	`

	rows, err := s.db.Read().Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs by user: %w", err)
	}
	defer rows.Close()

	var urls []*models.URL
	for rows.Next() {
		var url models.URL
		err := rows.Scan(
			&url.ShortCode,
			&url.LongURL,
			&url.Clicks,
			&url.CreatedAt,
			&url.ExpiresAt,
			&url.QRCode,
			&url.UserID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		urls = append(urls, &url)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return urls, nil
}
