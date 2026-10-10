package notifrepo

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

var (
	_ app.NotificationTemplates = (*Repo)(nil)
	_ app.NotificationLog       = (*Repo)(nil)
)

// schema mirrors the two tables this adapter touches in
// db/notification/migrations/0001_init.up.sql, in SQLite's dialect.
const schema = `
CREATE TABLE templates (
  id         CHAR(36)     NOT NULL PRIMARY KEY,
  tenant_id  CHAR(36)     NOT NULL,
  name       VARCHAR(255) NOT NULL,
  channel    VARCHAR(16)  NOT NULL,
  locale     VARCHAR(16)  NOT NULL,
  subject    VARCHAR(255) NOT NULL DEFAULT '',
  body       TEXT         NOT NULL,
  version    INT          NOT NULL DEFAULT 1,
  status     VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE notification_log (
  id               CHAR(36)     NOT NULL PRIMARY KEY,
  tenant_id        CHAR(36)     NOT NULL,
  channel          VARCHAR(16)  NOT NULL,
  recipient_masked VARCHAR(255) NOT NULL,
  template_id      CHAR(36)     NULL,
  kind             VARCHAR(16)  NOT NULL,
  state            VARCHAR(16)  NOT NULL,
  provider         VARCHAR(32)  NULL,
  provider_msg_id  VARCHAR(128) NULL,
  latency_ms       BIGINT       NULL,
  error            TEXT         NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

func newRepo(t *testing.T) (*Repo, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1) // one connection = one in-memory database
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(schema).Error; err != nil {
		t.Fatal(err)
	}
	return New(db), db
}

func seedLog(t *testing.T, db *gorm.DB, id, tenant, state string) {
	t.Helper()
	err := db.Exec(`INSERT INTO notification_log (id, tenant_id, channel, recipient_masked, kind, state, error)
		VALUES (?, ?, 'email', 'a***@b.co', 'transactional', ?, ?)`, id, tenant, state, "earlier").Error
	if err != nil {
		t.Fatal(err)
	}
}

type logRow struct {
	State         string
	Provider      sql.NullString
	ProviderMsgID sql.NullString `gorm:"column:provider_msg_id"`
	LatencyMS     sql.NullInt64  `gorm:"column:latency_ms"`
	Error         sql.NullString
	UpdatedAt     time.Time
}

func readLog(t *testing.T, db *gorm.DB, id string) logRow {
	t.Helper()
	var row logRow
	if err := db.Table("notification_log").Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestFindTemplateIsTenantScoped(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	err := db.Exec(`INSERT INTO templates (id, tenant_id, name, channel, locale, subject, body, status)
		VALUES ('tpl-1', 't-1', 'Welcome', 'email', 'en', 'Hi {{name}}', 'Body', 'archived')`).Error
	if err != nil {
		t.Fatal(err)
	}

	tpl, found, err := repo.Find(ctx, "t-1", "tpl-1")
	if err != nil || !found || tpl.Subject != "Hi {{name}}" || tpl.Body != "Body" {
		t.Fatalf("got %+v found=%v err=%v", tpl, found, err)
	}
	if _, found, err := repo.Find(ctx, "t-2", "tpl-1"); err != nil || found {
		t.Fatalf("other tenant: found=%v err=%v", found, err)
	}
	if _, found, err := repo.Find(ctx, "t-1", "nope"); err != nil || found {
		t.Fatalf("missing: found=%v err=%v", found, err)
	}
}

func TestStateIsTenantScoped(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	seedLog(t, db, "n-1", "t-1", "queued")

	if state, found, err := repo.State(ctx, "t-1", "n-1"); err != nil || !found || state != "queued" {
		t.Fatalf("got %q found=%v err=%v", state, found, err)
	}
	if _, found, err := repo.State(ctx, "t-2", "n-1"); err != nil || found {
		t.Fatalf("other tenant: found=%v err=%v", found, err)
	}
}

func TestMarkSent(t *testing.T) {
	at := time.Date(2026, 10, 10, 8, 0, 0, 0, time.FixedZone("ICT", 7*3600))
	out := app.NotificationOutcome{Provider: "resend", ProviderMsgID: strings.Repeat("m", 200), LatencyMillis: 42, At: at}
	for from, wantSent := range map[string]bool{"queued": true, "failed": true, "sent": false, "suppressed": false} {
		repo, db := newRepo(t)
		seedLog(t, db, "n-1", "t-1", from)
		if err := repo.MarkSent(context.Background(), "n-1", out); err != nil {
			t.Fatalf("%s: %v", from, err)
		}
		row := readLog(t, db, "n-1")
		if !wantSent {
			if row.State != from || row.Provider.Valid {
				t.Fatalf("%s: row must be untouched, got %+v", from, row)
			}
			continue
		}
		if row.State != "sent" || row.Provider.String != "resend" || len(row.ProviderMsgID.String) != maxProviderMsgIDRunes ||
			row.LatencyMS.Int64 != 42 || row.Error.Valid || !row.UpdatedAt.Equal(at) {
			t.Fatalf("%s: got %+v", from, row)
		}
	}
}

func TestMarkSentStoresEmptyMessageIDAsNull(t *testing.T) {
	repo, db := newRepo(t)
	seedLog(t, db, "n-1", "t-1", "queued")
	if err := repo.MarkSent(context.Background(), "n-1", app.NotificationOutcome{Provider: "smtp", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if row := readLog(t, db, "n-1"); row.State != "sent" || row.ProviderMsgID.Valid {
		t.Fatalf("got %+v", row)
	}
}

func TestMarkFailedOnlyFromQueued(t *testing.T) {
	out := app.NotificationOutcome{Provider: "twilio", LatencyMillis: 7, Error: "twilio: status 400", At: time.Now()}
	for from, wantFailed := range map[string]bool{"queued": true, "sent": false, "suppressed": false} {
		repo, db := newRepo(t)
		seedLog(t, db, "n-1", "t-1", from)
		if err := repo.MarkFailed(context.Background(), "n-1", out); err != nil {
			t.Fatalf("%s: %v", from, err)
		}
		row := readLog(t, db, "n-1")
		if !wantFailed {
			if row.State != from || row.Error.String != "earlier" {
				t.Fatalf("%s: row must be untouched, got %+v", from, row)
			}
			continue
		}
		if row.State != "failed" || row.Provider.String != "twilio" || row.LatencyMS.Int64 != 7 || row.Error.String != "twilio: status 400" {
			t.Fatalf("got %+v", row)
		}
	}
}

func TestDriverErrorsAreWrapped(t *testing.T) {
	repo, db := newRepo(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	ctx := context.Background()
	if _, _, err := repo.Find(ctx, "t", "x"); err == nil {
		t.Fatal("find: want error")
	}
	if _, _, err := repo.State(ctx, "t", "x"); err == nil {
		t.Fatal("state: want error")
	}
	if err := repo.MarkSent(ctx, "x", app.NotificationOutcome{}); err == nil {
		t.Fatal("mark sent: want error")
	}
	if err := repo.MarkFailed(ctx, "x", app.NotificationOutcome{}); err == nil {
		t.Fatal("mark failed: want error")
	}
}
