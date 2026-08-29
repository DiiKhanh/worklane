# Template Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn worklane's OTP message templates into versioned, dashboard-authored data that the dispatcher renders from the database as the single source of truth, with a preview that renders through the exact delivery path.

**Architecture:** A new shared `pkg/templating` engine does allowlisted `{{code}}`/`{{expiry}}` substitution and is consumed by both the otp-api preview endpoint and the otp-dispatcher delivery path (so preview cannot lie). otp-api gains a JWT-authed template CRUD API over a two-table schema (`templates` + immutable `template_versions`) with a `draft → publish → rollback` lifecycle. The dispatcher resolves the active template for `(channel, locale)` via Redis cache-aside, falling back to env-config so the live OTP flow never breaks.

**Tech Stack:** Go 1.x (Gin, GORM, sarama), golang-migrate SQL, MySQL, Redis, Kafka/Redpanda; Next.js + React Query + TypeScript dashboard (vitest).

## Global Constraints

- Never use the em dash. Use a plain dash `-`.
- Immutability: template version content is never mutated; an edit creates a new version row.
- Selection key is global `(channel, locale)`; no `tenant_id` on templates in this project.
- Allowlisted variables are exactly `{{code}}` and `{{expiry}}`. Any other `{{token}}` fails validation at author time.
- The dispatcher MUST fall back to the env-config template on any missing-row / DB / Redis error, so OTP delivery never fails because of templates.
- Template CRUD is JWT-only (dashboard human user); an API-key caller must be rejected. `created_by` = JWT `Claims.Email`.
- `locale` defaults to `"en"` everywhere; an empty locale is treated as `"en"` (backward compatible with in-flight events).
- Commit after every task. Conventional commit messages (`feat:`, `test:`, `refactor:`, `docs:`). Do NOT add any agent co-author trailer.
- Follow hexagonal boundaries: `app` imports domain + contracts only, never adapters or `pkg/platform`; adapters may import `app`/`domain`/`pkg`.

---

## File structure

**New (backend)**
- `pkg/templating/templating.go` - `Render` + `Validate` + `Vars`.
- `pkg/templating/templating_test.go`.
- `db/otp/migrations/0004_template_studio.up.sql` / `.down.sql`.
- `services/otp-api/internal/app/template.go` - template use cases + ports.
- `services/otp-api/internal/app/template_test.go`.
- `services/otp-api/internal/adapters/outbound/mysqltemplates/repo.go` - GORM template repo.
- `services/otp-api/internal/adapters/outbound/mysqltemplates/repo_test.go` (optional integration; unit via app fakes).
- `services/otp-api/internal/adapters/inbound/http/templates.go` - template HTTP handlers.
- `services/otp-api/internal/adapters/inbound/http/templates_dto.go`.
- `services/otp-api/internal/adapters/inbound/http/templates_test.go`.
- `services/otp-dispatcher/internal/adapters/outbound/templatestore/store.go` - cache-aside template source.
- `services/otp-dispatcher/internal/adapters/outbound/templatestore/store_test.go`.

**Modified (backend)**
- `pkg/contracts/otp/event.go` - add `Locale` to `RequestedEvent`.
- `services/otp-api/internal/app/send.go`, `.../http/handlers.go`, `.../http/dto.go` - thread `locale`.
- `services/otp-api/internal/adapters/inbound/http/router.go`, `middleware.go`, `main.go` - template routes, JWT author, Redis for invalidation.
- `services/otp-dispatcher/internal/app/ports.go`, `sender.go`, `handler.go`, `template.go`, `main.go` - Sender becomes pure delivery; handler resolves+renders.

**Dashboard**
- `dashboard/lib/api/types.ts`, `source.ts`, `mock.ts`, `live.ts`, `lib/queries/keys.ts` (+ new `lib/queries/use-templates.ts`).
- `dashboard/components/templates/{templates-view,template-detail,new-template-dialog}.tsx` - swap fixtures for `DataSource`, add preview + publish + rollback.
- `dashboard/components/playground/*` - locale selector.

**Docs**
- `docs/architecture.md` (data model + flow), roadmap status.

---

## Task 1: `pkg/templating` render + validate engine

**Files:**
- Create: `pkg/templating/templating.go`
- Test: `pkg/templating/templating_test.go`

**Interfaces:**
- Produces:
  - `type Vars struct { Code string; Expiry string }`
  - `func Render(subject, body string, v Vars) (string, string)`
  - `func Validate(channel, subject, body string) error`
  - `var ErrUnknownVariable = errors.New("templating: unknown variable")`
  - `var ErrSubjectRequired = errors.New("templating: subject required for email")`

- [ ] **Step 1: Write the failing test**

```go
package templating

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderSubstitutesAllowlistedVars(t *testing.T) {
	sub, body := Render("Your code: {{code}}", "It is {{code}}, expires in {{ expiry }}.",
		Vars{Code: "123456", Expiry: "5 minutes"})
	if sub != "Your code: 123456" {
		t.Fatalf("subject: got %q", sub)
	}
	if body != "It is 123456, expires in 5 minutes." {
		t.Fatalf("body: got %q", body)
	}
}

func TestValidateRejectsUnknownVariable(t *testing.T) {
	err := Validate("email", "Hi {{code}}", "Hello {{name}}, code {{code}}")
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("want ErrUnknownVariable, got %v", err)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should name the offending variable, got %v", err)
	}
}

func TestValidateEmailRequiresSubject(t *testing.T) {
	if err := Validate("email", "", "body {{code}}"); !errors.Is(err, ErrSubjectRequired) {
		t.Fatalf("want ErrSubjectRequired, got %v", err)
	}
	if err := Validate("sms", "", "body {{code}}"); err != nil {
		t.Fatalf("sms with empty subject is valid, got %v", err)
	}
}

func TestValidateAllowsKnownVarsAndPlainText(t *testing.T) {
	if err := Validate("email", "Code {{code}}", "It is {{code}}, expires {{expiry}}."); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/templating/...`
Expected: FAIL (package/functions not defined).

- [ ] **Step 3: Write minimal implementation**

```go
// Package templating is the shared, dependency-free render engine for OTP messages.
// It performs allowlisted variable substitution ({{code}}, {{expiry}}) so the exact
// same code renders a preview in otp-api and a delivered message in otp-dispatcher -
// a preview therefore cannot diverge from what is sent. Substitution is a literal
// allowlist replace, never a template interpreter: there is no field walking and no
// code-execution surface.
package templating

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrUnknownVariable = errors.New("templating: unknown variable")
	ErrSubjectRequired = errors.New("templating: subject required for email")
)

// Vars carries the values available in the OTP flow today. Extend this (and the
// allowlist below) when sub-projects B/C add {{name}} and {{link}}.
type Vars struct {
	Code   string
	Expiry string
}

// tokenRe matches {{ name }} with optional inner whitespace.
var tokenRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// allowed is the substitution allowlist; the key is the variable name.
func allowed(v Vars) map[string]string {
	return map[string]string{"code": v.Code, "expiry": v.Expiry}
}

// Render replaces every allowlisted token in subject and body. Unknown tokens are
// left untouched (Validate is the gate that keeps them from ever being saved).
func Render(subject, body string, v Vars) (string, string) {
	vals := allowed(v)
	repl := func(s string) string {
		return tokenRe.ReplaceAllStringFunc(s, func(m string) string {
			name := tokenRe.FindStringSubmatch(m)[1]
			if val, ok := vals[name]; ok {
				return val
			}
			return m
		})
	}
	return repl(subject), repl(body)
}

// Validate enforces authoring rules: only allowlisted variables may appear, and email
// requires a non-empty subject.
func Validate(channel, subject, body string) error {
	if channel == "email" && subject == "" {
		return ErrSubjectRequired
	}
	names := allowed(Vars{})
	for _, field := range []string{subject, body} {
		for _, m := range tokenRe.FindAllStringSubmatch(field, -1) {
			if _, ok := names[m[1]]; !ok {
				return fmt.Errorf("%w: %q", ErrUnknownVariable, m[1])
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/templating/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/templating
git commit -m "feat: shared templating engine (allowlist {{code}}/{{expiry}})"
```

