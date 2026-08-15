//go:build integration

package mysqlrepo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/otp-api/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// Compile-time proof the adapter satisfies the port it is meant to implement.
var _ app.Repo = (*mysqlrepo.Repo)(nil)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}

// newRepo spins up a throwaway MySQL, migrates it, and returns a live Repo plus the raw
// gorm handle so tests can seed rows directly (keeping seed helpers out of production).
func newRepo(t *testing.T) (*mysqlrepo.Repo, *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("otp"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("secret"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if err := mysql.Migrate(dsn, filepath.Join(repoRoot(t), "db", "otp", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return mysqlrepo.New(db), db
}

func seedTenantAndKey(t *testing.T, db *gorm.DB, tenantID, hashedKey string) {
	t.Helper()
	if err := db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Demo").Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO api_keys (id, tenant_id, hashed_key, status) VALUES (?, ?, ?, 'active')",
		"key-"+tenantID, tenantID, hashedKey,
	).Error; err != nil {
		t.Fatalf("seed api key: %v", err)
	}
}

func TestRepo_FindAPIKey(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	seedTenantAndKey(t, db, "ten-1", "key-hash-abc")

	got, err := repo.FindAPIKey(ctx, "key-hash-abc")
	if err != nil {
		t.Fatalf("find api key: %v", err)
	}
	if got.TenantID != "ten-1" || got.Status != "active" {
		t.Fatalf("unexpected api key: %+v", got)
	}
	if _, err := repo.FindAPIKey(ctx, "does-not-exist"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestRepo_InsertRequest_UpdateState_List(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	seedTenantAndKey(t, db, "ten-1", "k")

	req := app.Request{
		ID: "req-1", TenantID: "ten-1", Recipient: "duykhanh@gmail.com",
		Channel: "email", State: "requested", CreatedAt: time.Now(),
	}
	if err := repo.InsertRequest(ctx, req); err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if err := repo.UpdateState(ctx, "req-1", "verified"); err != nil {
		t.Fatalf("update state: %v", err)
	}
	list, err := repo.ListRequests(ctx, "ten-1", 10)
	if err != nil {
		t.Fatalf("list requests: %v", err)
	}
	if len(list) != 1 || list[0].State != "verified" {
		t.Fatalf("want 1 verified request, got %+v", list)
	}
	// PII must be masked at rest.
	if list[0].Recipient != "d***@gmail.com" {
		t.Fatalf("recipient must be stored masked, got %q", list[0].Recipient)
	}
}
