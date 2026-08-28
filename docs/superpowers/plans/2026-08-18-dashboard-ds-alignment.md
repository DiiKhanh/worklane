# Dashboard Design-System Alignment Implementation Plan

> **CURRENT NOTE (2026-08-28):** Historical implementation plan. The dashboard now has working
> auth-svc sign-in and the roadmap screens exist in the UI; current behavior is summarized in
> [dashboard gallery](../../dashboard-gallery.md) and `dashboard/README.md`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the request-detail interaction and the Templates / Links / Campaigns / Login roadmap screens to the worklane dashboard, matching the extracted design system in the repo's real stack.

**Architecture:** Port the DS kit's layout, copy and interaction into the repo's conventions (Tailwind + `components/ui/` shadcn base-nova + `components/common/*` + TanStack + Recharts + Lucide). Request detail is grounded in real request + delivery-log data; the three roadmap screens read local fixtures under `lib/roadmap/` and carry a dashed "not in the codebase yet" notice; Login is a standalone unwired page.

**Tech Stack:** Next.js 16 App Router, React 19, TypeScript, Tailwind v4, shadcn/ui (base-nova), TanStack Query + Table, Zustand, Recharts, Motion, Lucide, vitest + testing-library.

## Global Constraints

- **Reference sources (verbatim on disk):** `/Users/duykhanh/Downloads/worklane Design System/ui_kits/dashboard/{Requests,RequestDetail,Templates,Links,Campaigns,Login}.jsx` and `data.js`. Port their layout and **copy verbatim** (sentence case, no emoji, middot `·` separators, identifiers in mono). Re-express inline styles as Tailwind + existing components; do NOT copy the kit's inline-style JS.
- **Next.js is modified:** read `node_modules/next/dist/docs/` (resolved from `dashboard/`) before writing any route/layout code. Keep the `dashboard/AGENTS.md` auto-block if present.
- **Data boundary:** roadmap screens (Templates/Links/Campaigns/Login) MUST NOT touch `lib/api/` or `DataSource`. They read local fixtures and component state only. Only request detail uses real queries.
- **Immutability:** new objects via spread, never mutate (e.g. composer/list state updates).
- **Copy rules:** roadmap note text is exactly `Roadmap screen. Not in the codebase yet - laid out only from shipped worklane patterns.` Login flag is exactly `Roadmap screen - the repository authenticates with a bearer key, not a session.`
- **No em dash** anywhere (use `-`). No `console.log`. Commit messages: conventional, no co-author line.
- **Gate before "done":** `pnpm lint`, `pnpm typecheck`, `pnpm test` all pass.
- Work on branch `feat/dashboard-ds-alignment` (already created).

---

## File Structure

**New primitives**
- `components/ui/dialog.tsx`, `components/ui/select.tsx`, `components/ui/textarea.tsx` - shadcn base-nova parts (via CLI).
- `components/common/roadmap-note.tsx` - the dashed roadmap notice.
- `components/common/data-table.tsx` - MODIFY: add `onRowClick` + chevron affordance.

**Request detail (real data)**
- `lib/api/index.ts` or a selector in the view - `logByRequest` lookup over the logs query.
- `lib/requests/lifecycle.ts` - pure stage-derivation helper + `Stage` type.
- `components/requests/request-detail.tsx` - detail view (lifecycle + attributes).
- `components/requests/requests-view.tsx` - MODIFY: hold open-row state, swap list ↔ detail.

**Roadmap fixtures + screens**
- `lib/roadmap/templates.ts`, `lib/roadmap/links.ts`, `lib/roadmap/campaigns.ts` - types + fixtures.
- `lib/roadmap/campaign-math.ts` - pure scope/window helpers (tested).
- `lib/roadmap/template-render.ts` - pure preview substitution + `{{var}}` split (tested).
- `components/templates/{templates-view,template-detail,new-template-dialog}.tsx`
- `components/links/{links-view,link-detail,clicks-area.tsx}`
- `components/campaigns/{campaigns-view,campaign-composer,campaign-detail}.tsx`
- `app/templates/page.tsx`, `app/campaigns/page.tsx`, `app/links/page.tsx`
- `app/login/page.tsx` + `components/login/login-view.tsx`

**Nav / docs**
- `components/shell/nav.tsx` - MODIFY: add 3 nav items with "soon" tag.
- `scripts/capture.ts`, `docs/dashboard-gallery.md` - MODIFY: add new screens.

---

## Task 1: Primitives - shadcn parts, roadmap note, DataTable onRowClick

**Files:**
- Create: `components/ui/dialog.tsx`, `components/ui/select.tsx`, `components/ui/textarea.tsx` (via shadcn CLI)
- Create: `components/common/roadmap-note.tsx`
- Modify: `components/common/data-table.tsx`
- Test: `components/common/data-table.test.tsx`

