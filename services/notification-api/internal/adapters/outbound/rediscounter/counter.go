// Package rediscounter is notification-api's Redis adapter. It implements app.Counter,
// the fixed-window counter behind the per-tenant and per-user send rate limits.
package rediscounter

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/notification-api/internal/app"
)

// Counter implements app.Counter.
type Counter struct{ c *goredis.Client }

func New(c *goredis.Client) *Counter { return &Counter{c: c} }

var _ app.Counter = (*Counter)(nil)

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
func (s *Counter) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := incrScript.Run(ctx, s.c, []string{key}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis: incr: %w", err)
	}
	return n, nil
}
