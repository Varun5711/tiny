package idgen

import (
	"math"
	"testing"
)

// TestEncodeBasicCases verifies that Encode produces the expected base62
// string for a representative set of inputs, including boundary values at
// each digit transition (0, 9, 10, 35, 36, 61, 62) and larger numbers.
func TestEncodeBasicCases(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"one", 1, "1"},
		{"nine", 9, "9"},
		{"ten", 10, "A"},
		{"35", 35, "Z"},
		{"36", 36, "a"},
		{"61", 61, "z"},
		{"62", 62, "10"},
		{"63", 63, "11"},
		{"124", 124, "20"},
		{"3844", 3844, "100"},
		{"small number", 125, "21"},
		{"medium number", 1234567890, "1LY7VK"},
		{"large number", 9876543210, "AmOy42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Encode(tt.input)
			if result != tt.expected {
				t.Errorf("Encode(%d) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDecodeBasicCases verifies that Decode correctly reverses the encoding
// for valid inputs and returns appropriate errors for invalid characters
// (symbols, spaces, punctuation) that fall outside the base62 alphabet.
func TestDecodeBasicCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		hasError bool
	}{
		{"zero", "0", 0, false},
		{"one", "1", 1, false},
		{"nine", "9", 9, false},
		{"ten", "A", 10, false},
		{"uppercase Z", "Z", 35, false},
		{"lowercase a", "a", 36, false},
		{"lowercase z", "z", 61, false},
		{"62", "10", 62, false},
		{"63", "11", 63, false},
		{"small", "21", 125, false},
		{"medium", "1LY7VK", 1234567890, false},
		{"large", "AmOy42", 9876543210, false},
		{"empty string", "", 0, true},
		{"invalid character !", "abc!", 0, true},
		{"invalid character @", "ab@c", 0, true},
		{"invalid character space", "ab c", 0, true},
		{"invalid character -", "ab-c", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decode(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Decode(%s) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Decode(%s) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Decode(%s) = %d; want %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}

// TestEncodeDecodeRoundTrip confirms the bijective property of the encoding:
// for every input number, Encode followed by Decode must return the original
// value. This guarantees zero collisions in the short URL code space.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	testNumbers := []int64{
		0, 1, 9, 10, 61, 62, 63, 100, 125,
		1000, 10000, 100000,
		1234567890,
		9876543210,
		math.MaxInt32,
		int64(math.MaxInt32) + 1,
	}

	for _, num := range testNumbers {
		t.Run("", func(t *testing.T) {
			encoded := Encode(num)
			decoded, err := Decode(encoded)

			if err != nil {
				t.Errorf("Decode error for %d: %v", num, err)
			}

			if decoded != num {
				t.Errorf("Round trip failed: %d -> %s -> %d", num, encoded, decoded)
			}
		})
	}
}

// TestDecodeInvalidCharacters ensures that all common non-alphanumeric
// characters are properly rejected by Decode, including symbols, whitespace,
// and escape characters that could appear in user-supplied short codes.
func TestDecodeInvalidCharacters(t *testing.T) {
	invalidStrings := []string{
		"abc!",
		"ab@c",
		"ab c",
		"ab-c",
		"ab+c",
		"ab/c",
		"ab\\c",
		"ab\nc",
		"ab\tc",
	}

	for _, str := range invalidStrings {
		t.Run(str, func(t *testing.T) {
			_, err := Decode(str)
			if err == nil {
				t.Errorf("Decode(%q) expected error for invalid character, got nil", str)
			}
		})
	}
}

// TestEncodeLargeNumbers verifies that the encoder handles values at the
// upper end of the int64 range, including math.MaxInt64. These values
// represent the theoretical maximum Snowflake IDs and produce the longest
// possible base62 strings (up to 11 characters).
func TestEncodeLargeNumbers(t *testing.T) {
	largeNumbers := []int64{
		1000000000000,
		9223372036854775807,
	}

	for _, num := range largeNumbers {
		t.Run("", func(t *testing.T) {
			encoded := Encode(num)
			if encoded == "" {
				t.Errorf("Encode(%d) returned empty string", num)
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Errorf("Decode error: %v", err)
			}
			if decoded != num {
				t.Errorf("Round trip failed for large number: %d -> %s -> %d", num, encoded, decoded)
			}
		})
	}
}

// BenchmarkEncode measures encoding throughput for a medium-sized integer.
func BenchmarkEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(1234567890)
	}
}

// BenchmarkDecode measures decoding throughput for a medium-length string.
func BenchmarkDecode(b *testing.B) {
	encoded := Encode(1234567890)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(encoded)
	}
}

// BenchmarkEncodeSmall measures encoding throughput for a small integer
// (short output string), testing the minimal-allocation fast path.
func BenchmarkEncodeSmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(125)
	}
}

// BenchmarkDecodeSmall measures decoding throughput for a short base62 string.
func BenchmarkDecodeSmall(b *testing.B) {
	encoded := Encode(125)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(encoded)
	}
}

// BenchmarkEncodeLarge measures encoding throughput for math.MaxInt64, which
// produces the longest possible base62 string and exercises the most loop
// iterations.
func BenchmarkEncodeLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(9223372036854775807)
	}
}

// BenchmarkDecodeLarge measures decoding throughput for the longest possible
// base62 string (11 characters for math.MaxInt64).
func BenchmarkDecodeLarge(b *testing.B) {
	encoded := Encode(9223372036854775807)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(encoded)
	}
}
