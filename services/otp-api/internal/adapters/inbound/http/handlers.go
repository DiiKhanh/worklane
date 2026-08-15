// Package http is otp-api's inbound (driving) adapter: it translates HTTP requests into
// use-case calls and maps results/errors back to HTTP. It depends on the OTPService
// inbound port (satisfied by *app.Service) and the app.Repo port for read endpoints.
package http

import (
	"context"
	"net/http"

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

func (h *Handlers) Send(c *gin.Context) {
	var body sendRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.Send(c.Request.Context(), app.SendInput{
		TenantID:       c.GetString(tenantCtxKey),
		Recipient:      body.Recipient,
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

func (h *Handlers) ListAPIKeys(c *gin.Context) {
	keys, err := h.repo.ListAPIKeys(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]apiKeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, apiKeyDTO{ID: k.ID, TenantID: k.TenantID, Status: k.Status})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ListRequests(c *gin.Context) {
	rows, err := h.repo.ListRequests(c.Request.Context(), c.GetString(tenantCtxKey), defaultListLimit)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]requestDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, requestDTO{ID: r.ID, Recipient: r.Recipient, Channel: r.Channel, State: r.State})
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
			LatencyMillis: l.LatencyMillis, Error: l.Error,
		})
	}
	c.JSON(http.StatusOK, out)
}
