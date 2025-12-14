package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "securePassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash")
	}

	if hash == password {
		t.Error("hash should not equal plaintext password")
	}
}

func TestHashPassword_DifferentHashes(t *testing.T) {
	password := "securePassword123"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("same password should produce different hashes due to salt")
	}
}

func TestCheckPassword_Correct(t *testing.T) {
	password := "securePassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	err = CheckPassword(hash, password)
	if err != nil {
		t.Errorf("expected correct password to match, got error: %v", err)
	}
}

func TestCheckPassword_Incorrect(t *testing.T) {
	password := "securePassword123"
	wrongPassword := "wrongPassword456"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	err = CheckPassword(hash, wrongPassword)
	if err == nil {
		t.Error("expected error for incorrect password")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	password := "securePassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	err = CheckPassword(hash, "")
	if err == nil {
		t.Error("expected error for empty password")
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash even for empty password")
	}
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	err := CheckPassword("not-a-valid-bcrypt-hash", "password")
	if err == nil {
		t.Error("expected error for invalid hash format")
	}
}