**Interfaces:**
- Produces: `<RoadmapNote />` (no props). `DataTable` gains `onRowClick?: (row: TData) => void`; when set, rows are clickable (role=button, Enter/Space, hover) and a trailing `ChevronRight` cell is appended.

- [ ] **Step 1: Add shadcn primitives**

Run from `dashboard/`:
```bash
pnpm dlx shadcn@latest add dialog select textarea
```
Verify the three files landed in `components/ui/` and import from `@/components/ui/*`. If the CLI prompts for style, accept the existing `base-nova`. If `select`/`dialog` pull a Radix/base-ui dep already present, no action needed.

- [ ] **Step 2: Create the roadmap note**

`components/common/roadmap-note.tsx`:
```tsx
import { Info } from "lucide-react";

export function RoadmapNote() {
  return (
    <div className="mb-4 flex items-center gap-2 rounded-xl border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
      <Info className="size-3.5 shrink-0" />
      Roadmap screen. Not in the codebase yet - laid out only from shipped worklane patterns.
    </div>
  );
}
```

- [ ] **Step 3: Write the failing DataTable test**

`components/common/data-table.test.tsx`:
```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ColumnDef } from "@tanstack/react-table";
import { DataTable } from "./data-table";

type Row = { id: string; name: string };
const columns: ColumnDef<Row>[] = [{ accessorKey: "name", header: "Name" }];
const data: Row[] = [{ id: "a", name: "Alpha" }];

describe("DataTable onRowClick", () => {
  it("fires onRowClick with the row when a row is activated", async () => {
    const onRowClick = vi.fn();
    render(<DataTable columns={columns} data={data} onRowClick={onRowClick} rowKey={(r) => r.id} />);
    await userEvent.click(screen.getByText("Alpha"));
    expect(onRowClick).toHaveBeenCalledWith(data[0]);
  });

  it("renders no clickable rows when onRowClick is absent", () => {
    render(<DataTable columns={columns} data={data} rowKey={(r) => r.id} />);
    expect(screen.queryByRole("button", { name: /alpha/i })).toBeNull();
  });
});
```

- [ ] **Step 4: Run test - verify it fails**

Run: `pnpm test -- data-table`
Expected: FAIL (onRowClick not handled).

- [ ] **Step 5: Implement onRowClick in DataTable**

In `components/common/data-table.tsx`: add `onRowClick?: (row: TData) => void;` to `DataTableProps`. Import `ChevronRight`. Build a local `cols` that appends a synthetic chevron column when `onRowClick` is set:
```tsx
const cols = onRowClick
  ? [...columns, {
      id: "__chevron",
      header: "",
      enableSorting: false,
      cell: () => <ChevronRight className="size-4 text-muted-foreground" />,
    } as ColumnDef<TData, TValue>]
  : columns;
```
Pass `cols` to `useReactTable`. On each body `<TableRow>`, when `onRowClick` is set add: `role="button"`, `tabIndex={0}`, `onClick={() => onRowClick(row.original)}`, `onKeyDown` firing on Enter/Space, and classes `cursor-pointer hover:bg-muted/50`. Keep the `colSpan` empty-state cell using `cols.length`.

- [ ] **Step 6: Run tests - verify pass**

Run: `pnpm test -- data-table` -> PASS. Then `pnpm typecheck`.

- [ ] **Step 7: Commit**

```bash
git add components/ui/dialog.tsx components/ui/select.tsx components/ui/textarea.tsx components/common/roadmap-note.tsx components/common/data-table.tsx components/common/data-table.test.tsx
git commit -m "feat(dashboard): add dialog/select/textarea, roadmap note, DataTable row click"
```

---

## Task 2: Request lifecycle derivation (pure) + log selector

**Files:**
- Create: `lib/requests/lifecycle.ts`
- Test: `lib/requests/lifecycle.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export type Stage = { key: "requested" | "sent" | "verified"; label: string; detail: string; icon: LucideIcon; state: "done" | "failed" | "pending" };
  export function lifecycleStages(state: OtpState, log?: DeliveryLog): Stage[];
  ```
  Stages are always the three-row skeleton requested -> sent -> verified. `state` marks each row done/failed/pending from the request `OtpState`: `requested` -> only row 0 done; `failed` -> row 0 done, row 1 failed; `sent`/`expired` -> rows 0-1 done, row 2 pending; `verified` -> all done. When a failed `log.error` exists it becomes row 1's detail.

- [ ] **Step 1: Write the failing test**

