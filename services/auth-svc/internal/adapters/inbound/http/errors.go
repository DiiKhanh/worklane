package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// writeError maps a domain error to an HTTP status. Credential failures collapse to a
// single 401 with a generic message so the endpoint cannot be used to enumerate users.
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
	case errors.Is(err, domain.ErrInactive):
		c.JSON(http.StatusForbidden, gin.H{"error": "user inactive"})
	case errors.Is(err, domain.ErrRateLimited):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, try again later"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
