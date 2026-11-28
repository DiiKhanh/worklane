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

// incrAttemptsScript bumps Attempts inside the stored JSON in one atomic step. Running
// it server-side closes two races a client-side GET+SET would have: concurrent wrong
// guesses cannot overwrite each other's count, and a key that expires between the read
// and the write is not recreated. KEEPTTL (Redis 6+) preserves the remaining lifetime,
// so the code still expires at issuance + TTL.
var incrAttemptsScript = goredis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return false
end
local rec = cjson.decode(raw)
rec.Attempts = (rec.Attempts or 0) + 1
redis.call('SET', KEYS[1], cjson.encode(rec), 'KEEPTTL')
return rec.Attempts
`)

// IncrAttempts records one failed verify attempt without extending the code's TTL.
// A missing (expired or deleted) key yields domain.ErrNotFound.
func (s *Store) IncrAttempts(ctx context.Context, key string) (int, error) {
	n, err := incrAttemptsScript.Run(ctx, s.c, []string{key}).Int()
	if errors.Is(err, goredis.Nil) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("redis: incr attempts: %w", err)
	}
	return n, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.c.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis: delete: %w", err)
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
