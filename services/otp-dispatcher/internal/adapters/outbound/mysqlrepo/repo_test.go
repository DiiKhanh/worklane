//go:build integration

package mysqlrepo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

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
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func newRepo(t *testing.T) (*mysqlrepo.Repo, *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("otp"), tcmysql.WithUsername("root"), tcmysql.WithPassword("secret"))
	if err != nil {
		t.Fatalf("start mysql: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })
	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	if err := mysql.Migrate(dsn, filepath.Join(repoRoot(t), "db", "otp", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return mysqlrepo.New(db), db
}

func TestRepo_UpdateStateAndInsertDeliveryLog(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()

	// Seed a request row for UpdateState to act on.
	if err := db.Exec("INSERT INTO tenants (id, name) VALUES ('t1','Demo')").Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO otp_requests (id, tenant_id, recipient_masked, channel, state) VALUES ('r1','t1','d***@x.co','email','requested')",
	).Error; err != nil {
		t.Fatalf("seed request: %v", err)
	}

	if err := repo.UpdateState(ctx, "r1", "sent"); err != nil {
		t.Fatalf("update state: %v", err)
	}
	var state string
	if err := db.Raw("SELECT state FROM otp_requests WHERE id='r1'").Scan(&state).Error; err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state != "sent" {
		t.Fatalf("want state sent, got %q", state)
	}

	if err := repo.InsertDeliveryLog(ctx, app.DeliveryLog{
		RequestID: "r1", TenantID: "t1", Provider: "resend", Status: "sent", LatencyMillis: 42,
	}); err != nil {
		t.Fatalf("insert delivery log: %v", err)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM delivery_logs WHERE request_id='r1' AND status='sent'").Scan(&count).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("want 1 delivery log, got %d", count)
	}
}
