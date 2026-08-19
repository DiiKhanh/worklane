package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// NewRouter builds the otp-api HTTP handler: all /v1 routes sit behind the authenticate
// middleware (user JWT or tenant API key via introspection), so every endpoint is
// tenant-scoped. Returned as a *gin.Engine, which is an http.Handler - convenient for
// httptest and for main.go.
func NewRouter(svc OTPService, repo app.Repo, verifier *security.Verifier, intro app.Introspector) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := &Handlers{svc: svc, repo: repo}

	v1 := r.Group("/v1")
	v1.Use(authenticate(verifier, intro))
	{
		v1.POST("/otp/send", h.Send)
		v1.POST("/otp/verify", h.Verify)
		v1.GET("/otp/requests", h.ListRequests)
		v1.GET("/delivery-logs", h.ListDeliveryLogs)
	}
	return r
}
