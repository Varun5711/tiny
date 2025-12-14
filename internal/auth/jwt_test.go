package auth

import (
	"testing"
	"time"
)

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

func TestValidateToken_Malformed(t *testing.T) {
	manager := NewJWTManager("test-secret-key", time.Hour)

	_, err := manager.ValidateToken("not-a-valid-token")
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	manager := NewJWTManager("test-secret-key", time.Hour)

	_, err := manager.ValidateToken("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}
