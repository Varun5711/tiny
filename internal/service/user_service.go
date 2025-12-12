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

type UserService struct {
	pb.UnimplementedUserServiceServer
	userStorage *storage.UserStorage
	jwtManager  *auth.JWTManager
}

func NewUserService(userStorage *storage.UserStorage, jwtManager *auth.JWTManager) *UserService {
	return &UserService{
		userStorage: userStorage,
		jwtManager:  jwtManager,
	}
}

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
