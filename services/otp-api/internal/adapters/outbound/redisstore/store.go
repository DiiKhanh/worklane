// Package redisstore is otp-api's Redis adapter. It implements two ports on one Redis
// client: app.CodeStore (the active OTP hash record) and app.Counter (rate-limit,
// cooldown, and idempotency markers).
package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// Store implements app.CodeStore and app.Counter.
type Store struct{ c *goredis.Client }

func New(c *goredis.Client) *Store { return &Store{c: c} }

// --- app.CodeStore ---

// Save stores the CodeRecord as JSON with an expiry. Storing JSON (not the raw code)
// keeps hash+salt+attempts together under one atomic SET, and the TTL means an
// unverified code disappears on its own.
func (s *Store) Save(ctx context.Context, key string, rec app.CodeRecord, ttl time.Duration) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("redis: marshal record: %w", err)
	}
	if err := s.c.Set(ctx, key, b, ttl).Err(); err != nil {
		return fmt.Errorf("redis: save: %w", err)
	}
	return nil
}

// Get returns the record, translating a missing key into domain.ErrNotFound so the
// application layer never has to know about redis.Nil.
func (s *Store) Get(ctx context.Context, key string) (app.CodeRecord, error) {
	b, err := s.c.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return app.CodeRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.CodeRecord{}, fmt.Errorf("redis: get: %w", err)
	}
	var rec app.CodeRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return app.CodeRecord{}, fmt.Errorf("redis: unmarshal record: %w", err)
	}
	return rec, nil
}

// The cap check, increment, and read of the hash share one Redis operation. This
// binds each slot to the same issuance as the hash and preserves its original expiry.
var reserveAttemptScript = goredis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return false
end
local rec = cjson.decode(raw)
local attempts = rec.Attempts or 0
if attempts >= tonumber(ARGV[1]) then
  return ''
end
rec.Attempts = attempts + 1
local updated = cjson.encode(rec)
redis.call('SET', KEYS[1], updated, 'KEEPTTL')
return updated
`)

func (s *Store) ReserveAttempt(ctx context.Context, key string, maxAttempts int) (app.CodeRecord, error) {
	raw, err := reserveAttemptScript.Run(ctx, s.c, []string{key}, maxAttempts).Text()
	if errors.Is(err, goredis.Nil) {
		return app.CodeRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.CodeRecord{}, fmt.Errorf("redis: reserve attempt: %w", err)
	}
	if raw == "" {
		return app.CodeRecord{}, domain.ErrTooManyAttempts
	}
	var rec app.CodeRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return app.CodeRecord{}, fmt.Errorf("redis: unmarshal reserved record: %w", err)
	}
	return rec, nil
}

var consumeScript = goredis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw or cjson.decode(raw).Salt ~= ARGV[1] then
  return 0
end
return redis.call('DEL', KEYS[1])
`)

func (s *Store) Consume(ctx context.Context, key, requestID string) error {
	deleted, err := consumeScript.Run(ctx, s.c, []string{key}, requestID).Int()
	if err != nil {
		return fmt.Errorf("redis: consume code: %w", err)
	}
	if deleted == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- app.Counter ---

// Incr increments the counter and, on the first hit in a window, sets the TTL. Setting
// the expiry only when the value becomes 1 gives a fixed rolling window: the window
// starts at the first request and the whole counter rolls off together, instead of
// each Incr pushing the expiry further out.
func (s *Store) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := s.c.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: incr: %w", err)
	}
	if n == 1 {
		if err := s.c.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, fmt.Errorf("redis: expire: %w", err)
		}
	}
	return n, nil
}

func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	n, err := s.c.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis: exists: %w", err)
	}
	return n > 0, nil
}

// Set writes a presence marker (value "1") with a TTL - used for cooldown and
// idempotency keys where only existence matters.
func (s *Store) Set(ctx context.Context, key string, ttl time.Duration) error {
	if err := s.c.Set(ctx, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("redis: set: %w", err)
	}
	return nil
}