`lib/requests/lifecycle.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { lifecycleStages } from "./lifecycle";

const mark = (s: ReturnType<typeof lifecycleStages>) => s.map((x) => x.state);

describe("lifecycleStages", () => {
  it("requested: only the first stage is done", () => {
    expect(mark(lifecycleStages("requested"))).toEqual(["done", "pending", "pending"]);
  });
  it("failed: delivery stage marked failed, uses log error as detail", () => {
    const s = lifecycleStages("failed", { requestId: "r", provider: "smtp", status: "failed", latencyMs: 0, error: "smtp: 550 mailbox unavailable", createdAt: "" });
    expect(mark(s)).toEqual(["done", "failed", "pending"]);
    expect(s[1].detail).toContain("550");
  });
  it("verified: all stages done", () => {
    expect(mark(lifecycleStages("verified"))).toEqual(["done", "done", "done"]);
  });
  it("sent and expired reach the delivery stage but not verified", () => {
    expect(mark(lifecycleStages("sent"))).toEqual(["done", "done", "pending"]);
    expect(mark(lifecycleStages("expired"))).toEqual(["done", "done", "pending"]);
  });
});
```

- [ ] **Step 2: Run test - verify fails**

Run: `pnpm test -- lifecycle` -> FAIL (module missing).

- [ ] **Step 3: Implement `lib/requests/lifecycle.ts`**

