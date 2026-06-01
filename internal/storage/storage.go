// Package storage defines the repository interfaces and their concrete
// implementations for persisting URL and user data.
//
// The package follows the repository pattern: the Storage interface declares
// all operations the service layer needs, and PostgresStorage provides the
// production implementation backed by PostgreSQL (with read-replica routing
// via database.DBManager). This separation allows the service layer to remain
// database-agnostic and makes it straightforward to swap in an in-memory or
// mock implementation for testing.
package storage

import (
	"context"
	"time"

	"github.com/Varun5711/shorternit/internal/models"
)

// Storage is the primary repository interface for URL operations.
//
// Every method accepts a context.Context to support request-scoped deadlines,
// cancellation, and tracing. Implementations are expected to be safe for
// concurrent use by multiple goroutines (the gRPC server dispatches each RPC
// on its own goroutine).
//
// Read vs. write routing is an implementation detail -- callers interact only
// with this interface and never choose which database replica to hit.
type Storage interface {
	// Save persists a new shortened URL record. The caller is responsible for
	// populating the ShortCode (via Snowflake ID generation) and timestamps
	// before calling Save.
	Save(ctx context.Context, url *models.URL) error

	// GetByShortCode retrieves a URL by its short code. Returns (nil, nil) if
	// no matching, non-expired URL exists -- this lets the service layer
	// distinguish "not found" from a real database error.
	GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error)

	// IncrementClicks atomically bumps the click counter for the given short
	// code. Returns an error if the short code does not exist.
	IncrementClicks(ctx context.Context, shortCode string) error

	// List returns all non-expired URLs ordered by creation time (newest first).
	// Prefer ListPaginated for production use to avoid unbounded result sets.
	List(ctx context.Context) ([]*models.URL, error)

	// ListByUserID returns all non-expired URLs owned by the given user.
	// Prefer ListByUserIDPaginated for production use.
	ListByUserID(ctx context.Context, userID string) ([]*models.URL, error)

	// AliasExists checks whether a short code / custom alias already exists.
	// The check may be served by a read replica, so it is subject to
	// replication lag. Use AliasExistsPrimary when strong consistency is
	// required (e.g., inside a distributed lock before creating a custom alias).
	AliasExists(ctx context.Context, alias string) (bool, error)

	// AliasExistsPrimary performs the same existence check as AliasExists but
	// always reads from the primary database to guarantee a strongly-consistent
	// result. This is critical for the custom-alias creation flow where a stale
	// read could allow duplicate aliases.
	AliasExistsPrimary(ctx context.Context, alias string) (bool, error)

	// CreateCustomURL inserts a URL record that uses a user-chosen alias
	// instead of a Snowflake-generated short code. The alias must have been
	// validated and locked before calling this method.
	CreateCustomURL(ctx context.Context, alias, longURL string, expiresAt *time.Time, qrCode, userID string) error

	// Delete hard-deletes a URL record by short code. Returns an error if the
	// short code does not exist.
	Delete(ctx context.Context, shortCode string) error

	// DeleteExpiredURLs removes all URL records whose expiration time has
	// passed. Returns the number of rows deleted. This is typically called by
	// a background cleanup job on a scheduled interval.
	DeleteExpiredURLs(ctx context.Context) (int64, error)

	// ListPaginated returns a page of non-expired URLs along with the total
	// count of matching records. limit and offset control pagination.
	ListPaginated(ctx context.Context, limit, offset int32) ([]*models.URL, int32, error)

	// ListByUserIDPaginated returns a page of non-expired URLs owned by the
	// given user along with the total count.
	ListByUserIDPaginated(ctx context.Context, userID string, limit, offset int32) ([]*models.URL, int32, error)
}
