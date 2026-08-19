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

func TestRepo_InsertRequest_UpdateState_List(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

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
