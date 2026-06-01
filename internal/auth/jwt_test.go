package auth

import (
	"testing"
	"time"
)

// TestNewJWTManager verifies that the constructor correctly stores the secret
// key and token duration in the returned manager instance.
func TestNewJWTManager(t *testing.T) {
	manager := NewJWTManager("test-secret", time.Hour)

	if manager == nil {
		t.Fatal("expected JWTManager to be created")
	}
	if manager.secretKey != "test-secret" {
		t.Errorf("expected secretKey 'test-secret', got '%s'", manager.secretKey)
	}
	if manager.tokenDuration != time.Hour {
		t.Errorf("expected tokenDuration 1h, got %v", manager.tokenDuration)
	}
}

// TestGenerateToken verifies that GenerateToken produces a non-empty token
// string and an expiration time that is approximately tokenDuration from now
// (with a 1-minute tolerance to account for test execution time).
func TestGenerateToken(t *testing.T) {
	manager := NewJWTManager("test-secret-key", time.Hour)

	token, expiresAt, err := manager.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}

	expectedExpiry := time.Now().Add(time.Hour)
	if expiresAt.Before(expectedExpiry.Add(-time.Minute)) || expiresAt.After(expectedExpiry.Add(time.Minute)) {
		t.Errorf("expiry time not within expected range")
	}
}

// TestValidateToken_Valid confirms the happy path: a freshly generated token
// should validate successfully and yield the correct UserID and Email claims.
func TestValidateToken_Valid(t *testing.T) {
	manager := NewJWTManager("test-secret-key", time.Hour)

	token, _, err := manager.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("unexpected error validating token: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("expected UserID 'user-123', got '%s'", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got '%s'", claims.Email)
	}
}

// TestValidateToken_Expired verifies that tokens past their expiration time
// are rejected. A negative duration (-1h) is used to create an already-expired
// token without needing to manipulate the system clock.
func TestValidateToken_Expired(t *testing.T) {
	manager := NewJWTManager("test-secret-key", -time.Hour)

	token, _, err := manager.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = manager.ValidateToken(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

// TestValidateToken_InvalidSignature ensures that a token signed with one
// secret key is rejected when validated with a different secret key. This
// protects against tokens forged with a compromised or guessed key.
func TestValidateToken_InvalidSignature(t *testing.T) {
	manager1 := NewJWTManager("secret-key-1", time.Hour)
	manager2 := NewJWTManager("secret-key-2", time.Hour)

	token, _, err := manager1.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = manager2.ValidateToken(token)
	if err == nil {
		t.Error("expected error for token with wrong signature")
	}
}

// TestValidateToken_Malformed ensures that arbitrary strings that do not
// conform to the JWT three-part structure (header.payload.signature) are
// properly rejected.
func TestValidateToken_Malformed(t *testing.T) {
	manager := NewJWTManager("test-secret-key", time.Hour)

	_, err := manager.ValidateToken("not-a-valid-token")
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

// TestValidateToken_EmptyToken verifies that an empty string is rejected
// rather than causing a panic or returning nil claims.
func TestValidateToken_EmptyToken(t *testing.T) {
	manager := NewJWTManager("test-secret-key", time.Hour)

	_, err := manager.ValidateToken("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}