---

## Task 2: Two-table schema migration + seed

**Files:**
- Create: `db/otp/migrations/0004_template_studio.up.sql`
- Create: `db/otp/migrations/0004_template_studio.down.sql`

**Interfaces:**
- Produces the `templates` (restructured) and `template_versions` tables and two seeded, published templates: OTP email `en` and OTP SMS `en`.

**Assumption to verify first:** the current `templates` table has no writer in the codebase (confirmed: nothing SELECTs/INSERTs it), so it is empty in every environment. The up-migration drops and recreates it.

- [ ] **Step 1: Write the up migration**

```sql
-- 0004_template_studio.up.sql
-- Promote the unused single-row `templates` table into a versioned two-table model.
-- The old table has no writer in the codebase and is empty in all environments.
DROP TABLE IF EXISTS templates;

CREATE TABLE templates (
  id                CHAR(36)     NOT NULL PRIMARY KEY,
  name              VARCHAR(120) NOT NULL,
  channel           VARCHAR(16)  NOT NULL,
  locale            VARCHAR(16)  NOT NULL,
  status            VARCHAR(16)  NOT NULL DEFAULT 'active',
  active_version_id CHAR(36)     NULL,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_templates_channel_locale (channel, locale)
);

CREATE TABLE template_versions (
  id          CHAR(36)     NOT NULL PRIMARY KEY,
  template_id CHAR(36)     NOT NULL,
  version_no  INT          NOT NULL,
  subject     VARCHAR(255) NOT NULL DEFAULT '',
  body        TEXT         NOT NULL,
  status      VARCHAR(16)  NOT NULL DEFAULT 'draft',
  note        VARCHAR(255) NOT NULL DEFAULT '',
  created_by  VARCHAR(120) NOT NULL DEFAULT '',
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_versions_template_no (template_id, version_no),
  INDEX idx_versions_template (template_id)
);

-- Seed the current env-config templates as the first published versions, so the DB is
-- the source of truth from day one (env-config remains only as the dispatcher fallback).
INSERT INTO templates (id, name, channel, locale, status, active_version_id) VALUES
  ('11111111-1111-1111-1111-111111111111', 'OTP email', 'email', 'en', 'active', '11111111-0000-0000-0000-000000000001'),
  ('22222222-2222-2222-2222-222222222222', 'OTP SMS',   'sms',   'en', 'active', '22222222-0000-0000-0000-000000000001');

INSERT INTO template_versions (id, template_id, version_no, subject, body, status, note, created_by) VALUES
  ('11111111-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 1,
   'Your verification code', 'Your verification code is {{code}}. It expires in {{expiry}}.', 'published', 'seed', 'system'),
  ('22222222-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', 1,
   '', 'Your verification code is {{code}}. It expires in {{expiry}}.', 'published', 'seed', 'system');
```

- [ ] **Step 2: Write the down migration**

```sql
-- 0004_template_studio.down.sql
DROP TABLE IF EXISTS template_versions;
DROP TABLE IF EXISTS templates;

-- Restore the original (unused) shape so the down path is a true inverse.
CREATE TABLE templates (
  id CHAR(36) PRIMARY KEY,
  channel VARCHAR(16) NOT NULL,
  locale VARCHAR(16) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  body TEXT NOT NULL
);
```

- [ ] **Step 3: Apply and verify against a local DB**

Run (compose MySQL must be up):
```bash
go run ./services/otp-api & sleep 3; kill %1   # otp-api runs mysql.Migrate at startup
# or apply directly with your migrate tool, then:
docker compose exec -T mysql mysql -uroot -psecret otp -e \
  "SELECT t.channel,t.locale,v.version_no,v.status FROM templates t JOIN template_versions v ON v.id=t.active_version_id;"
```
Expected: two rows, `email/en/1/published` and `sms/en/1/published`.

- [ ] **Step 4: Commit**

```bash
git add db/otp/migrations/0004_template_studio.up.sql db/otp/migrations/0004_template_studio.down.sql
git commit -m "feat: versioned templates schema + seed OTP email/sms (en)"
```

---

## Task 3: Add `Locale` to the OTP contract and send path

**Files:**
- Modify: `pkg/contracts/otp/event.go` (RequestedEvent)
- Modify: `services/otp-api/internal/app/send.go` (SendInput + event)
- Modify: `services/otp-api/internal/adapters/inbound/http/dto.go` (sendRequest)
- Modify: `services/otp-api/internal/adapters/inbound/http/handlers.go` (Send)
- Test: `services/otp-api/internal/app/send_test.go` (existing test file; add a case) or `usecase_test.go`

**Interfaces:**
- Consumes: `contracts.RequestedEvent` (Task later: dispatcher reads `.Locale`).
- Produces: `RequestedEvent.Locale string`; `SendInput.Locale string`; `sendRequest.Locale string`.

- [ ] **Step 1: Write the failing test** (in `services/otp-api/internal/app/usecase_test.go`, add)

```go
func TestSendPublishesLocaleDefaultingToEn(t *testing.T) {
	svc, deps := newTestService(t) // existing helper that captures the published event
	_, err := svc.Send(context.Background(), app.SendInput{
		TenantID: "t1", Recipient: "d@example.com", Channel: "email", // Locale empty
	})
	if err != nil {
		t.Fatal(err)
	}
	evt := deps.lastPublished().(contracts.RequestedEvent)
	if evt.Locale != "en" {
		t.Fatalf("empty locale must default to en, got %q", evt.Locale)
	}
}
```

> If `newTestService`/`lastPublished` helpers differ, mirror the existing send test's fake `Publisher` that records the last event.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/app/...`
Expected: FAIL (`Locale` undefined / not defaulted).

- [ ] **Step 3: Implement**

In `pkg/contracts/otp/event.go`, add the field:
```go
type RequestedEvent struct {
	RequestID string `json:"request_id"`
	TenantID  string `json:"tenant_id"`
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Locale    string `json:"locale"` // empty is treated as "en" by the dispatcher
	Code      string `json:"code"`   // never logged
}
```

In `services/otp-api/internal/app/send.go`, extend `SendInput` and default+publish locale:
```go
type SendInput struct {
	TenantID       string
	Recipient      string
	Channel        string
	Locale         string // "" defaults to "en"
	IdempotencyKey string
}
```
Inside `Send`, after computing `channel`:
```go
	locale := in.Locale
	if locale == "" {
		locale = "en"
	}
```
and set it on the event:
```go
	evt := contracts.RequestedEvent{
		RequestID: requestID, TenantID: in.TenantID, Recipient: in.Recipient,
		Channel: string(channel), Locale: locale, Code: code,
	}
```

In `dto.go`:
```go
type sendRequest struct {
	Recipient string `json:"recipient" binding:"required"`
	Channel   string `json:"channel"`
	Locale    string `json:"locale"`
}
```

