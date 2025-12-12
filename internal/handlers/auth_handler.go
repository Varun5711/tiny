package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/user"
)

type AuthHandler struct {
	userClient pb.UserServiceClient
	log        *logger.Logger
}

func NewAuthHandler(userClient pb.UserServiceClient) *AuthHandler {
	return &AuthHandler{
		userClient: userClient,
		log:        logger.New("auth-handler"),
	}
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type ProfileResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.Register(ctx, &pb.RegisterRequest{
		Email:    req.Email,
		Password: req.Password,
		Name:     req.Name,
	})
	if err != nil {
		h.log.Error("Failed to register user: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	authResp := AuthResponse{
		UserID: resp.UserId,
		Email:  resp.Email,
		Name:   resp.Name,
		Token:  resp.Token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.Login(ctx, &pb.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		h.log.Error("Failed to login: %v", err)
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	authResp := AuthResponse{
		UserID:    resp.UserId,
		Email:     resp.Email,
		Name:      resp.Name,
		Token:     resp.Token,
		ExpiresAt: resp.ExpiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResp)
}

func (h *AuthHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.userClient.GetProfile(ctx, &pb.GetProfileRequest{
		Token: token,
	})
	if err != nil {
		h.log.Error("Failed to get profile: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	profile := ProfileResponse{
		UserID:    resp.User.Id,
		Email:     resp.User.Email,
		Name:      resp.User.Name,
		CreatedAt: resp.User.CreatedAt,
		UpdatedAt: resp.User.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}