```ts
import { CirclePlus, Send, ShieldCheck, type LucideIcon } from "lucide-react";
import type { DeliveryLog, OtpState } from "@/lib/api/types";

export type StageState = "done" | "failed" | "pending";
export type Stage = { key: "requested" | "sent" | "verified"; label: string; detail: string; icon: LucideIcon; state: StageState };

const BASE: Omit<Stage, "state" | "detail">[] = [
  { key: "requested", label: "Requested", icon: CirclePlus },
  { key: "sent", label: "Delivered", icon: Send },
  { key: "verified", label: "Verified", icon: ShieldCheck },
];
const DETAIL: Record<Stage["key"], string> = {
  requested: "Code issued and hashed",
  sent: "Handed to the provider",
  verified: "Code confirmed, single use",
};

export function lifecycleStages(state: OtpState, log?: DeliveryLog): Stage[] {
  const reached = state === "failed" ? 1 : state === "requested" ? 0 : state === "verified" ? 2 : 1;
  const failed = state === "failed";
  return BASE.map((b, i) => {
    const isFail = failed && i === 1;
    const st: StageState = isFail ? "failed" : i <= reached ? "done" : "pending";
    const detail = isFail && log?.error ? log.error : b.key === "sent" && st === "failed" ? "Delivery failed" : DETAIL[b.key];
    return { ...b, state: st, detail: isFail ? "Delivery failed" : DETAIL[b.key], ...(isFail && log?.error ? { detail: log.error } : {}) } as Stage;
  });
}
```
(Note: the ternary above simplifies to - detail is `log.error` when failed row and error present, else the stage's default. Keep it readable; the test only asserts row-1 detail contains the error and the state markers.)

- [ ] **Step 4: Run test - verify pass**

Run: `pnpm test -- lifecycle` -> PASS.

- [ ] **Step 5: Commit**

```bash
git add lib/requests/lifecycle.ts lib/requests/lifecycle.test.ts
git commit -m "feat(dashboard): request lifecycle stage derivation"
```

---

## Task 3: Request detail view + master-detail wiring

**Files:**
- Create: `components/requests/request-detail.tsx`
- Modify: `components/requests/requests-view.tsx`
- Test: `components/requests/request-detail.test.tsx`

**Interfaces:**
- Consumes: `lifecycleStages` (Task 2), `useLogs()` (`lib/queries/use-logs`), `timeAgo`, `StateBadge`, `CopyButton`, `Panel`, `SectionHeading`, `Button`.
- Produces: `<RequestDetail request={OtpRequest} log={DeliveryLog | undefined} onBack={() => void} />`.

- [ ] **Step 1: Write the failing test**

`components/requests/request-detail.test.tsx`:
```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RequestDetail } from "./request-detail";
import type { OtpRequest } from "@/lib/api/types";

const req: OtpRequest = { id: "req_9f3a2c11", recipient: "d***@gmail.com", channel: "email", state: "verified", createdAt: new Date().toISOString() };

describe("RequestDetail", () => {
  it("shows the request id, state, and lifecycle stages", () => {
    render(<RequestDetail request={req} log={undefined} onBack={vi.fn()} />);
    expect(screen.getAllByText("req_9f3a2c11").length).toBeGreaterThan(0);
    expect(screen.getByText("Requested")).toBeInTheDocument();
    expect(screen.getByText("Verified")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run - verify fails.** `pnpm test -- request-detail` -> FAIL.

- [ ] **Step 3: Implement `request-detail.tsx`**

Port layout from `ui_kits/dashboard/Requests.jsx` `RequestDetail` (lines ~253-284) into Tailwind:
- Back `Button variant="ghost" size="sm"` with `ArrowLeft` + "OTP requests", `-ml-2.5 mb-3`.
- `SectionHeading` title "Request detail", description `request.id`, action `<StateBadge state={request.state} />`.
- Two-column grid `grid gap-4 md:grid-cols-2`:
  - `Panel title="Lifecycle" description="Events emitted for this code"` rendering the timeline: map `lifecycleStages(request.state, log)`; each row is a `grid grid-cols-[28px_1fr] gap-3`, a 28px rounded icon chip tinted by state colour (`var(--state-verified)` done, `var(--state-failed)` failed, else muted), a connector line except last, then label + `text-xs muted` detail. Colours via `color-mix(in oklch, <c> 14%, transparent)` background like `StateBadge`.
  - `Panel title="Attributes"` with rows (label left muted, value right): Request id (mono + `CopyButton`), Recipient (mono muted), Channel (capitalize), Provider (`log?.provider ?? "-"` mono muted), Latency (`log && log.latencyMs > 0` -> `{log.latencyMs} ms` mono, else omit row), Created (`timeAgo(request.createdAt) || "-"`). Do NOT render template/attempts rows (no real source).
- If `log?.error`, add a "Provider response" block: a bordered `state-failed`-tinted box with the error in mono, plus muted line "Failed sends are retried with exponential backoff, then routed to the DLQ."

- [ ] **Step 4: Wire master-detail in `requests-view.tsx`**

- Add `const [openId, setOpenId] = useState<string | null>(null);` and `const { data: logs } = useLogs();`.
- Add `onRowClick={(r) => setOpenId(r.id)}` to the `<DataTable>`.
- When `openId` set, find `const request = data?.find((r) => r.id === openId)` and `const log = logs?.find((l) => l.requestId === openId)`; if `request`, return `<RequestDetail request={request} log={log} onBack={() => setOpenId(null)} />` before the list markup.
- Update the requests list `SectionHeading`/page copy is in `app/requests/page.tsx`; append "Select a request for its lifecycle." to its description to match the kit.

- [ ] **Step 5: Run - verify pass.** `pnpm test -- request-detail` -> PASS. Then `pnpm typecheck`.

- [ ] **Step 6: Commit**

```bash
git add components/requests/request-detail.tsx components/requests/request-detail.test.tsx components/requests/requests-view.tsx app/requests/page.tsx
git commit -m "feat(dashboard): OTP request master-detail with lifecycle timeline"
```

---

## Task 4: Roadmap fixtures + pure helpers

**Files:**
- Create: `lib/roadmap/templates.ts`, `lib/roadmap/links.ts`, `lib/roadmap/campaigns.ts`
- Create: `lib/roadmap/template-render.ts`, `lib/roadmap/campaign-math.ts`
- Test: `lib/roadmap/template-render.test.ts`, `lib/roadmap/campaign-math.test.ts`

**Interfaces:**
- Produces typed fixtures + helpers used by Tasks 5-7:
  - `templates.ts`: `type Template = { id; name; channel: "email"|"sms"; locale: string; version: number; status: "active"|"expired"; updated: string; subject: string; body: string; versions: {v;when;by;note}[]; sends: {id;recipient;state;when}[] }`; `export const TEMPLATES: Template[]`; `export const TEMPLATE_VARS = ["{{code}}","{{name}}","{{link}}","{{expiry}}"]`.
  - `links.ts`: `type Link = { code; target; clicks; ctr; created; createdFull; series: number[]; recent: {ts;ref;geo;device}[] }`; `export const LINKS: Link[]`; `export const LINK_KPIS = { created: 412, clicks: 2861, clickRate: 0.24, cacheHit: 0.97 }`.
  - `campaigns.ts`: `type Campaign`, `type Audience`, `type CampaignTemplate`; `export const CAMPAIGNS`, `AUDIENCES`, `CAMPAIGN_TEMPLATES`.
  - `template-render.ts`: `export function renderPreview(tpl: {subject;body}): {subject;body}` (substitutes `{{code}}`->418209, `{{name}}`->Daniel, `{{link ...}}`->wl.link/7Xq2Ab); `export function splitVars(text: string): {text: string; isVar: boolean}[]`.
  - `campaign-math.ts`: `export function recipientScope(a: {isCsv;size;csvLoaded}): number`; `export function sendWindowMinutes(size: number, rate: number): number` (0 when size or rate is 0; else `max(1, round(size/rate))`).

- [ ] **Step 1: Port fixtures verbatim**

Copy the data arrays from `ui_kits/dashboard/{Templates,Links,Campaigns}.jsx` and `data.js` into the three fixture files as typed `const`s. Keep every string verbatim (subjects, bodies, notes, masked recipients, geo). Export the types above.

- [ ] **Step 2: Write failing helper tests**

`lib/roadmap/template-render.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { renderPreview, splitVars } from "./template-render";

describe("renderPreview", () => {
  it("substitutes code, name, and link tokens", () => {
    const out = renderPreview({ subject: "Your code: {{code}}", body: 'Hi {{name}}, open {{link "https://x"}}' });
    expect(out.subject).toBe("Your code: 418209");
    expect(out.body).toBe("Hi Daniel, open wl.link/7Xq2Ab");
  });
});
describe("splitVars", () => {
  it("marks {{...}} segments as variables", () => {
    const parts = splitVars("a {{code}} b");
    expect(parts.filter((p) => p.isVar).map((p) => p.text)).toEqual(["{{code}}"]);
  });
});
```

`lib/roadmap/campaign-math.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { recipientScope, sendWindowMinutes } from "./campaign-math";

describe("recipientScope", () => {
  it("uses audience size when not CSV", () => {
    expect(recipientScope({ isCsv: false, size: 8241, csvLoaded: false })).toBe(8241);
  });
  it("is 0 for CSV until a file is loaded, then 218", () => {
    expect(recipientScope({ isCsv: true, size: 0, csvLoaded: false })).toBe(0);
    expect(recipientScope({ isCsv: true, size: 0, csvLoaded: true })).toBe(218);
  });
});
describe("sendWindowMinutes", () => {
  it("returns 0 when size or rate is 0", () => {
    expect(sendWindowMinutes(0, 600)).toBe(0);
    expect(sendWindowMinutes(8241, 0)).toBe(0);
  });
  it("rounds size/rate up to at least 1", () => {
    expect(sendWindowMinutes(8241, 600)).toBe(14);
    expect(sendWindowMinutes(100, 600)).toBe(1);
  });
});
```

- [ ] **Step 3: Run - verify fail.** `pnpm test -- roadmap` -> FAIL.

- [ ] **Step 4: Implement the helpers**

`template-render.ts`:
```ts
export function splitVars(text: string): { text: string; isVar: boolean }[] {
  return text.split(/(\{\{[^}]+\}\})/g).filter(Boolean).map((t) => ({ text: t, isVar: /^\{\{[^}]+\}\}$/.test(t) }));
}
export function renderPreview(tpl: { subject: string; body: string }) {
  const sub = (s: string) =>
    s.replace(/\{\{code\}\}/g, "418209").replace(/\{\{name\}\}/g, "Daniel").replace(/\{\{\s*link[^}]*\}\}/g, "wl.link/7Xq2Ab");
  return { subject: sub(tpl.subject), body: sub(tpl.body) };
}
```
`campaign-math.ts`:
```ts
export function recipientScope(a: { isCsv: boolean; size: number; csvLoaded: boolean }): number {
  return a.isCsv ? (a.csvLoaded ? 218 : 0) : a.size;
}
export function sendWindowMinutes(size: number, rate: number): number {
  if (!size || !rate) return 0;
  return Math.max(1, Math.round(size / rate));
}
```

- [ ] **Step 5: Run - verify pass.** `pnpm test -- roadmap` -> PASS.

- [ ] **Step 6: Commit**

```bash
git add lib/roadmap
git commit -m "feat(dashboard): roadmap fixtures and pure helpers (templates/links/campaigns)"
```

---

## Task 5: Templates screen

**Files:**
- Create: `components/templates/templates-view.tsx`, `components/templates/template-detail.tsx`, `components/templates/new-template-dialog.tsx`
- Create: `app/templates/page.tsx`
- Test: `components/templates/templates-view.test.tsx`

**Interfaces:**
- Consumes: `TEMPLATES`, `TEMPLATE_VARS`, `renderPreview`, `splitVars` (Task 4); `RoadmapNote`, `DataTable`, `Panel`, `SectionHeading`, `StateBadge`, `Badge`, `Button`, `Input`, `Label`, `Textarea`, `Select`, `Dialog`, `Tabs`, `CopyButton`.
- Produces: `app/templates/page.tsx` default export rendering `<TemplatesView />`.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TemplatesView } from "./templates-view";

describe("TemplatesView", () => {
  it("renders the roadmap note and the template list heading", () => {
    render(<TemplatesView />);
    expect(screen.getByText(/Roadmap screen/)).toBeInTheDocument();
    expect(screen.getByText("Template studio")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run - verify fails.** `pnpm test -- templates-view` -> FAIL.

- [ ] **Step 3: Implement the screen**

Port `ui_kits/dashboard/Templates.jsx` into three files, Tailwind + repo components:
- `templates-view.tsx` (`"use client"`): holds `rows` (init `TEMPLATES`), `open` template, `creating` bool. Renders `SectionHeading` ("Template studio", "Author the message once; the dispatcher renders from it. Select a template to edit.", action = "New template" `Button` with `Plus`), `<RoadmapNote />`, then `DataTable` (columns: name+id, channel `Badge`, locale mono, `v{version}` mono, status `StateBadge state={status==="active"?"active":"expired"}`, updated) with `onRowClick={setOpen}`. Renders `<NewTemplateDialog>` and, when `open`, `<TemplateDetail>`.
- `new-template-dialog.tsx`: `Dialog` with name `Input`, channel/locale `Select`, "start from" `Select` (blank + existing templates). On create, build a draft `Template` (v1, status "expired", `sends: []`) via spread and call `onCreate`.
- `template-detail.tsx`: back button -> "Template studio"; `SectionHeading` (name, id, actions "Test send"/"Save version"); a meta chip row (channel `Badge`, locale/version mono, status `StateBadge`, updated); two-col grid Editor (subject `Input` when email, body `Textarea` mono, variable-insert chips from `TEMPLATE_VARS`) + Preview (`Tabs` + preview card using `renderPreview`; body via `splitVars` is only for the editor highlight - preview shows substituted text). Below: Version history `Panel` (list from `tpl.versions`, first row `StateBadge active`, others "Restore" ghost `Button`) + Recent sends `Panel` (`DataTable` of `tpl.sends`, or inline "No sends from this template yet." when empty).
- `app/templates/page.tsx`:
```tsx
import { TemplatesView } from "@/components/templates/templates-view";
export default function TemplatesPage() {
  return <TemplatesView />;
}
```

- [ ] **Step 4: Run - verify pass.** `pnpm test -- templates-view` -> PASS. `pnpm typecheck`.

- [ ] **Step 5: Commit**

```bash
git add components/templates app/templates
git commit -m "feat(dashboard): Templates roadmap screen (list, detail, new-template dialog)"
```

---

## Task 6: Links screen

**Files:**
- Create: `components/links/links-view.tsx`, `components/links/link-detail.tsx`, `components/links/clicks-area.tsx`
- Create: `app/links/page.tsx`
- Test: `components/links/links-view.test.tsx`

**Interfaces:**
- Consumes: `LINKS`, `LINK_KPIS` (Task 4); `RoadmapNote`, `StatCard`, `CountUp`, `DataTable`, `Panel`, `SectionHeading`, `Input`, `Button`, `CopyButton`. Recharts for the chart.
- Produces: `<ClicksArea series={number[]} />` (14-day area chart, styled like `sends-area.tsx`); `app/links/page.tsx`.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LinksView } from "./links-view";

describe("LinksView", () => {
  it("renders the roadmap note and the Links heading", () => {
    render(<LinksView />);
    expect(screen.getByText(/Roadmap screen/)).toBeInTheDocument();
    expect(screen.getByText("Links")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run - verify fails.** `pnpm test -- links-view` -> FAIL.

- [ ] **Step 3: Implement**

- `clicks-area.tsx` (`"use client"`): Recharts `ResponsiveContainer`/`AreaChart` over `series.map((v,i)=>({i,v}))`, single `Area dataKey="v"` colour `var(--state-verified)` with a gradient def (0.35 -> 0.02), dashed `CartesianGrid vertical={false}`, muted 11px `YAxis` (width 30), hidden/minimal `XAxis`, `isAnimationActive={false}`. Match `sends-area.tsx` styling.
- `links-view.tsx`: `open` link state. List view: `SectionHeading` ("Links", "Short links carried by notifications, and what they earn. Select a link for its clicks."), `<RoadmapNote />`, four `StatCard`s from `LINK_KPIS` (icons `Link`, `MousePointerClick`, `TrendingUp`, `Zap`; wrap values in `CountUp`, using `format` for the percent ones), a "Shorten a URL" `Panel` (Input + "Create" `Button`), and a `DataTable` (short code `wl.link/{code}` + `CopyButton` copying `https://wl.link/{code}`, target mono truncated, clicks mono, ctr `Math.round(ctr*100)%`, created) with `onRowClick={setOpen}`.
- `link-detail.tsx`: back -> "Links"; `SectionHeading` (`wl.link/{code}`, target, actions "Open target"/"Copy link"); four `StatCard`s (total clicks, ctr, peak day = `Math.max(...series)`, redirect "302"); `Panel "Clicks over time" "Last 14 days"` with `<ClicksArea series={link.series} />`; `Panel "Recent clicks"` with `DataTable` of `link.recent`.
- `app/links/page.tsx`: render `<LinksView />`.

