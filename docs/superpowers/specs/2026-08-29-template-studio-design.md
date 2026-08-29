# Template Studio - Design (sub-project A)

**Status:** Approved 2026-08-29. First sub-project of the
[notification platform + link service roadmap](../../roadmap/2026-08-16-notification-platform-and-link-service.md)
(gate cleared 2026-08-29: OTP email + SMS both live in prod).

**Next step:** implementation plan (writing-plans).

## 1. Context & problem

worklane is OTP-only. Today the message templates are **not** data - they are baked
into env config and read at process start:

- Dispatcher builds one email template from `OTP_EMAIL_SUBJECT` / `OTP_EMAIL_BODY`
  and one SMS body from `smsBodyFmt`, each a `printf`-style string with a single
  `%s` for the code (`services/otp-dispatcher/internal/app/sender.go`,
  `main.go`). Selection is by `channel` only, global, no locale.
- The `templates` table (`db/otp/migrations/0001_init.up.sql`:
  `id, channel, locale, subject, body`) exists but **no code reads or writes it**.
- The dashboard already ships a rich **UI-only** fixture
  (`dashboard/lib/roadmap/templates.ts`: `name, channel, locale, version, status,
  versions[], sends[]`, variables `{{code}} {{name}} {{link}} {{expiry}}`) and a
  placeholder route `dashboard/app/(app)/templates`, none of it wired to a `DataSource`.

**Goal of A:** close that gap. Promote `templates` to a real, versioned CRUD API;
let editors author templates in the dashboard with a preview that renders through
the *exact* delivery path; make the dispatcher render from the database as the
single source of truth, with a safe fallback so the live OTP flow never breaks.

**Learning goals** (this is a deliberate system-design practice project): template
**versioning**, **safe variable substitution**, **cache-aside** with invalidation,
and a **preview that cannot lie** (shared render code between preview and delivery).

## 2. Scope

**In scope**
- Two-table versioned `templates` schema (logical entity + immutable versions).
- Template CRUD + `draft → publish → rollback` lifecycle, hosted by **otp-api**.
- A shared render/validate engine (`pkg/templating`) used by both the preview
  endpoint and the dispatcher.
- Live preview endpoint that renders without saving or sending.
- Dispatcher reads the active template from the DB via **Redis cache-aside**, with
  **fallback to env-config** on miss/error.
- `locale` threaded through the OTP send request and event (default `en`).
- Seed the current env-config templates as the first published versions.

**Out of scope (deferred)**
- Test-send into the Playground (later pass).
- Variables `{{name}}` (needs user data - sub-project B) and `{{link}}` (needs the
  URL shortener - sub-project C). Only `{{code}}` and `{{expiry}}` are wired.
- Per-tenant templates (templates are global for now; no `tenant_id`).
- Push channel, HTML email format, and a notification `type`/`key` dimension (B).

## 3. Decisions (settled during brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| First-cut ambition | **Full A** (CRUD + versioning + preview + dispatcher-reads-DB) | Matches the roadmap's learning goals and the already-built UI. Test-send deferred. |
| Selection model | **Global by `(channel, locale)`** | Exercises locale selection without multi-tenant template management, honest to an OTP-only, solo stage. |
| Versioning storage | **Two tables** (`templates` + immutable `template_versions`), `active_version_id` pointer | Textbook clean, matches the UI's version history with author + note, single clear source for "what is live". |
| Variables + engine | **`{{code}}` + `{{expiry}}` only, custom allowlist replacer** | Honest to the OTP flow's available data; allowlist + unknown-var rejection is the safe-substitution learning, no template injection. |
| Dispatcher read path | **Redis cache-aside + fallback env-config** | Exercises cache-aside + invalidate-on-publish (learning); fallback keeps the live OTP flow from ever breaking on a missing/misconfigured template. |
| Publish flow | **Save draft → explicit Publish** | Lets an editor compose and preview repeatedly before a version affects the live OTP flow. |

## 4. Data model

Replaces the unused `templates` table. Both live in the **otp** database
(owned by otp-api).

