// Package user defines the domain model and request/response types for user
// accounts in the tiny URL shortener. User identities are stored in PostgreSQL
// and referenced by the URL model via UserID to associate shortened links with
// their owners.
package user

import "time"

// User represents a registered account in the system. The PasswordHash field
// is tagged with json:"-" so it is never accidentally leaked in API responses.
// Timestamps are managed by the storage layer (set at INSERT time and updated
// on profile changes).
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateUserRequest carries the fields needed to register a new user. The
// plain-text Password is accepted here and hashed (bcrypt) in the service
// layer before being passed to storage.
type CreateUserRequest struct {
	Email    string
	Name     string
	Password string
}

// LoginRequest carries credentials for authenticating an existing user.
type LoginRequest struct {
	Email    string
	Password string
}

// AuthResponse is the internal result of a successful authentication. It is
// mapped into the protobuf LoginResponse or RegisterResponse by the service
// layer before being sent over the wire.
type AuthResponse struct {
	UserID    string
	Email     string
	Name      string
	Token     string
	ExpiresAt time.Time
}
