package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/models"
	"github.com/jackc/pgx/v5"
)

type PostgresStorage struct {
	db *database.DBManager
}

func NewPostgresStorage(db *database.DBManager) *PostgresStorage {
	return &PostgresStorage{
		db: db,
	}
}

func (s *PostgresStorage) Save(url *models.URL) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO urls (short_code, long_url, clicks, expires_at, qr_code, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := s.db.Write().Exec(ctx, query,
		url.ShortCode,
		url.LongURL,
		url.Clicks,
		url.ExpiresAt,
		url.QRCode,
		url.UserID,
		url.CreatedAt,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to save URL: %w", err)
	}

	return nil
}

func (s *PostgresStorage) GetByShortCode(shortCode string) (*models.URL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT short_code, long_url, clicks, created_at, expires_at, COALESCE(qr_code, '')
		FROM urls
		WHERE short_code = $1
		AND (expires_at IS NULL OR expires_at > NOW())
	`

	var url models.URL
	err := s.db.Read().QueryRow(ctx, query, shortCode).Scan(
		&url.ShortCode,
		&url.LongURL,
		&url.Clicks,
		&url.CreatedAt,
		&url.ExpiresAt,
		&url.QRCode,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return &url, nil
}

func (s *PostgresStorage) IncrementClicks(shortCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE urls
		SET clicks = clicks + 1,
			updated_at = NOW()
		WHERE short_code = $1
	`

	cmdTag, err := s.db.Write().Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment clicks: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("URL with short code %s not found", shortCode)
	}

	return nil
}

func (s *PostgresStorage) List() ([]*models.URL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT short_code, long_url, clicks, created_at, expires_at, COALESCE(qr_code, ''), COALESCE(user_id, '')
		FROM urls
		WHERE (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
	`

	rows, err := s.db.Read().Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs: %w", err)
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

func (p *PostgresStorage) AliasExists(ctx context.Context, alias string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)`
	err := p.db.Read().QueryRow(ctx, query, alias).Scan(&exists)
	return exists, err
}

func (p *PostgresStorage) AliasExistsPrimary(ctx context.Context, alias string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)`
	err := p.db.Write().QueryRow(ctx, query, alias).Scan(&exists)
	return exists, err
}

func (p *PostgresStorage) CreateCustomURL(ctx context.Context, alias, longURL string, expiresAt *time.Time, qrCode, userID string) error {
	query := `
		INSERT INTO urls (short_code, long_url, expires_at, qr_code, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING created_at
	`

	var createdAt time.Time
	err := p.db.Write().QueryRow(ctx, query, alias, longURL, expiresAt, qrCode, userID).Scan(&createdAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return errors.New("alias already taken")
		}
		return err
	}

	return nil
}

func (p *PostgresStorage) DeleteExpiredURLs(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM urls
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`

	cmdTag, err := p.db.Write().Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired URLs: %w", err)
	}

	return cmdTag.RowsAffected(), nil
}
