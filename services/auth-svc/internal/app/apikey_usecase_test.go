package app

import (
	"context"
	"errors"
	"testing"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// keyRepo is a fake Repo for API key use case tests.
type keyRepo struct {
	keys    map[string]domain.APIKey // keyed by hashed_key
	byID    map[string]domain.APIKey // keyed by id
	lastKey string                   // last inserted hashed key
}

func newKeyRepo() *keyRepo {
	return &keyRepo{
		keys: make(map[string]domain.APIKey),
		byID: make(map[string]domain.APIKey),
	}
}

func (r *keyRepo) FindUserByEmail(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}

func (r *keyRepo) FindAPIKeyByHash(_ context.Context, hash string) (domain.APIKey, error) {
	k, ok := r.keys[hash]
	if !ok {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	return k, nil
}

func (r *keyRepo) InsertAPIKey(_ context.Context, tenantID, hashedKey string) (string, error) {
	id := "gen-" + hashedKey[:8]
	k := domain.APIKey{ID: id, TenantID: tenantID, Status: "active"}
	r.keys[hashedKey] = k
	r.byID[id] = k
	r.lastKey = hashedKey
	return id, nil
}

func (r *keyRepo) ListAPIKeys(_ context.Context, tenantID string) ([]domain.APIKey, error) {
	var out []domain.APIKey
	for _, k := range r.keys {
		if k.TenantID == tenantID {
			out = append(out, k)
		}
	}
	return out, nil
}

func (r *keyRepo) RevokeAPIKey(_ context.Context, tenantID, id string) error {
	k, ok := r.byID[id]
	if !ok || k.TenantID != tenantID {
		return domain.ErrAPIKeyNotFound
	}
	k.Status = "revoked"
	r.byID[id] = k
	// Update the hash-keyed map too.
	for h, v := range r.keys {
		if v.ID == id {
			r.keys[h] = k
		}
	}
	return nil
}

func keySvc(repo Repo) *Service {
	return New(Deps{Repo: repo, Issuer: fakeIssuer{}, Limiter: allowLimiter{ok: true}, Clock: fixedClock{}})
}

func TestIntrospect_ActiveKey(t *testing.T) {
	repo := newKeyRepo()
	svc := keySvc(repo)

	plain, _, err := svc.CreateAPIKey(context.Background(), "t1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tid, active, err := svc.Introspect(context.Background(), plain)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if !active {
		t.Fatal("expected active=true")
	}
	if tid != "t1" {
		t.Fatalf("tenant: got %q, want t1", tid)
	}
}

func TestIntrospect_UnknownKey(t *testing.T) {
	svc := keySvc(newKeyRepo())

	tid, active, err := svc.Introspect(context.Background(), "no-such-key")
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if active {
		t.Fatal("expected active=false for unknown key")
	}
	if tid != "" {
		t.Fatalf("tenant should be empty, got %q", tid)
	}
}

func TestIntrospect_RevokedKey(t *testing.T) {
	repo := newKeyRepo()
	svc := keySvc(repo)

	plain, id, err := svc.CreateAPIKey(context.Background(), "t1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.RevokeAPIKey(context.Background(), "t1", id); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, active, err := svc.Introspect(context.Background(), plain)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if active {
		t.Fatal("expected active=false for revoked key")
	}
}

func TestCreateAPIKey_ReturnsPlaintextAndStoresHash(t *testing.T) {
	repo := newKeyRepo()
	svc := keySvc(repo)

	plain, id, err := svc.CreateAPIKey(context.Background(), "t1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if plain == "" {
		t.Fatal("plaintext must not be empty")
	}
	if id == "" {
		t.Fatal("id must not be empty")
	}

	// The repo must hold the hash, not the plaintext.
	hash := security.HashKey(plain)
	if repo.lastKey != hash {
		t.Fatalf("stored hash %q != HashKey(plain) %q", repo.lastKey, hash)
	}
	if _, ok := repo.keys[plain]; ok {
		t.Fatal("repo must not store plaintext as key")
	}
}

func TestRevokeAPIKey_Delegates(t *testing.T) {
	repo := newKeyRepo()
	svc := keySvc(repo)

	_, id, err := svc.CreateAPIKey(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RevokeAPIKey(context.Background(), "t1", id); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Revoking a non-existent key returns the sentinel.
	err = svc.RevokeAPIKey(context.Background(), "t1", "no-such-id")
	if !errors.Is(err, domain.ErrAPIKeyNotFound) {
		t.Fatalf("want ErrAPIKeyNotFound, got %v", err)
	}
}
