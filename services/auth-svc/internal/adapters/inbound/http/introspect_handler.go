package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) Introspect(c *gin.Context) {
	var body introspectRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, active, err := h.svc.Introspect(c.Request.Context(), body.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, introspectResponse{Active: active, TenantID: tenantID})
}
