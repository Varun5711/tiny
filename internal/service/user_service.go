package service

import (
	"context"

	"github.com/Varun5711/shorternit/internal/auth"
	usermodel "github.com/Varun5711/shorternit/internal/models/user"
	"github.com/Varun5711/shorternit/internal/storage"
	pb "github.com/Varun5711/shorternit/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UserService implements the gRPC UserServiceServer interface, handling user
// registration, authentication, profile management, and JWT token operations.
// It delegates persistence to UserStorage (PostgreSQL) and token signing /
// verification to JWTManager.
//
// The interaction pattern mirrors URLService: validate the protobuf request,
// call the storage layer, then map the result into a protobuf response.
// Passwords are hashed with bcrypt before storage and never leave the server.
type UserService struct {
	pb.UnimplementedUserServiceServer
	userStorage *storage.UserStorage // PostgreSQL-backed user persistence.
	jwtManager  *auth.JWTManager     // Handles JWT creation and validation.
}

// NewUserService creates a UserService with its required dependencies.
func NewUserService(userStorage *storage.UserStorage, jwtManager *auth.JWTManager) *UserService {
	return &UserService{
		userStorage: userStorage,
		jwtManager:  jwtManager,
	}
}

// Register handles the gRPC Register RPC. The flow is:
//  1. Validate required fields and enforce a minimum password length (8 chars).
//  2. Check that no account with the same email exists (read-path query).
//  3. Hash the password with bcrypt via auth.HashPassword.
//  4. Insert the new user into PostgreSQL via UserStorage.CreateUser.
//  5. Immediately issue a JWT so the client is authenticated after signup
//     without a separate Login round-trip.
func (s *UserService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "password is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if len(req.Password) < 8 {
		return nil, status.Error(codes.InvalidArgument, "password must be at least 8 characters")
	}

	existingUser, err := s.userStorage.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check existing user: %v", err)
	}
	if existingUser != nil {
		return nil, status.Error(codes.AlreadyExists, "user with this email already exists")
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
	}

	user, err := s.userStorage.CreateUser(ctx, &usermodel.CreateUserRequest{
		Email:    req.Email,
		Name:     req.Name,
		Password: req.Password,
	}, passwordHash)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	token, _, err := s.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}

	return &pb.RegisterResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Token:     token,
		CreatedAt: user.CreatedAt.Unix(),
	}, nil
}

// Login handles the gRPC Login RPC. It looks up the user by email, verifies
// the password against the stored bcrypt hash, and returns a signed JWT on
// success. Both "user not found" and "wrong password" return the same
// user-facing message ("invalid email or password") to prevent email
// enumeration attacks, but they use different gRPC status codes (NotFound vs.
// Unauthenticated) so server-side observability can distinguish the two.
func (s *UserService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "password is required")
	}

	user, err := s.userStorage.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "invalid email or password")
	}

	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid email or password")
	}

	token, expiresAt, err := s.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}

	return &pb.LoginResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
	}, nil
}

// GetProfile handles the gRPC GetProfile RPC. It validates the JWT, extracts
// the user ID from the token claims, and fetches the full user record from
// PostgreSQL. This is a token-authenticated endpoint -- the user ID is derived
// from the JWT, not from the request body, preventing users from viewing
// other accounts.
func (s *UserService) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	claims, err := s.jwtManager.ValidateToken(req.Token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	user, err := s.userStorage.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return &pb.GetProfileResponse{
		User: &pb.User{
			Id:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt.Unix(),
			UpdatedAt: user.UpdatedAt.Unix(),
		},
	}, nil
}

// UpdateProfile handles the gRPC UpdateProfile RPC. Like GetProfile, it
// extracts the user ID from the JWT so a user can only modify their own
// profile. The updated name and email are written to PostgreSQL and the
// refreshed record is returned.
func (s *UserService) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	claims, err := s.jwtManager.ValidateToken(req.Token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	user, err := s.userStorage.UpdateUser(ctx, claims.UserID, req.Name, req.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update user: %v", err)
	}

	return &pb.UpdateProfileResponse{
		User: &pb.User{
			Id:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt.Unix(),
			UpdatedAt: user.UpdatedAt.Unix(),
		},
	}, nil
}

// ValidateToken handles the gRPC ValidateToken RPC. It is a lightweight
// stateless check -- no database call is made. The JWT signature and expiry
// are verified, and if valid, the embedded user ID and expiration are returned.
// Invalid or expired tokens return Valid=false with no gRPC error, allowing
// the API gateway to distinguish "bad token" from "server error".
func (s *UserService) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	if req.Token == "" {
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	claims, err := s.jwtManager.ValidateToken(req.Token)
	if err != nil {
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	return &pb.ValidateTokenResponse{
		Valid:     true,
		UserId:    claims.UserID,
		ExpiresAt: claims.ExpiresAt.Unix(),
	}, nil
}
