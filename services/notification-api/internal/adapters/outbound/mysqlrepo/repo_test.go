package mysqlrepo

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// schema mirrors db/notification/migrations/0001_init.up.sql in SQLite's dialect, so
// the adapter's queries run in-process without a MySQL server.
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
CREATE TABLE template_versions (
  id          CHAR(36)     NOT NULL PRIMARY KEY,
  template_id CHAR(36)     NOT NULL,
  version     INT          NOT NULL,
  subject     VARCHAR(255) NOT NULL DEFAULT '',
  body        TEXT         NOT NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT uq_tpl_version UNIQUE (template_id, version)
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
);
CREATE TABLE notification_settings (
  tenant_id CHAR(36)     NOT NULL,
  user_ref  VARCHAR(255) NOT NULL,
  channel   VARCHAR(16)  NOT NULL,
  enabled   BOOLEAN      NOT NULL DEFAULT TRUE,
  PRIMARY KEY (tenant_id, user_ref, channel)
);
CREATE TABLE notification_events (
  id              INTEGER      PRIMARY KEY AUTOINCREMENT,
  notification_id CHAR(36)     NOT NULL,
  type            VARCHAR(16)  NOT NULL,
  ts              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  meta            VARCHAR(255) NULL
);`

var t0 = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func newTestDB(t *testing.T) *gorm.DB {
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
	return db
}

// closedDB returns a database whose connection is already closed, to drive every
// method's driver-error path.
func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	return db
}

// --- templates ---

func tpl(id, tenant string, at time.Time) domain.Template {
	return domain.Template{
		ID: id, TenantID: tenant, Name: "Welcome", Channel: domain.ChannelEmail, Locale: "en",
		Subject: "Hi {{name}}", Body: "Hello {{name}}", Version: 1, Status: domain.TemplateActive,
		CreatedAt: at, UpdatedAt: at,
	}
}

func mustInsertTpl(t *testing.T, r *Templates, tp domain.Template) {
	t.Helper()
	if err := r.Insert(context.Background(), tp); err != nil {
		t.Fatalf("insert template %s: %v", tp.ID, err)
	}
}

func TestTemplates_InsertThenFind(t *testing.T) {
	r := NewTemplates(newTestDB(t))
	// A non-UTC input must come back as the same instant in UTC.
	local := t0.In(time.FixedZone("ICT", 7*3600))
	mustInsertTpl(t, r, tpl("tpl-1", "t1", local))

	got, err := r.Find(context.Background(), "t1", "tpl-1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if want := tpl("tpl-1", "t1", t0); got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestTemplates_FindIsTenantScoped(t *testing.T) {
	r := NewTemplates(newTestDB(t))
	mustInsertTpl(t, r, tpl("tpl-1", "t1", t0))
	for _, tc := range []struct{ tenant, id string }{{"t2", "tpl-1"}, {"t1", "nope"}} {
		if _, err := r.Find(context.Background(), tc.tenant, tc.id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Find(%s, %s) err = %v, want ErrNotFound", tc.tenant, tc.id, err)
		}
	}
}

func TestTemplates_InsertDuplicateIDFails(t *testing.T) {
	r := NewTemplates(newTestDB(t))
	mustInsertTpl(t, r, tpl("tpl-1", "t1", t0))
	if err := r.Insert(context.Background(), tpl("tpl-1", "t1", t0)); err == nil {
		t.Fatal("want an error for a duplicate id")
	}
}

func TestTemplates_ListNewestFirstAndTenantScoped(t *testing.T) {
	r := NewTemplates(newTestDB(t))
	mustInsertTpl(t, r, tpl("tpl-a", "t1", t0))
	mustInsertTpl(t, r, tpl("tpl-b", "t1", t0.Add(time.Hour)))
	mustInsertTpl(t, r, tpl("tpl-c", "t1", t0)) // same second as tpl-a: id breaks the tie
	mustInsertTpl(t, r, tpl("tpl-x", "t2", t0))

	got, err := r.List(context.Background(), "t1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var ids []string
	for _, g := range got {
		ids = append(ids, g.ID)
	}
	if want := []string{"tpl-b", "tpl-c", "tpl-a"}; !equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}

	empty, err := r.List(context.Background(), "t3")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty tenant: %v, %v; want a non-nil empty slice", empty, err)
	}
}

// update builds the (next, prior) pair the app layer hands to Update for cur.
func update(cur domain.Template, body string, at time.Time) (domain.Template, domain.TemplateVersion) {
	prior := domain.TemplateVersion{
		ID: "ver-" + cur.ID + "-" + string(rune('0'+cur.Version)), TemplateID: cur.ID,
		Version: cur.Version, Subject: cur.Subject, Body: cur.Body, CreatedAt: at,
	}
	next := cur
	next.Name, next.Locale, next.Subject, next.Body = "Welcome v2", "vi", "Chao {{name}}", body
	next.Version, next.UpdatedAt = cur.Version+1, at
	return next, prior
}

func versions(t *testing.T, db *gorm.DB, templateID string) []templateVersionRow {
	t.Helper()
	var rows []templateVersionRow
	if err := db.Where("template_id = ?", templateID).Order("version ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read versions: %v", err)
	}
	return rows
}

func TestTemplates_UpdateBumpsVersionAndSnapshotsPrior(t *testing.T) {
	db := newTestDB(t)
	r := NewTemplates(db)
	cur := tpl("tpl-1", "t1", t0)
	mustInsertTpl(t, r, cur)

	later := t0.Add(time.Hour)
	next, prior := update(cur, "Xin chao {{name}}", later)
	// Fields Update must not write, even if the caller got them wrong.
	next.Channel, next.Status, next.CreatedAt = domain.ChannelSMS, domain.TemplateArchived, later
	if err := r.Update(context.Background(), next, prior); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := r.Find(context.Background(), "t1", "tpl-1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	want := cur
	want.Name, want.Locale, want.Subject, want.Body = "Welcome v2", "vi", "Chao {{name}}", "Xin chao {{name}}"
	want.Version, want.UpdatedAt = 2, later
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	snaps := versions(t, db, "tpl-1")
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	s := snaps[0]
	if s.ID != prior.ID || s.Version != 1 || s.Subject != cur.Subject || s.Body != cur.Body || !s.CreatedAt.Equal(later) {
		t.Fatalf("snapshot = %+v, want the version-1 content", s)
	}
}

func TestTemplates_UpdateStaleVersionConflicts(t *testing.T) {
	db := newTestDB(t)
	r := NewTemplates(db)
	cur := tpl("tpl-1", "t1", t0)
	mustInsertTpl(t, r, cur)

	// Two writers both read version 1; the first wins.
	next, prior := update(cur, "first", t0.Add(time.Minute))
	if err := r.Update(context.Background(), next, prior); err != nil {
		t.Fatalf("first update: %v", err)
	}
	loser, loserPrior := update(cur, "second", t0.Add(2*time.Minute))
	loserPrior.ID = "ver-loser"
	if err := r.Update(context.Background(), loser, loserPrior); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale update err = %v, want ErrConflict", err)
	}

	got, _ := r.Find(context.Background(), "t1", "tpl-1")
	if got.Body != "first" || got.Version != 2 {
		t.Fatalf("the losing update was applied: %+v", got)
	}
	if n := len(versions(t, db, "tpl-1")); n != 1 {
		t.Fatalf("snapshots = %d, want 1 (the loser must not snapshot)", n)
	}
}

func TestTemplates_UpdateMissingOrOtherTenantIsNotFound(t *testing.T) {
	db := newTestDB(t)
	r := NewTemplates(db)
	cur := tpl("tpl-1", "t1", t0)
	mustInsertTpl(t, r, cur)

	foreign := cur
	foreign.TenantID = "t2"
	next, prior := update(foreign, "hijack", t0)
	if err := r.Update(context.Background(), next, prior); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant err = %v, want ErrNotFound", err)
	}
	next, prior = update(tpl("nope", "t1", t0), "x", t0)
	if err := r.Update(context.Background(), next, prior); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing err = %v, want ErrNotFound", err)
	}
	if got, _ := r.Find(context.Background(), "t1", "tpl-1"); got != cur {
		t.Fatalf("template changed: %+v", got)
	}
	if n := len(versions(t, db, "tpl-1")); n != 0 {
		t.Fatalf("snapshots = %d, want 0", n)
	}
}

// If the snapshot cannot be written the content change must roll back with it: a
// template at version N+1 with no record of version N would break rollback.
func TestTemplates_UpdateRollsBackWhenSnapshotFails(t *testing.T) {
	db := newTestDB(t)
	r := NewTemplates(db)
	cur := tpl("tpl-1", "t1", t0)
	mustInsertTpl(t, r, cur)
	// Occupy (template_id, version 1) so the snapshot insert hits uq_tpl_version.
	if err := db.Create(&templateVersionRow{ID: "squatter", TemplateID: "tpl-1", Version: 1, Body: "x", CreatedAt: t0}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	next, prior := update(cur, "new body", t0.Add(time.Minute))
	err := r.Update(context.Background(), next, prior)
	if err == nil || errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want a plain storage error", err)
	}
	if got, _ := r.Find(context.Background(), "t1", "tpl-1"); got != cur {
		t.Fatalf("update was not rolled back: %+v", got)
	}
}

func TestTemplates_DriverErrors(t *testing.T) {
	r := NewTemplates(closedDB(t))
	ctx := context.Background()
	cur := tpl("tpl-1", "t1", t0)
	next, prior := update(cur, "x", t0)
	errs := map[string]error{
		"insert": r.Insert(ctx, cur),
		"update": r.Update(ctx, next, prior),
	}
	_, errs["find"] = r.Find(ctx, "t1", "tpl-1")
	_, errs["list"] = r.List(ctx, "t1")
	assertPlainErrors(t, errs)
}

// --- notification_log ---

func notif(id, tenant string, at time.Time) domain.Notification {
	return domain.Notification{
		ID: id, TenantID: tenant, Channel: domain.ChannelEmail, RecipientMasked: "d***@gmail.com",
		TemplateID: "tpl-1", Kind: domain.KindTransactional, State: domain.StateQueued,
		CreatedAt: at, UpdatedAt: at,
	}
}

func mustInsertNotif(t *testing.T, r *Log, n domain.Notification) {
	t.Helper()
	if err := r.Insert(context.Background(), n); err != nil {
		t.Fatalf("insert notification %s: %v", n.ID, err)
	}
}

func TestLog_InsertThenFind(t *testing.T) {
	r := NewLog(newTestDB(t))
	full := notif("n-full", "t1", t0.In(time.FixedZone("ICT", 7*3600)))
	full.State, full.Provider, full.ProviderMsgID, full.LatencyMS, full.Error = domain.StateFailed, "ses", "msg-9", 120, "boom"
	mustInsertNotif(t, r, full)
	mustInsertNotif(t, r, notif("n-min", "t1", t0))

	got, err := r.Find(context.Background(), "t1", "n-full")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	want := full
	want.CreatedAt, want.UpdatedAt = t0, t0
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got, err := r.Find(context.Background(), "t1", "n-min"); err != nil || got != notif("n-min", "t1", t0) {
		t.Fatalf("minimal row: %+v, %v", got, err)
	}
}

// Unset optional fields are NULL in the table, not empty strings or a zero latency.
func TestLog_InsertStoresUnsetFieldsAsNull(t *testing.T) {
	db := newTestDB(t)
	n := notif("n-1", "t1", t0)
	n.TemplateID = ""
	mustInsertNotif(t, NewLog(db), n)

	var nulls int64
	err := db.Table("notification_log").Where("id = ? AND template_id IS NULL AND provider IS NULL AND "+
		"provider_msg_id IS NULL AND latency_ms IS NULL AND error IS NULL", "n-1").Count(&nulls).Error
	if err != nil || nulls != 1 {
		t.Fatalf("null columns: count=%d err=%v", nulls, err)
	}
}

func TestLog_InsertDuplicateIDIsAlreadyExists(t *testing.T) {
	r := NewLog(newTestDB(t))
	mustInsertNotif(t, r, notif("n-1", "t1", t0))

	dup := notif("n-1", "t1", t0.Add(time.Minute))
	dup.RecipientMasked = "x***@other.com"
	if err := r.Insert(context.Background(), dup); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
	// The primary key is global: the same id under another tenant is a conflict too.
	if err := r.Insert(context.Background(), notif("n-1", "t2", t0)); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("other tenant err = %v, want ErrAlreadyExists", err)
	}
	got, _ := r.Find(context.Background(), "t1", "n-1")
	if got != notif("n-1", "t1", t0) {
		t.Fatalf("the first row was overwritten: %+v", got)
	}
}

func TestLog_FindIsTenantScoped(t *testing.T) {
	r := NewLog(newTestDB(t))
	mustInsertNotif(t, r, notif("n-1", "t1", t0))
	for _, tc := range []struct{ tenant, id string }{{"t2", "n-1"}, {"t1", "nope"}} {
		if _, err := r.Find(context.Background(), tc.tenant, tc.id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Find(%s, %s) err = %v, want ErrNotFound", tc.tenant, tc.id, err)
		}
	}
}

func TestLog_ListNewestFirstLimitedAndTenantScoped(t *testing.T) {
	r := NewLog(newTestDB(t))
	mustInsertNotif(t, r, notif("n-a", "t1", t0))
	mustInsertNotif(t, r, notif("n-b", "t1", t0.Add(time.Hour)))
	mustInsertNotif(t, r, notif("n-c", "t1", t0)) // same second as n-a: id breaks the tie
	mustInsertNotif(t, r, notif("n-x", "t2", t0.Add(2*time.Hour)))

	list := func(limit int) []string {
		t.Helper()
		got, err := r.List(context.Background(), "t1", limit)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		ids := []string{}
		for _, g := range got {
			ids = append(ids, g.ID)
		}
		return ids
	}
	if got, want := list(10), []string{"n-b", "n-c", "n-a"}; !equal(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if got, want := list(2), []string{"n-b", "n-c"}; !equal(got, want) {
		t.Fatalf("limited ids = %v, want %v", got, want)
	}

	empty, err := r.List(context.Background(), "t3", 10)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty tenant: %v, %v; want a non-nil empty slice", empty, err)
	}
}

func TestLog_EventsOldestFirst(t *testing.T) {
	db := newTestDB(t)
	r := NewLog(db)
	// Writing events is the dispatcher's job, so the adapter has no method for it.
	seed := func(notificationID, typ string, ts time.Time, meta any) {
		t.Helper()
		err := db.Exec("INSERT INTO notification_events (notification_id, type, ts, meta) VALUES (?, ?, ?, ?)",
			notificationID, typ, ts.UTC(), meta).Error
		if err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	seed("n-1", "clicked", t0.Add(time.Hour), "abc123")
	seed("n-1", "delivered", t0, nil)
	seed("n-1", "opened", t0, nil) // same second as delivered: insertion order breaks the tie
	seed("n-2", "delivered", t0, nil)

	got, err := r.Events(context.Background(), "n-1")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	want := []domain.Event{
		{Type: "delivered", TS: t0},
		{Type: "opened", TS: t0},
		{Type: "clicked", TS: t0.Add(time.Hour), Meta: "abc123"},
	}
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	none, err := r.Events(context.Background(), "n-none")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("no events: %v, %v; want a non-nil empty slice", none, err)
	}
}

func TestLog_MarkFailed(t *testing.T) {
	r := NewLog(newTestDB(t))
	mustInsertNotif(t, r, notif("n-1", "t1", t0))
	mustInsertNotif(t, r, notif("n-other", "t1", t0))

	later := t0.Add(time.Minute)
	if err := r.MarkFailed(context.Background(), "n-1", "publish failed", later); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	want := notif("n-1", "t1", t0)
	want.State, want.Error, want.UpdatedAt = domain.StateFailed, "publish failed", later
	if got, _ := r.Find(context.Background(), "t1", "n-1"); got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got, _ := r.Find(context.Background(), "t1", "n-other"); got != notif("n-other", "t1", t0) {
		t.Fatalf("another row was touched: %+v", got)
	}
}

// A row that already left queued (the dispatcher recorded a delivery) is never
// rewritten to failed, and an unknown id is not an error.
func TestLog_MarkFailedOnlyAffectsQueuedRows(t *testing.T) {
	r := NewLog(newTestDB(t))
	sent := notif("n-sent", "t1", t0)
	sent.State, sent.Provider = domain.StateSent, "ses"
	mustInsertNotif(t, r, sent)

	if err := r.MarkFailed(context.Background(), "n-sent", "publish failed", t0.Add(time.Minute)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if got, _ := r.Find(context.Background(), "t1", "n-sent"); got != sent {
		t.Fatalf("sent row was rewritten: %+v", got)
	}
	if err := r.MarkFailed(context.Background(), "nope", "publish failed", t0); err != nil {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestLog_DriverErrors(t *testing.T) {
	r := NewLog(closedDB(t))
	ctx := context.Background()
	errs := map[string]error{
		"insert":      r.Insert(ctx, notif("n-1", "t1", t0)),
		"mark failed": r.MarkFailed(ctx, "n-1", "x", t0),
	}
	_, errs["find"] = r.Find(ctx, "t1", "n-1")
	_, errs["list"] = r.List(ctx, "t1", 10)
	_, errs["events"] = r.Events(ctx, "n-1")
	assertPlainErrors(t, errs)
}

// --- notification_settings ---

func TestSettings_UpsertThenList(t *testing.T) {
	r := NewSettings(newTestDB(t))
	ctx := context.Background()
	upsert := func(tenant, user, channel string, enabled bool) {
		t.Helper()
		if err := r.Upsert(ctx, tenant, user, domain.Setting{Channel: channel, Enabled: enabled}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	// The opt-out is the case that matters: false must not fall back to the column
	// default of TRUE.
	upsert("t1", "u1", domain.ChannelSMS, false)
	upsert("t1", "u1", domain.ChannelEmail, true)
	upsert("t1", "u2", domain.ChannelEmail, false)
	upsert("t2", "u1", domain.ChannelEmail, false)

	got, err := r.List(ctx, "t1", "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []domain.Setting{{Channel: domain.ChannelEmail, Enabled: true}, {Channel: domain.ChannelSMS, Enabled: false}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	// A second write to the same key replaces the value instead of failing on the
	// primary key, in both directions.
	upsert("t1", "u1", domain.ChannelSMS, true)
	upsert("t1", "u1", domain.ChannelEmail, false)
	got, err = r.List(ctx, "t1", "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want = []domain.Setting{{Channel: domain.ChannelEmail, Enabled: false}, {Channel: domain.ChannelSMS, Enabled: true}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("after overwrite: got %+v, want %+v", got, want)
	}

	none, err := r.List(ctx, "t1", "nobody")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("no rows: %v, %v; want a non-nil empty slice", none, err)
	}
}

func TestSettings_DriverErrors(t *testing.T) {
	r := NewSettings(closedDB(t))
	ctx := context.Background()
	errs := map[string]error{"upsert": r.Upsert(ctx, "t1", "u1", domain.Setting{Channel: domain.ChannelEmail})}
	_, errs["list"] = r.List(ctx, "t1", "u1")
	assertPlainErrors(t, errs)
}

// --- helpers ---

// assertPlainErrors checks each driver failure surfaced as an error that is not one of
// the domain sentinels - a broken connection must never look like "not found".
func assertPlainErrors(t *testing.T, errs map[string]error) {
	t.Helper()
	for op, err := range errs {
		if err == nil {
			t.Errorf("%s: want an error on a closed database", op)
			continue
		}
		for _, sentinel := range []error{domain.ErrNotFound, domain.ErrAlreadyExists, domain.ErrConflict} {
			if errors.Is(err, sentinel) {
				t.Errorf("%s: driver error surfaced as %v", op, sentinel)
			}
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
