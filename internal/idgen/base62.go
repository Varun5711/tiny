package idgen

import "fmt"

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var base62Index [256]int

func init() {
	for i := range base62Index {
		base62Index[i] = -1
	}
	for i := 0; i < len(base62Chars); i++ {
		base62Index[base62Chars[i]] = i
	}
}

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

	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}

	return string(res)
}

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
