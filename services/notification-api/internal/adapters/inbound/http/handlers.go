// Package http is notification-api's inbound (driving) adapter: it translates HTTP
// requests into use-case calls and maps results/errors back to HTTP. It depends only on
// the NotificationService inbound port (satisfied by *app.Service).
package http

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/notification-api/internal/app"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// NotificationService is the inbound port the HTTP layer needs. Defining it here (at the
// point of use) keeps this adapter decoupled from the concrete *app.Service and lets
// tests inject a fake.
type NotificationService interface {
	Send(ctx context.Context, in app.SendInput) (app.SendResult, error)
	ListNotifications(ctx context.Context, tenantID string) ([]domain.Notification, error)
	Notification(ctx context.Context, tenantID, id string) (app.NotificationDetail, error)

	CreateTemplate(ctx context.Context, in app.CreateTemplateInput) (domain.Template, error)
	UpdateTemplate(ctx context.Context, in app.UpdateTemplateInput) (domain.Template, error)
	GetTemplate(ctx context.Context, tenantID, id string) (domain.Template, error)
	ListTemplates(ctx context.Context, tenantID string) ([]domain.Template, error)
	PreviewTemplate(ctx context.Context, tenantID, id string, vars map[string]string) (app.Preview, error)

	Preferences(ctx context.Context, tenantID, userRef string) ([]domain.Setting, error)
	SetPreference(ctx context.Context, in app.SetPreferenceInput) error
}

var _ NotificationService = (*app.Service)(nil)

// Handlers holds the port the HTTP endpoints call.
type Handlers struct {
	svc NotificationService
}

// maxBodyBytes caps every request body. The largest legitimate one is a template whose
// body fills its TEXT column (65535 bytes), which JSON escaping can roughly double.
const maxBodyBytes = 256 << 10

// bind decodes the JSON body into dst, answering 400 itself when the body is missing,
// malformed, mistyped or over maxBodyBytes. It reports whether the handler may go on.
func bind(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	if err := c.ShouldBindJSON(dst); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	return true
}

// Health is an unauthenticated liveness/readiness probe. It reports only that the process
// is serving - it deliberately does not touch MySQL/Redis, so a slow dependency cannot make
// k8s kill an otherwise healthy pod.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// --- templates ---

func (h *Handlers) ListTemplates(c *gin.Context) {
	ts, err := h.svc.ListTemplates(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, templatesFromDomain(ts))
}

func (h *Handlers) CreateTemplate(c *gin.Context) {
	var body templateRequest
	if !bind(c, &body) {
		return
	}
	t, err := h.svc.CreateTemplate(c.Request.Context(), app.CreateTemplateInput{
		TenantID: c.GetString(tenantCtxKey),
		Name:     body.Name, Channel: body.Channel, Locale: body.Locale,
		Subject: body.Subject, Body: body.Body,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, templateFromDomain(t))
}

func (h *Handlers) GetTemplate(c *gin.Context) {
	t, err := h.svc.GetTemplate(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, templateFromDomain(t))
}

// UpdateTemplate replaces the template's content (a full PUT, not a patch). The channel
// is fixed at creation, so a "channel" in the body is ignored.
func (h *Handlers) UpdateTemplate(c *gin.Context) {
	var body templateRequest
	if !bind(c, &body) {
		return
	}
	t, err := h.svc.UpdateTemplate(c.Request.Context(), app.UpdateTemplateInput{
		TenantID: c.GetString(tenantCtxKey), ID: c.Param("id"),
		Name: body.Name, Locale: body.Locale, Subject: body.Subject, Body: body.Body,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, templateFromDomain(t))
}

// PreviewTemplate renders the stored template with sample variables. The body is
// optional: previewing with no variables at all is a legitimate request.
func (h *Handlers) PreviewTemplate(c *gin.Context) {
	var body previewRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := h.svc.PreviewTemplate(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("id"), body.Variables)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, previewFromApp(p))
}

// --- notifications ---

// Send accepts a notification for asynchronous delivery: 202 once it is logged and
// queued (or logged as suppressed by an opt-out), 200 when the idempotency key matched
// an earlier send and that notification is returned instead.
func (h *Handlers) Send(c *gin.Context) {
	var body sendRequest
	if !bind(c, &body) {
		return
	}
	res, err := h.svc.Send(c.Request.Context(), app.SendInput{
		TenantID: c.GetString(tenantCtxKey),
		Channel:  body.Channel, Recipient: body.Recipient, TemplateID: body.TemplateID,
		Variables: body.Variables, Kind: body.Kind,
		UserRef: body.UserRef, IdempotencyKey: body.IdempotencyKey,
	})
	if errors.Is(err, domain.ErrNotFound) {
		// The only thing Send looks up is the template. A bare 404 on a collection POST
		// would read as "no such endpoint", so say what is missing.
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	status := http.StatusAccepted
	if res.Duplicate {
		status = http.StatusOK
	}
	c.JSON(status, sendResponse{NotificationID: res.NotificationID, State: res.State})
}

func (h *Handlers) ListNotifications(c *gin.Context) {
	ns, err := h.svc.ListNotifications(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, notificationsFromDomain(ns))
}

func (h *Handlers) GetNotification(c *gin.Context) {
	d, err := h.svc.Notification(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, detailFromApp(d))
}

// --- preferences ---

func (h *Handlers) GetPreferences(c *gin.Context) {
	userRef := c.Query("user_ref")
	settings, err := h.svc.Preferences(c.Request.Context(), c.GetString(tenantCtxKey), userRef)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, preferencesFromDomain(userRef, settings))
}

// SetPreference turns one channel on or off for a user and echoes the stored setting.
func (h *Handlers) SetPreference(c *gin.Context) {
	var body preferenceRequest
	if !bind(c, &body) {
		return
	}
	err := h.svc.SetPreference(c.Request.Context(), app.SetPreferenceInput{
		TenantID: c.GetString(tenantCtxKey),
		UserRef:  body.UserRef, Channel: body.Channel, Enabled: *body.Enabled,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_ref": body.UserRef, "channel": body.Channel, "enabled": *body.Enabled})
}
