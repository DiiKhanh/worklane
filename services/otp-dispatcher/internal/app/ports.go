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

// SMSProvider sends one text message and returns the provider's message id.
type SMSProvider interface {
	Send(ctx context.Context, to, body string) (providerMsgID string, err error)
}

// Sender delivers one already-rendered message over a specific channel. Name is recorded
// on the delivery log as the provider label. Rendering (template resolution + variable
// substitution) happens in the handler, so a Sender is pure delivery.
type Sender interface {
	Name() string
	Send(ctx context.Context, to, subject, body string) (msgID string, err error)
}

// Template is a raw (unrendered) message template resolved for a (channel, locale).
type Template struct {
	Subject string
	Body    string
}

// TemplateSource resolves the active template for a (channel, locale). found=false means
// no active row exists, so the handler renders from its env fallback instead.
type TemplateSource interface {
	Active(ctx context.Context, channel, locale string) (tpl Template, found bool, err error)
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
