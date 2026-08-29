// Package http is otp-api's inbound (driving) adapter: it translates HTTP requests into
// use-case calls and maps results/errors back to HTTP. It depends on the OTPService
// inbound port (satisfied by *app.Service) and the app.Repo port for read endpoints.
package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// OTPService is the inbound port the HTTP layer needs. Defining it here (at the point of
// use) keeps this adapter decoupled from the concrete *app.Service and lets tests inject
// a fake.
type OTPService interface {
	Send(ctx context.Context, in app.SendInput) (app.SendResult, error)
	Verify(ctx context.Context, in app.VerifyInput) error
}

// Handlers holds the ports the HTTP endpoints call.
type Handlers struct {
	svc  OTPService
	repo app.Repo
}

const defaultListLimit = 100

// Health is an unauthenticated liveness/readiness probe. It reports only that the process
// is serving - it deliberately does not touch MySQL/Redis, so a slow dependency cannot make
// k8s kill an otherwise healthy pod.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handlers) Send(c *gin.Context) {
	var body sendRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.Send(c.Request.Context(), app.SendInput{
		TenantID:       c.GetString(tenantCtxKey),
		Recipient:      body.Recipient,
		Channel:        body.Channel,
		Locale:         body.Locale,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, sendResponse{RequestID: res.RequestID})
}

func (h *Handlers) Verify(c *gin.Context) {
	var body verifyRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Verify(c.Request.Context(), app.VerifyInput{
		TenantID:  c.GetString(tenantCtxKey),
		Recipient: body.Recipient,
		Code:      body.Code,
	}); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "verified"})
}

func (h *Handlers) ListRequests(c *gin.Context) {
	rows, err := h.repo.ListRequests(c.Request.Context(), c.GetString(tenantCtxKey), defaultListLimit)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]requestDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, requestDTO{
			ID: r.ID, Recipient: r.Recipient, Channel: r.Channel, State: r.State,
			CreatedAt: r.CreatedAt.UTC(),
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ListDeliveryLogs(c *gin.Context) {
	rows, err := h.repo.ListDeliveryLogs(c.Request.Context(), c.GetString(tenantCtxKey), defaultListLimit)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]deliveryLogDTO, 0, len(rows))
	for _, l := range rows {
		out = append(out, deliveryLogDTO{
			RequestID: l.RequestID, Provider: l.Provider, Status: l.Status,
			LatencyMillis: l.LatencyMillis, Error: l.Error, CreatedAt: l.CreatedAt.UTC(),
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) Stats(c *gin.Context) {
	stats, err := h.repo.Stats(c.Request.Context(), c.GetString(tenantCtxKey), time.Now().UTC())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, statsFromApp(stats))
}

func statsFromApp(stats app.Stats) statsDTO {
	series := make([]statsPointDTO, 0, len(stats.Series))
	for _, point := range stats.Series {
		series = append(series, statsPointDTO{
			T: point.T.UTC(), Requested: point.Requested, Sent: point.Sent,
			Verified: point.Verified, Failed: point.Failed,
		})
	}
	return statsDTO{
		SentToday:        stats.SentToday,
		VerifyRate:       stats.VerifyRate,
		Failed:           stats.Failed,
		P50LatencyMillis: stats.P50LatencyMillis,
		Series:           series,
		Funnel: statsFunnelDTO{
			Requested: stats.Funnel.Requested,
			Sent:      stats.Funnel.Sent,
			Verified:  stats.Funnel.Verified,
		},
	}
}
