//go:build integration

package mysqlrepo_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/auth-svc/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Compile-time proof the adapter satisfies the port.
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

// newRepo spins up a throwaway MySQL, migrates the shared otp schema (which includes the
// users table), and returns a live Repo plus the gorm handle to seed rows directly.
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

func seedUser(t *testing.T, db *gorm.DB, u domain.User) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO users (id, tenant_id, email, password_hash, status) VALUES (?, ?, ?, ?, ?)",
		u.ID, u.TenantID, u.Email, u.PasswordHash, u.Status,
	).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestFindUserByEmail_NotFound(t *testing.T) {
	repo, _ := newRepo(t)
	_, err := repo.FindUserByEmail(context.Background(), "nobody@x.co")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestFindUserByEmail_Found(t *testing.T) {
	repo, db := newRepo(t)
	seedUser(t, db, domain.User{ID: "u1", TenantID: "t1", Email: "a@b.co", PasswordHash: "h", Status: "active"})
	u, err := repo.FindUserByEmail(context.Background(), "a@b.co")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if u.ID != "u1" || u.TenantID != "t1" || u.PasswordHash != "h" {
		t.Fatalf("row wrong: %+v", u)
	}
}
