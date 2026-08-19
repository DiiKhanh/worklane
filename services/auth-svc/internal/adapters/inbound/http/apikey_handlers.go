package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListAPIKeys(c *gin.Context) {
	keys, err := h.svc.ListAPIKeys(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]apiKeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, apiKeyDTO{
			ID: k.ID, TenantID: k.TenantID, Status: k.Status,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) CreateAPIKey(c *gin.Context) {
	plain, id, err := h.svc.CreateAPIKey(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, createKeyResponse{ID: id, Key: plain})
}

func (h *Handlers) RevokeAPIKey(c *gin.Context) {
	if err := h.svc.RevokeAPIKey(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
