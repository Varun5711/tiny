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

type UserStorage struct {
	db *database.DBManager
}

func NewUserStorage(db *database.DBManager) *UserStorage {
	return &UserStorage{db: db}
}

func (s *UserStorage) CreateUser(ctx context.Context, req *usermodel.CreateUserRequest, passwordHash string) (*usermodel.User, error) {
	userID := uuid.New().String()
	now := time.Now()

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

func (s *UserStorage) GetUserByEmail(ctx context.Context, email string) (*usermodel.User, error) {
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

func (s *UserStorage) GetUserByID(ctx context.Context, userID string) (*usermodel.User, error) {
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

func (s *UserStorage) UpdateUser(ctx context.Context, userID string, name, email string) (*usermodel.User, error) {
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
