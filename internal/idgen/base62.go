package idgen

import "fmt"

// base62Chars defines the 62-character alphabet used for encoding. The order
// is digits (0-9), uppercase letters (A-Z), then lowercase letters (a-z).
//
// This alphabet is specifically chosen for URL shortening because:
//   - All 62 characters are URL-safe without percent-encoding (RFC 3986
//     unreserved characters), so short codes work directly in browser
//     address bars, HTML links, and QR codes.
//   - Base62 is more compact than hexadecimal (base16) or base36, producing
//     shorter codes for the same numeric range. A 63-bit Snowflake ID
//     encodes to at most 11 characters in base62, compared to 16 in hex.
//   - Unlike Base64, it avoids '+', '/', and '=' which require escaping in
//     URLs, cookies, and filenames.
//   - The encoding is deterministic and bijective (one-to-one): each int64
//     maps to exactly one string and vice versa, guaranteeing zero
//     collisions when the input IDs are unique (as the Snowflake generator
//     ensures).
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// base62Index is a reverse lookup table that maps each ASCII byte value to
// its position in the base62Chars alphabet (0-61), or -1 if the byte is not
// a valid base62 character. Using a fixed 256-entry array avoids hash-map
// overhead and provides O(1) character-to-value lookups during decoding.
var base62Index [256]int

func init() {
	// Initialize all entries to -1 (invalid) so that any non-base62 byte
	// is immediately detected during decoding.
	for i := range base62Index {
		base62Index[i] = -1
	}
	// Populate valid entries: base62Index['0']=0, ..., base62Index['z']=61.
	for i := 0; i < len(base62Chars); i++ {
		base62Index[base62Chars[i]] = i
	}
}

// Encode converts a non-negative int64 into its base62 string representation.
// The encoding uses repeated division by 62, collecting remainders to build
// the result in reverse (least-significant digit first), then reverses the
// byte slice to produce the final most-significant-digit-first string.
//
// For the Snowflake IDs produced by this package, the output is typically
// 7-11 characters long, making it ideal for short URL codes.
//
// Examples:
//
//	Encode(0)          => "0"
//	Encode(61)         => "z"
//	Encode(62)         => "10"
//	Encode(1234567890) => "1LY7VK"
func Encode(num int64) string {
	if num == 0 {
		return "0"
	}

	res := make([]byte, 0)
	for num > 0 {
		rem := num % 62
		res = append(res, base62Chars[rem])
		num /= 62
	}

	// Reverse the slice: the division loop produces digits from least
	// significant to most significant, but the string representation
	// should read most significant first (like decimal numbers).
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}

	return string(res)
}

// Decode converts a base62-encoded string back to its int64 value. It
// processes each character left-to-right, multiplying the accumulator by 62
// and adding the character's positional value (Horner's method).
//
// Returns an error if the string is empty or contains any character outside
// the base62 alphabet (0-9, A-Z, a-z).
func Decode(str string) (int64, error) {
	if str == "" {
		return 0, fmt.Errorf("empty string cannot be decoded")
	}

	var num int64 = 0
	for i := 0; i < len(str); i++ {
		val := base62Index[str[i]]
		if val == -1 {
			return 0, fmt.Errorf("invalid base62 character: %c", str[i])
		}
		num = num*62 + int64(val)
	}
	return num, nil
}
