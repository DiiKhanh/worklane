# worklane dashboard

Developer dashboard for the worklane OTP / verification platform. Dark-first,
built with Next.js (App Router), shadcn/ui, TanStack Query + Table, Zustand,
React Hook Form + Zod, Recharts, and Motion.

Screenshots: [../docs/dashboard-gallery.md](../docs/dashboard-gallery.md).

## Run

```bash
pnpm install
pnpm dev          # http://localhost:3000
```

The app ships with realistic **mock data** by default - no backend needed.

## Data source

The whole UI talks to one `DataSource` interface (`lib/api/source.ts`). Which
implementation it uses is chosen by an env var:

```bash
# .env.local
NEXT_PUBLIC_DATA_SOURCE=mock          # in-memory fixtures (default)
# or
NEXT_PUBLIC_DATA_SOURCE=live          # real otp-api
NEXT_PUBLIC_API_BASE=http://localhost:8888
```

In `live` mode the dashboard calls the otp-api REST endpoints
(`/v1/api-keys`, `/v1/otp/requests`, `/v1/delivery-logs`, `/v1/otp/send|verify`)
with a Bearer key. The Overview aggregate tiles are mock-only until the API
exposes a stats endpoint - see the design spec.

Architecture rule: server data lives only in the TanStack Query cache; Zustand
holds UI state only (filters, token). Theme is owned by next-themes.

## Test

```bash
pnpm test         # vitest (unit + hooks)
pnpm typecheck    # tsc --noEmit
pnpm lint
```

## Re-capture screenshots

```bash
pnpm build
pnpm start -p 3100 &
BASE=http://localhost:3100 pnpm capture   # -> ../docs/assets/dashboard/*.png
```

Captures run with reduced motion so animation-aware components render their
final state, keeping the images deterministic.
