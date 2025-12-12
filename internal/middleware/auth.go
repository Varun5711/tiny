package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/user"
)

type contextKey string

const UserIDKey contextKey = "user_id"

type AuthMiddleware struct {
	userClient pb.UserServiceClient
	log        *logger.Logger
}

func NewAuthMiddleware(userClient pb.UserServiceClient) *AuthMiddleware {
	return &AuthMiddleware{
		userClient: userClient,
		log:        logger.New("auth-middleware"),
	}
}

func (m *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		}

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

		ctx = context.WithValue(r.Context(), UserIDKey, resp.UserId)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}
