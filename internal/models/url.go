package models

import "time"

type URL struct {
	ID        int64     `json:"id"`
	ShortCode string    `json:"short_code"`
	LongURL   string    `json:"long_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateURLRequest struct {
	LongURL string `json:"long_url"`
}

type CreateURLResponse struct {
	ShortCode string    `json:"short_code"`
	ShortURL  string    `json:"short_url"` // Full URL: http://localhost:8080/{ShortCode}
	LongURL   string    `json:"long_url"`
	CreatedAt time.Time `json:"created_at"`
}

type ListURLsResponse struct {
	URLs []URL `json:"urls"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
