package app

import (
	"context"
	"time"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Repo reads identity rows.
type Repo interface {
	FindUserByEmail(ctx context.Context, email string) (domain.User, error)
}

// TokenIssuer mints a signed access token. Implemented by *security.Issuer.
type TokenIssuer interface {
	Issue(userID, tenantID, email string, now time.Time) (token string, expiresAt time.Time, err error)
}

// RateLimiter caps login attempts. Allow returns false when the caller is over budget.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// Clock abstracts time for testable expiry.
type Clock interface{ Now() time.Time }