```sql
-- logical template: exactly one per (channel, locale) for OTP
templates
  id                CHAR(36) PRIMARY KEY
  name              VARCHAR(120) NOT NULL      -- "OTP email"
  channel           VARCHAR(16)  NOT NULL      -- email | sms
  locale            VARCHAR(16)  NOT NULL      -- en | vi | ...
  status            VARCHAR(16)  NOT NULL      -- active | archived (dispatcher selects active only)
  active_version_id CHAR(36)     NULL          -- FK -> template_versions.id; NULL until first publish
  created_at        DATETIME     NOT NULL
  updated_at        DATETIME     NOT NULL
  UNIQUE (channel, locale)                     -- enforces the selection key

-- immutable snapshot of one edit
template_versions
  id          CHAR(36)     PRIMARY KEY
  template_id CHAR(36)     NOT NULL           -- FK -> templates.id
  version_no  INT          NOT NULL           -- 1,2,3... monotonic per template
  subject     VARCHAR(255) NOT NULL           -- email; empty for SMS
  body        TEXT         NOT NULL
  status      VARCHAR(16)  NOT NULL           -- draft | published | superseded
  note        VARCHAR(255) NOT NULL DEFAULT ''-- change note
  created_by  VARCHAR(120) NOT NULL           -- author (JWT subject/email)
  created_at  DATETIME     NOT NULL
  UNIQUE (template_id, version_no)
```

`active_version_id` is a nullable FK to `template_versions.id`. The pointer is set
only by Publish, so the FK is added after both tables exist (or left as a logical
reference, matching the repo's cross-database soft-reference convention - see
`docs/architecture.md` section 5).

**Version lifecycle**

```
create/edit ─▶ draft ──publish──▶ published (active) ──(newer publish)──▶ superseded
                                      ▲                                        │
                                      └────────────── rollback (re-publish) ───┘
```

Publish is transactional: set the chosen version `published`, set
`templates.active_version_id` to it, mark the previously-published version
`superseded`, bump `templates.updated_at`. Content is immutable - an edit always
creates a **new** draft version, never mutates an existing one.

## 5. Backend

### 5.1 Shared render engine - `pkg/templating` (new)

The one place substitution happens. Consumed by **both** the preview endpoint and
the dispatcher, so preview renders byte-for-byte what delivery renders.

```go
package templating

// Vars carries the values available in the OTP flow today.
type Vars struct {
    Code   string
    Expiry string // e.g. "5 minutes", formatted by the caller from TTL config
}

// Render substitutes the allowlisted variables in subject and body.
func Render(subject, body string, v Vars) (outSubject, outBody string, err error)

// Validate rejects unknown {{...}} tokens and enforces channel rules
// (subject required for email; single-segment length guidance for SMS).
func Validate(channel, subject, body string) error
```

- **Allowlist:** exactly `{{code}}` and `{{expiry}}`. Any other `{{token}}`
  (including `{{name}}`, `{{link}}`) fails `Validate` at author time, so unusable
  variables can never reach the delivery path.
- **Safety:** substitution is a literal allowlisted replace, never a template
  interpreter - no code execution, no field walking, no injection surface. Values
  (`code`, `expiry`) are escaped per channel; email and SMS bodies are plain text
  in A, so escaping is a passthrough today but the seam is in place for a future
  HTML format.
- Replaces the current `app.Template{Subject, BodyFmt}.Render(code)` printf path in
  the dispatcher.

### 5.2 otp-api - Template CRUD + preview API

otp-api owns the `otp` database, so it hosts the template API. **JWT-authed**
(dashboard human user); templates are global (no `tenant_id`), so any authenticated
dashboard user manages them. The machine API-key path stays OTP-send-only.

| Method & path | Purpose |
|---|---|
| `GET /v1/templates` | List logical templates + active-version summary |
| `GET /v1/templates/:id` | One template with its `versions[]` and active content |
| `POST /v1/templates` | Create a template (`name, channel, locale`) + first draft version |
| `POST /v1/templates/:id/versions` | Create a new draft version (`subject, body, note`) = edit |
| `POST /v1/templates/:id/versions/:vid/publish` | Publish a version → active; invalidate the Redis key |
| `POST /v1/templates/preview` | Render `subject/body` from supplied content + sample vars; **no save, no send** |

- Create and new-version calls run `templating.Validate` before persisting; invalid
  variables or a missing email subject return `400` with a clear message.
