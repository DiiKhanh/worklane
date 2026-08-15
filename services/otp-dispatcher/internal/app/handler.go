package app

import (
	"context"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

// Config holds the topics, provider label, and email template for the dispatch handler.
type Config struct {
	SentTopic    string
	FailedTopic  string
	DLQTopic     string
	ProviderName string // recorded on delivery logs (e.g. "resend", "smtp")
	Template     Template
}

// Deps bundles the ports the handler depends on.
type Deps struct {
	Mail  EmailProvider
	Repo  Repo
	Pub   Publisher
	Clock Clock
}

// Handler is the async delivery use case: render, send, record, publish.
type Handler struct {
	d   Deps
	cfg Config
}

func NewHandler(d Deps, cfg Config) *Handler { return &Handler{d: d, cfg: cfg} }

// Handle processes one otp.requested event. A provider failure is treated as terminal:
// we record it, mark the request failed, and route the event to the DLQ (no drainer in
// the MVP) - then return nil so the message is not redelivered. Infrastructure errors
// (repo/publish) are returned so Kafka redelivers the message (at-least-once).
func (h *Handler) Handle(ctx context.Context, evt contracts.RequestedEvent) error {
	subject, body := h.cfg.Template.Render(evt.Code)

	start := h.d.Clock.Now()
	msgID, sendErr := h.d.Mail.Send(ctx, evt.Recipient, subject, body)
	latency := h.d.Clock.Now().Sub(start).Milliseconds()

	if sendErr != nil {
		if err := h.d.Repo.InsertDeliveryLog(ctx, DeliveryLog{
			RequestID: evt.RequestID, TenantID: evt.TenantID, Provider: h.cfg.ProviderName,
			Status: contracts.StateFailed, LatencyMillis: latency, Error: sendErr.Error(),
		}); err != nil {
			return err
		}
		if err := h.d.Repo.UpdateState(ctx, evt.RequestID, contracts.StateFailed); err != nil {
			return err
		}
		if err := h.d.Pub.Publish(ctx, h.cfg.FailedTopic, evt); err != nil {
			return err
		}
		if err := h.d.Pub.Publish(ctx, h.cfg.DLQTopic, evt); err != nil {
			return err
		}
		return nil
	}

	if err := h.d.Repo.InsertDeliveryLog(ctx, DeliveryLog{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Provider: h.cfg.ProviderName,
		Status: contracts.StateSent, LatencyMillis: latency, Error: "",
	}); err != nil {
		return err
	}
	if err := h.d.Repo.UpdateState(ctx, evt.RequestID, contracts.StateSent); err != nil {
		return err
	}
	_ = msgID // provider message id is available for richer logging later
	return h.d.Pub.Publish(ctx, h.cfg.SentTopic, evt)
}
