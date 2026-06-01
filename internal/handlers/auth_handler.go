package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/user"
)

// AuthHandler exposes user authentication endpoints (register, login, profile).
// It acts as a thin HTTP-to-gRPC adapter: request validation and JSON
// serialization happen here, while credential hashing, token generation, and
// user persistence are handled by the backend User gRPC service.
type AuthHandler struct {
	userClient pb.UserServiceClient
	log        *logger.Logger
}

// NewAuthHandler creates an AuthHandler backed by the given gRPC user service
// client. The caller is responsible for establishing and managing the gRPC
// connection lifecycle.
func NewAuthHandler(userClient pb.UserServiceClient) *AuthHandler {
	return &AuthHandler{
		userClient: userClient,
		log:        logger.New("auth-handler"),
	}
}

// RegisterRequest is the JSON body expected by the Register endpoint.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// LoginRequest is the JSON body expected by the Login endpoint.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse is the JSON body returned after a successful register or login.
// It includes a JWT token that clients must send in subsequent authenticated
// requests via the Authorization header.
type AuthResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// ProfileResponse is the JSON body returned by the GetProfile endpoint.
type ProfileResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Register handles POST /auth/register. It creates a new user account via the
// gRPC user service and returns a JWT token on success, so the client can
// immediately make authenticated requests without a separate login step.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cap the request body to prevent abuse via oversized payloads.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Apply a 10-second deadline so a slow or unresponsive user service
	// does not block the HTTP connection indefinitely.
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
	_ = json.NewEncoder(w).Encode(authResp)
}

// Login handles POST /auth/login. It verifies credentials via the gRPC user
// service and returns a JWT token with an expiration timestamp. The error
// message is deliberately vague ("Invalid email or password") to avoid leaking
// whether a given email address is registered.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
	_ = json.NewEncoder(w).Encode(authResp)
}

// GetProfile handles GET /auth/profile. It extracts the Bearer token from the
// Authorization header and asks the gRPC user service to resolve it into a
// user profile. Unlike the other auth endpoints, this one performs its own
// token extraction instead of relying on the auth middleware, so it can be
// mounted on routes that do not use RequireAuth.
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

	// Strip the "Bearer " prefix if present, accepting both prefixed and
	// bare tokens for flexibility with different client implementations.
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
	_ = json.NewEncoder(w).Encode(profile)
}
