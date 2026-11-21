package redisstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

func TestMappingHitsAndLateWritesPreserveAbsoluteExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	start := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	server.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, err := redisstore.New(client, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mapping := app.CachedMapping{Link: domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com/"}, ValidUntil: start.Add(time.Minute)}
	if err := cache.Store(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	server.FastForward(59 * time.Second)
	server.SetTime(start.Add(59 * time.Second))
	for range 3 {
		got, err := cache.Load(ctx, "G")
		if err != nil || !got.ValidUntil.Equal(mapping.ValidUntil) {
			t.Fatalf("cache hit extended validity: %v", err)
		}
	}
	// A late refill at second 59 still has only one second of Redis lifetime.
	if err := cache.Store(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	if ttl := client.PTTL(ctx, "link:mapping:G").Val(); ttl != time.Second {
		t.Fatalf("late refill reset the TTL: %s", ttl)
	}
	server.FastForward(time.Second)
	server.SetTime(start.Add(time.Minute))
	if err := cache.Store(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(ctx, "G"); err != redis.Nil {
		t.Fatalf("write at the original deadline resurrected expired state: %v", err)
	}
}
