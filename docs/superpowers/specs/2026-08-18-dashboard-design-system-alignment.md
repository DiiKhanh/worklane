# Dashboard - design-system alignment & roadmap screens

- **Date:** 2026-08-18
- **Status:** Historical design note; implemented and evolved by 2026-08-28
- **Area:** `dashboard/` (Next.js App Router frontend)
- **Source of truth:** `worklane Design System` kit (`ui_kits/dashboard/`), which was
  itself extracted from this repo's `dashboard/` tree.
- **Current state:** dashboard uses auth-svc email/password sign-in, stores a JWT, and the roadmap
  screens exist in the UI but are not backed by live backend APIs yet. See
  [dashboard gallery](../../dashboard-gallery.md) and `dashboard/README.md`.

## Context

The `worklane Design System` was derived *from* this dashboard, so the five shipped
screens (Overview, API keys, OTP requests, Delivery logs, Playground) already match
its tokens, radii, type and motion. The kit adds material the repo does not yet have:

1. **Request detail** - a row → detail interaction on the existing OTP requests screen
   (grounded in real data).
2. **Templates**, **Links**, **Campaigns** - new screens flagged in the kit as roadmap
   proposals, not backed by any API.
3. **Login** - a proposed OTP-loop sign-in; the real dashboard authenticates with a
   bearer key from an env var and has no session UI.

This spec aligns the dashboard with the design system by porting these into the repo's
real conventions.

## Goals

- Add the request-detail interaction to OTP requests, grounded in real request + log data.
- Add Templates, Links and Campaigns as roadmap screens driven by **local fixtures**,
  each carrying the kit's dashed "roadmap, not in the codebase yet" notice.
- Add a standalone Login roadmap page, unwired from auth.
- Keep the five shipped screens as-is apart from a light drift audit.
- Match the repo's conventions exactly (Tailwind + `components/ui/` shadcn base-nova +
  TanStack Query/Table + Zustand + Recharts + Lucide). The kit's inline-style prototype
  code is a reference for layout and copy, **not** code to copy verbatim.

## Non-goals

- No backend, `DataSource`, mock/live adapter, or API contract work for the roadmap
  screens. They are UI-only and read local fixtures directly.
- No real authentication, session, or route protection for Login.
- No rework of the five shipped screens beyond small drift fixes.
- No CSV parsing, real scheduling, or real send logic in the campaign composer - the
  estimates are computed from fixture numbers only.

## Guiding principle

Port the kit's **layout, copy and interaction**, re-expressed in the repo's stack:
Tailwind utility classes and the existing `components/common/*` + `components/ui/*`
components, client screens with local state, Lucide icons via `lucide-react`. Copy is
reproduced verbatim from the kit (developer-tool voice: sentence case, no emoji, middot
separators, identifiers in mono).

## Architecture

### Routing (App Router)

| Route | Type | Data |
| --- | --- | --- |
| `/requests` | existing, enhanced | real (TanStack Query) |
| `/templates` | new, in shell | local fixtures |
| `/campaigns` | new, in shell | local fixtures |
| `/links` | new, in shell | local fixtures |
| `/login` | new, standalone (no shell) | none (presentational) |

`dashboard/AGENTS.md` warns this Next.js version has breaking changes; read
`node_modules/next/dist/docs/` before writing any route/layout code.

### New UI primitives

`components/ui/` currently lacks three shadcn parts the new screens need. Add them via
the shadcn CLI in the repo's `base-nova` style so they match the existing layer:

- `dialog` - New template / New campaign / send-confirm dialogs.
- `select` - channel / locale / audience / template pickers.
- `textarea` - template and campaign message bodies.

(`tabs`, `separator`, `input`, `label`, `button`, `badge`, `card`, `table`, `tooltip`
already exist.)

### 1. Request detail (real data)

Inline master→detail: clicking a request row swaps `RequestsView` for a detail view with
a back button ("← OTP requests"), matching the kit's wired `Requests.jsx`.

- `components/common/data-table.tsx` gains an **optional** `onRowClick?: (row) => void`
  prop and, when present, renders rows as buttons/clickable with an appended chevron
  column. Additive and backward-compatible - existing call sites (`api-keys`, `logs`,
  detail sub-tables) pass nothing and are unchanged.
- Detail view: `SectionHeading` (title "Request detail", description = request id,
  action = `StateBadge`), then a two-column grid of two `Panel`s:
  - **Lifecycle** - a vertical timeline derived from the request `state`. Stages:
    requested → sent → verified, with failed/expired stopping short. Timeline text pulls
    real values from the joined `DeliveryLog` where available (provider, latency, error).
  - **Attributes** - id (with `CopyButton`), recipient, channel, provider, latency,
    created (`timeAgo`). Fields the repo has no real source for (template id, attempt
    count) are **omitted**, not faked.
- The joined log comes from the existing logs query via a `logByRequest(id)` selector
  (find the log whose `requestId` matches) - no new query or endpoint. Server data stays
  in the TanStack cache; only the "which row is open" bit is component state.

### 2–4. Templates / Links / Campaigns (roadmap, local fixtures)

Fixtures live per-feature under `lib/roadmap/`:

- `lib/roadmap/templates.ts` - `Template` type + `TEMPLATES` array (ported shapes: id,
  name, channel, locale, version, status, subject, body, versions[], sends[]).
- `lib/roadmap/links.ts` - `Link` type + `LINKS` array (code, target, clicks, ctr,
  created, 14-day `series`, `recent[]`), plus the summary KPI numbers.
