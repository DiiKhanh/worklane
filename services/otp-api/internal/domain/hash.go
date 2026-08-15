package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// HashCode returns the hex SHA-256 of salt+code. A per-request random salt means two
// requests for the same code produce different hashes, defeating precomputation
// (rainbow tables) against the small numeric code space.
func HashCode(code, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + code))
	return hex.EncodeToString(sum[:])
}

// VerifyHash reports whether code (with salt) hashes to hash, comparing in constant
// time. subtle.ConstantTimeCompare takes the same duration regardless of where the
// first mismatch is, so it does not leak how many leading characters matched via
// timing - the classic side channel when verifying tokens/OTPs.
func VerifyHash(hash, code, salt string) bool {
	got := HashCode(code, salt)
	return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1
}