- [ ] **Step 4: Run - verify pass.** `pnpm test -- links-view` -> PASS. `pnpm typecheck`.

- [ ] **Step 5: Commit**

```bash
git add components/links app/links
git commit -m "feat(dashboard): Links roadmap screen with Recharts click analytics"
```

---

## Task 7: Campaigns screen

**Files:**
- Create: `components/campaigns/campaigns-view.tsx`, `components/campaigns/campaign-composer.tsx`, `components/campaigns/campaign-detail.tsx`
- Create: `app/campaigns/page.tsx`
- Test: `components/campaigns/campaign-composer.test.tsx`

**Interfaces:**
- Consumes: `CAMPAIGNS`, `AUDIENCES`, `CAMPAIGN_TEMPLATES` (Task 4), `recipientScope`, `sendWindowMinutes` (Task 4); `RoadmapNote`, `DataTable`, `Panel`, `StatCard`, `SectionHeading`, `StateBadge`, `Badge`, `Button`, `Input`, `Label`, `Select`, `Textarea`, `Dialog`, `Tabs`, `Separator`, `CopyButton`.
- Produces: `app/campaigns/page.tsx`.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CampaignComposer } from "./campaign-composer";

describe("CampaignComposer", () => {
  it("shows recipient scope for the default audience and enables send", () => {
    render(<CampaignComposer onBack={vi.fn()} onQueue={vi.fn()} />);
    // Newsletter subscribers = 8241 in scope
    expect(screen.getByText("8,241")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /send campaign/i })).toBeEnabled();
  });
});
```

- [ ] **Step 2: Run - verify fails.** `pnpm test -- campaign-composer` -> FAIL.

- [ ] **Step 3: Implement**

Port `ui_kits/dashboard/Campaigns.jsx`:
- `campaigns-view.tsx`: `rows` (init `CAMPAIGNS`), `view` = `{mode:"list"|"new"|"detail", cmp?}`. List: `SectionHeading` ("Campaigns", "Send an OTP batch or a marketing message to a saved audience or an uploaded list.", action "New campaign"), `<RoadmapNote />`, `DataTable` (name+id, type `Badge`, channel `Badge`, audience muted, recipients tabular, status via a `CampaignStatus` helper: sent->`StateBadge verified`, sending->`StateBadge sent`, scheduled->`Badge outline`, draft->`Badge secondary`, sent-when) with `onRowClick`.
- `campaign-composer.tsx` (`"use client"`): the full composer with `useState` per field (defaults from the kit). Derive `audience`, `tpl`, `isCsv`, `size = recipientScope({isCsv, size: audience.size, csvLoaded: !!csvName})`, `minutes = sendWindowMinutes(size, rate)`. Left column Panels Audience/Message/Delivery; sticky Review `Panel` showing `size` + summary rows + "Send campaign"/"Schedule campaign" (disabled when `!size`) + confirm `Dialog`. On confirm call `onQueue` with a new campaign object (spread, no mutation).
- `campaign-detail.tsx`: back -> "Campaigns"; `SectionHeading` (name, id, actions Duplicate + Pause when sending/scheduled); meta chip row; four outcome `StatCard`s (recipients/delivered/opened/clicked, `hint` = `pct` of scope); Recent sends `Panel` with a `DataTable` of the fixed sample rows from the kit.
- `app/campaigns/page.tsx`: render `<CampaignsView />`.

- [ ] **Step 4: Run - verify pass.** `pnpm test -- campaign-composer` -> PASS. `pnpm typecheck`.

- [ ] **Step 5: Commit**

```bash
git add components/campaigns app/campaigns
git commit -m "feat(dashboard): Campaigns roadmap screen (list, composer, detail)"
```

---

## Task 8: Login roadmap page

**Files:**
- Create: `app/login/page.tsx`, `components/login/login-view.tsx`
- Test: `components/login/login-view.test.tsx`

**Interfaces:**
- Produces: standalone `/login` page (no shell). `<LoginView />` is self-contained (local state only, no auth).

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { LoginView } from "./login-view";

describe("LoginView", () => {
  it("moves from email step to code step after submitting an email", async () => {
    render(<LoginView />);
    await userEvent.type(screen.getByLabelText(/work email/i), "you@company.com");
    await userEvent.click(screen.getByRole("button", { name: /send code/i }));
    expect(screen.getByText(/Enter your code/i)).toBeInTheDocument();
    expect(screen.getByText(/Roadmap screen/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run - verify fails.** `pnpm test -- login-view` -> FAIL.

- [ ] **Step 3: Implement**

Port `ui_kits/dashboard/Login.jsx` to `login-view.tsx` (`"use client"`): centered 380px card, wordmark dot + "worklane", `Panel` with two-step form (email -> code), the flag paragraph "Roadmap screen - the repository authenticates with a bearer key, not a session." Submitting the code form does nothing real (no redirect). `app/login/page.tsx` renders `<LoginView />`. Because `app/layout.tsx` wraps everything in the shell, render Login as a standalone route: put the shell in a route group (e.g. `app/(app)/layout.tsx`) OR have `login/page.tsx` render outside the sidebar by keeping the shell in `layout.tsx` but detecting the route. **Simplest per Next docs:** move the shell (sidebar+topbar) out of the root `layout.tsx` into a `(dashboard)` route group layout, leaving root layout as just providers/fonts; `/login` sits outside the group. Read `node_modules/next/dist/docs/` on route groups first; if that restructuring is risky, instead keep root layout minimal and render the shell in each page via a shared component. Pick the route-group approach if docs confirm it is stable.

- [ ] **Step 4: Run - verify pass.** `pnpm test -- login-view` -> PASS. Manually confirm `/login` renders without the sidebar and the shell still wraps the other routes. `pnpm typecheck`.

- [ ] **Step 5: Commit**

```bash
git add app components/login
git commit -m "feat(dashboard): Login roadmap page outside the app shell"
```

---

## Task 9: Nav additions

**Files:**
- Modify: `components/shell/nav.tsx`
- Test: `components/shell/nav.test.tsx` (create if absent)

**Interfaces:**
- Consumes: existing `NAV_ITEMS` shape `{ href; label; icon }`. Extend items with optional `soon?: boolean`.

- [ ] **Step 1: Add the three items**

Import `FileText, Megaphone, Link as LinkIcon` from lucide-react. Append to `NAV_ITEMS`:
```ts
{ href: "/templates", label: "Templates", icon: FileText, soon: true },
{ href: "/campaigns", label: "Campaigns", icon: Megaphone, soon: true },
{ href: "/links", label: "Links", icon: LinkIcon, soon: true },
```
Add `soon?: boolean` to `NavItem`. In the render, when `item.soon`, append a muted pill after the label: `<span className="ml-auto rounded-full border border-border px-1.5 py-0.5 text-[10px] text-muted-foreground">soon</span>`.

- [ ] **Step 2: Test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Nav } from "./nav";
describe("Nav", () => {
  it("lists the roadmap screens", () => {
    render(<Nav />);
    expect(screen.getByText("Templates")).toBeInTheDocument();
    expect(screen.getByText("Campaigns")).toBeInTheDocument();
    expect(screen.getByText("Links")).toBeInTheDocument();
  });
});
```
If `Nav` needs a router mock, wrap with the same pattern used elsewhere or mock `next/navigation`'s `usePathname` to return `"/"`.

