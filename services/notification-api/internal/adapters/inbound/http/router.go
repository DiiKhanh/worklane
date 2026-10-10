package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/platform/metrics"
	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/notification-api/internal/app"
)

// NewRouter builds the notification-api HTTP handler. Everything under /v1 sits behind
// the authenticate middleware (user JWT or tenant API key via introspection) and is
// tenant-scoped. Returned as a *gin.Engine, which is an http.Handler - convenient for
// httptest and for main.go.
func NewRouter(svc NotificationService, verifier *security.Verifier, intro app.Introspector) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(metrics.Middleware())

	h := &Handlers{svc: svc}

	r.GET("/healthz", h.Health)
	r.GET("/metrics", metrics.Handler())

	v1 := r.Group("/v1")
	v1.Use(authenticate(verifier, intro))
	{
		v1.GET("/templates", h.ListTemplates)
		v1.POST("/templates", h.CreateTemplate)
		v1.GET("/templates/:id", h.GetTemplate)
		v1.PUT("/templates/:id", h.UpdateTemplate)
		v1.POST("/templates/:id/preview", h.PreviewTemplate)

		v1.POST("/notifications", h.Send)
		v1.GET("/notifications", h.ListNotifications)
		v1.GET("/notifications/:id", h.GetNotification)

		v1.GET("/preferences", h.GetPreferences)
		v1.PUT("/preferences", h.SetPreference)
	}
	return r
}
