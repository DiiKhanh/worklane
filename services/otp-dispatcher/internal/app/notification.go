package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	"github.com/duykhanh/worklane/pkg/templating"
)

// ErrInvalidNotification marks an event that can never be processed (no id, no tenant,
// or no notification_log row). The inbound adapter drops it instead of redelivering.
var ErrInvalidNotification = errors.New("invalid notification event")

// maxErrorRunes bounds the failure reason stored on notification_log and published on
// the failed / DLQ topics; provider error bodies are unbounded.
const maxErrorRunes = 1000

// PermanentError wraps a delivery error that retrying cannot fix (e.g. the provider
// rejected the recipient). Its message is the wrapped error's, so logs are unchanged.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent marks err as not worth retrying. Providers use it for definitive rejections;
// any other Sender error is treated as transient by the notification handler.
func Permanent(err error) error { return &PermanentError{Err: err} }

// NotificationOutcome is the result of a delivery, written onto the notification_log row.
type NotificationOutcome struct {
	Provider      string
	ProviderMsgID string
	LatencyMillis int64
	Error         string
	At            time.Time
}

// NotificationTemplates loads a tenant's template from the notification DB.
// found=false means the tenant has no template with that id.
type NotificationTemplates interface {
	Find(ctx context.Context, tenantID, id string) (tpl Template, found bool, err error)
}

// NotificationLog reads and advances notification_log rows. MarkSent and MarkFailed are
// guarded transitions: they must never rewrite a row that is already sent, and matching
// no row is not an error.
type NotificationLog interface {
	State(ctx context.Context, tenantID, id string) (state string, found bool, err error)
	MarkSent(ctx context.Context, id string, o NotificationOutcome) error
	MarkFailed(ctx context.Context, id string, o NotificationOutcome) error
}

// LinkShortener resolves a {{link "url"}} directive to a short URL for a tenant.
type LinkShortener func(ctx context.Context, tenantID, longURL string) (shortURL string, err error)

// NotificationConfig holds the follow-up topics and the retry policy.
type NotificationConfig struct {
	SentTopic   string
	FailedTopic string
	DLQTopic    string
	MaxAttempts int           // total delivery attempts per message, at least 1
	BaseBackoff time.Duration // wait before the 2nd attempt; doubles for each later one
}

// NotificationDeps bundles the ports the notification handler depends on. Senders is the
// same per-channel registry the OTP handler uses. Shorten may be nil, in which case a
// template containing a {{link}} fails delivery. Sleep waits for d or until ctx is done.
type NotificationDeps struct {
	Senders   map[string]Sender
	Templates NotificationTemplates
	Log       NotificationLog
	Pub       Publisher
	Clock     Clock
	Shorten   LinkShortener
	Sleep     func(ctx context.Context, d time.Duration) error
}

// NotificationHandler is the generic delivery use case: load the template, render it
// with the event's variables, deliver, record the outcome and publish sent / failed.
type NotificationHandler struct {
	d   NotificationDeps
	cfg NotificationConfig
}

func NewNotificationHandler(d NotificationDeps, cfg NotificationConfig) *NotificationHandler {
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	return &NotificationHandler{d: d, cfg: cfg}
}

