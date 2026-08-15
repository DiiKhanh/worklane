# Walkthrough: Phase 2-3 (adapters + services)

A tech-lead-style tour of how the pieces we built in Phase 2 (adapters) and Phase 3 (services) fit
together. Phase 1 gave us a pure core that talks only in interfaces; here we plug real infrastructure
into those interfaces and assemble two runnable microservices. Read this alongside the concept-by-
concept notes in [go-notes.md](./go-notes.md).

## 0. Where we were, where we are

- **After Phase 1:** a pure hexagon core - `domain` (rules) + `app` (use cases over ports). It could
  not touch a database or send an email; it only knew interfaces (`CodeStore`, `Repo`, `Publisher`...).
- **After Phase 2:** each of those interfaces has a real implementation (an *adapter*): Redis, MySQL,
  Kafka, Resend. The core still does not import them - the adapters import the core.
- **After Phase 3:** two `main.go` composition roots wire adapters into the core and expose it -
  `otp-api` over HTTP (Gin), `otp-dispatcher` as a Kafka consumer. The system is now a real,
  runnable thing (once Phase 4 gives it a compose stack).

## 1. The full journey of one OTP (end to end)

This is the payoff - trace a single `send` then `verify` across everything we built.

```
  curl POST /v1/otp/send  (Authorization: Bearer <key>)
        │
        ▼
  [otp-api]  Gin router
    ├─ apiKeyAuth middleware      → hash key → mysqlrepo.FindAPIKey → put tenant_id in ctx
    ├─ Send handler               → bind JSON → app.Service.Send(SendInput)
    │     app.Service.Send:
    │        ├─ Counter.Exists/Set   ──► redisstore  (idempotency)
    │        ├─ RateLimiter          ──► redisstore  (Counter: recipient/tenant/cooldown)
    │        ├─ domain.GenerateCode + HashCode        (pure)
    │        ├─ CodeStore.Save       ──► redisstore  (hash+salt, TTL)
    │        ├─ Repo.InsertRequest   ──► mysqlrepo    (audit row, recipient masked)
    │        └─ Publisher.Publish    ──► kafka producer  ("otp.requested")
    └─ 202 Accepted { request_id }
        │
        │   (Kafka decouples the two services here)
        ▼
  [otp-dispatcher]  kafka consumer group
    ├─ consumer.Handle(raw)       → kafka.Unwrap envelope → contracts.RequestedEvent
    └─ app.Handler.Handle(evt):
          ├─ Template.Render(code)
          ├─ EmailProvider.Send     ──► resendmail (real email)
          ├─ Repo.InsertDeliveryLog ──► mysqlrepo
          ├─ Repo.UpdateState       ──► mysqlrepo  ("sent" | "failed")
          └─ Publisher.Publish      ──► kafka  ("otp.sent" | "otp.failed" + "otp.dlq")

  ... later ...
  curl POST /v1/otp/verify  → otp-api → app.Service.Verify:
        ├─ CodeStore.Get          ──► redisstore   (missing → 410 Gone)
        ├─ attempt-lock guard                       (over cap → 429)
        ├─ domain.VerifyHash (constant-time)        (wrong → 401)
        └─ CodeStore.Delete + Repo.UpdateState("verified")
```

The single most important thing to notice: **every arrow marked `──►` crosses a port**. The use case
(`app.Service`, `app.Handler`) never names Redis/MySQL/Kafka/Resend. It calls an interface; the
composition root decided which concrete thing is on the other side.

## 2. Phase 2 pattern: one adapter = one (or more) port(s)

Every adapter follows the same shape:

1. It lives in `internal/adapters/outbound/<x>` (a driven adapter) or `inbound/<x>` (a driver).
2. It implements a port method-for-method. We prove it at compile time with
   `var _ app.Repo = (*mysqlrepo.Repo)(nil)`.
3. It maps between the app's types and the infrastructure's types **at the boundary** - GORM row
   structs, JSON envelopes, Resend request bodies stay *inside* the adapter and never leak inward.
4. It translates infrastructure errors into domain errors where the app cares -
   `redis.Nil → domain.ErrNotFound`.

Concrete highlights (the "why", not just the "what"):

- **redisstore** implements *two* ports (`CodeStore` + `Counter`) on one client. `Incr` sets the TTL
  only on the first hit so a rate-limit window is fixed, not sliding-forever.
- **mysqlrepo** uses GORM but keeps row structs private and drives schema with explicit
  `golang-migrate` SQL, not `AutoMigrate` (reviewable, reversible).
- **kafka** (in `pkg/platform`, shared) wraps sarama with a typed `Envelope{MsgType, Data}` and marks
  a message only after the handler succeeds - **at-least-once** delivery, which is why consumers must
  be idempotent.
- **resendmail** injects its `baseURL` + `*http.Client` so an `httptest` stub can stand in for the
  real API - the same dependency-injection trick, applied to an outbound HTTP call.

How we trust them: **testcontainers**. Each adapter's `//go:build integration` test boots the *real*
service (MySQL 8, Redis 7, Redpanda) in a throwaway container and runs the adapter against it. That
catches real bugs a mock would hide - a wrong column, a Redis command quirk, a migration that will
not apply.

## 3. Phase 3 pattern: services assemble the hexagon

A service is: **inbound adapter → use case (ports) → outbound adapters**, wired in `main.go`.

- **otp-api** (`services/otp-api`): Gin is the inbound adapter. The handler depends on an
  `OTPService` interface it declares itself (so tests inject a fake) plus `app.Repo` for the read
  endpoints. Auth is a middleware that resolves the API key to a tenant and stashes it in the request
  context. One `errors.go` maps every domain sentinel to an HTTP status - the only place transport
  codes are decided.
- **otp-dispatcher** (`services/otp-dispatcher`): the Kafka consumer is the inbound adapter. Its
  `consumer.Handle` decodes the envelope (a transport concern) and calls `app.Handler.Handle(evt)` -
  so the app layer receives a clean `contracts.RequestedEvent`, never raw Kafka bytes.

**The composition root** (`main.go`) is the one file allowed to know every concrete type. It reads
config, opens MySQL/Redis/Kafka, builds the adapters, injects them into `app.New(...)`, and starts
serving - with graceful shutdown on `SIGINT`/`SIGTERM`. Read it top to bottom and you see the whole
service's shape; there are no hidden globals.

## 4. Why two services, and why Kafka between them

`otp-api` and `otp-dispatcher` are separate deployables that **share no Go code** (Go's `internal/`
rule enforces it - that is why the dispatcher has its own little `mysqlrepo`). They meet in exactly
two places, both deliberate:

- the **database schema** (one set of migrations), and
- the **Kafka event contract** (`pkg/contracts/otp`).

Kafka between them is the core architectural decision: `otp-api` publishes `otp.requested` and returns
`202` immediately, so the API stays fast even if Resend is slow or down. Delivery happens
independently in the dispatcher and is retryable; a permanent failure is routed to `otp.dlq`. This
sync/async split is the strongest distributed-systems story in the project.

## 5. What is NOT here yet (Phase 4-5)

- No `docker-compose` stack, Traefik gateway, or Dockerfiles yet - so nothing runs *together* outside
  tests. That is Phase 4, which also adds an end-to-end `send → verify` test (via a MailHog inbox) and
  the local web UIs (Adminer, RedisInsight, Redpanda Console).
- No seed CLI to mint an API key, and no dashboard. That is Phase 5.

Everything above is written and tested in isolation; Phase 4 is where it first breathes as one system.
