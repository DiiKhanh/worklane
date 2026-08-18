package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
)

// NewRouter builds the auth-svc HTTP handler. /auth/login is public; /auth/me sits behind
// JWT verification with the public key.
func NewRouter(svc AuthService, verifier *security.Verifier) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := &Handlers{svc: svc}

	auth := r.Group("/auth")
	auth.POST("/login", h.Login)
	auth.GET("/me", jwtAuth(verifier), h.Me)
	return r
}
