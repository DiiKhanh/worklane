// Package http is auth-svc's inbound (driving) adapter: it translates HTTP requests into
// use-case calls and maps results/errors back to HTTP.
package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
)

// AuthService is the inbound port this adapter needs.
type AuthService interface {
	Login(ctx context.Context, email, password string) (app.LoginResult, error)
}

type Handlers struct{ svc AuthService }

func (h *Handlers) Login(c *gin.Context) {
	var body loginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.Login(c.Request.Context(), body.Email, body.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		Token:     res.Token,
		ExpiresAt: res.ExpiresAt,
		User:      userDTO{ID: res.User.ID, Email: res.User.Email, TenantID: res.User.TenantID},
	})
}

// Me echoes the authenticated identity from the verified JWT (no DB hit needed).
func (h *Handlers) Me(c *gin.Context) {
	c.JSON(http.StatusOK, userDTO{
		ID:       c.GetString(userCtxKey),
		Email:    c.GetString(emailCtxKey),
		TenantID: c.GetString(tenantCtxKey),
	})
}
