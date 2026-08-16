# OTP Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `worklane` developer dashboard - a polished dark-first Next.js app for the OTP/verification platform, rendered against a swappable mock/live data layer, with captured screenshots.

**Architecture:** Next.js App Router SPA-style client. All server data flows through one `DataSource` interface (mock now, live Go API later) consumed only via TanStack Query. Zustand holds UI state only. shadcn/ui + Tailwind for components, Recharts for charts, Framer Motion for tasteful motion.

**Tech Stack:** Next.js 15 (App Router, TS), React 19, Tailwind CSS v4, shadcn/ui, TanStack Query v5, TanStack Table v8, Zustand v5, React Hook Form + Zod, Recharts, Framer Motion (`motion`), Geist fonts, Vitest + Testing Library, Playwright.

## Global Constraints

- Node >= 20. Package manager: `pnpm`.
- App lives at repo root `dashboard/`. All paths below are relative to `dashboard/` unless prefixed with `docs/` or repo root.
- TypeScript strict mode on. No `any` in committed code.
- Never use the em dash; use a plain `-`.
- Commit messages: conventional commits, no co-author line.
- Server data lives ONLY in TanStack Query cache. Zustand holds ONLY UI state. Never copy fetched data into Zustand.
- Data source is selected by `NEXT_PUBLIC_DATA_SOURCE` (`mock` | `live`), default `mock`.
- Dark-first, theme-aware via CSS variables. Primary accent indigo/violet `#7c5cff`. Semantic OTP state colors: requested=amber, sent=blue, verified=emerald, failed=red, expired=zinc.
- Respect `prefers-reduced-motion` in every animation.
- TS types mirror Go DTOs exactly (see Task 3): ApiKey `{id,tenantId,status}`, OtpRequest `{id,recipient,channel,state}`, DeliveryLog `{requestId,provider,status,latencyMs,error?}`.

---

## File Structure

```
dashboard/
  app/
    layout.tsx                 # root layout: fonts, providers, shell
    providers.tsx              # QueryClientProvider + theme
    globals.css                # Tailwind + CSS variable tokens (light/dark)
    page.tsx                   # Overview
    api-keys/page.tsx
    requests/page.tsx
    logs/page.tsx
    playground/page.tsx
  components/
    shell/sidebar.tsx, topbar.tsx, nav.tsx, tenant-switcher.tsx, theme-toggle.tsx
    ui/                        # shadcn primitives (button, card, table, badge, input, ...)
    common/state-badge.tsx, copy-button.tsx, data-table.tsx, count-up.tsx, stat-card.tsx,
           empty-state.tsx, live-dot.tsx, section-heading.tsx
    charts/sends-area.tsx, funnel.tsx
    overview/kpis.tsx, activity-feed.tsx
    playground/send-form.tsx, verify-form.tsx
  lib/
    api/types.ts, source.ts, mock.ts, live.ts, index.ts
    queries/keys.ts, use-api-keys.ts, use-requests.ts, use-logs.ts, use-overview.ts, use-otp.ts
    store/ui.ts                # Zustand: theme, filters, token
    utils.ts                   # cn(), formatters
  test/setup.ts
  scripts/capture.ts           # Playwright screenshots
  vitest.config.ts, playwright.config.ts, next.config.ts, tailwind/postcss config, tsconfig.json
docs/assets/dashboard/*.png    # captured screenshots
docs/dashboard-gallery.md
```

---

## Task 1: Scaffold app, tooling, and data-source env switch

**Files:**
- Create: `dashboard/package.json`, `dashboard/next.config.ts`, `dashboard/tsconfig.json`, `dashboard/postcss.config.mjs`, `dashboard/app/globals.css`, `dashboard/app/layout.tsx`, `dashboard/app/page.tsx`, `dashboard/.env.example`, `dashboard/.gitignore`, `dashboard/vitest.config.ts`, `dashboard/test/setup.ts`, `dashboard/lib/utils.ts`

**Interfaces:**
- Produces: a booting Next app; `cn(...classes)` helper in `lib/utils.ts`.

