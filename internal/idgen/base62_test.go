package idgen

import (
	"math"
	"testing"
)

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

func BenchmarkEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(1234567890)
	}
}

func BenchmarkDecode(b *testing.B) {
	encoded := Encode(1234567890)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(encoded)
	}
}

func BenchmarkEncodeSmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(125)
	}
}

func BenchmarkDecodeSmall(b *testing.B) {
	encoded := Encode(125)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(encoded)
	}
}

func BenchmarkEncodeLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(9223372036854775807)
	}
}

func BenchmarkDecodeLarge(b *testing.B) {
	encoded := Encode(9223372036854775807)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(encoded)
	}
}
