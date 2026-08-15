package domain

import "errors"

// Sentinel errors returned by the OTP domain and application layers. Adapters map
// these to transport-specific codes (e.g. HTTP status) at the edge.
var (
	ErrRateLimited     = errors.New("otp: rate limited")
	ErrCooldown        = errors.New("otp: resend cooldown active")
	ErrTooManyAttempts = errors.New("otp: too many verify attempts")
	ErrNotFound        = errors.New("otp: no active code")
	ErrCodeMismatch    = errors.New("otp: code mismatch")
	ErrExpired         = errors.New("otp: code expired")
)