- [ ] **Step 1:** Scaffold with pnpm. Run:
```bash
cd dashboard 2>/dev/null || mkdir dashboard && cd dashboard
pnpm dlx create-next-app@latest . --ts --tailwind --app --eslint --src-dir=false --import-alias "@/*" --no-turbopack --use-pnpm --yes
```
- [ ] **Step 2:** Add deps:
```bash
pnpm add @tanstack/react-query @tanstack/react-table zustand react-hook-form zod @hookform/resolvers recharts motion geist class-variance-authority clsx tailwind-merge lucide-react
pnpm add -D vitest @testing-library/react @testing-library/user-event @testing-library/jest-dom jsdom @vitejs/plugin-react playwright @playwright/test
pnpm exec playwright install chromium
```
- [ ] **Step 3:** Init shadcn (dark-first, neutral base):
```bash
pnpm dlx shadcn@latest init -d
pnpm dlx shadcn@latest add button card table badge input label sonner dropdown-menu tabs skeleton separator tooltip
```
- [ ] **Step 4:** Write `lib/utils.ts` (if not created by shadcn):
```ts
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```
- [ ] **Step 5:** Write `.env.example`:
```
NEXT_PUBLIC_DATA_SOURCE=mock
NEXT_PUBLIC_API_BASE=http://localhost:8888
```
- [ ] **Step 6:** Configure Vitest. `vitest.config.ts`:
```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";
export default defineConfig({
  plugins: [react()],
  test: { environment: "jsdom", setupFiles: ["./test/setup.ts"], globals: true },
  resolve: { alias: { "@": path.resolve(__dirname, ".") } },
});
```
`test/setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
```
Add to `package.json` scripts: `"test": "vitest run"`, `"test:watch": "vitest"`.
- [ ] **Step 7:** Run `pnpm build` and `pnpm test`. Expected: build succeeds, `vitest` reports "no test files" (exit 0 acceptable) - fix config until clean.
- [ ] **Step 8:** Commit:
```bash
git add dashboard && git commit -m "chore: scaffold worklane dashboard (next.js, shadcn, tanstack, vitest)"
```

---

## Task 2: Design tokens, fonts, and theme store

**Files:**
- Modify: `dashboard/app/globals.css`, `dashboard/app/layout.tsx`
- Create: `dashboard/lib/store/ui.ts`, `dashboard/components/shell/theme-toggle.tsx`, `dashboard/lib/store/ui.test.ts`

**Interfaces:**
- Produces: `useUIStore` (Zustand) with `{ theme: "dark"|"light", setTheme, toggleTheme, filters, setFilter, token, setToken }`; CSS vars `--background --foreground --card --border --primary --state-{requested,sent,verified,failed,expired}` in both themes; `<ThemeToggle/>`.

- [ ] **Step 1: Write failing test** `lib/store/ui.test.ts`:
```ts
import { describe, it, expect, beforeEach } from "vitest";
import { useUIStore } from "./ui";
describe("ui store", () => {
  beforeEach(() => useUIStore.setState({ theme: "dark" }));
  it("toggles theme", () => {
    expect(useUIStore.getState().theme).toBe("dark");
    useUIStore.getState().toggleTheme();
    expect(useUIStore.getState().theme).toBe("light");
  });
  it("holds ui filters only, not server data", () => {
    useUIStore.getState().setFilter("state", "verified");
    expect(useUIStore.getState().filters.state).toBe("verified");
  });
});
```
- [ ] **Step 2:** Run `pnpm test lib/store/ui.test.ts`. Expected: FAIL (module missing).
- [ ] **Step 3:** Implement `lib/store/ui.ts`:
```ts
import { create } from "zustand";
type Theme = "dark" | "light";
type Filters = { state?: string; search?: string };
type UIState = {
  theme: Theme; setTheme: (t: Theme) => void; toggleTheme: () => void;
  filters: Filters; setFilter: (k: keyof Filters, v?: string) => void;
  token: string; setToken: (t: string) => void;
};
export const useUIStore = create<UIState>((set) => ({
  theme: "dark",
  setTheme: (theme) => set({ theme }),
  toggleTheme: () => set((s) => ({ theme: s.theme === "dark" ? "light" : "dark" })),
  filters: {},
  setFilter: (k, v) => set((s) => ({ filters: { ...s.filters, [k]: v } })),
  token: "",
  setToken: (token) => set({ token }),
}));
```
- [ ] **Step 4:** Run test. Expected: PASS.
- [ ] **Step 5:** Write tokens in `globals.css`. Define `:root` (light) and `.dark` (dark, default) CSS variables per Global Constraints palette; near-black `--background: 240 6% 4%` in dark; `--primary: 255 100% 68%` (indigo/violet). Add semantic `--state-*` vars. Apply Geist + Geist Mono in `layout.tsx`:
```tsx
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
// html className: `${GeistSans.variable} ${GeistMono.variable} dark`
```
Map `--font-sans`/`--font-mono` to the Geist variables in globals.css.
- [ ] **Step 6:** Implement `components/shell/theme-toggle.tsx` - a client button that flips `useUIStore` theme and toggles the `dark` class on `document.documentElement`. Respect no flash: read initial from store.
- [ ] **Step 7:** Run `pnpm test` and `pnpm build`. Expected: PASS + build OK.
- [ ] **Step 8:** Commit: `git add -A && git commit -m "feat(dashboard): design tokens, geist fonts, theme store + toggle"`

