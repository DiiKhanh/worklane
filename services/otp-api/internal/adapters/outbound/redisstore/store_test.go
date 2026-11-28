//go:build integration

package redisstore_test

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	platformredis "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/services/otp-api/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// Compile-time proof the adapter satisfies both ports it must implement.
var (
	_ app.CodeStore = (*redisstore.Store)(nil)
	_ app.Counter   = (*redisstore.Store)(nil)
)

// newStore returns a live Store plus the raw client so tests can assert low-level
// details (like TTL) without adding test-only methods to the production adapter.
func newStore(t *testing.T) (*redisstore.Store, *goredis.Client) {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("start redis container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	client, err := platformredis.Open(url)
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	return redisstore.New(client), client
}

func TestStore_SaveGetConsume(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()

	const key = "otp:code:t1:a@b.co"
	rec := app.CodeRecord{Hash: "h", Salt: "req-1", Attempts: 2}
	if err := s.Save(ctx, key, rec, time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != rec {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, rec)
	}
	if err := s.Consume(ctx, key, "superseded-request"); err != domain.ErrNotFound {
		t.Fatalf("consume superseded code: got %v, want ErrNotFound", err)
	}

	const consumers = 2
	results := make(chan error, consumers)
	for range consumers {
		go func() { results <- s.Consume(ctx, key, rec.Salt) }()
	}
	consumed := 0
	for range consumers {
		switch err := <-results; err {
		case nil:
			consumed++
		case domain.ErrNotFound:
		default:
			t.Errorf("consume current code: %v", err)
		}
	}
	if consumed != 1 {
		t.Fatalf("successful consumers = %d, want 1", consumed)
	}
	if _, err := s.Get(ctx, key); err != domain.ErrNotFound {
		t.Fatalf("get after consume: got %v, want ErrNotFound", err)
	}
}

func TestStore_ReserveAttempt_KeepsExpiryFixed(t *testing.T) {
	s, client := newStore(t)
	ctx := context.Background()
	const key = "otp:code:t1:a@b.co"
	rec := app.CodeRecord{Hash: "h", Salt: "req-1"}
	if err := s.Save(ctx, key, rec, 5*time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	before := client.PExpireTime(ctx, key).Val()
	// Let time pass so a TTL reset would land on a visibly later deadline.
	time.Sleep(50 * time.Millisecond)

	reserved, err := s.ReserveAttempt(ctx, key, 3)
	if err != nil {
		t.Fatalf("reserve attempt: %v", err)
	}
	rec.Attempts = 1
	if reserved != rec {
		t.Fatalf("reserved record = %+v, want %+v", reserved, rec)
	}
	if after := client.PExpireTime(ctx, key).Val(); after != before {
		t.Fatalf("expiry moved from %v to %v; attempts must not extend the code's lifetime", before, after)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != rec {
		t.Fatalf("stored record after reservation = %+v, want %+v", got, rec)
	}
}

func TestStore_ReserveAttempt_ExpiredKeyIsNotRecreated(t *testing.T) {
	s, client := newStore(t)
	ctx := context.Background()
	const key = "otp:code:t1:a@b.co"
	if err := s.Save(ctx, key, app.CodeRecord{Hash: "h", Salt: "req-1"}, 50*time.Millisecond); err != nil {
		t.Fatalf("save: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if _, err := s.ReserveAttempt(ctx, key, 3); err != domain.ErrNotFound {
		t.Fatalf("reserve attempt on expired code: got %v, want ErrNotFound", err)
	}
	if n := client.Exists(ctx, key).Val(); n != 0 {
		t.Fatal("reservation recreated the expired code")
	}
}

func TestStore_ReserveAttempt_ConcurrentSlotsAreUnique(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	const key = "otp:code:t1:a@b.co"
	if err := s.Save(ctx, key, app.CodeRecord{Hash: "h", Salt: "req-1"}, time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	const attempts = 20
	counts := make(chan int, attempts)
	for range attempts {
		go func() {
			rec, err := s.ReserveAttempt(ctx, key, attempts)
			if err != nil {
				t.Errorf("reserve attempt: %v", err)
			}
			counts <- rec.Attempts
		}()
	}
	seen := make(map[int]bool, attempts)
	for range attempts {
		count := <-counts
		if count < 1 || count > attempts || seen[count] {
			t.Errorf("invalid or duplicate reserved attempt number %d", count)
		}
		seen[count] = true
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Attempts != attempts {
		t.Fatalf("concurrent attempts lost updates: got %d want %d", got.Attempts, attempts)
	}
}

func TestStore_GetMissing_IsNotFound(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Get(context.Background(), "nope"); err != domain.ErrNotFound {
		t.Fatalf("missing key want ErrNotFound, got %v", err)
	}
}

func TestStore_IncrAndTTL(t *testing.T) {
	s, client := newStore(t)
	ctx := context.Background()
	n1, err := s.Incr(ctx, "otp:rl:rcpt:t1:a@b.co", time.Hour)
	if err != nil {
		t.Fatalf("incr: %v", err)
	}
	n2, _ := s.Incr(ctx, "otp:rl:rcpt:t1:a@b.co", time.Hour)
	if n1 != 1 || n2 != 2 {
		t.Fatalf("incr should count 1,2 got %d,%d", n1, n2)
	}
	// A counter with a window must not live forever, or limits would never reset.
	ttl := client.TTL(ctx, "otp:rl:rcpt:t1:a@b.co").Val()
	if ttl <= 0 {
		t.Fatalf("first incr must set a positive TTL, got %v", ttl)
	}
}

func TestStore_ExistsAndSet(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	ok, _ := s.Exists(ctx, "otp:cd:t1:a@b.co")
	if ok {
		t.Fatal("cooldown marker should not exist yet")
	}
	if err := s.Set(ctx, "otp:cd:t1:a@b.co", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	ok, _ = s.Exists(ctx, "otp:cd:t1:a@b.co")
	if !ok {
		t.Fatal("cooldown marker should exist after Set")
	}
}
