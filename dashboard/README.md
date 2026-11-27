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

The app ships with realistic **mock data** by default - no backend or login needed.
Open `/` or `/playground` directly. The `/login` page always calls the real auth
service, including when the dashboard uses mock data.

## Data source

The whole UI talks to one `DataSource` interface (`lib/api/source.ts`). Which
implementation it uses is chosen by an env var:

```bash
# .env.local
NEXT_PUBLIC_DATA_SOURCE=mock          # in-memory fixtures (default)
# or
NEXT_PUBLIC_DATA_SOURCE=live          # real otp-api
NEXT_PUBLIC_API_BASE=http://localhost
```

`NEXT_PUBLIC_API_BASE` must point to the shared gateway that routes both `/auth/*`
and `/v1/*`. The compose stack exposes that gateway at `http://localhost`;
port 8888 belongs to otp-api alone and does not serve login. Restart the dev
server after changing these variables; production builds embed them at build time.

Live operator accounts are created by `services/seed`, with an explicit email and
password. Vercel hosts only the dashboard and does not create accounts. The seed
Job manifest under `deploy/k8s/overlays/prod/seed-job.yaml` contains example
credentials, not evidence of the current production password. Passwords are
stored as Argon2id hashes and cannot be read back. Re-running the seed command
creates new records; it is not a password reset operation.

Playground shows send/verify failures inline and announces completed results.
The send result identifies the submitted recipient and links the development-code
handoff to that recipient even if the input is edited later. Results appear
immediately for repeated pointer and keyboard submissions, without movement.

In `live` mode the dashboard calls the auth/otp REST endpoints
(`/auth/login`, `/auth/me`, `/auth/api-keys`, `/v1/otp/requests`, `/v1/delivery-logs`,
`/v1/stats`, `/v1/otp/send|verify`) with a Bearer token. The Overview screen reads tenant-scoped
rolling 24h aggregates from `/v1/stats`.

The Links screen calls link-svc's `/v1/links` routes through the same gateway, so the
gateway must route `/v1/links*` to link-svc. Short URLs shown and copied are the
`short_url` values link-svc returns from its `LINK_PUBLIC_BASE`. See
[../docs/features/link-dashboard.md](../docs/features/link-dashboard.md).

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
