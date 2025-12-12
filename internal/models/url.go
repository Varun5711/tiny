package models

import "time"

type URL struct {
	ShortCode string     `json:"short_code"`
	ShortURL  string     `json:"short_url,omitempty"`
	LongURL   string     `json:"long_url"`
	Clicks    int64      `json:"clicks"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	QRCode    string     `json:"qr_code,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
}

type CreateURLRequest struct {
	LongURL   string     `json:"long_url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateURLResponse struct {
	ShortCode string     `json:"short_code"`
	ShortURL  string     `json:"short_url"`
	LongURL   string     `json:"long_url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	QRCode    string     `json:"qr_code,omitempty"`
}

type CreateCustomURLRequest struct {
	Alias     string     `json:"alias"`
	LongURL   string     `json:"long_url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateCustomURLResponse struct {
	ShortCode string     `json:"short_code"`
	ShortURL  string     `json:"short_url"`
	LongURL   string     `json:"long_url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	QRCode    string     `json:"qr_code,omitempty"`
}

type ListURLsResponse struct {
	URLs    []URL `json:"urls"`
	Total   int32 `json:"total,omitempty"`
	HasMore bool  `json:"has_more,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
