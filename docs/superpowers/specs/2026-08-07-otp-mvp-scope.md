# OTP Platform - MVP Scope ("Walking Skeleton v1")

- **Date:** 2026-08-07
- **Status:** Approved for planning
- **Author:** duykhanh
- **Companion to:** [2026-08-06-otp-verification-platform-design.md](./2026-08-06-otp-verification-platform-design.md)

## 1. Intent

This document narrows the full design spec into a concrete, buildable **MVP**. It does not
replace the design spec; it selects the first slice to build.

**Guiding decision:** the full distributed architecture (GoFrame microservices, APISIX, Kafka,
k3s) **is the point** of this project (CV showcase). So the MVP is **not** a feature-rich product
on a simple stack - it is a **walking skeleton**: one thin end-to-end thread that runs through
*every* architectural layer, deployed for real, then thickened later.

**Stack alignment:** the backend mirrors the author's production stack — **GoFrame v2, MySQL,
Redis, Kafka via IBM/sarama, Kustomize deploy** — so it doubles as interview material. Conventions
are captured in [../reference/goframe-backend-conventions.md](../reference/goframe-backend-conventions.md).

The "fastest MVP" pressure applies to **feature breadth** and to the **frontend** (Next.js +
shadcn), not to the architecture. We minimize *what* the thread does, not *which layers* it
crosses.

## 2. The MVP thread (end-to-end)

One email OTP: `send` → real email with a 6-digit code → `verify`, running through APISIX →
`otp-api` → Kafka → `dispatcher` → email provider, deployed to k3s over HTTPS.

```mermaid
sequenceDiagram
    participant C as Client (curl)
    participant GW as APISIX
    participant API as otp-api
    participant R as Redis
    participant PG as MySQL
    participant K as Kafka
    participant D as dispatcher
    participant E as Email provider

    C->>GW: POST /v1/otp/send (API key)
    GW->>GW: TLS, verify API key, edge rate-limit
    GW->>API: forward
    API->>API: validate + business rate-limit
    API->>R: store code HASH + TTL, counters
    API->>PG: insert otp_requests (requested)
    API->>K: publish otp.requested
    API-->>C: 202 Accepted (request id)
    K->>D: consume otp.requested
    D->>E: render template + send email
    E-->>D: result
    D->>PG: insert delivery_logs
    D->>K: publish otp.sent / otp.failed

    Note over C,E: --- later ---
    C->>GW: POST /v1/otp/verify (code)
    GW->>API: forward
    API->>R: constant-time hash compare, attempt counter
    API->>PG: update otp_requests (verified / rejected)
    API-->>C: accept / reject
```

## 3. In scope (MVP)

| Area | Decision |
|------|----------|
| **Channel** | **Email only** via **Resend** (hosted API, free-tier). SMS is deferred so provider brand-name approval never blocks the MVP. |
| **Services** | Two GoFrame v2 services: `otp-api` (HTTP, sync path) + `dispatcher` (Kafka consumer via sarama, async path). **No** worker/cron yet. |
| **Gateway** | APISIX: TLS termination, API-key auth plugin, coarse edge rate limiting, routing to `otp-api`. |
| **Kafka** | Topics `otp.requested`, `otp.sent`, `otp.failed`. `otp.dlq` is **declared** and the dispatcher may publish to it on failure, but there is **no drainer** in the MVP. |
| **Redis** | OTP code **hash** + TTL, rate-limit counters, resend-cooldown markers, verify-attempt counters. |
| **MySQL** | `tenants`, `api_keys`, `otp_requests`, `delivery_logs`, `templates` (accessed via GoFrame `gf gen dao` DAOs). |
| **Core domain** ⭐ | Crypto-random code generation, hash-only storage, constant-time verification, **four-layer rate limiting** (per recipient / per tenant / resend cooldown / verify-attempt lock), `Idempotency-Key`, and the `requested → sent \| failed → verified \| expired` state model. Fully unit-tested. |
| **Tenant / API key** | Created via a **CLI seed script** (or a protected admin endpoint). The data model stays multi-tenant; only the *creation UX* is deferred. |
| **Dashboard** | New Next.js + shadcn/ui app (from a shadcn block). **Three read-only screens:** (1) API keys list, (2) OTP request history, (3) delivery-logs status via React Query polling. Stack per §8 of the design spec (TanStack Query, Zustand, RHF+Zod, TanStack Table). |
| **Deploy** | **Code-first:** build and green the thread on docker-compose locally, then package **Kustomize manifests** (base + `develop` overlay) and deploy to **k3s** on a self-hosted machine, reachable at `https://<domain>` over HTTPS. The **dashboard deploys separately to Vercel**. Hosting/networking (self-hosted k3s host, Cloudflare, Tenten domain) is specified in its own doc - see [2026-08-07-self-hosted-infra-setup.md](./2026-08-07-self-hosted-infra-setup.md). |
| **Testing** | Unit (domain logic, 80% target), integration via testcontainers (Redis / MySQL / Kafka), end-to-end `send → verify` through APISIX against running services. |

