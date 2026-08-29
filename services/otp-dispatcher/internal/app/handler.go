package app

import (
	"context"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/pkg/templating"
)

// Config holds the follow-up topics plus template-rendering settings for the handler.
type Config struct {
	SentTopic   string
	FailedTopic string
	DLQTopic    string
	ExpiryText  string              // value substituted for {{expiry}}, e.g. "5 minutes"
	Fallback    map[string]Template // keyed by channel; used when no active DB template exists
}

// Deps bundles the ports the handler depends on. Senders is keyed by channel
// (e.g. "email", "sms"); Templates resolves the active template for (channel, locale).
type Deps struct {
	Senders   map[string]Sender
	Templates TemplateSource
	Repo      Repo
	Pub       Publisher
	Clock     Clock
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
	sender, ok := h.d.Senders[evt.Channel]
	if !ok {
		// Unknown channel: record a failed delivery and route to the DLQ (no redelivery).
		return h.recordFailure(ctx, evt, "unknown", 0, "unsupported channel: "+evt.Channel)
	}

	// Resolve the active template for (channel, locale). Any missing row / DB / cache
	// problem falls back to the env template, so OTP delivery never breaks on templates.
	locale := evt.Locale
	if locale == "" {
		locale = "en"
	}
	tpl, found, terr := h.d.Templates.Active(ctx, evt.Channel, locale)
	if terr != nil || !found {
		tpl = h.cfg.Fallback[evt.Channel]
	}
	subject, body := templating.Render(tpl.Subject, tpl.Body,
		templating.Vars{Code: evt.Code, Expiry: h.cfg.ExpiryText})

	start := h.d.Clock.Now()
	msgID, sendErr := sender.Send(ctx, evt.Recipient, subject, body)
	latency := h.d.Clock.Now().Sub(start).Milliseconds()

	if sendErr != nil {
		return h.recordFailure(ctx, evt, sender.Name(), latency, sendErr.Error())
	}

	if err := h.d.Repo.InsertDeliveryLog(ctx, DeliveryLog{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Provider: sender.Name(),
		Status: contracts.StateSent, LatencyMillis: latency, Error: "",
	}); err != nil {
		return err
	}
	if err := h.d.Repo.UpdateState(ctx, evt.RequestID, contracts.StateSent); err != nil {
		return err
	}
	_ = msgID // provider message id is available for richer logging later
	return h.d.Pub.Publish(ctx, h.cfg.SentTopic, contracts.SentEvent{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Recipient: evt.Recipient,
		Channel: evt.Channel, Provider: sender.Name(),
	})
}

// recordFailure logs a failed delivery, marks the request failed, and fans the event out
// to failed + DLQ. It returns nil so Kafka does not redeliver a terminal failure.
func (h *Handler) recordFailure(ctx context.Context, evt contracts.RequestedEvent, provider string, latency int64, msg string) error {
	if err := h.d.Repo.InsertDeliveryLog(ctx, DeliveryLog{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Provider: provider,
		Status: contracts.StateFailed, LatencyMillis: latency, Error: msg,
	}); err != nil {
		return err
	}
	if err := h.d.Repo.UpdateState(ctx, evt.RequestID, contracts.StateFailed); err != nil {
		return err
	}
	failed := contracts.FailedEvent{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Recipient: evt.Recipient,
		Channel: evt.Channel, Provider: provider, Error: msg,
	}
	if err := h.d.Pub.Publish(ctx, h.cfg.FailedTopic, failed); err != nil {
		return err
	}
	return h.d.Pub.Publish(ctx, h.cfg.DLQTopic, failed)
}
