package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
)

// NewRouter builds the auth-svc HTTP handler. /auth/login is public; /auth/me and
// /auth/api-keys sit behind JWT verification; /internal/* is guarded by a shared secret.
func NewRouter(svc AuthService, verifier *security.Verifier, internalToken string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := &Handlers{svc: svc}

	auth := r.Group("/auth")
	auth.POST("/login", h.Login)
	auth.GET("/me", jwtAuth(verifier), h.Me)

	keys := auth.Group("/api-keys", jwtAuth(verifier))
	keys.GET("", h.ListAPIKeys)
	keys.POST("", h.CreateAPIKey)
	keys.DELETE("/:id", h.RevokeAPIKey)

	internal := r.Group("/internal", internalAuth(internalToken))
	internal.POST("/introspect", h.Introspect)

	return r
}
