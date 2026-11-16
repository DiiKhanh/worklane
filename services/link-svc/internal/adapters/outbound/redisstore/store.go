package redisstore

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

var (
	_ app.Quota = (*Store)(nil)
	_ app.Cache = (*Store)(nil)
)

type Store struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func New(client *redis.Client, limit int, window time.Duration) (*Store, error) {
	if limit < 1 || window < time.Millisecond {
		return nil, errors.New("link: positive quota and window required")
	}
	return &Store{client: client, limit: limit, window: window}, nil
}

// A fixed window begins with its first request. Increment and expiration are
// atomic so a process failure cannot leave a counter with no expiry. Once full,
// the counter stays bounded and rejected requests do not extend the window.
var take = redis.NewScript(`
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
local ttl = redis.call('PTTL', KEYS[1])
if count >= tonumber(ARGV[1]) and ttl > 0 then return ttl end
if count == 0 or ttl <= 0 then
  redis.call('SET', KEYS[1], 1, 'PX', ARGV[2])
else
  redis.call('INCR', KEYS[1])
end
return 0
`)

func (s *Store) Take(ctx context.Context, tenant string) (time.Duration, error) {
	millis, err := take.Run(ctx, s.client, []string{"link:create:" + tenant}, s.limit, s.window.Milliseconds()).Int64()
	return time.Duration(millis) * time.Millisecond, err
}

func (s *Store) Evict(ctx context.Context, code string) error {
	return s.client.Del(ctx, "link:mapping:"+code).Err()
}
