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

// PostgresStorage is the production implementation of the Storage interface,
// backed by PostgreSQL. It uses database.DBManager to route reads to replicas
// (via db.Read()) and writes to the primary (via db.Write()), enabling
// horizontal read scaling without changing the call sites.
type PostgresStorage struct {
	db *database.DBManager
}

// NewPostgresStorage creates a PostgresStorage that uses the given DBManager
// for all database access. The DBManager is expected to be fully initialized
// with connection pools for both the primary and replica(s).
func NewPostgresStorage(db *database.DBManager) *PostgresStorage {
	return &PostgresStorage{
		db: db,
	}
}

// Save inserts a new URL record into the urls table on the primary database.
// All fields are caller-provided except updated_at, which is set to NOW() at
// insert time. The write goes through db.Write() to ensure it hits the primary.
func (s *PostgresStorage) Save(ctx context.Context, url *models.URL) error {
	// INSERT a complete URL row. $1-$8 map to the URL struct fields plus the
	// current timestamp for updated_at.
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

// GetByShortCode fetches a single URL by its short code from a read replica.
// Expired URLs (expires_at <= NOW()) are excluded at the query level so
// callers never see stale links. Returns (nil, nil) when no matching row
// exists, allowing the service layer to distinguish "not found" from a real
// database error without sentinel error types.
func (s *PostgresStorage) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	// SELECT the URL only if it has not expired. COALESCE guards against NULL
	// qr_code values so the Go string field is always populated (empty string
	// rather than a scan error).
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

// IncrementClicks atomically increments the click counter for a URL on the
// primary database. The UPDATE also bumps updated_at so downstream consumers
// (analytics, replication) can detect the change. If no row matches the short
// code, an error is returned (the URL may have been deleted or never existed).
func (s *PostgresStorage) IncrementClicks(ctx context.Context, shortCode string) error {
	// Atomic increment: clicks = clicks + 1 avoids read-modify-write races
	// when multiple redirects happen concurrently.
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

// List returns all non-expired URLs ordered by creation time (newest first),
// reading from a replica. This method returns an unbounded result set -- for
// production paginated access, use ListPaginated instead.
func (s *PostgresStorage) List(ctx context.Context) ([]*models.URL, error) {
	// SELECT all active URLs. COALESCE on qr_code and user_id converts NULLs
	// to empty strings to avoid pgx scan errors on Go string fields.
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

// Delete hard-deletes a URL record from the primary database. Returns an
// error if the short code does not exist (RowsAffected == 0).
func (s *PostgresStorage) Delete(ctx context.Context, shortCode string) error {
	// Hard DELETE by short_code. No soft-delete is used because expired URLs
	// are already cleaned up by DeleteExpiredURLs.
	query := `DELETE FROM urls WHERE short_code = $1`
	cmdTag, err := s.db.Write().Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("URL with short code %s not found", shortCode)
	}
	return nil
}

// ListPaginated returns a single page of non-expired URLs along with the
// total count of matching rows, both served from a read replica. The total
// count is fetched first (a separate COUNT query) so the client can render
// pagination controls. Results are sorted newest-first.
func (s *PostgresStorage) ListPaginated(ctx context.Context, limit, offset int32) ([]*models.URL, int32, error) {
	var total int32
	// COUNT all active (non-expired) URLs to support client-side pagination.
	countQuery := `SELECT COUNT(*) FROM urls WHERE (expires_at IS NULL OR expires_at > NOW())`
	if err := s.db.Read().QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count URLs: %w", err)
	}

	query := `
		SELECT short_code, long_url, clicks, created_at, expires_at, COALESCE(qr_code, ''), COALESCE(user_id, '')
		FROM urls
		WHERE (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := s.db.Read().Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list URLs: %w", err)
	}
	defer rows.Close()

	var urls []*models.URL
	for rows.Next() {
		var url models.URL
		if err := rows.Scan(&url.ShortCode, &url.LongURL, &url.Clicks, &url.CreatedAt, &url.ExpiresAt, &url.QRCode, &url.UserID); err != nil {
			return nil, 0, fmt.Errorf("failed to scan row: %w", err)
		}
		urls = append(urls, &url)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating rows: %w", err)
	}
	return urls, total, nil
}

// ListByUserIDPaginated returns a single page of non-expired URLs owned by
// the specified user, along with the total count. Like ListPaginated, both
// queries run against a read replica.
func (s *PostgresStorage) ListByUserIDPaginated(ctx context.Context, userID string, limit, offset int32) ([]*models.URL, int32, error) {
	var total int32
	// COUNT only URLs belonging to this user that have not expired.
	countQuery := `SELECT COUNT(*) FROM urls WHERE user_id = $1 AND (expires_at IS NULL OR expires_at > NOW())`
	if err := s.db.Read().QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count URLs: %w", err)
	}

	query := `
		SELECT short_code, long_url, clicks, created_at, expires_at, COALESCE(qr_code, ''), COALESCE(user_id, '')
		FROM urls
		WHERE user_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := s.db.Read().Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list URLs: %w", err)
	}
	defer rows.Close()

	var urls []*models.URL
	for rows.Next() {
		var url models.URL
		if err := rows.Scan(&url.ShortCode, &url.LongURL, &url.Clicks, &url.CreatedAt, &url.ExpiresAt, &url.QRCode, &url.UserID); err != nil {
			return nil, 0, fmt.Errorf("failed to scan row: %w", err)
		}
		urls = append(urls, &url)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating rows: %w", err)
	}
	return urls, total, nil
}

