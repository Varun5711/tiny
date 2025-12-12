package storage

import (
	"context"
	"time"

	"github.com/Varun5711/shorternit/internal/models"
)

type Storage interface {
	Save(url *models.URL) error
	GetByShortCode(shortCode string) (*models.URL, error)
	IncrementClicks(shortCode string) error
	List() ([]*models.URL, error)
	ListByUserID(userID string) ([]*models.URL, error)
	AliasExists(ctx context.Context, alias string) (bool, error)
	AliasExistsPrimary(ctx context.Context, alias string) (bool, error)
	CreateCustomURL(ctx context.Context, alias, longURL string, expiresAt *time.Time, qrCode, userID string) error
	DeleteExpiredURLs(ctx context.Context) (int64, error)
}
