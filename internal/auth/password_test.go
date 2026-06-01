package auth

import (
	"testing"
)

// TestHashPassword verifies that HashPassword produces a non-empty hash
// that differs from the plaintext input (ensuring the password is not
// stored in the clear).
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

// TestHashPassword_DifferentHashes confirms that bcrypt's random salt causes
// two hashes of the same password to differ. This is a critical property:
// if two users choose the same password, their stored hashes must not match,
// preventing an attacker from identifying shared passwords in a database dump.
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

// TestCheckPassword_Correct verifies the happy path: a password should match
// its own hash.
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

// TestCheckPassword_Incorrect verifies that a wrong password is rejected,
// ensuring the comparison is actually checking the password content and not
// just the hash format.
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

// TestCheckPassword_EmptyPassword ensures that an empty string does not
// accidentally match a non-empty password's hash.
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

// TestHashPassword_EmptyPassword verifies that hashing an empty string
// succeeds (bcrypt does not reject empty inputs) and produces a non-empty
// hash. Whether to allow empty passwords is a policy decision enforced
// at a higher layer; the hash function itself should handle any input.
func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash even for empty password")
	}
}

// TestCheckPassword_InvalidHash ensures that a malformed hash string (not
// valid bcrypt output) produces an error rather than a false positive match.
func TestCheckPassword_InvalidHash(t *testing.T) {
	err := CheckPassword("not-a-valid-bcrypt-hash", "password")
	if err == nil {
		t.Error("expected error for invalid hash format")
	}
}
