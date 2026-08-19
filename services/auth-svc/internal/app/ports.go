package app

import (
	"context"
	"time"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Repo reads and writes identity rows (users and api keys).
type Repo interface {
	FindUserByEmail(ctx context.Context, email string) (domain.User, error)
	FindAPIKeyByHash(ctx context.Context, hashedKey string) (domain.APIKey, error)
	InsertAPIKey(ctx context.Context, tenantID, hashedKey string) (id string, err error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, tenantID, id string) error
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
