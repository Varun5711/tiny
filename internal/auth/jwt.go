// Package auth provides authentication primitives for the Tiny URL shortener,
// including JWT token generation/validation and bcrypt password hashing. These
// are used by the API gateway to authenticate users and protect URL management
// endpoints.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the custom JWT payload embedded in every access token.
// It extends the standard RegisteredClaims with application-specific fields
// for user identification. The UserID and Email are extracted from the token
// on each authenticated request to avoid a database lookup per request.
type Claims struct {
	// UserID is the unique identifier for the authenticated user.
	UserID string `json:"user_id"`

	// Email is the user's email address, included for convenience so
	// downstream handlers can personalize responses without a DB query.
	Email string `json:"email"`

	// RegisteredClaims embeds standard JWT fields: ExpiresAt, IssuedAt,
	// Issuer, Subject, etc. The jwt/v5 library automatically validates
	// the expiration time during parsing.
	jwt.RegisteredClaims
}

// JWTManager handles creation and validation of JSON Web Tokens using
// HMAC-SHA256 (HS256) symmetric signing. A single shared secret is used
// both for signing new tokens and verifying incoming tokens, which is
// appropriate for a monolithic or tightly-coupled microservice deployment
// where the secret can be securely shared via environment variables.
type JWTManager struct {
	// secretKey is the HMAC-SHA256 signing key. In production this should
	// be at least 32 bytes of cryptographically random data, loaded from
	// the JWT_SECRET environment variable.
	secretKey string

	// tokenDuration controls how long generated tokens remain valid.
	// After this duration the token's ExpiresAt claim will be in the past
	// and ValidateToken will reject it.
	tokenDuration time.Duration
}

// NewJWTManager creates a JWTManager with the given signing key and token
// lifetime. The secretKey must be kept confidential; if it is leaked,
// attackers can forge valid tokens for any user.
func NewJWTManager(secretKey string, tokenDuration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:     secretKey,
		tokenDuration: tokenDuration,
	}
}

// GenerateToken creates a signed JWT containing the given user ID and email.
// It returns the compact serialized token string, the expiration timestamp
// (useful for setting cookie MaxAge or returning in API responses), and any
// signing error.
//
// The token uses HS256 (HMAC-SHA256), which is a symmetric algorithm: the
// same secret is used for signing and verification. This is fast and avoids
// the complexity of RSA/ECDSA key management.
func (m *JWTManager) GenerateToken(userID, email string) (string, time.Time, error) {
	expiresAt := time.Now().Add(m.tokenDuration)

	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(m.secretKey))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, expiresAt, nil
}

// ValidateToken parses and validates a JWT string, returning the embedded
// Claims if the token is valid. Validation includes:
//
//  1. Signature verification using the manager's secret key.
//  2. Algorithm check: only HMAC signing methods are accepted, preventing
//     "alg: none" attacks where an attacker modifies the header to bypass
//     signature verification.
//  3. Expiration check: the jwt/v5 library automatically rejects tokens
//     whose ExpiresAt claim is in the past.
//
// Returns an error if the token is malformed, expired, or signed with a
// different key.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Guard against algorithm-switching attacks: ensure the token was
		// signed with an HMAC method, not RSA, ECDSA, or "none".
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.secretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
