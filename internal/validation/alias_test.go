package validation

import (
	"testing"
)

// TestValidateAlias_Valid ensures that well-formed aliases containing
// letters, digits, hyphens, and underscores pass validation without error.
func TestValidateAlias_Valid(t *testing.T) {
	validAliases := []string{
		"abc",
		"my-link",
		"my_link",
		"MyLink123",
		"test-url-2024",
		"a-b-c",
		"123abc",
	}

	for _, alias := range validAliases {
		err := ValidateAlias(alias)
		if err != nil {
			t.Errorf("expected '%s' to be valid, got error: %v", alias, err)
		}
	}
}

// TestValidateAlias_TooShort verifies that aliases under the 3-character
// minimum (including the empty string) are rejected with ErrAliasTooShort.
func TestValidateAlias_TooShort(t *testing.T) {
	shortAliases := []string{"a", "ab", ""}

	for _, alias := range shortAliases {
		err := ValidateAlias(alias)
		if err != ErrAliasTooShort {
			t.Errorf("expected ErrAliasTooShort for '%s', got: %v", alias, err)
		}
	}
}

// TestValidateAlias_TooLong verifies that aliases exceeding 50 characters
// are rejected with ErrAliasTooLong.
func TestValidateAlias_TooLong(t *testing.T) {
	longAlias := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"

	err := ValidateAlias(longAlias)
	if err != ErrAliasTooLong {
		t.Errorf("expected ErrAliasTooLong, got: %v", err)
	}
}

// TestValidateAlias_InvalidChars confirms that aliases containing spaces,
// dots, slashes, or other URL-unsafe characters are rejected.
func TestValidateAlias_InvalidChars(t *testing.T) {
	invalidAliases := []string{
		"my link",
		"my.link",
		"my@link",
		"my/link",
		"my?link",
		"my#link",
		"my&link",
	}

	for _, alias := range invalidAliases {
		err := ValidateAlias(alias)
		if err != ErrAliasInvalidChars {
			t.Errorf("expected ErrAliasInvalidChars for '%s', got: %v", alias, err)
		}
	}
}

// TestValidateAlias_Reserved checks that system-reserved words (api, admin,
// health, etc.) are blocked regardless of letter casing.
func TestValidateAlias_Reserved(t *testing.T) {
	reservedAliases := []string{
		"api",
		"admin",
		"health",
		"login",
		"logout",
		"register",
		"auth",
		"API",
		"Admin",
		"HEALTH",
	}

	for _, alias := range reservedAliases {
		err := ValidateAlias(alias)
		if err != ErrAliasReserved {
			t.Errorf("expected ErrAliasReserved for '%s', got: %v", alias, err)
		}
	}
}

// TestValidateAlias_Profanity verifies that exact profanity words are
// blocked, including case-insensitive variants (e.g., "PORN", "XXX").
func TestValidateAlias_Profanity(t *testing.T) {
	profaneAliases := []string{
		"porn",
		"xxx",
		"spam",
		"scam",
		"PORN",
		"XXX",
	}

	for _, alias := range profaneAliases {
		err := ValidateAlias(alias)
		if err != ErrAliasProfanity {
			t.Errorf("expected ErrAliasProfanity for '%s', got: %v", alias, err)
		}
	}
}

// TestValidateAlias_ContainsProfanity ensures that the substring scan
// catches profanity embedded inside longer aliases (e.g., "mypornsite").
func TestValidateAlias_ContainsProfanity(t *testing.T) {
	containsProfanity := []string{
		"mypornsite",
		"getxxxnow",
		"bestscam123",
	}

	for _, alias := range containsProfanity {
		err := ValidateAlias(alias)
		if err != ErrAliasProfanity {
			t.Errorf("expected ErrAliasProfanity for '%s' (contains profanity), got: %v", alias, err)
		}
	}
}

// TestValidateAlias_BoundaryLength exercises the exact boundary values:
// 3 chars (minimum valid), 50 chars (maximum valid), and 51 chars (too long).
func TestValidateAlias_BoundaryLength(t *testing.T) {
	alias3Chars := "abc"
	err := ValidateAlias(alias3Chars)
	if err != nil {
		t.Errorf("expected 3-char alias to be valid, got: %v", err)
	}

	alias50Chars := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx"
	err = ValidateAlias(alias50Chars)
	if err != nil {
		t.Errorf("expected 50-char alias to be valid, got: %v", err)
	}

	alias51Chars := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxy"
	err = ValidateAlias(alias51Chars)
	if err != ErrAliasTooLong {
		t.Errorf("expected 51-char alias to be too long, got: %v", err)
	}
}

// TestSuggestAlternatives verifies that the function returns the requested
// number of "-N" suffixed suggestions in ascending order.
func TestSuggestAlternatives(t *testing.T) {
	suggestions := SuggestAlternatives("mylink", 3)

	if len(suggestions) != 3 {
		t.Errorf("expected 3 suggestions, got %d", len(suggestions))
	}

	expected := []string{"mylink-1", "mylink-2", "mylink-3"}
	for i, suggestion := range suggestions {
		if suggestion != expected[i] {
			t.Errorf("expected suggestion '%s', got '%s'", expected[i], suggestion)
		}
	}
}

// TestSuggestAlternatives_ZeroCount confirms that asking for zero
// suggestions returns an empty (but non-nil) slice.
func TestSuggestAlternatives_ZeroCount(t *testing.T) {
	suggestions := SuggestAlternatives("mylink", 0)

	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(suggestions))
	}
}

// TestSuggestAlternatives_OneCount verifies the single-suggestion edge case.
func TestSuggestAlternatives_OneCount(t *testing.T) {
	suggestions := SuggestAlternatives("mylink", 1)

	if len(suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(suggestions))
	}

	if suggestions[0] != "mylink-1" {
		t.Errorf("expected 'mylink-1', got '%s'", suggestions[0])
	}
}
