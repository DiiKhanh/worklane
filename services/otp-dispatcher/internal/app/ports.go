// Package app is otp-dispatcher's application layer: the Handle use case that turns an
// otp.requested event into a sent email plus a delivery record. It depends on ports
// (EmailProvider, Repo, Publisher, Clock) and the shared contract - never on adapters or
// pkg/platform.
package app

import (
	"context"
	"time"
)

// EmailProvider sends one message and returns the provider's message id.
type EmailProvider interface {
	Send(ctx context.Context, to, subject, body string) (providerMsgID string, err error)
}

// DeliveryLog is one provider attempt, written for the dashboard/audit.
type DeliveryLog struct {
	RequestID     string
	TenantID      string
	Provider      string
	Status        string
	LatencyMillis int64
	Error         string
}

// Repo persists delivery outcomes and advances the request state.
type Repo interface {
	InsertDeliveryLog(ctx context.Context, l DeliveryLog) error
	UpdateState(ctx context.Context, id, to string) error
}

// Publisher emits follow-up events (sent / failed / dlq).
type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
}

// Clock abstracts time so latency measurement is testable.
type Clock interface{ Now() time.Time }
