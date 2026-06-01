package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword generates a bcrypt hash of the given plaintext password using
// bcrypt.DefaultCost (currently 10, meaning 2^10 = 1024 key expansion rounds).
//
// bcrypt is chosen over faster hash functions (SHA-256, MD5) because it is
// intentionally slow and includes a per-hash random salt, making brute-force
// and rainbow-table attacks impractical. Each call to HashPassword produces a
// different hash for the same input due to the embedded salt, so equality
// comparison of hashes is not meaningful -- use CheckPassword instead.
//
// The returned string contains the bcrypt version, cost factor, salt, and hash
// in a single portable format (e.g., "$2a$10$..."), suitable for direct
// storage in the database.
func HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedBytes), nil
}

// CheckPassword compares a bcrypt-hashed password with a plaintext candidate.
// It extracts the salt and cost factor from the stored hash, re-hashes the
// candidate password with the same parameters, and compares the results in
// constant time to prevent timing side-channel attacks.
//
// Returns nil if the password matches, or a bcrypt.ErrMismatchedHashAndPassword
// error (among others) if it does not.
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
