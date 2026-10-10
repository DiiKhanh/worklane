package rediscounter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// newCounter backs the adapter with an in-process miniredis, so the real Lua script
// runs without a Redis server.
func newCounter(t *testing.T) (*Counter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), MaxRetries: -1}) // no retry backoff when a test stops the server
	t.Cleanup(func() { _ = c.Close() })
	return New(c), mr
}

func TestIncr_FixedWindow(t *testing.T) {
	s, mr := newCounter(t)
	ctx := context.Background()
	const key = "notif:rl:tenant:t1"

	for want := int64(1); want <= 3; want++ {
		got, err := s.Incr(ctx, key, time.Minute)
		if err != nil || got != want {
			t.Fatalf("incr = %d, %v; want %d", got, err, want)
		}
		mr.FastForward(10 * time.Second)
	}
	// The TTL was set by the first hit only: three hits 10s apart leave 30s.
	if ttl := mr.TTL(key); ttl != 30*time.Second {
		t.Fatalf("ttl = %v, want 30s (later hits must not extend the window)", ttl)
	}

	mr.FastForward(31 * time.Second)
	got, err := s.Incr(ctx, key, time.Minute)
	if err != nil || got != 1 {
		t.Fatalf("after the window: incr = %d, %v; want 1", got, err)
	}
	if ttl := mr.TTL(key); ttl != time.Minute {
		t.Fatalf("new window ttl = %v, want 1m", ttl)
	}
}

func TestIncr_KeysAreIndependent(t *testing.T) {
	s, _ := newCounter(t)
	ctx := context.Background()
	if _, err := s.Incr(ctx, "notif:rl:tenant:t1", time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := s.Incr(ctx, "notif:rl:user:t1:u1", time.Minute)
	if err != nil || got != 1 {
		t.Fatalf("incr = %d, %v; want 1", got, err)
	}
}

func TestIncr_UnavailableIsAnError(t *testing.T) {
	s, mr := newCounter(t)
	mr.Close()
	if _, err := s.Incr(context.Background(), "k", time.Minute); err == nil {
		t.Fatal("want an error when redis is down")
	}
}
