package mysqlrepo

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	return &Repo{db: db}
}

func withAPIKeys(t *testing.T, repo *Repo) *Repo {
	t.Helper()
	sql := repo.db.Exec(`CREATE TABLE api_keys (
		id         VARCHAR(64) PRIMARY KEY,
		tenant_id  VARCHAR(64) NOT NULL,
		hashed_key VARCHAR(64) NOT NULL UNIQUE,
		status     VARCHAR(16) NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if sql.Error != nil {
		t.Fatalf("create api_keys table: %v", sql.Error)
	}
	return repo
}

func TestInsertAndFindAPIKeyByHash(t *testing.T) {
	repo := withAPIKeys(t, newTestRepo(t))
	ctx := context.Background()

	id, err := repo.InsertAPIKey(ctx, "tenant1", "hash_abc")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	key, err := repo.FindAPIKeyByHash(ctx, "hash_abc")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if key.ID != id {
		t.Fatalf("id mismatch: got %q, want %q", key.ID, id)
	}
	if key.TenantID != "tenant1" {
		t.Fatalf("tenant mismatch: got %q", key.TenantID)
	}
	if key.Status != "active" {
		t.Fatalf("status: got %q, want active", key.Status)
	}
}

func TestFindAPIKeyByHash_NotFound(t *testing.T) {
	repo := withAPIKeys(t, newTestRepo(t))
	_, err := repo.FindAPIKeyByHash(context.Background(), "no_such_hash")
	if !errors.Is(err, domain.ErrAPIKeyNotFound) {
		t.Fatalf("want ErrAPIKeyNotFound, got %v", err)
	}
}

func TestListAPIKeys_TenantScoped(t *testing.T) {
	repo := withAPIKeys(t, newTestRepo(t))
	ctx := context.Background()

	if _, err := repo.InsertAPIKey(ctx, "t1", "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertAPIKey(ctx, "t1", "h2"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertAPIKey(ctx, "t2", "h3"); err != nil {
		t.Fatal(err)
	}

	keys, err := repo.ListAPIKeys(ctx, "t1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("want 2 keys for t1, got %d", len(keys))
	}
	for _, k := range keys {
		if k.TenantID != "t1" {
			t.Fatalf("leaked key from tenant %q", k.TenantID)
		}
	}
}

func TestRevokeAPIKey_TenantScoped(t *testing.T) {
	repo := withAPIKeys(t, newTestRepo(t))
	ctx := context.Background()

	id, err := repo.InsertAPIKey(ctx, "t1", "h1")
	if err != nil {
		t.Fatal(err)
	}
	// Insert for another tenant.
	otherID, err := repo.InsertAPIKey(ctx, "t2", "h2")
	if err != nil {
		t.Fatal(err)
	}

	// Revoking with wrong tenant must fail.
	if err := repo.RevokeAPIKey(ctx, "t2", id); !errors.Is(err, domain.ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant revoke: want ErrAPIKeyNotFound, got %v", err)
	}

	// Revoking with correct tenant must succeed.
	if err := repo.RevokeAPIKey(ctx, "t1", id); err != nil {
		t.Fatalf("revoke own key: %v", err)
	}

	// Verify status changed.
	key, err := repo.FindAPIKeyByHash(ctx, "h1")
	if err != nil {
		t.Fatalf("find after revoke: %v", err)
	}
	if key.Status != "revoked" {
		t.Fatalf("status: got %q, want revoked", key.Status)
	}

	// Other tenant's key untouched.
	other, err := repo.FindAPIKeyByHash(ctx, "h2")
	if err != nil {
		t.Fatalf("find other: %v", err)
	}
	if other.ID != otherID || other.Status != "active" {
		t.Fatalf("other key modified: %+v", other)
	}
}
