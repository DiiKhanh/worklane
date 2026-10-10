package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/platform/metrics"
	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

// NewRouter builds the link-svc HTTP handler. /v1/links sits behind the authenticate
// middleware (user JWT or tenant API key via introspection) and is tenant-scoped; the
// redirect GET /:code is public, because a short link must work for anyone who has it.
// Returned as a *gin.Engine, which is an http.Handler - convenient for httptest and for
// main.go.
//
// Gin matches static segments before the :code wildcard, so /healthz, /metrics and /v1
// are never treated as codes.
func NewRouter(svc LinkService, verifier *security.Verifier, intro app.Introspector) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(metrics.Middleware())

	h := &Handlers{svc: svc}

	r.GET("/healthz", h.Health)
	r.GET("/metrics", metrics.Handler())
	r.GET("/:code", h.Redirect)

	v1 := r.Group("/v1")
	v1.Use(authenticate(verifier, intro))
	{
		v1.POST("/links", h.Create)
		v1.GET("/links", h.List)
		v1.GET("/links/:code", h.Detail)
	}
	return r
}
