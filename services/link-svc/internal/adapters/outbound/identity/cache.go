package identity

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

var _ app.Introspector = (*Cached)(nil)

// Cached wraps an Introspector with a Redis cache. Successful introspections
// are cached by the hashed API key; errors bypass the cache entirely.
type Cached struct {
	inner app.Introspector
	rc    *redis.Client
	ttl   time.Duration
}

// NewCached returns a caching wrapper around inner.
func NewCached(inner app.Introspector, rc *redis.Client, ttl time.Duration) *Cached {
	return &Cached{inner: inner, rc: rc, ttl: ttl}
}

// Introspect checks Redis first; on miss it calls the inner introspector and
// caches a successful result (best-effort).
func (c *Cached) Introspect(ctx context.Context, apiKey string) (string, error) {
	cacheKey := "introspect:" + security.HashKey(apiKey)

	// Cache hit?
	if tenant, err := c.rc.Get(ctx, cacheKey).Result(); err == nil {
		return tenant, nil
	}

	tenant, err := c.inner.Introspect(ctx, apiKey)
	if err != nil {
		return "", err
	}

	// Best-effort cache write; do not fail the request on Redis errors.
	_ = c.rc.Set(ctx, cacheKey, tenant, c.ttl).Err()
	return tenant, nil
}
