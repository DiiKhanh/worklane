// Package redis provides shared Redis wiring: opening a go-redis client from a URL.
// Infrastructure (pkg) - importable by adapters and composition roots, never by
// domain or app.
package redis

import (
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// Open builds a *redis.Client from a redis:// URL, e.g. "redis://localhost:6379/0".
func Open(url string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}
	return goredis.NewClient(opts), nil
}
