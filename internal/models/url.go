// Package models defines the core domain types used across the tiny URL
// shortener. These structs serve as the canonical representation of business
// entities and are shared between the service layer, storage layer, and HTTP/
// gRPC API boundaries. JSON struct tags are included so the same types can be
// serialized directly in REST responses from the API gateway.
package models

import "time"

// URL is the central domain entity representing a shortened URL.
//
// A URL can be created in two ways:
//   - System-generated: the ShortCode is derived from a Snowflake ID and
//     base62-encoded, guaranteeing global uniqueness without coordination.
//   - Custom alias: the user supplies a human-readable ShortCode that is
//     validated, locked via Redis, and checked against the primary database
//     before insertion.
//
// ExpiresAt is a pointer so that URLs without an explicit TTL are represented
// as NULL in the database and omitted from JSON responses (omitempty).
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

// CreateURLRequest is the REST API request body for creating a new shortened
// URL with a system-generated short code.
type CreateURLRequest struct {
	LongURL   string     `json:"long_url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateURLResponse is the REST API response returned after successfully
// creating a shortened URL. It includes the generated QR code (base64-encoded
// PNG) so clients can display it without a second round-trip.
type CreateURLResponse struct {
	ShortCode string     `json:"short_code"`
	ShortURL  string     `json:"short_url"`
	LongURL   string     `json:"long_url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	QRCode    string     `json:"qr_code,omitempty"`
}

// CreateCustomURLRequest is the REST API request body for creating a shortened
// URL with a user-chosen alias (e.g., "my-link") instead of a random code.
type CreateCustomURLRequest struct {
	Alias     string     `json:"alias"`
	LongURL   string     `json:"long_url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateCustomURLResponse mirrors CreateURLResponse but is returned by the
// custom-alias endpoint. The ShortCode field contains the user-chosen alias.
type CreateCustomURLResponse struct {
	ShortCode string     `json:"short_code"`
	ShortURL  string     `json:"short_url"`
	LongURL   string     `json:"long_url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	QRCode    string     `json:"qr_code,omitempty"`
}

// ListURLsResponse wraps a page of URL results along with pagination metadata
// so the client knows whether additional pages are available.
type ListURLsResponse struct {
	URLs    []URL `json:"urls"`
	Total   int32 `json:"total,omitempty"`
	HasMore bool  `json:"has_more,omitempty"`
}

// ErrorResponse is a generic envelope for API errors, providing both a
// machine-readable error code string and an optional human-readable message.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