---

## Task 3: Types + DataSource interface + mock and live sources

**Files:**
- Create: `dashboard/lib/api/types.ts`, `dashboard/lib/api/source.ts`, `dashboard/lib/api/mock.ts`, `dashboard/lib/api/live.ts`, `dashboard/lib/api/index.ts`, `dashboard/lib/api/mock.test.ts`, `dashboard/lib/api/index.test.ts`

**Interfaces:**
- Produces (consumed by every query hook in Task 4):
```ts
// types.ts
export type OtpState = "requested" | "sent" | "verified" | "failed" | "expired";
export type ApiKey = { id: string; tenantId: string; status: "active" | "revoked" };
export type OtpRequest = { id: string; recipient: string; channel: string; state: OtpState; createdAt: string };
export type DeliveryLog = { requestId: string; provider: string; status: "sent" | "failed"; latencyMs: number; error?: string; createdAt: string };
export type Overview = {
  sentToday: number; verifyRate: number; failed: number; p50LatencyMs: number;
  series: { t: string; requested: number; sent: number; verified: number; failed: number }[];
  funnel: { requested: number; sent: number; verified: number };
};
export type SendResult = { requestId: string; devCode?: string };
export type VerifyResult = { ok: boolean; status: "verified" | "mismatch" | "expired" | "locked" };
// source.ts
export interface DataSource {
  listApiKeys(): Promise<ApiKey[]>;
  listRequests(): Promise<OtpRequest[]>;
  listLogs(): Promise<DeliveryLog[]>;
  getOverview(): Promise<Overview>;
  send(recipient: string): Promise<SendResult>;
  verify(recipient: string, code: string): Promise<VerifyResult>;
}
```
- `index.ts` exports `getDataSource(): DataSource` selecting mock vs live by `NEXT_PUBLIC_DATA_SOURCE`.
- Note `createdAt` is added by the dashboard layer (Go DTOs omit it in list responses today); live.ts fills it with received value or `""`.

