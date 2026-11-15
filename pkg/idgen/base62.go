// Package idgen generates numeric identities and their case-sensitive Base62 codes.
package idgen

import "errors"

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var ErrInvalidCode = errors.New("idgen: invalid Base62 code")

// Encode returns the canonical Base62 representation, including "0" for zero.
func Encode(n uint64) string {
	var buf [11]byte
	i := len(buf)
	for {
		i--
		buf[i] = alphabet[n%62]
		n /= 62
		if n == 0 {
			return string(buf[i:])
		}
	}
}

// Decode accepts only canonical, case-sensitive codes that fit in a uint64.
func Decode(code string) (uint64, error) {
	if len(code) == 0 || len(code) > 11 || (len(code) > 1 && code[0] == '0') {
		return 0, ErrInvalidCode
	}
	var n uint64
	for i := 0; i < len(code); i++ {
		var digit uint64
		switch c := code[i]; {
		case c >= '0' && c <= '9':
			digit = uint64(c - '0')
		case c >= 'a' && c <= 'z':
			digit = uint64(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			digit = uint64(c-'A') + 36
		default:
			return 0, ErrInvalidCode
		}
		if n > (^uint64(0)-digit)/62 {
			return 0, ErrInvalidCode
		}
		n = n*62 + digit
	}
	return n, nil
}
