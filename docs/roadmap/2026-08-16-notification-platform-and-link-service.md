# Roadmap: Notification Platform + Link Service (future feature)

**Status:** Future - not scheduled. Idea captured during brainstorming.
**Hard gate:** Start only **after OTP email + SMS are live in production and stable.**
**Noted:** 2026-08-16.
**References the author wants to learn from:**
- Notification System - system-design-notes 10
  (https://github.com/liquidslr/system-design-notes/blob/main/10.%20Notification%20System/Readme.md)
- URL Shortener - system-design-notes 08
  (https://github.com/liquidslr/system-design-notes/blob/main/08.%20URL%20Shortener/Readme.md)

## 1. The seed idea

Build a **template studio page in the dashboard** to author messages, then fire
**notifications + short URLs** from them. Two goals at once: ship real product
value, and deliberately practice two canonical system-design patterns.

## 2. Vision

worklane today is OTP-only. The natural evolution is a **general notification
platform** (transactional + broadcast, multi-channel) whose messages carry
**trackable short links**. Both halves map cleanly onto what already exists:
the hexagonal services, the Kafka event bus, MySQL (incl. the `templates` table),
Redis, and the new dashboard with its swappable data layer.

## 3. Decomposition (3 sub-projects)

This spans independent subsystems, so it is **not one spec**. Each piece below
gets its own brainstorm → spec → plan cycle when its turn comes.

### A. Template Studio (dashboard + template API)

- **Dashboard page:** list / create / edit templates; pick channel
  (email / SMS / push), locale, subject + body with variables like
  `{{code}}`, `{{name}}`, `{{link}}`; live preview; test-send into the Playground;
  simple versioning.
- **Backend:** promote the existing `templates` MySQL table to a real CRUD API,
  and make the dispatcher render from it (single source of truth).
- **Learn:** template versioning, safe variable substitution, and preview that
  renders through the *exact* delivery path (so the preview cannot lie).

### B. Notification Service - generalize the dispatcher (maps to notes 10)

Generalize `otp-dispatcher` from OTP-only to any notification.

- New event `notification.requested` alongside `otp.requested`; channels
  email / SMS / push behind provider ports (reuse the `EmailProvider` pattern;
  add Twilio / FCM / APNS later).
- **Patterns to implement and learn:**
  - Message-queue buffering (already Kafka).
  - **Idempotency / dedup via event id** - designed in from day one.
  - **Retry + exponential backoff + DLQ** (partly exists for OTP).
  - **Per-user / per-tenant rate limiting** (Redis; the OTP limiter is a start).
  - **User preferences / opt-out** table, enforced server-side.
  - **Fan-out** workers; **circuit breaker** per provider.
  - **Engagement analytics** (delivered / open / click) feeding the dashboard.
- **Data:** `notification_log`, `notification_settings` (preferences),
  `device_tokens` (push). Generalize `delivery_logs`.

### C. Link Service - URL shortener (maps to notes 08)

A new, self-contained bounded context `link-svc`.

- **API:** `POST /v1/links` (create), `GET /:code` (redirect).
- **Code generation:** distributed unique id (Snowflake or a counter) then
  **base62** encoding - collision-free, and the cleanest thing to learn. Note the
  tradeoff vs hash + Bloom filter (fixed length but needs collision handling).
- **Redirect with 302, not 301**, so clicks stay trackable; emit `link.clicked`
  events → analytics. (301 is browser-cached and kills click tracking.)
- **Data:** `links(id PK, code UK, long_url, tenant_id, created_at)`; click events
  append-only. Cache hot codes in Redis (cache-aside read path).
- **Integrate:** a template variable `{{link "https://..."}}` auto-shortens at
  render time; per-notification CTR surfaces in the dashboard.
- **Learn:** base62 + distributed id gen, cache-aside, 301 vs 302, sharding /
  replication for scale, rate limiting by IP / tenant.

## 4. How it reuses existing worklane

- Monorepo + hexagonal shape, Kafka contracts, MySQL, Redis, dispatcher shape.
- The **dashboard's swappable data layer** makes new screens cheap: add
  `Templates`, `Notifications`, `Links / Analytics` screens against the same
  `DataSource` interface, mock fixtures first, wire live later - exactly the
  pattern already established.

## 5. Decisions to settle at spec time (not now)

- New services vs extend otp-api / dispatcher? Leaning: **extend the dispatcher**
  for B; **new small `link-svc`** for C (to practice a clean bounded context +
  id generation).
- id-gen: Snowflake vs DB-counter vs hash. Leaning Snowflake/counter → base62.
- Push (FCM / APNS) scope - likely defer past the first cut.
- Analytics store: MySQL first, columnar/OLAP only if volume demands it.

## 6. Suggested order & gates

0. **GATE:** OTP email + SMS in production, stable.
1. **A - Template Studio** (small, high UI value; unblocks the rest).
2. **C - Link Service** (self-contained; the purest system-design learning; ships
   independently).
3. **B - Notification Service** (largest; builds on A's templates and C's links).

## 7. Pitfalls to remember

- Design dedup / idempotency (event ids) in from the start, never bolt it on.
- Preview must render through the same template path as delivery, or it lies.
- Use 302 for short-link redirects; 301 caching destroys click analytics.
- base62 straight off an auto-increment leaks volume and is enumerable; use
  Snowflake or a hashed-then-encoded id if enumeration matters.
- Rate limits and preferences are enforced in the service, never trusted from
  the client.