In `handlers.go` `Send`, pass it through:
```go
	res, err := h.svc.Send(c.Request.Context(), app.SendInput{
		TenantID:       c.GetString(tenantCtxKey),
		Recipient:      body.Recipient,
		Channel:        body.Channel,
		Locale:         body.Locale,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/otp-api/... ./pkg/contracts/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/contracts/otp/event.go services/otp-api/internal/app/send.go services/otp-api/internal/adapters/inbound/http/dto.go services/otp-api/internal/adapters/inbound/http/handlers.go services/otp-api/internal/app/usecase_test.go
git commit -m "feat: thread locale through otp send request and event (default en)"
```

---

## Task 4: Template repo (GORM) in otp-api

**Files:**
- Create: `services/otp-api/internal/adapters/outbound/mysqltemplates/repo.go`
- Test: `services/otp-api/internal/adapters/outbound/mysqltemplates/repo_test.go` (SQLite-in-memory or dockerized MySQL; if the repo has no existing DB test harness, cover the repo through the app-layer fakes in Task 5 instead and keep this test minimal/skipped-by-tag)

**Interfaces:**
- Produces `*mysqltemplates.Repo` implementing the `app.TemplateRepo` port defined in Task 5:
  - `List(ctx) ([]app.Template, error)`
  - `Get(ctx, id string) (app.Template, []app.TemplateVersion, error)`
  - `Create(ctx, t app.Template, first app.TemplateVersion) error`
  - `AddVersion(ctx, v app.TemplateVersion) (versionNo int, err error)`
  - `Publish(ctx, templateID, versionID string) (channel, locale string, err error)`

- [ ] **Step 1: Write repo with GORM row models**

```go
// Package mysqltemplates is otp-api's outbound persistence adapter for template CRUD.
// It implements app.TemplateRepo over MySQL via GORM. Publish is transactional: it
// flips version statuses and repoints templates.active_version_id atomically.
package mysqltemplates

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type templateRow struct {
	ID              string
	Name            string
	Channel         string
	Locale          string
	Status          string
	ActiveVersionID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (templateRow) TableName() string { return "templates" }

type versionRow struct {
	ID         string
	TemplateID string
	VersionNo  int
	Subject    string
	Body       string
	Status     string
	Note       string
	CreatedBy  string
	CreatedAt  time.Time
}

func (versionRow) TableName() string { return "template_versions" }

func (r *Repo) List(ctx context.Context) ([]app.Template, error) {
	var rows []templateRow
	if err := r.db.WithContext(ctx).Order("channel, locale").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]app.Template, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAppTemplate(row))
	}
	return out, nil
}

func (r *Repo) Get(ctx context.Context, id string) (app.Template, []app.TemplateVersion, error) {
	var t templateRow
	if err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return app.Template{}, nil, app.ErrTemplateNotFound
		}
		return app.Template{}, nil, err
	}
	var vs []versionRow
	if err := r.db.WithContext(ctx).Where("template_id = ?", id).Order("version_no DESC").Find(&vs).Error; err != nil {
		return app.Template{}, nil, err
	}
	versions := make([]app.TemplateVersion, 0, len(vs))
	for _, v := range vs {
		versions = append(versions, toAppVersion(v))
	}
	return toAppTemplate(t), versions, nil
}

func (r *Repo) Create(ctx context.Context, t app.Template, first app.TemplateVersion) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&versionRow{
			ID: first.ID, TemplateID: t.ID, VersionNo: 1, Subject: first.Subject,
			Body: first.Body, Status: "draft", Note: first.Note, CreatedBy: first.CreatedBy,
			CreatedAt: first.CreatedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&templateRow{
			ID: t.ID, Name: t.Name, Channel: t.Channel, Locale: t.Locale,
			Status: "active", ActiveVersionID: nil, CreatedAt: t.CreatedAt, UpdatedAt: t.CreatedAt,
		}).Error
	})
}

func (r *Repo) AddVersion(ctx context.Context, v app.TemplateVersion) (int, error) {
	var next int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var max struct{ N int }
		if err := tx.Model(&versionRow{}).Select("COALESCE(MAX(version_no),0) AS n").
			Where("template_id = ?", v.TemplateID).Scan(&max).Error; err != nil {
			return err
		}
		next = max.N + 1
		return tx.Create(&versionRow{
			ID: v.ID, TemplateID: v.TemplateID, VersionNo: next, Subject: v.Subject,
			Body: v.Body, Status: "draft", Note: v.Note, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
		}).Error
	})
	return next, err
}

func (r *Repo) Publish(ctx context.Context, templateID, versionID string) (string, string, error) {
	var channel, locale string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t templateRow
		if err := tx.First(&t, "id = ?", templateID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return app.ErrTemplateNotFound
			}
			return err
		}
		var v versionRow
		if err := tx.First(&v, "id = ? AND template_id = ?", versionID, templateID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return app.ErrVersionNotFound
			}
			return err
		}
		// Supersede the currently published version, publish the chosen one, repoint active.
		if err := tx.Model(&versionRow{}).Where("template_id = ? AND status = ?", templateID, "published").
			Update("status", "superseded").Error; err != nil {
			return err
		}
		if err := tx.Model(&versionRow{}).Where("id = ?", versionID).Update("status", "published").Error; err != nil {
			return err
		}
		if err := tx.Model(&templateRow{}).Where("id = ?", templateID).
			Updates(map[string]any{"active_version_id": versionID, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		channel, locale = t.Channel, t.Locale
		return nil
	})
	return channel, locale, err
}

func toAppTemplate(r templateRow) app.Template {
	av := ""
	if r.ActiveVersionID != nil {
		av = *r.ActiveVersionID
	}
	return app.Template{
		ID: r.ID, Name: r.Name, Channel: r.Channel, Locale: r.Locale, Status: r.Status,
		ActiveVersionID: av, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toAppVersion(r versionRow) app.TemplateVersion {
	return app.TemplateVersion{
		ID: r.ID, TemplateID: r.TemplateID, VersionNo: r.VersionNo, Subject: r.Subject,
		Body: r.Body, Status: r.Status, Note: r.Note, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
	}
}
```

- [ ] **Step 2: Build**

Run: `go build ./services/otp-api/...`
Expected: fails until Task 5 defines `app.Template`, `app.TemplateVersion`, `app.TemplateRepo`, and the error sentinels. **Do Task 5 Step 1-3 (types + ports) before compiling**, or define the types first. (These two tasks are tightly coupled; if executing in isolation, pull the type/port block from Task 5 into this task.)

- [ ] **Step 3: Commit** (after Task 5 types exist and it builds)

```bash
git add services/otp-api/internal/adapters/outbound/mysqltemplates
git commit -m "feat: mysql template repo with transactional publish"
```

---

## Task 5: Template use cases + preview in otp-api app layer

**Files:**
- Create: `services/otp-api/internal/app/template.go`
- Test: `services/otp-api/internal/app/template_test.go`

**Interfaces:**
- Produces (consumed by Tasks 4 and 6):
  - Types: `Template{ID,Name,Channel,Locale,Status,ActiveVersionID string; CreatedAt,UpdatedAt time.Time}`, `TemplateVersion{ID,TemplateID string; VersionNo int; Subject,Body,Status,Note,CreatedBy string; CreatedAt time.Time}`.
  - Errors: `ErrTemplateNotFound`, `ErrVersionNotFound`, `ErrTemplateExists` (unique channel+locale), plus templating validation errors surfaced as-is.
  - Port: `TemplateRepo` (methods per Task 4) and `TemplateCache interface { Invalidate(ctx, channel, locale string) error }`.
  - Service: `TemplateService` with `List/Get/Create/AddVersion/Publish/Preview`.