- [ ] **Step 1:** Write `types.ts` and `source.ts` (above).
- [ ] **Step 2: Write failing test** `lib/api/mock.test.ts`:
```ts
import { describe, it, expect } from "vitest";
import { MockDataSource } from "./mock";
describe("MockDataSource", () => {
  it("returns realistic api keys and requests", async () => {
    const ds = new MockDataSource();
    expect((await ds.listApiKeys()).length).toBeGreaterThan(0);
    const reqs = await ds.listRequests();
    expect(reqs[0].recipient).toMatch(/\*\*\*/); // masked
  });
  it("send returns a request id and dev code", async () => {
    const ds = new MockDataSource();
    const r = await ds.send("dev@worklane.io");
    expect(r.requestId).toBeTruthy();
    expect(r.devCode).toMatch(/^\d{6}$/);
  });
  it("progresses a delivery log from requested to sent over time", async () => {
    const ds = new MockDataSource({ now: () => 0 });
    const before = await ds.listLogs();
    const pending = before.find((l) => l.status === "sent");
    // advance virtual clock: a freshly requested item becomes sent
    const ds2 = new MockDataSource({ now: () => 60_000 });
    const after = await ds2.listLogs();
    expect(after.filter((l) => l.status === "sent").length).toBeGreaterThanOrEqual(before.filter((l) => l.status === "sent").length);
    expect(pending).toBeDefined();
  });
});
```
- [ ] **Step 3:** Run `pnpm test lib/api/mock.test.ts`. Expected: FAIL.
- [ ] **Step 4:** Implement `mock.ts` - a `MockDataSource` class holding seeded fixtures (10-20 requests across all states, matching api keys, delivery logs). Accept an optional `{ now }` clock for tests. `listLogs()` computes status from an item's `createdAt` vs `now()` so recently-requested items flip to `sent` after a simulated delay, and `getOverview()` returns aggregates derived from the fixtures (sensible series + funnel). Mask recipients as `d***@gmail.com`. Add small artificial latency via `await sleep(120)` (skip when a test clock is injected).
- [ ] **Step 5:** Run test. Expected: PASS.
- [ ] **Step 6:** Implement `live.ts` - `LiveDataSource` using `fetch(NEXT_PUBLIC_API_BASE + path, { headers: { Authorization: 'Bearer ' + token } })`, mapping Go JSON (`request_id`, `tenant_id`, `latency_ms`) to the camelCase types. `getOverview()` throws `NotImplementedError` for now (Overview is mock-only until `/v1/stats` exists - see spec §3 known gap).
- [ ] **Step 7: Write failing test** `lib/api/index.test.ts` verifying `getDataSource()` returns a `MockDataSource` when `NEXT_PUBLIC_DATA_SOURCE` unset/`mock`, and `LiveDataSource` when `live`. Use `vi.stubEnv`.
- [ ] **Step 8:** Implement `index.ts` selector; run test. Expected: PASS.
- [ ] **Step 9:** Commit: `git add -A && git commit -m "feat(dashboard): swappable data layer (types, mock progression, live adapter)"`

---

## Task 4: Query provider + hooks (with polling)

**Files:**
- Create: `dashboard/app/providers.tsx`, `dashboard/lib/queries/keys.ts`, `dashboard/lib/queries/use-api-keys.ts`, `dashboard/lib/queries/use-requests.ts`, `dashboard/lib/queries/use-logs.ts`, `dashboard/lib/queries/use-overview.ts`, `dashboard/lib/queries/use-otp.ts`, `dashboard/lib/queries/use-logs.test.tsx`
- Modify: `dashboard/app/layout.tsx` (wrap children in `<Providers>`)

**Interfaces:**
- Produces: `useApiKeys()`, `useRequests()`, `useLogs()` (with `refetchInterval: 4000`), `useOverview()`, `useSend()`, `useVerify()` - each a thin wrapper over `getDataSource()`.
- Query keys centralized in `keys.ts`: `qk.apiKeys`, `qk.requests`, `qk.logs`, `qk.overview`.

- [ ] **Step 1:** Implement `providers.tsx` (client) creating a `QueryClient` (staleTime 10s) inside `useState`, wrapping `QueryClientProvider` + Sonner `<Toaster/>`.
- [ ] **Step 2:** Implement `keys.ts` and the five hooks. `use-logs.ts` sets `refetchInterval: 4000`.
- [ ] **Step 3: Write failing test** `use-logs.test.tsx`: render `useLogs()` in a `QueryClientProvider` with a test `QueryClient`, assert it eventually returns the mock logs.
```tsx
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLogs } from "./use-logs";
it("loads delivery logs from the data source", async () => {
  const qc = new QueryClient();
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  const { result } = renderHook(() => useLogs(), { wrapper });
  await waitFor(() => expect(result.current.data?.length).toBeGreaterThan(0));
});
```
- [ ] **Step 4:** Run test. Expected: PASS (uses mock default).
- [ ] **Step 5:** Wire `<Providers>` into `layout.tsx`. Run `pnpm build`. Expected: OK.
- [ ] **Step 6:** Commit: `git add -A && git commit -m "feat(dashboard): react-query provider and data hooks with log polling"`

