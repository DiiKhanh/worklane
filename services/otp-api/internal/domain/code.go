package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateCode returns a numeric OTP of exactly length digits (zero-padded), drawn
// from crypto/rand. length must be between 4 and 10.
//
// Why crypto/rand, not math/rand: an OTP is a security primitive. math/rand is a
// deterministic PRNG whose output can be predicted from prior values, which would let
// an attacker guess the next code. crypto/rand reads from the OS CSPRNG, so codes are
// unpredictable. We draw a single uniform integer in [0, 10^length) via rand.Int
// (which rejects modulo bias internally) and zero-pad it to a fixed width.
func GenerateCode(length int) (string, error) {
	if length < 4 || length > 10 {
		return "", fmt.Errorf("otp: invalid code length %d", length)
	}
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("otp: rand: %w", err)
	}
	return fmt.Sprintf("%0*d", length, n), nil
}