- [ ] **Step 3: Run - verify pass.** `pnpm test -- nav` -> PASS.

- [ ] **Step 4: Commit**

```bash
git add components/shell/nav.tsx components/shell/nav.test.tsx
git commit -m "feat(dashboard): add Templates/Campaigns/Links to sidebar nav"
```

---

## Task 10: Existing-screen drift audit

**Files:** read-only diff of `app/page.tsx`, `components/overview/*`, `app/api-keys/*`, `app/logs/*`, `components/logs/*`, `app/playground/*`, `components/playground/*` against `guidelines/*.card.html` and the kit screens.

- [ ] **Step 1: Compare each shipped screen to its DS specimen**

For spacing (4px step; cards p-4, panels p-5, grids gap-4, sections 24px), radii (cards/panels/tables 14px = `rounded-xl`, buttons/inputs 10px = `rounded-md`), type scale, and copy (sentence case, middot units), list any concrete drift. The DS was extracted from these screens, so expect little.

- [ ] **Step 2: Fix only concrete drift**

Apply surgical fixes for anything that genuinely diverges (a wrong token, a stray Title Case string, a spacing step). Do not refactor. If nothing drifted, note that in the commit body and skip.

- [ ] **Step 3: Verify + commit (only if changes)**

Run `pnpm lint && pnpm typecheck && pnpm test`. Commit:
```bash
git commit -am "fix(dashboard): align shipped screens to design-system tokens"
```
(If no drift, no commit; record the finding in the PR description.)