**Why keep the full core domain in the MVP:** it is pure, infra-independent code - cheap to write
and test - and it is the strongest interview material (anti-brute-force, anti-spam, idempotency).
Cutting it to "go faster" would cut the wrong thing.

## 4. Out of scope (deferred)

- **Fast-follow** (immediately after the happy path is green): `worker/cron` - DLQ drain with
  backoff + scheduled cleanup of expired OTP / stale audit rows.
- **Phase 2:** SMS channel + Twilio failover; multi-provider failover + retry/backoff hardening;
  LGTM observability + OpenTelemetry; dashboard auth (NextAuth/Auth.js) + self-serve API-key CRUD;
  HMAC request signing.
- **Phase 3:** push / in-app channels; ArgoCD / GitOps; autoscaling.
- **Outside the product entirely:** floci (a personal AWS-emulator playground).

## 5. Success criteria (verifiable)

1. `POST /v1/otp/send` with a seeded API key returns `202` and delivers a **real email** with a
   6-digit code.
2. `POST /v1/otp/verify` accepts the correct code and rejects a wrong or expired one.
3. The full path runs as `otp-api` + `dispatcher` behind **APISIX**, communicating over **Kafka**,
   deployed to **k3s**, reachable at `https://<domain>`.
4. Rate limiting and the verify-attempt lock are demonstrably enforced (tests prove it).
5. The dashboard shows the request and its delivery status by polling.
6. Domain unit tests pass at **80%** coverage; the e2e `send → verify` test passes.

## 6. Build order

1. Core domain package (TDD, no infra) → unit tests green.
2. `otp-api` GoFrame HTTP service wrapping the domain, Redis + MySQL wired, publishing `otp.requested`.
3. `dispatcher` GoFrame service consuming `otp.requested` (sarama), sending email, writing `delivery_logs`.
4. End-to-end on **docker-compose** (APISIX + Kafka + Redis + MySQL) → `send → verify` green.
5. Kustomize manifests → deploy to **k3s** on the self-hosted machine → HTTPS on the domain.
6. Next.js dashboard (3 read-only screens) wired to the read APIs.
7. **Fast-follow:** add `worker/cron`.

## 7. Resolved decisions

1. **Email provider:** **Resend** (hosted API).
2. **k3s host:** **self-hosted on a personal machine** (not a cloud VPS); domain registered at
   **Tenten**, exposed via **Cloudflare**. Full topology and setup live in
   [2026-08-07-self-hosted-infra-setup.md](./2026-08-07-self-hosted-infra-setup.md).
3. **Dashboard hosting:** **Vercel** (separate from the cluster). This makes the dashboard→API
   call **cross-origin**, so **CORS** for the Vercel origin must be configured on
   APISIX / `otp-api`.
4. **Edge TLS:** terminated at **Cloudflare** (not APISIX). APISIX remains the business gateway
   (API-key auth, rate limiting, routing) behind the Cloudflare edge.
