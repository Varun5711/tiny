package user

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateUserRequest struct {
	Email    string
	Name     string
	Password string
}

type LoginRequest struct {
	Email    string
	Password string
}

type AuthResponse struct {
	UserID    string
	Email     string
	Name      string
	Token     string
	ExpiresAt time.Time
}