---

## Task 11: Screenshots, gallery, final gate

**Files:**
- Modify: `scripts/capture.ts`, `docs/dashboard-gallery.md`

- [ ] **Step 1: Add new routes to the capture script**

Add `/templates`, `/campaigns`, `/links`, `/login`, and a request-detail capture (navigate `/requests`, click first row) to `scripts/capture.ts`'s route/shot list, following its existing pattern.

- [ ] **Step 2: Regenerate + embed**

```bash
pnpm build
pnpm start -p 3100 &
BASE=http://localhost:3100 pnpm capture
```
Add the new images to `docs/dashboard-gallery.md` with sentence-case captions.

- [ ] **Step 3: Full gate**

Run `pnpm lint && pnpm typecheck && pnpm test` - all green.

- [ ] **Step 4: Commit**

```bash
git add scripts/capture.ts docs/dashboard-gallery.md docs/assets/dashboard
git commit -m "docs(dashboard): capture and gallery for roadmap screens + request detail"
```

---

## Self-Review

- **Spec coverage:** request detail (T2-3), Templates (T5), Links (T6), Campaigns (T7), Login (T8), fixtures/UI-only boundary (T4 + Global Constraints), new primitives dialog/select/textarea (T1), DataTable onRowClick (T1), nav (T9), existing-screen audit (T10), tests + gate (each task + T11), capture/gallery (T11). All spec sections map to a task.
- **Placeholder scan:** logic and tests carry real code; presentational markup is a port from named on-disk reference files with explicit component mappings (an existing-codebase port, not a TODO). No "TBD"/"handle edge cases".
- **Type consistency:** `lifecycleStages(state, log?)`, `renderPreview`/`splitVars`, `recipientScope`/`sendWindowMinutes`, `RoadmapNote`, `DataTable onRowClick`, and the fixture types are referenced with the same names/signatures in their consumer tasks.
- **Route-group risk (T8):** flagged with a fallback (shared shell component) if route groups prove unstable in this Next build.
