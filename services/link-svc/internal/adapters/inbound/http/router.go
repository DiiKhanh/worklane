package http

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/platform/metrics"
	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type Management interface {
	Create(context.Context, string, string) (domain.Link, bool, error)
	Get(context.Context, string, string) (domain.Link, error)
	List(context.Context, string, *domain.Position, int) ([]domain.Link, error)
	Disable(context.Context, string, string) (domain.Link, error)
}

func NewRouter(svc Management, verifier *security.Verifier, intro app.Introspector, publicBase string) (*gin.Engine, error) {
	u, err := url.Parse(publicBase)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("link: LINK_PUBLIC_BASE must be an HTTP/HTTPS origin")
	}
	host, err := normalizedHost(u.Hostname())
	if err != nil {
		return nil, errors.New("link: invalid LINK_PUBLIC_BASE host")
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), metrics.Middleware())
	h := &Handlers{svc: svc, publicBase: strings.TrimRight(publicBase, "/"), shortHost: host}
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, HealthResponse{Status: "ok"}) })
	r.GET("/metrics", metrics.Handler())
	v1 := r.Group("/v1/links", authenticate(verifier, intro))
	v1.POST("", h.Create)
	v1.GET("", h.List)
	v1.GET("/:code", h.Get)
	v1.POST("/:code/disable", h.Disable)
	return r, nil
}
