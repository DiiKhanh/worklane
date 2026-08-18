package app

import (
	"context"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

// Config holds the follow-up topics for the dispatch handler. Provider label and template
// now live inside each Sender.
type Config struct {
	SentTopic   string
	FailedTopic string
	DLQTopic    string
}

// Deps bundles the ports the handler depends on. Senders is keyed by channel
// (e.g. "email", "sms"); each Sender renders and delivers its own message.
type Deps struct {
	Senders map[string]Sender
	Repo    Repo
	Pub     Publisher
	Clock   Clock
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

	start := h.d.Clock.Now()
	msgID, sendErr := sender.Send(ctx, evt.Recipient, evt.Code)
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
	return h.d.Pub.Publish(ctx, h.cfg.SentTopic, evt)
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
	if err := h.d.Pub.Publish(ctx, h.cfg.FailedTopic, evt); err != nil {
		return err
	}
	return h.d.Pub.Publish(ctx, h.cfg.DLQTopic, evt)
}