- [ ] **Step 1: Write the failing test**

```go
package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/duykhanh/worklane/pkg/templating"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

func TestCreateRejectsUnknownVariable(t *testing.T) {
	svc := app.NewTemplateService(&fakeTemplateRepo{}, &fakeCache{}, fixedClock{}, fixedID{})
	_, err := svc.Create(context.Background(), app.CreateTemplateInput{
		Name: "Welcome", Channel: "email", Locale: "en",
		Subject: "Hi", Body: "Hello {{name}}", Author: "a@b.co",
	})
	if !errors.Is(err, templating.ErrUnknownVariable) {
		t.Fatalf("want ErrUnknownVariable, got %v", err)
	}
}

func TestPublishInvalidatesCache(t *testing.T) {
	repo := &fakeTemplateRepo{publishChannel: "email", publishLocale: "en"}
	cache := &fakeCache{}
	svc := app.NewTemplateService(repo, cache, fixedClock{}, fixedID{})
	if err := svc.Publish(context.Background(), "tpl1", "ver2"); err != nil {
		t.Fatal(err)
	}
	if cache.invalidated != "email/en" {
		t.Fatalf("publish must invalidate the (channel,locale) cache key, got %q", cache.invalidated)
	}
}

func TestPreviewRendersThroughEngine(t *testing.T) {
	svc := app.NewTemplateService(&fakeTemplateRepo{}, &fakeCache{}, fixedClock{}, fixedID{})
	sub, body := svc.Preview("Code {{code}}", "It is {{code}}, expires {{expiry}}.")
	if sub != "Code 123456" || body != "It is 123456, expires 5 minutes." {
		t.Fatalf("preview mismatch: %q / %q", sub, body)
	}
}
```

> Provide small fakes (`fakeTemplateRepo`, `fakeCache`, `fixedClock`, `fixedID`) in the test file. `fakeCache.Invalidate` records `channel+"/"+locale`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/app/...`
Expected: FAIL (types/service undefined).

- [ ] **Step 3: Implement**

```go
package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/pkg/templating"
)

var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrVersionNotFound  = errors.New("template version not found")
	ErrTemplateExists   = errors.New("template already exists for channel+locale")
)

type Template struct {
	ID, Name, Channel, Locale, Status, ActiveVersionID string
	CreatedAt, UpdatedAt                               time.Time
}

type TemplateVersion struct {
	ID, TemplateID       string
	VersionNo            int
	Subject, Body, Status, Note, CreatedBy string
	CreatedAt            time.Time
}

// TemplateRepo is the durable store for templates (MySQL).
type TemplateRepo interface {
	List(ctx context.Context) ([]Template, error)
	Get(ctx context.Context, id string) (Template, []TemplateVersion, error)
	Create(ctx context.Context, t Template, first TemplateVersion) error
	AddVersion(ctx context.Context, v TemplateVersion) (versionNo int, err error)
	Publish(ctx context.Context, templateID, versionID string) (channel, locale string, err error)
}

// TemplateCache lets Publish invalidate the dispatcher's cache-aside key.
type TemplateCache interface {
	Invalidate(ctx context.Context, channel, locale string) error
}

// IDGen abstracts id creation for tests.
type IDGen interface{ New() string }

type TemplateService struct {
	repo  TemplateRepo
	cache TemplateCache
	clock Clock
	ids   IDGen
}

func NewTemplateService(repo TemplateRepo, cache TemplateCache, clock Clock, ids IDGen) *TemplateService {
	return &TemplateService{repo: repo, cache: cache, clock: clock, ids: ids}
}

type CreateTemplateInput struct {
	Name, Channel, Locale, Subject, Body, Note, Author string
}

func (s *TemplateService) Create(ctx context.Context, in CreateTemplateInput) (Template, error) {
	if err := templating.Validate(in.Channel, in.Subject, in.Body); err != nil {
		return Template{}, err
	}
	now := s.clock.Now()
	t := Template{ID: s.ids.New(), Name: in.Name, Channel: in.Channel, Locale: in.Locale,
		Status: "active", CreatedAt: now, UpdatedAt: now}
	first := TemplateVersion{ID: s.ids.New(), TemplateID: t.ID, VersionNo: 1, Subject: in.Subject,
		Body: in.Body, Status: "draft", Note: in.Note, CreatedBy: in.Author, CreatedAt: now}
	if err := s.repo.Create(ctx, t, first); err != nil {
		return Template{}, err
	}
	return t, nil
}

type AddVersionInput struct {
	TemplateID, Subject, Body, Note, Author string
}

func (s *TemplateService) AddVersion(ctx context.Context, in AddVersionInput) (TemplateVersion, error) {
	t, _, err := s.repo.Get(ctx, in.TemplateID)
	if err != nil {
		return TemplateVersion{}, err
	}
	if err := templating.Validate(t.Channel, in.Subject, in.Body); err != nil {
		return TemplateVersion{}, err
	}
	v := TemplateVersion{ID: s.ids.New(), TemplateID: in.TemplateID, Subject: in.Subject,
		Body: in.Body, Status: "draft", Note: in.Note, CreatedBy: in.Author, CreatedAt: s.clock.Now()}
	no, err := s.repo.AddVersion(ctx, v)
	if err != nil {
		return TemplateVersion{}, err
	}
	v.VersionNo = no
	return v, nil
}

func (s *TemplateService) Publish(ctx context.Context, templateID, versionID string) error {
	channel, locale, err := s.repo.Publish(ctx, templateID, versionID)
	if err != nil {
		return err
	}
	// Best-effort cache invalidation; a stale cache self-heals on TTL, so a cache error
	// must not fail an otherwise-successful publish.
	_ = s.cache.Invalidate(ctx, channel, locale)
	return nil
}

func (s *TemplateService) List(ctx context.Context) ([]Template, error) { return s.repo.List(ctx) }

func (s *TemplateService) Get(ctx context.Context, id string) (Template, []TemplateVersion, error) {
	return s.repo.Get(ctx, id)
}

