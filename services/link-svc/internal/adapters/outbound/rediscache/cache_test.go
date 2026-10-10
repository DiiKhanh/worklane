package rediscache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

// newStore backs the adapter with an in-process miniredis, so the real commands (and
// the Lua script) run without a Redis server.
func newStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), MaxRetries: -1}) // no retry backoff when a test stops the server
	t.Cleanup(func() { _ = c.Close() })
	return New(c), mr
}

func TestCache_SetThenGet(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()
	want := app.Target{LongURL: "https://example.com/a?x=1&y=<2>", TenantID: "t1"}

	if err := s.Set(ctx, "abc", want, time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := s.Get(ctx, "abc")
	if err != nil || !ok || got != want {
		t.Fatalf("get: %+v ok=%v err=%v", got, ok, err)
	}
	if ttl := mr.TTL("link:abc"); ttl != time.Hour {
		t.Fatalf("ttl on link:abc = %v, want 1h", ttl)
	}
}

func TestCache_MissIsNotAnError(t *testing.T) {
	s, _ := newStore(t)
	got, ok, err := s.Get(context.Background(), "nope")
	if err != nil || ok || got != (app.Target{}) {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
}

func TestCache_EntryExpires(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()
	if err := s.Set(ctx, "abc", app.Target{LongURL: "https://example.com", TenantID: "t1"}, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	mr.FastForward(time.Minute + time.Second)
	if _, ok, err := s.Get(ctx, "abc"); err != nil || ok {
		t.Fatalf("expired entry: ok=%v err=%v", ok, err)
	}
}

// An undecodable or empty entry must not be served as a redirect target.
func TestCache_CorruptEntryIsAnError(t *testing.T) {
	s, mr := newStore(t)
	for name, raw := range map[string]string{"not json": "https://example.com", "no url": `{"t":"t1"}`} {
		if err := mr.Set("link:abc", raw); err != nil {
			t.Fatalf("%s: seed: %v", name, err)
		}
		if _, ok, err := s.Get(context.Background(), "abc"); err == nil || ok {
			t.Fatalf("%s: want error and ok=false, got ok=%v err=%v", name, ok, err)
		}
	}
}

func TestCache_UnavailableIsAnError(t *testing.T) {
	s, mr := newStore(t)
	mr.Close()
	ctx := context.Background()
	if _, ok, err := s.Get(ctx, "abc"); err == nil || ok {
		t.Fatalf("get: want error, got ok=%v err=%v", ok, err)
	}
	if err := s.Set(ctx, "abc", app.Target{LongURL: "https://example.com"}, time.Hour); err == nil {
		t.Fatal("set: want error")
	}
	if _, err := s.Incr(ctx, "k", time.Minute); err == nil {
		t.Fatal("incr: want error")
	}
}

func TestIncr_FixedWindow(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()
	const key = "link:rl:tenant:t1"

	for want := int64(1); want <= 3; want++ {
		n, err := s.Incr(ctx, key, time.Minute)
		if err != nil || n != want {
			t.Fatalf("incr: got %d, %v; want %d", n, err, want)
		}
		if want == 1 {
			mr.FastForward(20 * time.Second)
		}
	}
	// Later hits must not push the expiry out: 40s of the first window remain.
	if ttl := mr.TTL(key); ttl != 40*time.Second {
		t.Fatalf("ttl = %v, want 40s", ttl)
	}

	mr.FastForward(41 * time.Second)
	if n, err := s.Incr(ctx, key, time.Minute); err != nil || n != 1 {
		t.Fatalf("new window: got %d, %v; want 1", n, err)
	}
}

func TestIncr_KeysAreIndependent(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.Incr(ctx, "link:rl:tenant:t1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Incr(ctx, "link:rl:tenant:t2", time.Minute); err != nil || n != 1 {
		t.Fatalf("got %d, %v; want 1", n, err)
	}
}
