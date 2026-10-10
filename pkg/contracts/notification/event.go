// Package notification holds the cross-service contract for the notification bounded
// context: the Kafka event payloads and the shared channel / kind / state vocabulary.
// Like the otp contract it is a dependency-free shared kernel - notification-api
// (producer) and the dispatcher (consumer) both import it, but it imports nothing itself.
package notification

// RequestedEvent is published by notification-api on the per-channel topic
// (notification.email.requested / notification.sms.requested) and consumed by the
// dispatcher, which loads the template, renders it with Variables and delivers it.
type RequestedEvent struct {
	NotificationID string            `json:"notification_id"` // idempotency key = notification_log.id
	TenantID       string            `json:"tenant_id"`
	Channel        string            `json:"channel"`
	Recipient      string            `json:"recipient"` // raw address: never logged, masked at rest
	TemplateID     string            `json:"template_id"`
	Variables      map[string]string `json:"variables"`
	Kind           string            `json:"kind"`
	UserRef        string            `json:"user_ref"` // tenant's own user identifier, may be empty
}

// PartitionKey makes all events for one notification land on the same Kafka partition,
// so a redelivered duplicate is seen after the original and can be skipped.
func (e RequestedEvent) PartitionKey() string { return e.NotificationID }

// SentEvent is published on notification.sent after a successful delivery. It carries
// neither the recipient nor the variables: downstream readers (analytics, webhooks)
// only need the identity of the notification.
type SentEvent struct {
	NotificationID string `json:"notification_id"`
	TenantID       string `json:"tenant_id"`
	Channel        string `json:"channel"`
	Provider       string `json:"provider"`
}

// PartitionKey keeps a notification's sent event on the same partition as its other events.
func (e SentEvent) PartitionKey() string { return e.NotificationID }

// FailedEvent is published on notification.failed and notification.dlq after a terminal
// delivery failure. Like SentEvent it omits the recipient and variables; it adds the
// error reason for diagnosis.
type FailedEvent struct {
	NotificationID string `json:"notification_id"`
	TenantID       string `json:"tenant_id"`
	Channel        string `json:"channel"`
	Provider       string `json:"provider"`
	Error          string `json:"error"`
}

// PartitionKey keeps a notification's failed event on the same partition as its other events.
func (e FailedEvent) PartitionKey() string { return e.NotificationID }

// Channel strings are the delivery channels of the first cut (push is deferred).
const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
)

// Kind strings decide whether a recipient's opt-out applies: marketing respects
// notification_settings, transactional bypasses it.
const (
	KindTransactional = "transactional"
	KindMarketing     = "marketing"
)

// State strings are the shared persistence/wire vocabulary for notification_log.state.
const (
	StateQueued     = "queued"
	StateSent       = "sent"
	StateFailed     = "failed"
	StateSuppressed = "suppressed"
)

// ValidChannel reports whether c is a supported delivery channel.
func ValidChannel(c string) bool { return c == ChannelEmail || c == ChannelSMS }

// ValidKind reports whether k is a supported notification kind.
func ValidKind(k string) bool { return k == KindTransactional || k == KindMarketing }