// Preview renders through the exact same engine the dispatcher uses, with sample values,
// so what an author sees is what a recipient gets.
func (s *TemplateService) Preview(subject, body string) (string, string) {
	return templating.Render(subject, body, templating.Vars{Code: "123456", Expiry: "5 minutes"})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/otp-api/internal/app/...`
Expected: PASS. Then `go build ./services/otp-api/...` (Task 4 repo now compiles).

- [ ] **Step 5: Commit**

```bash
git add services/otp-api/internal/app/template.go services/otp-api/internal/app/template_test.go
git commit -m "feat: template use cases (create/version/publish/preview) with validation"
```

---

## Task 6: Template HTTP API + Redis invalidation + JWT author guard

**Files:**
- Create: `services/otp-api/internal/adapters/inbound/http/templates.go`
- Create: `services/otp-api/internal/adapters/inbound/http/templates_dto.go`
- Test: `services/otp-api/internal/adapters/inbound/http/templates_test.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/middleware.go` (stash author email)
- Modify: `services/otp-api/internal/adapters/inbound/http/router.go` (routes + inject TemplateService)
- Create: `services/otp-api/internal/adapters/outbound/redisstore/templatecache.go` (implements `app.TemplateCache`)
- Modify: `services/otp-api/main.go` (build TemplateService + cache, pass to router)

**Interfaces:**
- Consumes: `app.TemplateService` (Task 5), `app.TemplateCache`.
- Produces: routes `GET/POST /v1/templates`, `GET /v1/templates/:id`, `POST /v1/templates/:id/versions`, `POST /v1/templates/:id/versions/:vid/publish`, `POST /v1/templates/preview`; context key `authorCtxKey = "actor_email"`.

- [ ] **Step 1: Write the failing test**

```go
func TestCreateTemplateRequiresJWT(t *testing.T) {
	r := newTestRouterWithAPIKeyCaller(t) // authenticate sets tenant but no actor_email
	w := doJSON(r, "POST", "/v1/templates", `{"name":"X","channel":"email","locale":"en","subject":"S","body":"{{code}}"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("api-key caller must be forbidden from template CRUD, got %d", w.Code)
	}
}

func TestPreviewRendersSampleValues(t *testing.T) {
	r := newTestRouterWithJWT(t)
	w := doJSON(r, "POST", "/v1/templates/preview", `{"channel":"email","subject":"Code {{code}}","body":"expires {{expiry}}"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}
	// body should contain rendered "123456" and "5 minutes"
	if !strings.Contains(w.Body.String(), "123456") || !strings.Contains(w.Body.String(), "5 minutes") {
		t.Fatalf("preview not rendered: %s", w.Body.String())
	}
}
```

> Reuse the existing `templates_test.go`/`handlers_test.go` harness style for building a router with a fake `TemplateService`. Provide a fake that records calls.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/adapters/inbound/http/...`
Expected: FAIL (routes/handlers undefined).

- [ ] **Step 3: Implement**

In `middleware.go`, add an author key and set it on the JWT path only:
```go
const authorCtxKey = "actor_email"
// ... inside the JWT branch, after c.Set(tenantCtxKey, claims.TenantID):
c.Set(authorCtxKey, claims.Email)
```

`templates_dto.go`:
```go
package http

import "time"

type createTemplateRequest struct {
	Name    string `json:"name" binding:"required"`
	Channel string `json:"channel" binding:"required"`
	Locale  string `json:"locale" binding:"required"`
	Subject string `json:"subject"`
	Body    string `json:"body" binding:"required"`
	Note    string `json:"note"`
}

type addVersionRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body" binding:"required"`
	Note    string `json:"note"`
}

type previewRequest struct {
	Channel string `json:"channel"`
	Subject string `json:"subject"`
	Body    string `json:"body" binding:"required"`
}

type templateDTO struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Channel         string    `json:"channel"`
	Locale          string    `json:"locale"`
	Status          string    `json:"status"`
	ActiveVersionID string    `json:"active_version_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type versionDTO struct {
	ID        string    `json:"id"`
	VersionNo int       `json:"version_no"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type templateDetailDTO struct {
	Template templateDTO  `json:"template"`
	Versions []versionDTO `json:"versions"`
}

type previewResponse struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
```

`templates.go` (handlers). The author guard returns 403 when `actor_email` is empty (API-key caller):
```go
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/templating"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// TemplateAPI is the inbound port the template endpoints need.
type TemplateAPI interface {
	List(ctx context.Context) ([]app.Template, error)
	Get(ctx context.Context, id string) (app.Template, []app.TemplateVersion, error)
	Create(ctx context.Context, in app.CreateTemplateInput) (app.Template, error)
	AddVersion(ctx context.Context, in app.AddVersionInput) (app.TemplateVersion, error)
	Publish(ctx context.Context, templateID, versionID string) error
	Preview(subject, body string) (string, string)
}

type TemplateHandlers struct{ svc TemplateAPI }

func author(c *gin.Context) (string, bool) {
	email := c.GetString(authorCtxKey)
	if email == "" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "template management requires a user login"})
		return "", false
	}
	return email, true
}

func (h *TemplateHandlers) Create(c *gin.Context) {
	email, ok := author(c)
	if !ok {
		return
	}
	var body createTemplateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.svc.Create(c.Request.Context(), app.CreateTemplateInput{
		Name: body.Name, Channel: body.Channel, Locale: body.Locale,
		Subject: body.Subject, Body: body.Body, Note: body.Note, Author: email,
	})
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toTemplateDTO(t))
}

