package validation

import (
	"testing"
)

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

func TestValidateAlias_TooShort(t *testing.T) {
	shortAliases := []string{"a", "ab", ""}

	for _, alias := range shortAliases {
		err := ValidateAlias(alias)
		if err != ErrAliasTooShort {
			t.Errorf("expected ErrAliasTooShort for '%s', got: %v", alias, err)
		}
	}
}

func TestValidateAlias_TooLong(t *testing.T) {
	longAlias := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"

	err := ValidateAlias(longAlias)
	if err != ErrAliasTooLong {
		t.Errorf("expected ErrAliasTooLong, got: %v", err)
	}
}

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

func TestSuggestAlternatives_ZeroCount(t *testing.T) {
	suggestions := SuggestAlternatives("mylink", 0)

	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestSuggestAlternatives_OneCount(t *testing.T) {
	suggestions := SuggestAlternatives("mylink", 1)

	if len(suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(suggestions))
	}

	if suggestions[0] != "mylink-1" {
		t.Errorf("expected 'mylink-1', got '%s'", suggestions[0])
	}
}
