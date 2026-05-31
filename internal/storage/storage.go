package storage

import (
	"context"
	"time"

	"github.com/Varun5711/shorternit/internal/models"
)

type Storage interface {
	Save(ctx context.Context, url *models.URL) error
	GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error)
	IncrementClicks(ctx context.Context, shortCode string) error
	List(ctx context.Context) ([]*models.URL, error)
	ListByUserID(ctx context.Context, userID string) ([]*models.URL, error)
	AliasExists(ctx context.Context, alias string) (bool, error)
	AliasExistsPrimary(ctx context.Context, alias string) (bool, error)
	CreateCustomURL(ctx context.Context, alias, longURL string, expiresAt *time.Time, qrCode, userID string) error
	DeleteExpiredURLs(ctx context.Context) (int64, error)
}
