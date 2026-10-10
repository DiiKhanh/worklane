// Package rediscache is link-svc's Redis adapter. It implements two ports on one Redis
// client: app.Cache (the code -> target cache-aside entry behind the redirect) and
// app.Counter (the per-tenant create rate limit).
package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

// keyPrefix namespaces cache entries: link:<code>. Codes are base62, so an entry can
// never collide with the colon-separated rate-limit keys (link:rl:...).
const keyPrefix = "link:"

// Store implements app.Cache and app.Counter.
type Store struct{ c *goredis.Client }

func New(c *goredis.Client) *Store { return &Store{c: c} }

var (
	_ app.Cache   = (*Store)(nil)
	_ app.Counter = (*Store)(nil)
)

// entry is the cached JSON value. It carries the tenant as well as the URL so a cache
// hit can publish link.clicked without a DB read. Short field names keep the hot keys
// small.
type entry struct {
	LongURL  string `json:"u"`
	TenantID string `json:"t"`
}

// --- app.Cache ---

// Get returns the cached target. A missing key is a miss (ok=false, nil error), so the
// application layer never has to know about redis.Nil. An entry that cannot be decoded
// is an error: the caller falls back to the DB and overwrites it.
func (s *Store) Get(ctx context.Context, code string) (app.Target, bool, error) {
	b, err := s.c.Get(ctx, keyPrefix+code).Bytes()
	if errors.Is(err, goredis.Nil) {
		return app.Target{}, false, nil
	}
	if err != nil {
		return app.Target{}, false, fmt.Errorf("redis: get: %w", err)
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		return app.Target{}, false, fmt.Errorf("redis: unmarshal target: %w", err)
	}
	if e.LongURL == "" {
		return app.Target{}, false, fmt.Errorf("redis: cached target for %s has no url", code)
	}
	return app.Target{LongURL: e.LongURL, TenantID: e.TenantID}, true, nil
}

// Set caches the target with an expiry, so a rarely used code falls out of memory on
// its own.
func (s *Store) Set(ctx context.Context, code string, t app.Target, ttl time.Duration) error {
	b, err := json.Marshal(entry{LongURL: t.LongURL, TenantID: t.TenantID})
	if err != nil {
		return fmt.Errorf("redis: marshal target: %w", err)
	}
	if err := s.c.Set(ctx, keyPrefix+code, b, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set: %w", err)
	}
	return nil
}

// --- app.Counter ---

// incrScript increments the counter and sets the TTL on the first hit of a window, as
// one atomic step. Doing INCR and EXPIRE as two commands would leave a counter with no
// expiry if the process died in between, rate-limiting that tenant forever.
var incrScript = goredis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// Incr increments the counter and returns the running count. Setting the expiry only
// when the value becomes 1 gives a fixed window: it starts at the first request and the
// whole counter rolls off together, instead of each Incr pushing the expiry further out.
func (s *Store) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := incrScript.Run(ctx, s.c, []string{key}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis: incr: %w", err)
	}
	return n, nil
}
