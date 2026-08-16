# OTP Dashboard - Design Spec

Date: 2026-08-16
Status: Approved (brainstorming)
Supersedes/extends: §8 of [2026-08-06-otp-verification-platform-design.md](./2026-08-06-otp-verification-platform-design.md)
and the dashboard row of [2026-08-07-otp-mvp-scope.md](./2026-08-07-otp-mvp-scope.md).

## 1. Goal

Build the `worklane` developer dashboard: a polished, dark-first Next.js app for an OTP /
verification platform. Two outcomes at once:

1. A **real, production-shaped app** (the one that later deploys to Vercel and wires to `otp-api`).
2. **Beautiful screens + captured screenshots** that make the project look great in docs/README.

Design-first: the app is built against a swappable data layer so it renders complete, realistic
screens immediately (mock fixtures), and is wired to the live Go API later by switching one adapter.

## 2. Location & stack

- **Location:** new Next.js app at repo root `dashboard/` (deploys separately to Vercel).
- **Framework:** Next.js (App Router, TypeScript).
- **UI:** Tailwind CSS + shadcn/ui (Radix), started from a shadcn dashboard block, re-styled for taste.
- **Server state:** TanStack Query (caching, dedup, `refetchInterval` polling).
- **UI state:** Zustand (filters, theme, in-memory API token) - **never** server data.
- **Tables:** TanStack Table (headless sort/filter/pagination).
- **Forms:** React Hook Form + Zod.
- **Charts:** Recharts, styled per the `dataviz` skill (theme-aware, semantic palette).
- **Motion:** Framer Motion, applied per `emil-design-eng` + `animate` skills.

## 3. Data layer (swappable) - the core rule

The UI calls one interface only; it does not know whether data is mock or live.

```
dashboard/lib/api/
  types.ts     // TS types mirroring Go DTOs: ApiKey, OtpRequest, DeliveryLog (+ Overview aggregates)
  source.ts    // interface DataSource { listApiKeys, listRequests, listLogs, getOverview, send, verify }
  live.ts      // real fetch -> NEXT_PUBLIC_API_BASE + Bearer key
  mock.ts      // realistic fixtures + simulated latency + delivery-log state progression
  index.ts     // selects source via NEXT_PUBLIC_DATA_SOURCE = mock | live (default: mock)
dashboard/lib/queries/*  // useQuery/useMutation hooks that call DataSource
```

- **Separation of concerns:** server data lives only in React Query's cache; Zustand holds only
  UI state. Do not copy fetched data into Zustand.
- **Mock realism:** mock delivery-logs progress `requested -> sent` over time so the Logs screen's
  polling looks alive when captured.

### Types (mirror of Go DTOs)

- `ApiKey`   `{ id, tenantId, status }`
- `OtpRequest` `{ id, recipient (masked), channel, state }`
- `DeliveryLog` `{ requestId, provider, status, latencyMs, error? }`
- `Overview` (aggregates) - derived, see known gap below.

### Known gap (surfaced tradeoff)

Overview KPIs/charts need aggregates the API does **not** expose today (only raw lists exist).
- Mock mode: rich aggregates provided directly.
- Live mode (later): either compute client-side from the list endpoints, or add a `/v1/stats`
  endpoint. Until then, Overview aggregate tiles are **mock-only** and marked as such in the UI so
  they never pretend to be live data.

## 4. Screens (5)

Layout: left sidebar (worklane logo, nav, tenant switcher, theme toggle) + thin topbar.

| Route | Content |
|-------|---------|
| `/` **Overview** | 4 KPI cards (Sent today · Verify rate · Failed · P50 latency) with count-up; area chart of sends over time split by state; requested->sent->verified funnel; recent-activity feed. |
| `/api-keys` | TanStack Table: id (masked + copy), tenant, status badge, created. Read-only ("New key" button present but disabled - Phase 2 create/revoke). |
| `/requests` | OTP history table: request id, masked recipient, channel, state badge, created. Filter by state + search. |
| `/logs` | Delivery logs: request id, provider, status, latency (mono), error. Polling via `refetchInterval` + a blinking "live" dot; new rows animate in. |
| `/playground` | Send form (recipient) -> shows `request_id`; verify form (recipient, code) -> result. In mock mode the code is revealed (as MailHog would) to demo the full loop. |

## 5. Visual system (dark-first, developer aesthetic)

- **Background** near-black (`~#0a0a0b`), subtly elevated surfaces, **thin borders** (white / 8%).
  Full **light mode** via theme-aware CSS variables.
- **Primary accent:** indigo/violet (`~#7c5cff`).
- **Semantic OTP states** (used consistently across badges and charts): requested = zinc/amber,
  sent = blue, verified = emerald, failed = red, expired = muted zinc.
- **Typography:** Geist (UI) + Geist Mono (codes, ids, latency); tight tracking on headings.
- **Motion** (subtle, purposeful): KPI count-up, badge transitions, new-log row entrance, copy
  feedback, sidebar active indicator. Respect `prefers-reduced-motion`.

## 6. Captures

Use Playwright to screenshot every screen at a fixed viewport in **both dark and light**, save to
`docs/assets/dashboard/*.png`, and embed a gallery under `docs/` (optionally the README). Optionally
one short GIF of the playground loop.

## 7. Testing

- **Vitest + Testing Library:** data-source switch, query hooks against mock, table/badge rendering,
  Zod form validation.
- **Playwright:** a few smoke E2E flows; doubles as the capture step.
- Coverage priority: the **logic** (data layer, hooks, mapping, mock progression) over purely
  presentational components. The achieved coverage level will be reported explicitly rather than
  chasing a fixed percentage on presentational code.

## 8. Out of scope (Phase 2+)

- Dashboard auth (NextAuth/Auth.js) and self-serve API-key create/revoke.
- Real-time push (SSE/WebSocket) - MVP uses polling.
- `/v1/stats` aggregate endpoint (Overview stays mock-only until it exists).
- Vercel deployment wiring (the app is built deploy-ready but deploy is a separate step).
