// Package middleware provides reusable HTTP middleware for the tiny URL
// shortener API gateway. Middleware is composed in a chain via standard
// func(http.Handler) http.Handler signatures. The recommended chaining order
// from outermost to innermost is:
//
//  1. Recovery   - catch panics so the process stays alive
//  2. RequestID  - assign a correlation ID for distributed tracing
//  3. Tracing    - start an OpenTelemetry span
//  4. CORS       - handle preflight and set access-control headers
//  5. RateLimit  - enforce per-IP request limits
//  6. Auth       - validate JWT and inject user_id into context
//
// This ordering ensures that panic recovery and observability wrap everything,
// CORS preflight requests short-circuit before auth, and rate limiting applies
// to both authenticated and unauthenticated traffic.
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/user"
)

// contextKey is an unexported type for context value keys to avoid collisions
// with keys defined in other packages.
type contextKey string

// UserIDKey is the context key under which the authenticated user's ID is
// stored after successful JWT validation.
const UserIDKey contextKey = "user_id"

// AuthMiddleware validates JWT tokens by calling the gRPC user service's
// ValidateToken RPC. It is intentionally stateless on the gateway side:
// the user service is the single source of truth for token validity, which
// allows centralized revocation without redeploying the gateway.
type AuthMiddleware struct {
	userClient pb.UserServiceClient
	log        *logger.Logger
}

// NewAuthMiddleware creates an AuthMiddleware backed by the given gRPC user
// service client. The client connection should be shared with AuthHandler to
// avoid opening duplicate connections.
func NewAuthMiddleware(userClient pb.UserServiceClient) *AuthMiddleware {
	return &AuthMiddleware{
		userClient: userClient,
		log:        logger.New("auth-middleware"),
	}
}

// RequireAuth wraps a handler to enforce JWT authentication. It extracts the
// Bearer token from the Authorization header, validates it via the gRPC user
// service, and injects the resulting user_id into the request context. Requests
// without a valid token receive a 401 Unauthorized response and are not
// forwarded to the wrapped handler.
func (m *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Accept both "Bearer <token>" and a bare token for flexibility.
		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		}

		// Apply a tight timeout to the validation RPC so a slow user service
		// does not hold up the entire request pipeline.
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp, err := m.userClient.ValidateToken(ctx, &pb.ValidateTokenRequest{
			Token: token,
		})
		if err != nil || !resp.Valid {
			m.log.Error("Invalid token: %v", err)
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Store the validated user ID in the context so downstream handlers
		// can retrieve it via GetUserID without re-parsing the token.
		ctx = context.WithValue(r.Context(), UserIDKey, resp.UserId)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUserID retrieves the authenticated user's ID from the context. Returns
// an empty string if the context does not contain a user ID (i.e., the request
// was not processed by RequireAuth or authentication failed).
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}