---

## Task 5: App shell (sidebar, topbar, nav)

**Files:**
- Create: `dashboard/components/shell/sidebar.tsx`, `topbar.tsx`, `nav.tsx`, `tenant-switcher.tsx`
- Modify: `dashboard/app/layout.tsx`

**Interfaces:**
- Consumes: `<ThemeToggle/>` (Task 2), `useUIStore` (Task 2).
- Produces: an `AppShell` composition rendering sidebar + topbar + `{children}`.

- [ ] **Step 1:** Build `nav.tsx` - array of `{ href, label, icon }` for the 5 routes (lucide icons: LayoutDashboard, KeyRound, ListChecks, Truck, FlaskConical). Active item highlighted via `usePathname()` with an animated indicator (Framer Motion `layoutId`), reduced-motion safe.
- [ ] **Step 2:** Build `sidebar.tsx` - worklane wordmark (violet dot + mono), nav, tenant switcher at bottom. Thin `border-r` with `--border`.
- [ ] **Step 3:** Build `topbar.tsx` - breadcrumb/page title slot, right side: theme toggle, mock/live badge reading `NEXT_PUBLIC_DATA_SOURCE`.
- [ ] **Step 4:** Build `tenant-switcher.tsx` - shadcn dropdown, static tenants for now; selection stored in `useUIStore` filters (UI state only).
- [ ] **Step 5:** Compose in `layout.tsx`. Run `pnpm build`. Manually verify at `pnpm dev` that all 5 routes render the shell (stub pages ok).
- [ ] **Step 6:** Commit: `git add -A && git commit -m "feat(dashboard): app shell with sidebar, topbar, nav"`

---

## Task 6: Shared UI primitives

**Files:**
- Create: `dashboard/components/common/state-badge.tsx`, `copy-button.tsx`, `data-table.tsx`, `count-up.tsx`, `stat-card.tsx`, `empty-state.tsx`, `live-dot.tsx`, `section-heading.tsx`
- Create tests: `dashboard/components/common/state-badge.test.tsx`, `dashboard/components/common/count-up.test.tsx`

**Interfaces:**
- Produces:
  - `<StateBadge state={OtpState|"sent"|"failed"} />` - colored pill using `--state-*` tokens.
  - `<CopyButton value={string} />` - copies + shows a check with a brief spring.
  - `<DataTable columns data />` - TanStack Table wrapper (sorting + basic pagination) styled with shadcn table.
  - `<CountUp value={number} format? />` - animates number from 0, reduced-motion => instant.
  - `<StatCard label value delta? icon />`, `<EmptyState/>`, `<LiveDot/>`, `<SectionHeading/>`.

- [ ] **Step 1: Write failing test** `state-badge.test.tsx`: renders label text for each state and applies a state class. Run -> FAIL.
- [ ] **Step 2:** Implement `state-badge.tsx` with a `cva` variant map keyed by state -> PASS.
- [ ] **Step 3: Write failing test** `count-up.test.tsx`: with `matchMedia` reduced-motion mocked true, renders final value immediately. Run -> FAIL.
- [ ] **Step 4:** Implement `count-up.tsx` (Framer Motion `useMotionValue`+`animate`, reduced-motion short-circuit) -> PASS.
- [ ] **Step 5:** Implement remaining primitives (`data-table`, `copy-button`, `stat-card`, `empty-state`, `live-dot`, `section-heading`). No test needed beyond render smoke; keep them small (< 120 lines each).
- [ ] **Step 6:** Run `pnpm test` + `pnpm build`. Commit: `git add -A && git commit -m "feat(dashboard): shared ui primitives (badge, table, count-up, copy)"`

---

## Task 7: Overview screen

**Files:**
- Create: `dashboard/components/overview/kpis.tsx`, `dashboard/components/overview/activity-feed.tsx`, `dashboard/components/charts/sends-area.tsx`, `dashboard/components/charts/funnel.tsx`
- Modify: `dashboard/app/page.tsx`