// Handle processes one notification.<channel>.requested event.
//
// Only a row still in state queued is delivered: sent means a redelivered duplicate,
// suppressed was never meant to go out, and failed means notification-api already told
// the caller the send failed (they retry with a new idempotency key), so delivering it
// now would double-send.
//
// A terminal failure (unknown channel, missing template, render error, permanent
// provider error, or transient errors on every attempt) is recorded as failed and fanned
// out to failed + DLQ, then nil is returned so Kafka does not redeliver it.
// Infrastructure errors (repo/publish) are returned so the message is redelivered.
func (h *NotificationHandler) Handle(ctx context.Context, evt contracts.RequestedEvent) error {
	if evt.NotificationID == "" || evt.TenantID == "" {
		return fmt.Errorf("%w: missing notification or tenant id", ErrInvalidNotification)
	}
	state, found, err := h.d.Log.State(ctx, evt.TenantID, evt.NotificationID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: no log row for %s", ErrInvalidNotification, evt.NotificationID)
	}
	if state != contracts.StateQueued {
		return nil
	}

	sender, ok := h.d.Senders[evt.Channel]
	if !ok {
		return h.fail(ctx, evt, "unknown", 0, "unsupported channel: "+evt.Channel)
	}
	tpl, found, err := h.d.Templates.Find(ctx, evt.TenantID, evt.TemplateID)
	if err != nil {
		return err
	}
	if !found {
		return h.fail(ctx, evt, sender.Name(), 0, "template not found")
	}
	msg, err := templating.RenderMessage(ctx, tpl.Subject, tpl.Body, evt.Variables, h.shortener(evt.TenantID))
	if err != nil {
		return h.fail(ctx, evt, sender.Name(), 0, "render: "+err.Error())
	}

	start := h.d.Clock.Now()
	msgID, sendErr := h.deliver(ctx, sender, evt.Recipient, msg)
	latency := h.d.Clock.Now().Sub(start).Milliseconds()
	if sendErr != nil {
		if ctx.Err() != nil {
			// Shutting down mid-retry: leave the row queued so the redelivery retries.
			return ctx.Err()
		}
		return h.fail(ctx, evt, sender.Name(), latency, sendErr.Error())
	}

	if err := h.d.Log.MarkSent(ctx, evt.NotificationID, NotificationOutcome{
		Provider: sender.Name(), ProviderMsgID: msgID, LatencyMillis: latency, At: h.d.Clock.Now(),
	}); err != nil {
		return err
	}
	return h.d.Pub.Publish(ctx, h.cfg.SentTopic, contracts.SentEvent{
		NotificationID: evt.NotificationID, TenantID: evt.TenantID,
		Channel: evt.Channel, Provider: sender.Name(),
	})
}

// shortener binds the tenant-scoped port to the render engine's signature. A nil port
// stays nil so RenderMessage reports ErrNoShortener for a template that has a link.
func (h *NotificationHandler) shortener(tenantID string) templating.Shortener {
	if h.d.Shorten == nil {
		return nil
	}
	return func(ctx context.Context, longURL string) (string, error) {
		return h.d.Shorten(ctx, tenantID, longURL)
	}
}

// deliver sends the rendered message, retrying transient errors with exponential
// backoff up to MaxAttempts. A PermanentError stops immediately.
func (h *NotificationHandler) deliver(ctx context.Context, s Sender, to string, msg templating.Message) (string, error) {
	backoff := h.cfg.BaseBackoff
	for attempt := 1; ; attempt++ {
		msgID, err := s.Send(ctx, to, msg.Subject, msg.Body)
		if err == nil {
			return msgID, nil
		}
		var perm *PermanentError
		if errors.As(err, &perm) || attempt >= h.cfg.MaxAttempts {
			return "", err
		}
		if serr := h.d.Sleep(ctx, backoff); serr != nil {
			return "", serr
		}
		backoff *= 2
	}
}

// fail records a terminal failure and fans it out to failed + DLQ. It returns nil on
// success so Kafka does not redeliver the message.
func (h *NotificationHandler) fail(ctx context.Context, evt contracts.RequestedEvent, provider string, latency int64, reason string) error {
	reason = truncateRunes(reason, maxErrorRunes)
	if err := h.d.Log.MarkFailed(ctx, evt.NotificationID, NotificationOutcome{
		Provider: provider, LatencyMillis: latency, Error: reason, At: h.d.Clock.Now(),
	}); err != nil {
		return err
	}
	failed := contracts.FailedEvent{
		NotificationID: evt.NotificationID, TenantID: evt.TenantID,
		Channel: evt.Channel, Provider: provider, Error: reason,
	}
	if err := h.d.Pub.Publish(ctx, h.cfg.FailedTopic, failed); err != nil {
		return err
	}
	return h.d.Pub.Publish(ctx, h.cfg.DLQTopic, failed)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
