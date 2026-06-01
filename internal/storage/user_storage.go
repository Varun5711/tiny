package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Varun5711/shorternit/internal/database"
	usermodel "github.com/Varun5711/shorternit/internal/models/user"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UserStorage provides PostgreSQL-backed persistence for user accounts. Like
// PostgresStorage, it uses database.DBManager to route writes to the primary
// and reads to replicas, though in practice user lookups (login, profile) are
// latency-sensitive enough that replica lag rarely matters.
type UserStorage struct {
	db *database.DBManager
}

// NewUserStorage creates a UserStorage backed by the given DBManager.
func NewUserStorage(db *database.DBManager) *UserStorage {
	return &UserStorage{db: db}
}

// CreateUser inserts a new user into the users table on the primary database.
// A UUID v4 is generated for the user ID to avoid sequential enumeration.
// The pre-hashed password (bcrypt) is stored; the plaintext is never persisted.
// RETURNING is used to capture the inserted row so the caller receives the
// exact values written (including server-side timestamp precision).
func (s *UserStorage) CreateUser(ctx context.Context, req *usermodel.CreateUserRequest, passwordHash string) (*usermodel.User, error) {
	userID := uuid.New().String()
	now := time.Now()

	// INSERT a new user row and return the created fields. $1-$6 map to
	// id, email, name, password_hash, created_at, updated_at.
	query := `
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, email, name, created_at, updated_at
	`

	var user usermodel.User
	err := s.db.Write().QueryRow(ctx, query,
		userID,
		req.Email,
		req.Name,
		passwordHash,
		now,
		now,
	).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &user, nil
}

// GetUserByEmail looks up a user by email address on a read replica. It
// returns (nil, nil) if no user matches, letting the service layer distinguish
// "not found" from a database error. The password_hash is included in the
// result because this method is used during login to verify credentials.
func (s *UserStorage) GetUserByEmail(ctx context.Context, email string) (*usermodel.User, error) {
	// SELECT the full user row including password_hash (needed for login
	// credential verification).
	query := `
		SELECT id, email, name, password_hash, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user usermodel.User
	err := s.db.Read().QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetUserByID fetches a user by their UUID from a read replica. Unlike
// GetUserByEmail, the password_hash is omitted from the SELECT because this
// method is used for profile retrieval where credentials are not needed.
// Returns (nil, nil) when the user does not exist.
func (s *UserStorage) GetUserByID(ctx context.Context, userID string) (*usermodel.User, error) {
	// SELECT user profile fields (no password_hash -- not needed for profile
	// display).
	query := `
		SELECT id, email, name, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user usermodel.User
	err := s.db.Read().QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// UpdateUser modifies a user's name and email on the primary database. The
// updated_at column is set to NOW() by PostgreSQL so the timestamp reflects
// the exact write time. RETURNING gives back the full updated row, avoiding a
// second SELECT round-trip.
func (s *UserStorage) UpdateUser(ctx context.Context, userID string, name, email string) (*usermodel.User, error) {
	// UPDATE name and email, bump updated_at, and return the refreshed row.
	query := `
		UPDATE users
		SET name = $1, email = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, email, name, created_at, updated_at
	`

	var user usermodel.User
	err := s.db.Write().QueryRow(ctx, query, name, email, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &user, nil
}
