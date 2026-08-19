package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health is an unauthenticated liveness/readiness probe: process-only, no DB/Redis touch.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