- Publish is the transactional lifecycle transition in section 4, and on success
  deletes the Redis cache key `tmpl:{channel}:{locale}` so the dispatcher re-reads.

### 5.3 Locale threading (touches the OTP contract - contained, backward-compatible)

- `pkg/contracts/otp` `RequestedEvent` gains `Locale string`. Empty is treated as
  `"en"` everywhere, so old in-flight events stay valid.
- otp-api `POST /v1/otp/send` accepts an optional `locale` (default `en`, validated
  against a small allowlist), and puts it on the published event.
- The dashboard Playground gains a locale selector (small addition).
- The dispatcher selects the template by `event.Locale`.

### 5.4 Dispatcher - read from DB with cache-aside + fallback

Per OTP event, to resolve the active template for `(channel, locale)`:

```
1. Redis GET tmpl:{channel}:{locale}
2. miss  → SELECT v.subject, v.body
             FROM templates t
             JOIN template_versions v ON v.id = t.active_version_id
             WHERE t.channel=? AND t.locale=? AND t.status='active'
           → on hit, SET the Redis key (with TTL) and use it
3. no row / DB or Redis error → FALLBACK to the env-config template
                                 (current OTP_EMAIL_* / smsBodyFmt)
4. render via pkg/templating.Render(subject, body, Vars{Code, Expiry})
```

The fallback guarantees the live OTP flow never fails because of a missing or
misconfigured template. Cache is invalidated by Publish (5.2), so a published
change takes effect on the next send.

## 6. Dashboard (frontend)

- **`DataSource`** (`dashboard/lib/api/source.ts`) gains: `listTemplates`,
  `getTemplate`, `createTemplate`, `createVersion`, `publishVersion`,
  `previewTemplate`. Implement in both `mock.ts` (reusing the existing
  `lib/roadmap/templates.ts` shape) and the live HTTP source, so screens stay
  source-agnostic - the established swappable-data-layer pattern.
- **Templates page** (`app/(app)/templates`): list → detail editor (subject/body
  with variable-insert chips for `{{code}}`/`{{expiry}}`), a **live preview** panel
  that calls `previewTemplate` (the real render path), a version history with
  Publish / Rollback, and status badges.
- **Playground:** add the locale selector (5.3).
- React Query hooks in `dashboard/lib/queries/` following the existing `use-*.ts`
  pattern.

## 7. Migration & seed

- New migration under `db/otp/migrations/`: add the new columns to `templates`,
  create `template_versions`, add the `active_version_id` reference. The old
  `templates` table is effectively empty (nothing writes it), so no data backfill is
  expected; if any rows exist they are folded into a first `published` version.
- **Seed** the current env-config templates (OTP email `en`, OTP SMS `en`) as the
  first published versions, so the DB is the source of truth from day one and
  env-config is only the fallback.
- Follow the database-migrations conventions already in the repo (up/down pair).

## 8. Testing (TDD)

- `pkg/templating`: allowlist substitution, unknown-variable rejection, expiry
  formatting, per-channel `Validate` rules.
- otp-api: CRUD handlers, publish lifecycle (draft→published→superseded, active
  pointer), validation failures, preview output, Redis key invalidation on publish.
- dispatcher: template selection by `(channel, locale)`, cache-aside hit/miss,
  fallback to env-config on missing row / error, render equivalence with preview.
- frontend: vitest for the mock `DataSource` methods and the templates screen
  components, per the existing `dashboard/test/` pattern.

## 9. Rollout (zero OTP downtime)

The env-config fallback makes every step independently safe:

1. Apply migration + seed.
2. Deploy otp-api (adds the template API; OTP send unaffected).
3. Deploy the dispatcher (reads DB + cache-aside, falls back to env-config).

No step opens a window where a template problem can break the live OTP flow.

## 10. Open items to confirm at plan time

- Redis cache TTL for `tmpl:{channel}:{locale}` (short, e.g. 5-10 min) vs
  invalidate-only. Leaning: both - a TTL as a safety net plus explicit invalidation
  on publish.
- Exact `locale` allowlist (`en`, `vi`, ...) and the dashboard selector's options.
- Whether `archive` needs its own endpoint in A or can wait (leaning: wait).
```

