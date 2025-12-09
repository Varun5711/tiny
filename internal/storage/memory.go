package storage

import (
	"fmt"
	"sync"

	"github.com/Varun5711/shorternit/internal/models"
)

type MemoryStorage struct {
	mu   sync.RWMutex
	urls map[string]*models.URL
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		urls: make(map[string]*models.URL),
	}
}

func (s *MemoryStorage) Save(url *models.URL) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[url.ShortCode]; exists {
		return fmt.Errorf("URL with short code %s already exists", url.ShortCode)
	}

	s.urls[url.ShortCode] = url
	return nil
}

func (s *MemoryStorage) GetByShortCode(shortCode string) (*models.URL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	url, exists := s.urls[shortCode]
	if !exists {
		return nil, nil
	}

	return url, nil
}

func (s *MemoryStorage) IncrementClicks(shortCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	url, exists := s.urls[shortCode]
	if !exists {
		return fmt.Errorf("URL with short code %s not found", shortCode)
	}

	url.Clicks++
	return nil
}

func (s *MemoryStorage) List() ([]*models.URL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	urls := make([]*models.URL, 0, len(s.urls))
	for _, url := range s.urls {
		urls = append(urls, url)
	}

	return urls, nil
}
