// Package otp holds the cross-service contract for the OTP bounded context: the
// Kafka event payloads and the shared state vocabulary. It is a dependency-free
// shared kernel - both otp-api (producer) and otp-dispatcher (consumer) import it,
// but it imports nothing itself.
package otp

// RequestedEvent is published by otp-api on otp.requested and consumed by
// otp-dispatcher. Both sides marshal/unmarshal this exact shape.
type RequestedEvent struct {
	RequestID string `json:"request_id"`
	TenantID  string `json:"tenant_id"`
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Code      string `json:"code"` // never logged
}

// PartitionKey makes all events for one request land on the same Kafka partition, so a
// consumer sees them in order. The kafka producer uses this when the event provides it.
func (e RequestedEvent) PartitionKey() string { return e.RequestID }

// SentEvent is published on otp.sent after a successful delivery. It deliberately omits
// the OTP Code: the secret belongs only on otp.requested (producer -> dispatcher), never
// on downstream topics that analytics, webhooks, or a DLQ drainer may read.
type SentEvent struct {
	RequestID string `json:"request_id"`
	TenantID  string `json:"tenant_id"`
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Provider  string `json:"provider"`
}

// PartitionKey keeps a request's sent event on the same partition as its other events.
func (e SentEvent) PartitionKey() string { return e.RequestID }

// FailedEvent is published on otp.failed and otp.dlq after a terminal delivery failure.
// Like SentEvent it omits the Code; it adds the provider and error reason for diagnosis.
type FailedEvent struct {
	RequestID string `json:"request_id"`
	TenantID  string `json:"tenant_id"`
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Provider  string `json:"provider"`
	Error     string `json:"error"`
}

// PartitionKey keeps a request's failed event on the same partition as its other events.
func (e FailedEvent) PartitionKey() string { return e.RequestID }

// State strings are the shared persistence/wire vocabulary for otp_requests.state.
const (
	StateRequested = "requested"
	StateSent      = "sent"
	StateFailed    = "failed"
	StateVerified  = "verified"
	StateExpired   = "expired"
)