// AliasExists checks whether a short code / custom alias already exists in
// the urls table. The query runs against a read replica, so the result may be
// stale under replication lag. Use AliasExistsPrimary when strong consistency
// is required.
func (p *PostgresStorage) AliasExists(ctx context.Context, alias string) (bool, error) {
	var exists bool
	// EXISTS subquery: returns true if at least one row matches, without
	// transferring any row data -- efficient for existence checks.
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)`
	err := p.db.Read().QueryRow(ctx, query, alias).Scan(&exists)
	return exists, err
}

// AliasExistsPrimary performs the same existence check as AliasExists but
// forces the query through the primary database (db.Write()) to guarantee a
// strongly-consistent read. This is essential inside the custom-alias creation
// flow where a distributed lock is held: a stale replica read could falsely
// report the alias as available, leading to a duplicate-key error on INSERT.
func (p *PostgresStorage) AliasExistsPrimary(ctx context.Context, alias string) (bool, error) {
	var exists bool
	// Same EXISTS query as AliasExists, but routed to the primary for strong
	// consistency.
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)`
	err := p.db.Write().QueryRow(ctx, query, alias).Scan(&exists)
	return exists, err
}

// CreateCustomURL inserts a URL with a user-chosen alias as the short code.
// Unlike Save (which takes a fully-populated URL struct), this method lets
// PostgreSQL generate the timestamps via NOW() and uses RETURNING to capture
// the server-side created_at. If the alias violates the unique constraint on
// short_code, the duplicate-key error is translated into a user-friendly
// "alias already taken" message.
func (p *PostgresStorage) CreateCustomURL(ctx context.Context, alias, longURL string, expiresAt *time.Time, qrCode, userID string) error {
	// INSERT with server-generated timestamps. RETURNING created_at lets us
	// capture the exact timestamp without a follow-up SELECT.
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

// DeleteExpiredURLs bulk-deletes all URL records whose expiration timestamp
// has passed. It is designed to be called periodically by a background cleanup
// goroutine. Returns the number of rows removed so the caller can log or
// meter the cleanup volume.
func (p *PostgresStorage) DeleteExpiredURLs(ctx context.Context) (int64, error) {
	// DELETE all rows where expires_at is in the past. URLs with a NULL
	// expires_at live forever and are excluded by the IS NOT NULL guard.
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
