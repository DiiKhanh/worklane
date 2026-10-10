// Package http is link-svc's inbound (driving) adapter: it translates HTTP requests into
// use-case calls and maps results/errors back to HTTP. It depends only on the LinkService
// inbound port (satisfied by *app.Service).
package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

// LinkService is the inbound port the HTTP layer needs. Defining it here (at the point of
// use) keeps this adapter decoupled from the concrete *app.Service and lets tests inject
// a fake.
type LinkService interface {
	Shorten(ctx context.Context, in app.ShortenInput) (app.ShortenResult, error)
	Resolve(ctx context.Context, in app.ResolveInput) (string, error)
	ListLinks(ctx context.Context, tenantID string) ([]app.LinkSummary, error)
	LinkDetail(ctx context.Context, tenantID, code string) (app.LinkDetail, error)
}

var _ LinkService = (*app.Service)(nil)

// Handlers holds the port the HTTP endpoints call.
type Handlers struct {
	svc LinkService
}

// maxCreateBodyBytes caps the POST /v1/links body. The only field is a URL of at most
// 2048 characters, so anything much larger is not a legitimate request.
const maxCreateBodyBytes = 16 << 10

// Health is an unauthenticated liveness/readiness probe. It reports only that the process
// is serving - it deliberately does not touch MySQL/Redis, so a slow dependency cannot make
// k8s kill an otherwise healthy pod.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Create shortens a URL for the caller's tenant: 201 for a new link, 200 when the tenant
// had already shortened the same URL and the existing mapping is returned.
func (h *Handlers) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateBodyBytes)
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.Shorten(c.Request.Context(), app.ShortenInput{
		TenantID: c.GetString(tenantCtxKey),
		LongURL:  body.LongURL,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	c.JSON(status, createResponse{Code: res.Code, ShortURL: res.ShortURL})
}

func (h *Handlers) List(c *gin.Context) {
	links, err := h.svc.ListLinks(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, linksFromApp(links))
}

func (h *Handlers) Detail(c *gin.Context) {
	detail, err := h.svc.LinkDetail(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("code"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, detailFromApp(detail))
}

// Redirect is the public hot path: 302 to the long URL, 404 for an unknown code. It is a
// 302 rather than a 301 on purpose - browsers cache a 301 and stop hitting the service,
// which would silently end click tracking for that visitor.
func (h *Handlers) Redirect(c *gin.Context) {
	longURL, err := h.svc.Resolve(c.Request.Context(), app.ResolveInput{
		Code:     c.Param("code"),
		Referer:  c.Request.Referer(),
		UA:       c.Request.UserAgent(),
		ClientIP: c.ClientIP(),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Redirect(http.StatusFound, longURL)
}
