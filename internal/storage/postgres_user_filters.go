package storage

import (
	"context"
	"fmt"

	"github.com/Varun5711/shorternit/internal/models"
)

// ListByUserID returns all non-expired URLs owned by the given user, ordered
// by creation time (newest first). The query runs against a read replica and
// filters out expired rows at the database level. For paginated access, use
// ListByUserIDPaginated instead to avoid unbounded result sets.
func (s *PostgresStorage) ListByUserID(ctx context.Context, userID string) ([]*models.URL, error) {
	// SELECT all active URLs for a specific user. COALESCE converts NULL
	// qr_code and user_id values to empty strings so pgx can scan them into
	// Go string fields without error.
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
