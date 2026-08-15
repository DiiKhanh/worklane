package app

import (
	"context"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

type fakeCounter struct {
	counts map[string]int64
	marks  map[string]bool
}

func newFakeCounter() *fakeCounter {
	return &fakeCounter{counts: map[string]int64{}, marks: map[string]bool{}}
}
func (f *fakeCounter) Incr(_ context.Context, k string, _ time.Duration) (int64, error) {
	f.counts[k]++
	return f.counts[k], nil
}
func (f *fakeCounter) Exists(_ context.Context, k string) (bool, error) { return f.marks[k], nil }
func (f *fakeCounter) Set(_ context.Context, k string, _ time.Duration) error {
	f.marks[k] = true
	return nil
}

func TestRateLimiter_PerRecipient(t *testing.T) {
	rl := NewRateLimiter(newFakeCounter(), LimitConfig{
		PerRecipientMax: 3, PerRecipientWindow: time.Hour,
		PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: 0,
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
			t.Fatalf("send %d should pass: %v", i, err)
		}
	}
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != domain.ErrRateLimited {
		t.Fatalf("4th send want ErrRateLimited, got %v", err)
	}
}

func TestRateLimiter_PerTenant(t *testing.T) {
	rl := NewRateLimiter(newFakeCounter(), LimitConfig{
		PerRecipientMax: 100, PerRecipientWindow: time.Hour,
		PerTenantMax: 2, PerTenantWindow: time.Hour, ResendCooldown: 0,
	})
	ctx := context.Background()
	// Two different recipients, same tenant: the tenant quota (2) caps the third.
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
		t.Fatalf("send 1: %v", err)
	}
	if err := rl.CheckAndCount(ctx, "t1", "b@b.co"); err != nil {
		t.Fatalf("send 2: %v", err)
	}
	if err := rl.CheckAndCount(ctx, "t1", "c@b.co"); err != domain.ErrRateLimited {
		t.Fatalf("3rd send want ErrRateLimited (tenant cap), got %v", err)
	}
}

func TestRateLimiter_Cooldown(t *testing.T) {
	rl := NewRateLimiter(newFakeCounter(), LimitConfig{
		PerRecipientMax: 10, PerRecipientWindow: time.Hour,
		PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: time.Minute,
	})
	ctx := context.Background()
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
		t.Fatalf("first send should pass: %v", err)
	}
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != domain.ErrCooldown {
		t.Fatalf("immediate resend want ErrCooldown, got %v", err)
	}
}
