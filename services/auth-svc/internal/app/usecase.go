// Package app is auth-svc's application layer: the Login use case that turns a credential
// pair into a signed access token. It depends on ports (Repo, TokenIssuer, RateLimiter,
// Clock) and pkg/security - never on adapters or pkg/platform.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Deps are the injected ports.
type Deps struct {
	Repo    Repo
	Issuer  TokenIssuer
	Limiter RateLimiter
	Clock   Clock
}

// Service is the auth application layer.
type Service struct{ d Deps }

func New(d Deps) *Service { return &Service{d: d} }

// LoginResult is what a successful login returns.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      domain.User
}

// Login validates credentials and mints a token. Every credential failure (unknown email,
// wrong password) collapses to ErrInvalidCredentials so the API cannot be used to probe
// which emails exist.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	allowed, err := s.d.Limiter.Allow(ctx, "login:"+email)
	if err != nil {
		return LoginResult{}, err
	}
	if !allowed {
		return LoginResult{}, domain.ErrRateLimited
	}

	u, err := s.d.Repo.FindUserByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if u.Status != "active" {
		return LoginResult{}, domain.ErrInactive
	}
	ok, err := security.VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		return LoginResult{}, domain.ErrInvalidCredentials
	}

	tok, exp, err := s.d.Issuer.Issue(u.ID, u.TenantID, u.Email, s.d.Clock.Now())
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: tok, ExpiresAt: exp, User: u}, nil
}

// Introspect hashes the plaintext API key, looks it up, and returns the owning tenant.
// A missing or revoked key is not an error - it returns ("", false, nil).
func (s *Service) Introspect(ctx context.Context, plaintextKey string) (tenantID string, active bool, err error) {
	hash := security.HashKey(plaintextKey)
	k, err := s.d.Repo.FindAPIKeyByHash(ctx, hash)
	if errors.Is(err, domain.ErrAPIKeyNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if k.Status != "active" {
		return "", false, nil
	}
	return k.TenantID, true, nil
}

// CreateAPIKey generates a new API key, stores only its hash, and returns the plaintext
// (shown once) together with the persisted row id.
func (s *Service) CreateAPIKey(ctx context.Context, tenantID string) (plaintext, id string, err error) {
	plain, err := security.GenerateAPIKey()
	if err != nil {
		return "", "", err
	}
	hash := security.HashKey(plain)
	id, err = s.d.Repo.InsertAPIKey(ctx, tenantID, hash)
	if err != nil {
		return "", "", err
	}
	return plain, id, nil
}

// ListAPIKeys returns all API keys for a tenant.
func (s *Service) ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error) {
	return s.d.Repo.ListAPIKeys(ctx, tenantID)
}

// RevokeAPIKey marks an API key as revoked.
func (s *Service) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	return s.d.Repo.RevokeAPIKey(ctx, tenantID, id)
}
