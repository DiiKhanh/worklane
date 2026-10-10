package mysqlrepo

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// schema mirrors db/link/migrations/0001_init.up.sql in SQLite's dialect, so the
// adapter's queries run in-process without a MySQL server.
const schema = `
CREATE TABLE links (
  id            BIGINT       NOT NULL PRIMARY KEY,
  code          VARCHAR(16)  NOT NULL,
  tenant_id     CHAR(36)     NOT NULL,
  long_url      TEXT         NOT NULL,
  long_url_hash CHAR(64)     NOT NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT uq_links_code UNIQUE (code),
  CONSTRAINT uq_links_tenant_url UNIQUE (tenant_id, long_url_hash)
);
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

func link(id int64, code, tenant, url string, at time.Time) domain.Link {
	return domain.Link{ID: id, Code: code, TenantID: tenant, LongURL: url, LongURLHash: domain.HashURL(url), CreatedAt: at}
}

func mustInsert(t *testing.T, r *Repo, l domain.Link) {
	t.Helper()
	if err := r.Insert(context.Background(), l); err != nil {
		t.Fatalf("insert %s: %v", l.Code, err)
	}
}

// addClick seeds a link_clicks row directly: writing clicks is link-dispatcher's job,
// so the production adapter has no method for it.
func addClick(t *testing.T, r *Repo, code string, ts time.Time, referer, ua any) {
	t.Helper()
	err := r.db.Exec("INSERT INTO link_clicks (code, tenant_id, ts, referer, ua) VALUES (?, ?, ?, ?, ?)",
		code, "t1", ts.UTC(), referer, ua).Error
	if err != nil {
		t.Fatalf("seed click: %v", err)
	}
}

func TestInsertAndFind(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	// A non-UTC input must come back as the same instant in UTC.
	want := link(1, "abc", "t1", "https://example.com/a", t0.In(time.FixedZone("ICT", 7*3600)))
	mustInsert(t, r, want)

	for name, find := range map[string]func() (domain.Link, error){
		"by code":     func() (domain.Link, error) { return r.FindByCode(ctx, "abc") },
		"by url hash": func() (domain.Link, error) { return r.FindByURLHash(ctx, "t1", want.LongURLHash) },
	} {
		got, err := find()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !got.CreatedAt.Equal(t0) || got.CreatedAt.Location() != time.UTC {
			t.Fatalf("%s: created_at = %v, want %v in UTC", name, got.CreatedAt, t0)
		}
		got.CreatedAt = want.CreatedAt
		if got != want {
			t.Fatalf("%s: got %+v, want %+v", name, got, want)
		}
	}
}

func TestFind_NotFound(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	mustInsert(t, r, link(1, "abc", "t1", "https://example.com/a", t0))

	if _, err := r.FindByCode(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindByCode: want ErrNotFound, got %v", err)
	}
	// The hash exists, but under another tenant.
	if _, err := r.FindByURLHash(ctx, "t2", domain.HashURL("https://example.com/a")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindByURLHash: want ErrNotFound, got %v", err)
	}
}

func TestInsert_DuplicateTenantURL(t *testing.T) {
	r := newTestRepo(t)
	mustInsert(t, r, link(1, "abc", "t1", "https://example.com/a", t0))

	err := r.Insert(context.Background(), link(2, "abd", "t1", "https://example.com/a", t0))
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("want ErrAlreadyExists, got %v", err)
	}
}

func TestInsert_SameURLOtherTenant(t *testing.T) {
	r := newTestRepo(t)
	mustInsert(t, r, link(1, "abc", "t1", "https://example.com/a", t0))
	mustInsert(t, r, link(2, "abd", "t2", "https://example.com/a", t0))
}

// A code collision is not a dedup hit: reporting it as ErrAlreadyExists would make
// Shorten look up a link that does not exist for this tenant + URL.
func TestInsert_CodeCollisionIsPlainError(t *testing.T) {
	r := newTestRepo(t)
	mustInsert(t, r, link(1, "abc", "t1", "https://example.com/a", t0))

	err := r.Insert(context.Background(), link(2, "abc", "t1", "https://example.com/b", t0))
	if err == nil || errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("want a plain error, got %v", err)
	}
}

func TestListByTenant(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	mustInsert(t, r, link(1, "old", "t1", "https://example.com/1", t0.Add(-2*time.Hour)))
	mustInsert(t, r, link(2, "mid", "t1", "https://example.com/2", t0.Add(-time.Hour)))
	mustInsert(t, r, link(3, "new", "t1", "https://example.com/3", t0))
	mustInsert(t, r, link(4, "other", "t2", "https://example.com/1", t0))
	addClick(t, r, "old", t0, nil, nil)
	addClick(t, r, "old", t0, nil, nil)
	addClick(t, r, "new", t0, nil, nil)
	addClick(t, r, "other", t0, nil, nil)

	got, err := r.ListByTenant(ctx, "t1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	wantCodes, wantClicks := []string{"new", "mid", "old"}, []int64{1, 0, 2}
	if len(got) != len(wantCodes) {
		t.Fatalf("got %d links, want %d", len(got), len(wantCodes))
	}
	for i, s := range got {
		if s.Link.Code != wantCodes[i] || s.Clicks != wantClicks[i] {
			t.Fatalf("row %d: got %s/%d, want %s/%d", i, s.Link.Code, s.Clicks, wantCodes[i], wantClicks[i])
		}
	}
	if got[2].Link.ID != 1 || got[2].Link.TenantID != "t1" || got[2].Link.LongURL != "https://example.com/1" ||
		!got[2].Link.CreatedAt.Equal(t0.Add(-2*time.Hour)) {
		t.Fatalf("link fields not mapped: %+v", got[2].Link)
	}

	limited, err := r.ListByTenant(ctx, "t1", 2)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 2 || limited[0].Link.Code != "new" {
		t.Fatalf("limit not applied newest first: %+v", limited)
	}

	none, err := r.ListByTenant(ctx, "nobody", 10)
	if err != nil || len(none) != 0 {
		t.Fatalf("unknown tenant: got %v, %v", none, err)
	}
}

func TestCountClicks(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	addClick(t, r, "abc", t0, nil, nil)
	addClick(t, r, "abc", t0.Add(-30*24*time.Hour), nil, nil)
	addClick(t, r, "xyz", t0, nil, nil)

	if n, err := r.CountClicks(ctx, "abc"); err != nil || n != 2 {
		t.Fatalf("abc: got %d, %v", n, err)
	}
	if n, err := r.CountClicks(ctx, "none"); err != nil || n != 0 {
		t.Fatalf("none: got %d, %v", n, err)
	}
}

func TestDailyClicks(t *testing.T) {
	r := newTestRepo(t)
	day := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, time.UTC) }
	addClick(t, r, "abc", day(7, 23, 59), nil, nil) // before since
	addClick(t, r, "abc", day(8, 0, 0), nil, nil)   // exactly since: included
	addClick(t, r, "abc", day(8, 23, 59), nil, nil)
	addClick(t, r, "abc", day(10, 1, 0), nil, nil) // day 9 has none: stays sparse
	addClick(t, r, "xyz", day(9, 1, 0), nil, nil)  // another code

	got, err := r.DailyClicks(context.Background(), "abc", day(8, 0, 0))
	if err != nil {
		t.Fatalf("daily: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 days", got)
	}
	if !got[0].Day.Equal(day(8, 0, 0)) || got[0].Clicks != 2 || got[0].Day.Location() != time.UTC {
		t.Fatalf("day 0: %+v", got[0])
	}
	if !got[1].Day.Equal(day(10, 0, 0)) || got[1].Clicks != 1 {
		t.Fatalf("day 1: %+v", got[1])
	}
}

func TestRecentClicks(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	addClick(t, r, "abc", t0.Add(-2*time.Minute), "https://ref.example", "Mozilla/5.0 (iPhone)")
	addClick(t, r, "abc", t0, nil, nil) // NULL referer / ua
	addClick(t, r, "abc", t0.Add(-time.Minute), "", "curl/8")
	addClick(t, r, "xyz", t0, "https://other.example", "x")

	got, err := r.RecentClicks(ctx, "abc", 2)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d clicks, want 2", len(got))
	}
	if !got[0].TS.Equal(t0) || got[0].Referer != "" || got[0].UA != "" {
		t.Fatalf("newest: %+v", got[0])
	}
	if !got[1].TS.Equal(t0.Add(-time.Minute)) || got[1].UA != "curl/8" {
		t.Fatalf("second: %+v", got[1])
	}

	all, err := r.RecentClicks(ctx, "abc", 10)
	if err != nil || len(all) != 3 || all[2].Referer != "https://ref.example" || all[2].UA != "Mozilla/5.0 (iPhone)" {
		t.Fatalf("all: %+v, %v", all, err)
	}
}

func TestErrorsAreWrapped(t *testing.T) {
	r := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := r.Insert(ctx, link(1, "abc", "t1", "https://example.com/a", t0)); err == nil || errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Insert: want a plain error, got %v", err)
	}
	if _, err := r.FindByCode(ctx, "abc"); err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindByCode: want a plain error, got %v", err)
	}
	if _, err := r.ListByTenant(ctx, "t1", 10); err == nil {
		t.Fatal("ListByTenant: want error")
	}
	if _, err := r.CountClicks(ctx, "abc"); err == nil {
		t.Fatal("CountClicks: want error")
	}
	if _, err := r.DailyClicks(ctx, "abc", t0); err == nil {
		t.Fatal("DailyClicks: want error")
	}
	if _, err := r.RecentClicks(ctx, "abc", 10); err == nil {
		t.Fatal("RecentClicks: want error")
	}
}
