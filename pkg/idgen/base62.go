// Package idgen is the shared, dependency-free id toolkit for the link service: a
// Snowflake generator for unique 64-bit ids and a Base62 codec that turns an id into a
// short URL-safe code. Encoding a unique id (instead of hashing the URL) means codes
// never collide, so the create path needs no "does this code exist" DB round-trip.
package idgen

import (
	"errors"
	"math"
)

var (
	ErrEmptyCode   = errors.New("idgen: empty base62 string")
	ErrInvalidChar = errors.New("idgen: invalid base62 character")
	ErrOverflow    = errors.New("idgen: base62 value overflows uint64")
)

// alphabet is the digit order: value 0 is '0', 10 is 'a', 36 is 'A'.
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const base = uint64(len(alphabet))

// maxEncodedLen is the length of math.MaxUint64 in base62 ("lYGhA16ahyf").
const maxEncodedLen = 11

// Encode returns the base62 form of n with no padding, so 0 encodes to "0".
func Encode(n uint64) string {
	if n == 0 {
		return alphabet[:1]
	}
	// Fill from the right: repeated division yields the least significant digit first.
	var buf [maxEncodedLen]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = alphabet[n%base]
		n /= base
	}
	return string(buf[i:])
}

// Decode is the inverse of Encode. It rejects empty input, characters outside the
// alphabet, and values that do not fit in a uint64 (instead of silently wrapping).
func Decode(s string) (uint64, error) {
	if s == "" {
		return 0, ErrEmptyCode
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		d, ok := digit(s[i])
		if !ok {
			return 0, ErrInvalidChar
		}
		if n > (math.MaxUint64-d)/base {
			return 0, ErrOverflow
		}
		n = n*base + d
	}
	return n, nil
}

func digit(c byte) (uint64, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c - '0'), true
	case c >= 'a' && c <= 'z':
		return uint64(c-'a') + 10, true
	case c >= 'A' && c <= 'Z':
		return uint64(c-'A') + 36, true
	}
	return 0, false
}
