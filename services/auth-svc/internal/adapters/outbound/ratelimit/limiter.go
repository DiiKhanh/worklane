// Package ratelimit is auth-svc's Redis-backed login throttle: a fixed-window counter that
// implements the app.RateLimiter port.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter is a fixed-window counter: INCR a per-key counter, set the TTL on first hit,
// and reject once it exceeds max. Simple and good enough for login throttling.
type Limiter struct {
	rc     *redis.Client
	max    int
	window time.Duration
}

func New(rc *redis.Client, max int, window time.Duration) *Limiter {
	return &Limiter{rc: rc, max: max, window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	n, err := l.rc.Incr(ctx, "rl:"+key).Result()
	if err != nil {
		return false, fmt.Errorf("ratelimit: incr: %w", err)
	}
	if n == 1 {
		if err := l.rc.Expire(ctx, "rl:"+key, l.window).Err(); err != nil {
			return false, fmt.Errorf("ratelimit: expire: %w", err)
		}
	}
	return n <= int64(l.max), nil
}