- `lib/roadmap/campaigns.ts` - `Campaign`, `Audience`, `CampaignTemplate` types +
  `CAMPAIGNS`, `AUDIENCES`, `CAMPAIGN_TEMPLATES` arrays.

Each screen is a client component holding `useState` for list ↔ detail ↔ composer, reading
its fixtures directly. None touch `lib/api/` or `DataSource`.

A shared `components/common/roadmap-note.tsx` renders the dashed notice
("Roadmap screen. Not in the codebase yet - laid out only from shipped worklane
patterns.") used by all three list views.

**Templates** (`/templates`, `components/templates/*`)
- List: `DataTable` (name+id, channel badge, locale, version, status, updated, chevron)
  with a "New template" action opening a `Dialog` (name, channel, locale, start-from).
- Detail: editor (subject + body `Textarea`, variable-insert chips, `{{var}}` highlight),
  live preview (`{{code}}`/`{{name}}`/`{{link}}` substituted), version history, and a
  "recent sends" sub-table.

**Links** (`/links`, `components/links/*`)
- List: four KPI `StatCard`s (with `CountUp`), a "shorten a URL" input + button, and a
  `DataTable` (short code + copy, target, clicks, ctr, created, chevron).
- Detail: four KPI cards, a **Recharts** clicks-over-time area chart (14 days, matching
  `components/charts/sends-area.tsx` styling - gradient fill, dashed gridlines, muted
  ticks), and a recent-clicks sub-table. Uses Recharts, not the kit's hand-SVG.

**Campaigns** (`/campaigns`, `components/campaigns/*`)
- List: `DataTable` (name+id, type badge, channel, audience, recipients, status, sent,
  chevron); status maps `sent`→verified badge, `sending`→sent badge, `scheduled`/`draft`
  → outline/secondary `Badge`. "New campaign" action → composer.
- Composer: audience panel (list select + CSV-upload affordance), message panel (template
  select, from-name, subject, body, test-send), delivery panel (send-now / schedule tabs,
  throttle), and a sticky **Review** panel with live recipient-scope and send-window
  estimates, plus a send-confirm `Dialog`. All estimates are pure functions of fixture
  numbers (`size`, `rate` → minutes; scope from selected audience/CSV).
- Detail: four outcome `StatCard`s (recipients/delivered/opened/clicked with % of scope)
  and a recent-sends sub-table.

### 5. Login (roadmap, standalone)

`/login` - its own page outside the shell (no sidebar/topbar), centered card: wordmark
dot + "worklane", a `Panel` with the two-step email → 6-digit-code form, and the flag
"Roadmap screen - the repository authenticates with a bearer key, not a session."
Local `useState` only; submitting does nothing real (no redirect, no auth).

### Nav

Append to `NAV_ITEMS` in `components/shell/nav.tsx` after Playground:
`{ /templates, "Templates", FileText }`, `{ /campaigns, "Campaigns", Megaphone }`,
`{ /links, "Links", Link }`, each rendered with a muted "soon" tag to echo the kit.
Login is **not** added to nav (pre-auth).

### Existing-screen audit

Diff Overview, API keys, OTP requests (list portion), Delivery logs and Playground against
the DS specimen/guideline cards. Fix only concrete drift (a spacing step, a token, a copy
string). Record findings; if nothing drifted, say so. No refactors.

## Data model (fixtures)

Types are local to `lib/roadmap/` and mirror the kit's `data.js`. Recipients stay masked
(`d***@gmail.com`); identifiers render verbatim in mono. Fixture "time ago" strings are
literal (e.g. "2h ago") - the roadmap screens do not compute from timestamps, matching the
kit.

## Error handling & edge cases

- Request detail: a request with no matching log (e.g. `state === "requested"`) shows the
  lifecycle's first stage only and omits provider/latency/error rows. No crash on missing
  log.
- Empty fixture lists (e.g. a template with no sends) render the kit's inline empty text,
  not a broken table.
- Composer with no audience / empty CSV → recipient scope 0, send button disabled, window
  shows "-". Throttle of 0 is guarded (no divide-by-zero; window falls back to "-").
- All new interactive controls keep the DS interaction states (focus ring, press scale,
  disabled 50%) inherited from the shared components - no bespoke state styling.

## Testing

Unit (vitest + testing-library), focused on real logic, not presentational markup:

- Lifecycle-stage derivation from `OtpState` (each state → expected reached stage /
  failure branch).
- Campaign composer math: recipient scope from audience vs CSV; send-window minutes from
  size/rate; disabled-send and "-" window on zero scope/rate.
- Template preview substitution and `{{var}}` highlighting.
- Link CTR / number formatting.
- Render smoke tests: each new screen mounts and shows its roadmap note; request detail
  renders from a fixture request+log.

`DataTable onRowClick` gets a test (click fires callback; absence renders no chevron and
no click handler).

Gate: `pnpm lint`, `pnpm typecheck`, `pnpm test` all clean.

## Screenshots / docs

Extend `scripts/capture.ts` and `docs/dashboard-gallery.md` with the new screens
(templates list + detail, links list + detail, campaigns list + composer + detail, login,
request detail). Captures run with reduced motion for deterministic images.

## Out of scope / future

- Wiring roadmap screens into a real `DataSource` (mock + live) - deferred until the
  platform generalization work begins (gated on OTP prod launch per the roadmap).
- Real auth/session for Login.
- CSV parsing and real campaign scheduling/sending.
