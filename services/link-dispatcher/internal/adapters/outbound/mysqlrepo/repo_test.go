package mysqlrepo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

// schema mirrors link_clicks from db/link/migrations/0001_init.up.sql in SQLite's
// dialect, so the adapter's insert runs in-process without a MySQL server.
const schema = `
CREATE TABLE link_clicks (
  id         INTEGER      PRIMARY KEY AUTOINCREMENT,
  code       VARCHAR(16)  NOT NULL,
  tenant_id  CHAR(36)     NOT NULL,
  ts         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  referer    VARCHAR(255) NULL,
  ua         VARCHAR(255) NULL,
  ip_hash    CHAR(64)     NULL
);`

var t0 = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // every connection to :memory: is its own database
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return New(db)
}

type stored struct {
	ID       int64
	Code     string
	TenantID string
	TS       time.Time `gorm:"column:ts"`
	Referer  sql.NullString
	UA       sql.NullString `gorm:"column:ua"`
	IPHash   sql.NullString `gorm:"column:ip_hash"`
}

func readAll(t *testing.T, r *Repo) []stored {
	t.Helper()
	var rows []stored
	if err := r.db.Table("link_clicks").Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("read clicks: %v", err)
	}
	return rows
}

func TestInsertClick(t *testing.T) {
	r := newTestRepo(t)
	// A non-UTC input must be stored as the same instant in UTC.
	click := app.Click{
		Code: "aB3xYz", TenantID: "t1", TS: t0.In(time.FixedZone("ICT", 7*3600)),
		Referer: "https://news.example.com/", UA: "Mozilla/5.0", IPHash: "hash",
	}
	if err := r.InsertClick(context.Background(), click); err != nil {
		t.Fatalf("InsertClick: %v", err)
	}

	rows := readAll(t, r)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.Code != "aB3xYz" || got.TenantID != "t1" || !got.TS.Equal(t0) {
		t.Errorf("row = %+v, want code/tenant/ts of the click", got)
	}
	if got.Referer.String != click.Referer || got.UA.String != click.UA || got.IPHash.String != click.IPHash {
		t.Errorf("row meta = %+v, want the click's referer/ua/ip hash", got)
	}

	// link-svc buckets by DATE(ts), so the stored wall-clock date must be the UTC one.
	var day string
	if err := r.db.Raw("SELECT CAST(DATE(ts) AS CHAR) FROM link_clicks").Scan(&day).Error; err != nil {
		t.Fatalf("read day: %v", err)
	}
	if day != "2026-10-10" {
		t.Errorf("DATE(ts) = %q, want 2026-10-10", day)
	}
}

func TestInsertClickStoresEmptyMetaAsNull(t *testing.T) {
	r := newTestRepo(t)
	if err := r.InsertClick(context.Background(), app.Click{Code: "abc", TenantID: "t1", TS: t0}); err != nil {
		t.Fatalf("InsertClick: %v", err)
	}
	got := readAll(t, r)[0]
	if got.Referer.Valid || got.UA.Valid || got.IPHash.Valid {
		t.Errorf("row = %+v, want NULL referer, ua and ip_hash", got)
	}
}

func TestInsertClickIsAppendOnly(t *testing.T) {
	r := newTestRepo(t)
	click := app.Click{Code: "abc", TenantID: "t1", TS: t0}
	for range 2 { // a redelivered event is simply counted again
		if err := r.InsertClick(context.Background(), click); err != nil {
			t.Fatalf("InsertClick: %v", err)
		}
	}
	rows := readAll(t, r)
	if len(rows) != 2 || rows[0].ID == rows[1].ID {
		t.Fatalf("rows = %+v, want two rows with distinct ids", rows)
	}
}

func TestInsertClickWrapsDatabaseError(t *testing.T) {
	r := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.InsertClick(ctx, app.Click{Code: "abc", TenantID: "t1", TS: t0}); err == nil {
		t.Fatal("InsertClick with a cancelled context = nil, want an error")
	}
}
