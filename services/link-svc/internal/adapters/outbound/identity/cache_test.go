package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/identity"
)

// countingIntrospector records how many times the inner introspector is hit.
type countingIntrospector struct {
	tenant string
	calls  int
}

func (c *countingIntrospector) Introspect(_ context.Context, _ string) (string, error) {
	c.calls++
	return c.tenant, nil
}

func TestCached_HitsInnerOnceThenCaches(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	inner := &countingIntrospector{tenant: "t1"}
	cached := identity.NewCached(inner, rc, 5*time.Minute)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		tenant, err := cached.Introspect(ctx, "same-key")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if tenant != "t1" {
			t.Fatalf("call %d: want t1, got %q", i, tenant)
		}
	}

	if inner.calls != 1 {
		t.Fatalf("inner called %d times, want 1 (cache should absorb the rest)", inner.calls)
	}
}
