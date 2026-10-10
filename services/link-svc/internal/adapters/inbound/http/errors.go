package http

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// writeError maps a domain error to an HTTP status. This single mapping is the only
// place transport codes are decided, so the use cases stay transport-agnostic.
func writeError(c *gin.Context, err error) {
	status := statusFor(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		// Never leak internal error detail to clients on unexpected failures; keep it in
		// the server log instead.
		log.Printf("link: %s %s: %v", c.Request.Method, c.FullPath(), err)
		msg = "internal error"
	}
	c.JSON(status, gin.H{"error": msg})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound // 404
	case errors.Is(err, domain.ErrInvalidURL), errors.Is(err, domain.ErrURLTooLong):
		return http.StatusBadRequest // 400 - malformed request
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests // 429
	default:
		return http.StatusInternalServerError // 500
	}
}
