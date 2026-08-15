package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// writeError maps a domain error to an HTTP status. This single mapping is the only
// place transport codes are decided, so the use cases stay transport-agnostic.
func writeError(c *gin.Context, err error) {
	status := statusFor(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		// Never leak internal error detail to clients on unexpected failures.
		msg = "internal error"
	}
	c.JSON(status, gin.H{"error": msg})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrRateLimited),
		errors.Is(err, domain.ErrCooldown),
		errors.Is(err, domain.ErrTooManyAttempts):
		return http.StatusTooManyRequests // 429
	case errors.Is(err, domain.ErrCodeMismatch):
		return http.StatusUnauthorized // 401
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrExpired):
		return http.StatusGone // 410 - the code existed conceptually but is no longer usable
	default:
		return http.StatusInternalServerError // 500
	}
}
