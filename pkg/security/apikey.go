// Package security holds cross-service credential helpers: generating API keys and
// hashing them for storage/lookup. It is shared (pkg) because both the seed CLI (which
// mints and stores a key's hash) and otp-api (which hashes an incoming key to look it
// up) must agree on the exact hashing - they cannot import each other's internal code.
package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateAPIKey returns a 256-bit random, URL-safe token (base64url, no padding). The
// plaintext is shown to the user once; only its hash is stored.
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("security: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashKey returns the hex SHA-256 of the key. API keys are high-entropy random tokens
// (unlike user passwords), so a fast single hash is appropriate and lets us look up a
// key by its hash directly. Storing only the hash means a database leak never exposes
// usable keys.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
