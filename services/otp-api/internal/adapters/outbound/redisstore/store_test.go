//go:build integration

package redisstore_test

import (
	"context"
	"sync"
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

func TestStore_SaveGetDelete(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()

	rec := app.CodeRecord{Hash: "h", Salt: "req-1", Attempts: 2}
	if err := s.Save(ctx, "otp:code:t1:a@b.co", rec, time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Get(ctx, "otp:code:t1:a@b.co")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != rec {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, rec)
	}
	if err := s.Delete(ctx, "otp:code:t1:a@b.co"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, "otp:code:t1:a@b.co"); err != domain.ErrNotFound {
		t.Fatalf("get after delete want ErrNotFound, got %v", err)
	}
}

// A wrong guess must not extend a code's lifetime: the absolute expiry set at issuance
// stays exactly where it was after the attempt is recorded.
func TestStore_IncrAttempts_KeepsExpiryFixed(t *testing.T) {
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

	n, err := s.IncrAttempts(ctx, key)
	if err != nil {
		t.Fatalf("incr attempts: %v", err)
	}
	if n != 1 {
		t.Fatalf("first failed attempt want count 1, got %d", n)
	}
	if after := client.PExpireTime(ctx, key).Val(); after != before {
		t.Fatalf("expiry moved from %v to %v; attempts must not extend the code's lifetime", before, after)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if want := (app.CodeRecord{Hash: "h", Salt: "req-1", Attempts: 1}); got != want {
		t.Fatalf("record after incr = %+v, want %+v", got, want)
	}
}

func TestStore_IncrAttempts_ExpiredKeyIsNotRecreated(t *testing.T) {
	s, client := newStore(t)
	ctx := context.Background()
	const key = "otp:code:t1:a@b.co"
	if err := s.Save(ctx, key, app.CodeRecord{Hash: "h", Salt: "req-1"}, 50*time.Millisecond); err != nil {
		t.Fatalf("save: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if _, err := s.IncrAttempts(ctx, key); err != domain.ErrNotFound {
		t.Fatalf("incr on expired code want ErrNotFound, got %v", err)
	}
	if n := client.Exists(ctx, key).Val(); n != 0 {
		t.Fatal("incr must not resurrect an expired code")
	}
}

func TestStore_IncrAttempts_ConcurrentAttemptsAllCount(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	const key = "otp:code:t1:a@b.co"
	if err := s.Save(ctx, key, app.CodeRecord{Hash: "h", Salt: "req-1"}, time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	const attempts = 20
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.IncrAttempts(ctx, key); err != nil {
				t.Errorf("incr attempts: %v", err)
			}
		}()
	}
	wg.Wait()
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