func (h *TemplateHandlers) Preview(c *gin.Context) {
	if _, ok := author(c); !ok {
		return
	}
	var body previewRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := templating.Validate(body.Channel, body.Subject, body.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, rendered := h.svc.Preview(body.Subject, body.Body)
	c.JSON(http.StatusOK, previewResponse{Subject: sub, Body: rendered})
}

// List, Get, AddVersion, Publish follow the same shape (author guard, bind, call svc,
// map result). Publish returns 204 on success.

func writeTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, app.ErrTemplateNotFound), errors.Is(err, app.ErrVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, templating.ErrUnknownVariable), errors.Is(err, templating.ErrSubjectRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
```

> Implement `List`, `Get`, `AddVersion`, `Publish` handlers and the `toTemplateDTO`/`toVersionDTO` mappers in the same file, mirroring `Create`.

`redisstore/templatecache.go`:
```go
package redisstore

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// TemplateCache implements app.TemplateCache: it deletes the dispatcher's cache-aside
// key so a published change is re-read on the next send.
type TemplateCache struct{ rc *redis.Client }

func NewTemplateCache(rc *redis.Client) *TemplateCache { return &TemplateCache{rc: rc} }

func (c *TemplateCache) Invalidate(ctx context.Context, channel, locale string) error {
	return c.rc.Del(ctx, fmt.Sprintf("tmpl:%s:%s", channel, locale)).Err()
}
```
> Confirm the redis client type used by `redisstore` (it wraps `*redis.Client` from `pkg/platform/redis`); match the existing constructor signature.

`router.go` - add the template group inside the authed `v1` block:
```go
func NewRouter(svc OTPService, repo app.Repo, tsvc TemplateAPI, verifier *security.Verifier, intro app.Introspector) *gin.Engine {
	// ... existing setup ...
	th := &TemplateHandlers{svc: tsvc}
	{
		// existing otp routes ...
		v1.GET("/templates", th.List)
		v1.POST("/templates", th.Create)
		v1.GET("/templates/:id", th.Get)
		v1.POST("/templates/:id/versions", th.AddVersion)
		v1.POST("/templates/:id/versions/:vid/publish", th.Publish)
		v1.POST("/templates/preview", th.Preview)
	}
}
```

`main.go` - build the service and cache and pass to the router:
```go
	tcache := redisstore.NewTemplateCache(rc)
	tsvc := app.NewTemplateService(mysqltemplates.New(db), tcache, realClock{}, idGen{})
	// ...
	Handler: otphttp.NewRouter(svc, repo, tsvc, verifier, introspector),
```
> Add a small `idGen` type in main.go using the existing `newID()` approach (`crypto/rand` hex) to satisfy `app.IDGen`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/otp-api/...`
Expected: PASS. Then `go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add services/otp-api
git commit -m "feat: template CRUD + preview HTTP API, JWT author guard, redis invalidation"
```

---

## Task 7: Dispatcher reads templates via cache-aside with env fallback

**Files:**
- Modify: `services/otp-dispatcher/internal/app/ports.go` (Sender signature + new `TemplateSource` port + `Template` type)
- Modify: `services/otp-dispatcher/internal/app/sender.go` (senders become pure delivery)
- Modify: `services/otp-dispatcher/internal/app/handler.go` (resolve + render before send)
- Delete/replace: `services/otp-dispatcher/internal/app/template.go` (printf Render removed)
- Create: `services/otp-dispatcher/internal/adapters/outbound/templatestore/store.go` (MySQL + Redis cache-aside)
- Test: `services/otp-dispatcher/internal/app/handler_test.go` (extend), `services/otp-dispatcher/internal/adapters/outbound/templatestore/store_test.go`
- Modify: `services/otp-dispatcher/main.go` (open Redis, build TemplateSource, env fallback map)

**Interfaces:**
- Produces:
  - `type Template struct { Subject, Body string }`
  - `type TemplateSource interface { Active(ctx context.Context, channel, locale string) (Template, bool, error) }`
  - `Sender.Send(ctx context.Context, to, subject, body string) (msgID string, err error)`
  - Handler config gains `Fallback map[string]Template` (keyed by channel) and `ExpiryText string`.

- [ ] **Step 1: Write the failing test** (extend `handler_test.go`)

```go
func TestHandleRendersFromTemplateSource(t *testing.T) {
	email := &fakeSender{name: "resend"}
	src := &fakeTemplateSource{tpl: app.Template{Subject: "Code {{code}}", Body: "It is {{code}}, expires {{expiry}}."}, found: true}
	h := newHandlerWithTemplates(map[string]app.Sender{"email": email}, src,
		map[string]app.Template{"email": {Subject: "FB", Body: "fallback {{code}}"}}, "5 minutes")
	if err := h.Handle(context.Background(), emailEvt()); err != nil { // emailEvt has Locale "en", Code "123456"
		t.Fatal(err)
	}
	if email.lastSubject != "Code 123456" || email.lastBody != "It is 123456, expires 5 minutes." {
		t.Fatalf("rendered from DB template expected, got %q / %q", email.lastSubject, email.lastBody)
	}
}

func TestHandleFallsBackWhenNoActiveTemplate(t *testing.T) {
	email := &fakeSender{name: "resend"}
	src := &fakeTemplateSource{found: false} // no active row
	h := newHandlerWithTemplates(map[string]app.Sender{"email": email}, src,
		map[string]app.Template{"email": {Subject: "FB", Body: "fallback {{code}}"}}, "5 minutes")
	if err := h.Handle(context.Background(), emailEvt()); err != nil {
		t.Fatal(err)
	}
	if email.lastBody != "fallback 123456" {
		t.Fatalf("must fall back to env template, got %q", email.lastBody)
	}
}
```

> Update `fakeSender` to record `lastSubject/lastBody` and match the new `Send(ctx,to,subject,body)` signature. Add `fakeTemplateSource`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-dispatcher/...`
Expected: FAIL (signature/port/type mismatch).

- [ ] **Step 3: Implement**

`ports.go` - change `Sender` and add the template port + type:
```go
type Sender interface {
	Name() string
	Send(ctx context.Context, to, subject, body string) (msgID string, err error)
}

// Template is a raw (unrendered) message template.
type Template struct {
	Subject string
	Body    string
}

// TemplateSource resolves the active template for a (channel, locale). found=false means
// no active row exists, so the caller uses its env fallback.
type TemplateSource interface {
	Active(ctx context.Context, channel, locale string) (tpl Template, found bool, err error)
}
```

`sender.go` - senders now just deliver:
```go
type emailSender struct {
	p    EmailProvider
	name string
}

func NewEmailSender(p EmailProvider, name string) Sender { return &emailSender{p: p, name: name} }
func (s *emailSender) Name() string { return s.name }
func (s *emailSender) Send(ctx context.Context, to, subject, body string) (string, error) {
	return s.p.Send(ctx, to, subject, body)
}

type smsSender struct {
	p    SMSProvider
	name string
}

func NewSMSSender(p SMSProvider, name string) Sender { return &smsSender{p: p, name: name} }
func (s *smsSender) Name() string { return s.name }
func (s *smsSender) Send(ctx context.Context, to, subject, body string) (string, error) {
	_ = subject // SMS has no subject
	return s.p.Send(ctx, to, body)
}
```
Delete `template.go` (the printf `Template.Render` is replaced by `pkg/templating`).

`handler.go` - add fields to `Config` and resolve+render in `Handle`:
```go
import (
	"context"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/pkg/templating"
)

type Config struct {
	SentTopic   string
	FailedTopic string
	DLQTopic    string
	ExpiryText  string               // e.g. "5 minutes" for {{expiry}}
	Fallback    map[string]Template  // keyed by channel; used when no active DB template
}

type Deps struct {
	Senders   map[string]Sender
	Templates TemplateSource
	Repo      Repo
	Pub       Publisher
	Clock     Clock
}

// inside Handle, replacing sender.Send(ctx, evt.Recipient, evt.Code):
	locale := evt.Locale
	if locale == "" {
		locale = "en"
	}
	tpl, found, terr := h.d.Templates.Active(ctx, evt.Channel, locale)
	if terr != nil || !found {
		// Fallback keeps OTP delivery alive on any template/DB/cache problem.
		tpl = h.cfg.Fallback[evt.Channel]
	}
	subject, body := templating.Render(tpl.Subject, tpl.Body,
		templating.Vars{Code: evt.Code, Expiry: h.cfg.ExpiryText})

	start := h.d.Clock.Now()
	msgID, sendErr := sender.Send(ctx, evt.Recipient, subject, body)
	latency := h.d.Clock.Now().Sub(start).Milliseconds()
```

`templatestore/store.go` - cache-aside over MySQL + Redis:
```go
// Package templatestore implements app.TemplateSource with Redis cache-aside over the
// templates/template_versions tables. The cache key is tmpl:{channel}:{locale}; otp-api
// deletes it on publish. A short TTL is a safety net so a missed invalidation self-heals.
package templatestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

type Store struct {
	db  *gorm.DB
	rc  *redis.Client
	ttl time.Duration
}

func New(db *gorm.DB, rc *redis.Client, ttl time.Duration) *Store {
	return &Store{db: db, rc: rc, ttl: ttl}
}

func key(channel, locale string) string { return fmt.Sprintf("tmpl:%s:%s", channel, locale) }

func (s *Store) Active(ctx context.Context, channel, locale string) (app.Template, bool, error) {
	k := key(channel, locale)
	if cached, err := s.rc.Get(ctx, k).Result(); err == nil {
		var t app.Template
		if json.Unmarshal([]byte(cached), &t) == nil {
			return t, true, nil
		}
	}
	var row struct {
		Subject string
		Body    string
	}
	err := s.db.WithContext(ctx).
		Table("templates AS t").
		Select("v.subject, v.body").
		Joins("JOIN template_versions v ON v.id = t.active_version_id").
		Where("t.channel = ? AND t.locale = ? AND t.status = 'active'", channel, locale).
		Scan(&row).Error
	if err != nil {
		return app.Template{}, false, err
	}
	if row.Body == "" && row.Subject == "" {
		return app.Template{}, false, nil // no active row -> caller falls back
	}
	t := app.Template{Subject: row.Subject, Body: row.Body}
	if b, err := json.Marshal(t); err == nil {
		_ = s.rc.Set(ctx, k, b, s.ttl).Err()
	}
	return t, true, nil
}

var _ = errors.Is // keep imports tidy if unused during scaffolding
```
> Verify `gorm`'s empty-row behavior: `Scan` into a struct leaves zero-values and returns nil error when no row matches, so the `Subject==""&&Body==""` check is the "not found" signal. If preferred, use `Count` or `ErrRecordNotFound` via `First` on a model instead.

`main.go` - open Redis, build the store + fallback map + expiry text, and remove the old `emailTpl`/`smsBodyFmt` template wiring:
```go
	redisURL := config.Env("REDIS_URL", "redis://localhost:6379/0")
	rc, err := redisplatform.Open(redisURL)
	if err != nil {
		log.Fatalf("otp-dispatcher: redis: %v", err)
	}
	tmplTTL := config.EnvDuration("TEMPLATE_CACHE_TTL", 10*time.Minute)
	templates := templatestore.New(db, rc, tmplTTL)

	fallback := map[string]app.Template{
		"email": {
			Subject: config.Env("OTP_EMAIL_SUBJECT", "Your verification code"),
			Body:    config.Env("OTP_EMAIL_BODY", "Your verification code is {{code}}. It expires in {{expiry}}."),
		},
		"sms": {
			Body: config.Env("OTP_SMS_BODY", "Your verification code is {{code}}. It expires in {{expiry}}."),
		},
	}

	handler := app.NewHandler(app.Deps{
		Senders:   map[string]app.Sender{"email": app.NewEmailSender(mail, providerLabel), "sms": app.NewSMSSender(sms, "twilio")},
		Templates: templates,
		Repo:      repo, Pub: prod, Clock: realClock{},
	}, app.Config{
		SentTopic:   config.Env("KAFKA_TOPIC_SENT", "otp.sent"),
		FailedTopic: config.Env("KAFKA_TOPIC_FAILED", "otp.failed"),
		DLQTopic:    config.Env("KAFKA_TOPIC_DLQ", "otp.dlq"),
		ExpiryText:  config.Env("OTP_EXPIRY_TEXT", "5 minutes"),
		Fallback:    fallback,
	})
```
> Note: the env fallback bodies now use `{{code}}`/`{{expiry}}` (rendered by `pkg/templating`), NOT `%s`. Update `deploy/k8s/base/config.yaml` and any compose env accordingly in this task.

- [ ] **Step 4: Run tests + build**

Run: `go test ./services/otp-dispatcher/... && go build ./...`
Expected: PASS / clean build.

- [ ] **Step 5: Commit**

```bash
git add services/otp-dispatcher deploy
git commit -m "refactor: dispatcher renders from DB templates via cache-aside, env fallback"
```

---

## Task 8: Dashboard types, DataSource interface, mock

**Files:**
- Modify: `dashboard/lib/api/types.ts`
- Modify: `dashboard/lib/api/source.ts`
- Modify: `dashboard/lib/api/mock.ts`
- Test: `dashboard/lib/api/mock.test.ts` (extend)

**Interfaces:**
- Produces TS types `Template`, `TemplateVersion`, `TemplateDetail`, `PreviewResult` and `DataSource` methods:
  `listTemplates()`, `getTemplate(id)`, `createTemplate(input)`, `addVersion(id, input)`, `publishVersion(id, versionId)`, `previewTemplate(input)`.

- [ ] **Step 1: Write the failing test** (mock.test.ts)

```ts
import { describe, it, expect } from "vitest";
import { MockDataSource } from "./mock";

describe("MockDataSource templates", () => {
  it("previews with sample values through the same var names", async () => {
    const ds = new MockDataSource();
    const out = await ds.previewTemplate({ channel: "email", subject: "Code {{code}}", body: "expires {{expiry}}" });
    expect(out.subject).toContain("123456");
    expect(out.body).toContain("5 minutes");
  });

  it("publishing a version updates the active version", async () => {
    const ds = new MockDataSource();
    const [t] = await ds.listTemplates();
    const detail = await ds.getTemplate(t.id);
    const draft = await ds.addVersion(t.id, { subject: detail.template.name, body: "New {{code}}", note: "edit" });
    await ds.publishVersion(t.id, draft.id);
    const after = await ds.getTemplate(t.id);
    expect(after.template.activeVersionId).toBe(draft.id);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (in `dashboard/`): `pnpm vitest run lib/api/mock.test.ts`
Expected: FAIL (methods undefined).

- [ ] **Step 3: Implement**

Add to `types.ts`:
```ts
export type TemplateStatus = "active" | "archived";
export type VersionStatus = "draft" | "published" | "superseded";

export type Template = {
  id: string;
  name: string;
  channel: "email" | "sms";
  locale: string;
  status: TemplateStatus;
  activeVersionId: string;
  updatedAt: string;
};

export type TemplateVersion = {
  id: string;
  versionNo: number;
  subject: string;
  body: string;
  status: VersionStatus;
  note: string;
  createdBy: string;
  createdAt: string;
};

export type TemplateDetail = { template: Template; versions: TemplateVersion[] };
export type PreviewInput = { channel: string; subject: string; body: string };
export type PreviewResult = { subject: string; body: string };
export type CreateTemplateInput = { name: string; channel: "email" | "sms"; locale: string; subject: string; body: string; note?: string };
export type AddVersionInput = { subject: string; body: string; note?: string };
```

Add to `source.ts` `DataSource`:
```ts
  listTemplates(): Promise<Template[]>;
  getTemplate(id: string): Promise<TemplateDetail>;
  createTemplate(input: CreateTemplateInput): Promise<Template>;
  addVersion(id: string, input: AddVersionInput): Promise<TemplateVersion>;
  publishVersion(id: string, versionId: string): Promise<void>;
  previewTemplate(input: PreviewInput): Promise<PreviewResult>;
```

Implement in `mock.ts` (seed from an in-memory copy of the existing `lib/roadmap/templates.ts` fixture, mapped to the new shape). `previewTemplate` does an allowlist replace of `{{code}}`→`123456`, `{{expiry}}`→`5 minutes` (mirror `pkg/templating` semantics so mock and live agree).

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm vitest run lib/api/mock.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add dashboard/lib/api
git commit -m "feat(dashboard): template types + DataSource methods + mock impl"
```

---

## Task 9: Dashboard live HTTP source

**Files:**
- Modify: `dashboard/lib/api/live.ts`
- Test: `dashboard/lib/api/live.test.ts` (extend, mock `fetch`)

**Interfaces:**
- Consumes the otp-api routes from Task 6. Maps snake_case JSON to the camelCase types from Task 8.

- [ ] **Step 1: Write the failing test**

```ts
it("previewTemplate POSTs to /v1/templates/preview and maps the result", async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ subject: "Code 123456", body: "expires 5 minutes" }) });
  vi.stubGlobal("fetch", fetchMock);
  const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
  const out = await ds.previewTemplate({ channel: "email", subject: "Code {{code}}", body: "expires {{expiry}}" });
  expect(out.subject).toBe("Code 123456");
  expect(fetchMock.mock.calls[0][0]).toContain("/v1/templates/preview");
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run lib/api/live.test.ts`
Expected: FAIL.

- [ ] **Step 3: Implement** the six methods in `LiveDataSource` using the existing `get`/auth-header helpers plus a `post<T>` helper (add one if absent). Map fields: `active_version_id`→`activeVersionId`, `version_no`→`versionNo`, `created_by`→`createdBy`, `updated_at`→`updatedAt`, timestamps as ISO strings. `publishVersion` POSTs and expects 204.

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm vitest run lib/api/live.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add dashboard/lib/api/live.ts dashboard/lib/api/live.test.ts
git commit -m "feat(dashboard): live template API client"
```

---

## Task 10: Dashboard query hooks

**Files:**
- Modify: `dashboard/lib/queries/keys.ts`
- Create: `dashboard/lib/queries/use-templates.ts`

**Interfaces:**
- Produces `useTemplates()`, `useTemplate(id)`, and mutation hooks `useCreateTemplate()`, `useAddVersion(id)`, `usePublishVersion(id)`, `usePreview()`, following the existing `use-*.ts` pattern and invalidating `qk.templates` on success.

- [ ] **Step 1:** Add keys:
```ts
export const qk = {
  apiKeys: ["api-keys"] as const,
  requests: ["requests"] as const,
  logs: ["logs"] as const,
  overview: ["overview"] as const,
  templates: ["templates"] as const,
  template: (id: string) => ["templates", id] as const,
};
```

- [ ] **Step 2:** Write `use-templates.ts`:
```ts
"use client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import type { AddVersionInput, CreateTemplateInput, PreviewInput } from "@/lib/api/types";
import { qk } from "./keys";

export function useTemplates() {
  return useQuery({ queryKey: qk.templates, queryFn: () => getDataSource().listTemplates() });
}

export function useTemplate(id: string) {
  return useQuery({ queryKey: qk.template(id), queryFn: () => getDataSource().getTemplate(id), enabled: !!id });
}

export function usePublishVersion(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (versionId: string) => getDataSource().publishVersion(id, versionId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.template(id) });
      qc.invalidateQueries({ queryKey: qk.templates });
    },
  });
}

export function usePreview() {
  return useMutation({ mutationFn: (input: PreviewInput) => getDataSource().previewTemplate(input) });
}

// useCreateTemplate and useAddVersion follow usePublishVersion's shape, invalidating
// qk.templates (and qk.template(id) for versions).
```

- [ ] **Step 3:** Run: `pnpm vitest run` (existing suite still green) and `pnpm tsc --noEmit`.
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add dashboard/lib/queries
git commit -m "feat(dashboard): template query + mutation hooks"
```

---

## Task 11: Wire templates screen to the API (list, editor, preview, publish, rollback)

**Files:**
- Modify: `dashboard/components/templates/templates-view.tsx` (replace `TEMPLATES` fixture + `useState` with `useTemplates`)
- Modify: `dashboard/components/templates/template-detail.tsx` (load `useTemplate`, add editor + live preview panel via `usePreview`, publish/rollback via `usePublishVersion`, new draft via `useAddVersion`)
- Modify: `dashboard/components/templates/new-template-dialog.tsx` (submit via `useCreateTemplate`)
- Test: `dashboard/components/templates/templates-view.test.tsx` (extend to assert it renders from the mock DataSource, not the fixture)

**Interfaces:**
- Consumes the hooks from Task 10. Removes the import of `@/lib/roadmap/templates` from these components (leave the roadmap file in place; it is no longer imported by production components).

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TemplatesView } from "./templates-view";

it("lists templates from the data source", async () => {
  const qc = new QueryClient();
  render(<QueryClientProvider client={qc}><TemplatesView /></QueryClientProvider>);
  await waitFor(() => expect(screen.getByText(/OTP email/i)).toBeInTheDocument());
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run components/templates/templates-view.test.tsx`
Expected: FAIL (still bound to fixture / not wrapped in query client).

- [ ] **Step 3: Implement**
- `templates-view.tsx`: `const { data: rows = [] } = useTemplates();` drive the `DataTable` from `rows`; keep the existing columns/`Badge`/`StateBadge` styling. Remove `TEMPLATES`/`useState<Template[]>` seed.
- `template-detail.tsx`: `const { data } = useTemplate(id);` render version history from `data.versions`; add a subject/body editor with `{{code}}`/`{{expiry}}` insert chips; a **Preview** panel calling `usePreview()` on change (debounced) that shows rendered subject/body; a **Publish** button per draft version calling `usePublishVersion`; **Rollback** = publish an older version (same mutation). A **Save draft** action calls `useAddVersion`.
- `new-template-dialog.tsx`: submit via `useCreateTemplate`; on unknown-variable validation error from the API, surface the message inline.

- [ ] **Step 4: Run tests + typecheck + lint**

Run: `pnpm vitest run && pnpm tsc --noEmit && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add dashboard/components/templates
git commit -m "feat(dashboard): wire Template Studio to the API with live preview, publish, rollback"
```

---

## Task 12: Playground locale selector

**Files:**
- Modify: the Playground send form under `dashboard/components/playground/*` (locate the component that calls `send`)
- Modify: `dashboard/lib/api/source.ts` `send` signature if needed, `mock.ts`, `live.ts` to pass `locale`
- Test: the relevant playground component test (extend) or `live.test.ts` asserting `locale` is sent

**Interfaces:**
- `send(recipient, channel, locale?)` includes `locale` in the POST body to `/v1/otp/send`.

- [ ] **Step 1: Write the failing test**

```ts
it("send posts the chosen locale", async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ request_id: "r1" }) });
  vi.stubGlobal("fetch", fetchMock);
  const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
  await ds.send("d@e.com", "email", "vi");
  const bodySent = JSON.parse(fetchMock.mock.calls[0][1].body);
  expect(bodySent.locale).toBe("vi");
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run lib/api/live.test.ts`
Expected: FAIL.

- [ ] **Step 3: Implement** the optional `locale` param through `DataSource.send`, `MockDataSource.send`, `LiveDataSource.send` (add `locale` to the JSON body), and add a small locale `<Select>` (options `en`, `vi`) to the Playground send form defaulting to `en`.

- [ ] **Step 4: Run tests + typecheck**

Run: `pnpm vitest run && pnpm tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add dashboard/lib/api dashboard/components/playground
git commit -m "feat(dashboard): playground locale selector"
```

---

## Task 13: End-to-end smoke + docs

**Files:**
- Modify: `docs/architecture.md` (update data model section 5 to two tables; add a note that the dispatcher renders from DB with fallback)
- Modify: `docs/roadmap/2026-08-16-notification-platform-and-link-service.md` (mark sub-project A shipped)
- Verify: the existing compose smoke gate (`ci`/`docker compose`) still passes end-to-end.

- [ ] **Step 1: Run the full backend suite + build**

Run: `go test ./... && go build ./...`
Expected: PASS.

- [ ] **Step 2: Compose e2e smoke** (the same gate CI runs)

Run: bring up compose, seed a tenant/key, send an OTP for `email`/`en`, and confirm a `delivery_logs` row with `status=sent`; then edit + publish a template version via the API and confirm the next send reflects it (cache invalidated). Follow `docs/runbooks/*` for the exact smoke commands.
Expected: send returns `202`, delivery log `sent`, published change visible on next send.

- [ ] **Step 3: Update docs**

Update `docs/architecture.md` data model to the two-table shape and note the dispatcher's cache-aside + fallback read path. Mark Template Studio (A) as shipped in the roadmap.

- [ ] **Step 4: Commit**

```bash
git add docs
git commit -m "docs: Template Studio data model + dispatcher read path; roadmap A shipped"
```

---

## Self-review notes (author)

- **Spec coverage:** schema (T2), CRUD API (T4-T6), versioning draft→publish→rollback (T5 publish + T11 rollback), shared render engine / preview-cannot-lie (T1, T5 Preview, T6 preview route, T11 panel), dispatcher cache-aside + fallback (T7), locale threading (T3, T7, T12), dashboard swappable-data-layer wiring (T8-T11), seed (T2), rollout safety via fallback (T7 + T13 smoke). Deferred items (test-send, {{name}}/{{link}}, per-tenant, push, HTML email) are intentionally absent.
- **Coupling call-out:** Tasks 4 and 5 share the `app.Template*` types and `TemplateRepo` port; if executed by isolated subagents, implement Task 5's type/port block first (or fold it into Task 4) so both compile.
- **Type consistency:** `TemplateSource.Active` (dispatcher) returns `(Template, bool, error)`; `TemplateRepo.Publish` (otp-api) returns `(channel, locale string, error)`; cache key is `tmpl:{channel}:{locale}` in both `redisstore.TemplateCache.Invalidate` and `templatestore.Store`. Variables allowlist is `{{code}}`,`{{expiry}}` in engine, seed, mock preview, and UI chips.
