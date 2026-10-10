// Package app is notification-api's application layer: the Send, template, preference
// and read use cases and the outbound ports they depend on. It imports the pure domain,
// the shared contract and the shared render package, never an adapter, driver, or
// pkg/platform (enforced by the arch test).
package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// ErrInvalidAPIKey is returned by an Introspector for an unknown or revoked API key, as
// opposed to the identity service being unreachable.
var ErrInvalidAPIKey = errors.New("invalid api key")

// Introspector resolves an opaque API key to a tenant via auth-svc.
type Introspector interface {
	Introspect(ctx context.Context, apiKey string) (tenantID string, err error)
}

// Clock abstracts the current time so use cases are deterministic under test.
type Clock interface{ Now() time.Time }

// IDGen mints random UUIDs (36 chars, the width of the id columns).
type IDGen interface{ New() string }

// TemplateRepo is the durable template store (MySQL). Every lookup is tenant-scoped and
// returns domain.ErrNotFound for a missing row or one owned by another tenant.
type TemplateRepo interface {
	Insert(ctx context.Context, t domain.Template) error
	Find(ctx context.Context, tenantID, id string) (domain.Template, error)
	List(ctx context.Context, tenantID string) ([]domain.Template, error)
	// Update stores next and the snapshot of the content it replaces in one
	// transaction. It must only apply when the stored version is still prior.Version
	// (optimistic lock) and return domain.ErrConflict otherwise.
	Update(ctx context.Context, next domain.Template, prior domain.TemplateVersion) error
}

// LogRepo is the notification_log store (MySQL). Insert returns
// domain.ErrAlreadyExists when the id is taken: the primary key is the idempotency
// guard, so it also holds under concurrent duplicate sends.
type LogRepo interface {
	Insert(ctx context.Context, n domain.Notification) error
	Find(ctx context.Context, tenantID, id string) (domain.Notification, error)
	List(ctx context.Context, tenantID string, limit int) ([]domain.Notification, error)
	Events(ctx context.Context, notificationID string) ([]domain.Event, error)
	MarkFailed(ctx context.Context, id, reason string, at time.Time) error
}

// SettingsRepo is the notification_settings store. List returns only the stored rows;
// a channel without a row is enabled.
type SettingsRepo interface {
	List(ctx context.Context, tenantID, userRef string) ([]domain.Setting, error)
	Upsert(ctx context.Context, tenantID, userRef string, s domain.Setting) error
}

// Counter backs the send rate limits (Redis fixed window): Incr sets the TTL on the
// first hit of a window and returns the running count.
type Counter interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// Publisher publishes domain events to Kafka. The topic is passed in (config-driven),
// never hard-coded at the call site.
type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
}

// Shortener turns the target of a {{link "url"}} directive into a short URL on behalf
// of a tenant (link-svc in production). Preview uses the same port as delivery.
type Shortener interface {
	Shorten(ctx context.Context, tenantID, longURL string) (shortURL string, err error)
}
