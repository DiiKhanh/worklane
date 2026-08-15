package app

import (
	"context"
	"fmt"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// LimitConfig holds the four-layer send limits. Windows and the cooldown are
// durations; the *Max fields are counts within their window.
type LimitConfig struct {
	PerRecipientMax    int
	PerRecipientWindow time.Duration
	PerTenantMax       int
	PerTenantWindow    time.Duration
	ResendCooldown     time.Duration
}

// RateLimiter enforces per-recipient, per-tenant, and resend-cooldown limits over a
// Counter (Redis in production, a fake in tests). It is application-layer logic, not
// pure domain, because it orchestrates the Counter port.
type RateLimiter struct {
	c   Counter
	cfg LimitConfig
}

func NewRateLimiter(c Counter, cfg LimitConfig) *RateLimiter { return &RateLimiter{c: c, cfg: cfg} }

// CheckAndCount enforces cooldown, then the per-recipient and per-tenant quotas, and
// finally records the send by setting the cooldown marker.
//
// Order matters: cooldown is checked first so a blocked resend does not consume the
// recipient/tenant quota (otherwise a client spamming resends would exhaust its own
// hourly allowance while every call is rejected anyway). Counters are incremented with
// their window TTL so the first increment in a window sets expiry and the count rolls
// off automatically.
func (rl *RateLimiter) CheckAndCount(ctx context.Context, tenantID, recipient string) error {
	cdKey := fmt.Sprintf("otp:cd:%s:%s", tenantID, recipient)
	if rl.cfg.ResendCooldown > 0 {
		exists, err := rl.c.Exists(ctx, cdKey)
		if err != nil {
			return err
		}
		if exists {
			return domain.ErrCooldown
		}
	}

	rKey := fmt.Sprintf("otp:rl:rcpt:%s:%s", tenantID, recipient)
	n, err := rl.c.Incr(ctx, rKey, rl.cfg.PerRecipientWindow)
	if err != nil {
		return err
	}
	if int(n) > rl.cfg.PerRecipientMax {
		return domain.ErrRateLimited
	}

	tKey := fmt.Sprintf("otp:rl:tenant:%s", tenantID)
	tn, err := rl.c.Incr(ctx, tKey, rl.cfg.PerTenantWindow)
	if err != nil {
		return err
	}
	if int(tn) > rl.cfg.PerTenantMax {
		return domain.ErrRateLimited
	}

	if rl.cfg.ResendCooldown > 0 {
		if err := rl.c.Set(ctx, cdKey, rl.cfg.ResendCooldown); err != nil {
			return err
		}
	}
	return nil
}
