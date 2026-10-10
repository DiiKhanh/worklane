// Package app is link-svc's application layer: the Shorten/Resolve/ListLinks/LinkDetail
// use cases and the outbound ports they depend on. It imports the pure domain and the
// shared contract, never an adapter, driver, or pkg/platform (enforced by the arch test).
package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// ErrInvalidAPIKey is returned by an Introspector for an unknown or revoked API key, as
// opposed to the identity service being unreachable.
var ErrInvalidAPIKey = errors.New("invalid api key")

// Introspector resolves an opaque API key to a tenant via auth-svc.
type Introspector interface {
	Introspect(ctx context.Context, apiKey string) (tenantID string, err error)
}

// Target is what the redirect hot path needs for one code: where to send the visitor
// and which tenant the click belongs to. It is the value kept in the cache, so a cache
// hit can publish link.clicked without touching the DB.
type Target struct {
	LongURL  string
	TenantID string
}

// LinkStat is a link plus its lifetime click count, for the tenant's list view.
type LinkStat struct {
	Link   domain.Link
	Clicks int64
}

// DayCount is the number of clicks on one UTC calendar day.
type DayCount struct {
	Day    time.Time // midnight UTC
	Clicks int64
}

// Click is one persisted link_clicks row (written by link-dispatcher).
type Click struct {
	TS      time.Time
	Referer string
	UA      string
}

// Clock abstracts the current time so use cases are deterministic under test.
type Clock interface{ Now() time.Time }

// IDGen mints unique, positive 64-bit ids (pkg/idgen Snowflake in production).
type IDGen interface{ Next() int64 }

// Repo is the durable store (MySQL). Lookups return domain.ErrNotFound when the row is
// missing; Insert returns domain.ErrAlreadyExists when the tenant already has a link
// for the same long_url_hash (the unique index is the real dedup guard under races).
type Repo interface {
	Insert(ctx context.Context, l domain.Link) error
	FindByCode(ctx context.Context, code string) (domain.Link, error)
	FindByURLHash(ctx context.Context, tenantID, longURLHash string) (domain.Link, error)
	ListByTenant(ctx context.Context, tenantID string, limit int) ([]LinkStat, error)
	CountClicks(ctx context.Context, code string) (int64, error)
	DailyClicks(ctx context.Context, code string, since time.Time) ([]DayCount, error)
	RecentClicks(ctx context.Context, code string, limit int) ([]Click, error)
}

// Cache is the code -> Target cache-aside store (Redis). Get reports a miss with
// ok=false and a nil error; an error means the cache itself is unhealthy.
type Cache interface {
	Get(ctx context.Context, code string) (t Target, ok bool, err error)
	Set(ctx context.Context, code string, t Target, ttl time.Duration) error
}

// Counter backs the per-tenant create rate limit (Redis fixed window): Incr sets the
// TTL on the first hit of a window and returns the running count.
type Counter interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// Publisher publishes domain events to Kafka. The topic is passed in (config-driven),
// never hard-coded at the call site.
type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
}