**Interfaces:**
- Consumes: `useOverview()` (Task 4), `<StatCard>`, `<CountUp>`, `<StateBadge>`, `--state-*` tokens.
- Chart colors come from the semantic `--state-*` CSS variables (read via `getComputedStyle` or hard-mapped hsl), per the `dataviz` skill: theme-aware, one system.

- [ ] **Step 1:** `kpis.tsx` - 4 `<StatCard>` (Sent today, Verify rate %, Failed, P50 latency ms) fed by `useOverview()`, values via `<CountUp>`. Loading => `<Skeleton>`.
- [ ] **Step 2:** `sends-area.tsx` - Recharts stacked area of `overview.series` by state; muted grid, thin axes, tooltip themed to card surface. Wrap in `overflow-x-auto`.
- [ ] **Step 3:** `funnel.tsx` - requested -> sent -> verified horizontal funnel bars with counts and conversion %.
- [ ] **Step 4:** `activity-feed.tsx` - latest requests as a timeline with `<StateBadge>` and relative time.
- [ ] **Step 5:** Compose `app/page.tsx`: heading, KPI row, chart + funnel grid, activity feed. Add a small "mock data" note when source is mock (per spec known gap).
- [ ] **Step 6:** `pnpm build`, visually verify at `pnpm dev`. Commit: `git add -A && git commit -m "feat(dashboard): overview screen (kpis, sends chart, funnel, activity)"`

---

## Task 8: API Keys screen

**Files:**
- Create: `dashboard/app/api-keys/page.tsx`, `dashboard/components/api-keys/columns.tsx`

**Interfaces:**
- Consumes: `useApiKeys()`, `<DataTable>`, `<StateBadge>`, `<CopyButton>`.

- [ ] **Step 1:** Define columns: id (mono, truncated + `<CopyButton>`), tenantId, status (`<StateBadge>` active/revoked), created. 
- [ ] **Step 2:** Page: heading + disabled "New key" button with tooltip "Coming in Phase 2", `<DataTable>`; `<EmptyState>` when none; `<Skeleton>` while loading.
- [ ] **Step 3:** `pnpm build`, visual check. Commit: `git add -A && git commit -m "feat(dashboard): api keys screen"`

---

## Task 9: OTP Requests screen

**Files:**
- Create: `dashboard/app/requests/page.tsx`, `dashboard/components/requests/columns.tsx`, `dashboard/components/requests/filters.tsx`

**Interfaces:**
- Consumes: `useRequests()`, `useUIStore` filters, `<DataTable>`, `<StateBadge>`.

- [ ] **Step 1:** Columns: request id (mono + copy), recipient (masked), channel, state badge, created.
- [ ] **Step 2:** `filters.tsx` - state select (all/requested/sent/verified/failed/expired) + search input bound to `useUIStore` filters; filtering applied client-side over query data (do not mutate cache).
- [ ] **Step 3:** Page composition; `pnpm build`, visual check. Commit: `git add -A && git commit -m "feat(dashboard): otp requests screen with filters"`

---

## Task 10: Delivery Logs screen (live polling)

**Files:**
- Create: `dashboard/app/logs/page.tsx`, `dashboard/components/logs/columns.tsx`

**Interfaces:**
- Consumes: `useLogs()` (polling), `<DataTable>`, `<StateBadge>`, `<LiveDot>`, `<CopyButton>`.

- [ ] **Step 1:** Columns: request id (mono + copy), provider, status badge, latency (mono, ms), error (truncated + tooltip).
- [ ] **Step 2:** Page: heading with `<LiveDot>` + "Live - updates every 4s"; `<DataTable>`. New rows animate in via Framer Motion `AnimatePresence` (reduced-motion safe).
- [ ] **Step 3:** Verify at `pnpm dev` that rows visibly change as mock logs progress (poll). Commit: `git add -A && git commit -m "feat(dashboard): delivery logs screen with live polling"`

---

## Task 11: Playground screen (send + verify)

