package storage

import "github.com/Varun5711/shorternit/internal/models"

type Storage interface {
	Save(url *models.URL) error
	GetByShortCode(shortCode string) (*models.URL, error)
	IncrementClicks(shortCode string) error
	List() ([]*models.URL, error)
}