**Files:**
- Create: `dashboard/app/playground/page.tsx`, `dashboard/components/playground/send-form.tsx`, `dashboard/components/playground/verify-form.tsx`, `dashboard/lib/schemas.ts`, `dashboard/components/playground/send-form.test.tsx`

**Interfaces:**
- Consumes: `useSend()`, `useVerify()` (Task 4), RHF + Zod.
- Produces: `sendSchema` (`{ recipient: email }`), `verifySchema` (`{ recipient: email, code: 6 digits }`) in `lib/schemas.ts`.

- [ ] **Step 1: Write failing test** `send-form.test.tsx`: submitting an invalid email shows a validation message; valid email calls the mutation and renders the returned `requestId`. Run -> FAIL.
- [ ] **Step 2:** Implement `schemas.ts`, `send-form.tsx` (RHF + zodResolver, shows returned `requestId` + `devCode` in mock), `verify-form.tsx` (shows verified/mismatch/expired/locked result with `<StateBadge>`). Make test PASS.
- [ ] **Step 3:** Compose `playground/page.tsx` - two-column: send card | verify card, with a short explainer. A success on send offers a "Use this code" shortcut prefilling verify (mock devCode).
- [ ] **Step 4:** `pnpm test` + `pnpm build` + visual check. Commit: `git add -A && git commit -m "feat(dashboard): playground send/verify with rhf+zod"`

---

## Task 12: Screenshot capture + gallery

**Files:**
- Create: `dashboard/scripts/capture.ts`, `dashboard/playwright.config.ts`, `docs/dashboard-gallery.md`
- Output: `docs/assets/dashboard/{overview,api-keys,requests,logs,playground}-{dark,light}.png`

**Interfaces:**
- Consumes: the running dev server (`pnpm dev`) in mock mode.

- [ ] **Step 1:** `playwright.config.ts` - viewport 1440x900, deviceScaleFactor 2, baseURL `http://localhost:3000`.
- [ ] **Step 2:** `capture.ts` - for each route and each theme (toggle by clicking theme toggle or setting the `dark` class), wait for network idle + a stable selector, screenshot full page to `docs/assets/dashboard/<route>-<theme>.png`. Add script `"capture": "tsx scripts/capture.ts"` (add `tsx` dev dep).
- [ ] **Step 3:** Run capture:
```bash
pnpm dev & sleep 5; pnpm capture; kill %1
```
Verify 10 PNGs exist and look pixel-clean (no layout shift, no loading skeletons captured - wait for data).
- [ ] **Step 4:** Write `docs/dashboard-gallery.md` embedding the dark screenshots with captions; link the light variants.
- [ ] **Step 5:** Commit: `git add -A && git commit -m "docs(dashboard): capture screenshots and gallery"`

---

## Task 13: README/docs link + final verification

**Files:**
- Modify: `docs/architecture.md` (add a "Dashboard" note + link to gallery) or create `dashboard/README.md`.

- [ ] **Step 1:** Add `dashboard/README.md` - how to run (`pnpm dev`), the `NEXT_PUBLIC_DATA_SOURCE` switch, how to wire live (`=live` + `NEXT_PUBLIC_API_BASE` + token), how to re-capture.
- [ ] **Step 2:** Link the gallery from `docs/architecture.md`.
- [ ] **Step 3:** Full gate: `pnpm test` (all pass), `pnpm build` (clean), `pnpm exec tsc --noEmit` (no type errors), `pnpm lint`. Report coverage of `lib/` logic.
- [ ] **Step 4:** Commit: `git add -A && git commit -m "docs(dashboard): readme and architecture link"`

---

## Self-Review Notes

- Spec coverage: location/stack (T1-2), swappable data layer + known gap (T3, T7 note, T4 live overview NotImplemented), 5 screens (T7-11), visual system/tokens/motion (T2,T6, per-screen), captures (T12), testing (tests in T2,T3,T4,T6,T11 + gate T13). All spec §sections mapped.
- Type consistency: `DataSource` methods and DTO field names are defined once in Task 3 and referenced verbatim by Task 4 hooks and all screens.
- Overview aggregates are explicitly mock-only (live `getOverview` throws) - matches the surfaced tradeoff; no task pretends otherwise.
