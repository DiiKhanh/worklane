# Architecture (after Phases 1-4)

Visual reference for the OTP walking skeleton as actually built and running. Diagrams are Mermaid
(render in GitHub, most IDEs, or any Mermaid viewer). For the folder layout and the GoFrame→Gin
mapping see [reference/architecture-and-layout.md](./reference/architecture-and-layout.md); for the
concept-by-concept learning notes see [learning/go-notes.md](./learning/go-notes.md).

---

## 1. Runtime topology (docker-compose stack)

What runs, and how a request flows through it. The two services share nothing at runtime except the
MySQL schema and the Kafka event contract - they talk only over Kafka.

```mermaid
flowchart LR
    Client["Client<br/>(curl / dashboard)"]

    subgraph edge["Gateway"]
        Traefik["Traefik<br/>route /v1/* · rate-limit · CORS"]
    end

    subgraph api["otp-api (Gin) · sync"]
        API["HTTP :8888<br/>auth · validate · Send/Verify"]
    end

    subgraph disp["otp-dispatcher · async"]
        DISP["Kafka consumer<br/>render · send · log"]
    end

    Redis[("Redis<br/>code hash+TTL, counters")]
    MySQL[("MySQL<br/>identity db: tenants, users, api_keys<br/>otp db: otp_requests, delivery_logs, templates")]
    Kafka{{"Kafka / Redpanda<br/>otp.requested · sent · failed · dlq"}}
    Mail["Email<br/>MailHog (local) / Resend (prod)"]

    Client -->|"HTTPS /v1/*"| Traefik --> API
    API -->|"hash+TTL, rate-limit"| Redis
    API -->|"audit row"| MySQL
    API -->|"publish otp.requested"| Kafka
    Kafka -->|"consume"| DISP
    DISP -->|"send email"| Mail
    DISP -->|"delivery_logs, state"| MySQL
    DISP -->|"otp.sent / failed / dlq"| Kafka
    API -->|"202 request_id"| Client
```

Dev-only web UIs (also in compose, not part of the request path): Adminer (MySQL), RedisInsight,
Redpanda Console, MailHog, Traefik dashboard.

**Auth & identity:** human login (email+password -> EdDSA JWT) and machine API keys are owned
by a dedicated `auth-svc` backed by its own `identity` database (tenants, users, api_keys).
otp-api verifies JWTs locally with the public key, and resolves API keys by calling auth-svc's
network-internal `/internal/introspect` (Redis-cached). See
[dashboard-auth-identity-service-design](superpowers/specs/2026-08-19-dashboard-auth-identity-service-design.md).

---

## 2. End-to-end sequence (send, then verify)

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant GW as Traefik
    participant A as otp-api
    participant R as Redis
    participant DB as MySQL
    participant K as Kafka
    participant D as otp-dispatcher
    participant M as Email (MailHog/Resend)

    C->>GW: POST /v1/otp/send (Bearer key)
    GW->>A: forward (edge rate-limit + CORS)
    A->>A: hash key, resolve tenant, validate
    A->>R: check idempotency + rate limits
    A->>A: GenerateCode + HashCode (pure)
    A->>R: store hash+salt, TTL
    A->>DB: insert otp_requests (requested, masked)
    A->>K: publish otp.requested
    A-->>C: 202 { request_id }

    K->>D: consume otp.requested
    D->>M: render template + send email

    alt Email sent successfully
        M-->>D: ok
        D->>DB: insert delivery_logs, state = sent
        D->>K: publish otp.sent
    else Email delivery failed
        M-->>D: error
        D->>DB: insert delivery_logs, state = failed
        D->>K: publish otp.failed
        D->>K: publish otp.dlq
    end

    Note over C,M: --- later ---

    C->>GW: POST /v1/otp/verify (code)
    GW->>A: forward
    A->>R: get record + attempt-lock guard
    A->>A: VerifyHash (constant-time)

    alt Code valid
        A->>R: delete code (single use)
        A->>DB: state = verified
        A-->>C: 200 verified
    else Code mismatch
        A-->>C: 401 mismatch
    else Code expired or missing
        A-->>C: 410 gone
    else Too many attempts
        A-->>C: 429 locked
    end
```

---

## 3. Hexagon inside one service (otp-api)

Dependencies point inward only. The inbound adapter drives a use case; the use case depends on ports;
outbound adapters implement those ports. The composition root (`main.go`) is the only place that knows
the concrete adapters.

```mermaid
flowchart TB
    subgraph svc["otp-api"]
        direction TB
        IN["inbound/http<br/>Gin handlers · api-key middleware · DTOs"]
        APP["app<br/>Send / Verify use cases · RateLimiter · ports.go"]
        DOM["domain (pure)<br/>GenerateCode · HashCode · State · MaskRecipient"]
        RS["outbound/redisstore<br/>(CodeStore + Counter)"]
        MR["outbound/mysqlrepo<br/>(Repo)"]
        KB["outbound/kafkabus<br/>(Publisher)"]

        IN -->|calls| APP
        APP -->|uses| DOM
        APP -. "port: CodeStore/Counter" .-> RS
        APP -. "port: Repo" .-> MR
        APP -. "port: Publisher" .-> KB
    end

    RS --> Redis[("Redis")]
    MR --> MySQL[("MySQL")]
    KB --> Kafka{{"Kafka"}}

    MAIN["main.go (composition root)<br/>builds adapters, injects into app.New(...)"]
    MAIN -.wires.-> IN
    MAIN -.wires.-> RS
    MAIN -.wires.-> MR
    MAIN -.wires.-> KB
```

`otp-dispatcher` has the same shape: inbound is a Kafka consumer, the use case is `Handle`, and the
outbound ports are `EmailProvider` (resendmail **or** smtpmail, chosen by config), `Repo`, `Publisher`.

---

## 4. OTP request state machine

Enforced in `domain/state.go` (allowed transitions) and driven by the two services.

```mermaid
stateDiagram-v2
    [*] --> requested: otp-api issues code
    requested --> sent: dispatcher email ok
    requested --> failed: dispatcher email error (→ DLQ)
    sent --> verified: correct code
    sent --> expired: TTL elapsed / no verify
    verified --> [*]
    failed --> [*]
    expired --> [*]
```

---

## 5. Data model (MySQL)

> Database-per-service: `tenants`, `users`, and `api_keys` live in the **identity** database
> (owned by auth-svc); `otp_requests`, `delivery_logs`, and `templates` live in the **otp**
> database (owned by otp-api). The relationships below are logical - `tenant_id` is a soft
> reference across databases, with no cross-database foreign key.

```mermaid
erDiagram
    tenants ||--o{ api_keys : "has"
    tenants ||--o{ otp_requests : "owns"
    otp_requests ||--o{ delivery_logs : "produces"
    templates ||--o{ template_versions : "has"

    tenants {
        char id PK
        varchar name
        datetime created_at
    }
    api_keys {
        char id PK
        char tenant_id FK
        varchar hashed_key UK
        varchar status
    }
    otp_requests {
        char id PK
        char tenant_id FK
        varchar recipient_masked
        varchar channel
        varchar state
        datetime created_at
    }
    delivery_logs {
        bigint id PK
        char request_id
        char tenant_id
        varchar provider
        varchar status
        bigint latency_ms
        text error
    }
    templates {
        char id PK
        varchar name
        varchar channel
        varchar locale
        varchar status
        char active_version_id FK
    }
    template_versions {
        char id PK
        char template_id FK
        int version_no
        varchar subject
        text body
        varchar status
        varchar created_by
    }
```

> Note: the plaintext OTP code lives **only** in Redis (as a salted hash, TTL-bound) and is never in
> MySQL. `otp_requests.recipient_masked` stores `d***@gmail.com`, not the real address.

> **Template Studio (sub-project A):** `templates` is a logical entity (one per `channel` + `locale`),
> and each edit appends an immutable `template_versions` row. Publishing repoints
> `templates.active_version_id` (`draft → published → superseded`). The dispatcher resolves the
> active template for `(channel, locale)` via **Redis cache-aside** (`tmpl:{channel}:{locale}`,
> invalidated by otp-api on publish) and renders it through the shared `pkg/templating` engine -
> the same code the dashboard preview calls, so a preview cannot diverge from what is delivered.
> If no active row exists (or on any DB/cache error) the dispatcher **falls back to the env-config
> template**, so OTP delivery never breaks. Variables are the allowlisted `{{code}}` and `{{expiry}}`.

---

## 6. Monorepo, deployables

```mermaid
flowchart LR
    subgraph repo["worklane (one Go module)"]
        subgraph services["services/"]
            A["otp-api<br/>main.go + internal/*"]
            AUTH["auth-svc<br/>main.go + internal/*"]
            D["otp-dispatcher<br/>main.go + internal/*"]
            L["link-svc<br/>main.go + internal/*"]
            LD["link-dispatcher<br/>main.go + internal/*"]
            N["notification-api<br/>main.go + internal/*"]
            S["seed (CLI)"]
        end
        subgraph pkgs["pkg/ (shared)"]
            P1["platform/*<br/>config, mysql, redis, kafka"]
            P2["contracts/*<br/>otp, link, notification event schemas"]
            P3["security<br/>api-key hash/gen"]
            P4["idgen<br/>Snowflake + Base62"]
            P5["templating<br/>render engine"]
        end
    end

    A --> P1 & P2 & P3 & P5
    AUTH --> P1 & P3
    D --> P1 & P2 & P5
    L --> P1 & P2 & P3 & P4
    LD --> P1 & P2
    N --> P1 & P2 & P3 & P5
    S --> P1 & P3
    A -. "no code import<br/>(Kafka + schema only)" .- D
    L -. "Kafka link.clicked" .- LD
    N -. "Kafka notification.*.requested" .- D
    A -. "HTTP introspect<br/>(API keys)" .-> AUTH
    L -. "HTTP introspect" .-> AUTH
    N -. "HTTP introspect" .-> AUTH
```

Services never import each other's `internal/` (Go enforces it). otp-api and otp-dispatcher meet at
the shared `pkg/contracts` event schema and the `otp` database; otp-api calls auth-svc over HTTP for
API-key introspection. Those boundaries keep each service independently deployable and, later,
independently extractable into its own repo. link-svc / link-dispatcher and notification-api /
otp-dispatcher follow the same sync-API + async-worker split (sections 7 and 8).

---

## 7. Link Service (sub-project C)

> **Status (2026-10-12):** code-complete against the
> [link service spec](superpowers/specs/2026-08-19-link-service-url-shortener-design.md) and covered
> by in-process tests (faked ports, sqlite, miniredis). The compose / Traefik / CI wiring and
> `test/e2e/link_test.go` have **not been run against real infrastructure yet**, and there are no
> k8s manifests for these two services.

`link-svc` owns the `link` database and the redirect hot path; `link-dispatcher` only appends
clicks. They share the schema and the `link.clicked` contract, never code.

```mermaid
sequenceDiagram
    autonumber
    participant T as Tenant (dashboard / backend)
    participant V as Visitor
    participant L as link-svc
    participant R as Redis
    participant DB as MySQL (link)
    participant K as Kafka
    participant LD as link-dispatcher

    T->>L: POST /v1/links {long_url} (Bearer)
    L->>R: INCR link:rl:tenant:<id> (fixed window)
    L->>DB: find by (tenant_id, long_url_hash)
    alt already shortened
        L-->>T: 200 {code, short_url} (same mapping)
    else new URL
        L->>L: Snowflake id -> Base62 code
        L->>DB: insert links
        L->>R: SET link:<code> (warm cache, TTL 1h)
        L-->>T: 201 {code, short_url}
    end

    V->>L: GET /<code> (public host)
    L->>R: GET link:<code>
    opt cache miss
        L->>DB: find by code
        L->>R: SET link:<code>
    end
    L-->>V: 302 Location: long_url
    L-)K: publish link.clicked (background, best-effort)
    K->>LD: consume
    LD->>DB: insert link_clicks (ip_hash, never the raw IP)
```

Why it is built this way:

- **Snowflake -> Base62, not an auto-increment.** A plain counter makes codes enumerable
  (`code+1`) and leaks volume. A Snowflake id packs `[41 bits ms | 10 bits node | 12 bits seq]`, so
  ids are unique without a DB round trip. Pitfall: every replica needs its own `LINK_NODE_ID`
  (0..1023) or two pods can mint the same code; the setting is parsed strictly for that reason.
- **302, not 301.** Browsers cache a 301 and stop calling the service, which silently ends click
  tracking for that visitor.
- **The redirect never waits on Kafka.** The click is published on a detached goroutine behind a
  bounded semaphore (`LINK_MAX_INFLIGHT_CLICKS`); when the broker is slow, clicks are dropped
  rather than piling up goroutines. Analytics is best-effort, the redirect is the product.
- **Dedup needs a hash column.** `long_url` is `TEXT` and cannot carry a unique index, so
  `(tenant_id, sha256(long_url))` is the dedup key. The check-then-insert race is settled by that
  unique index: the loser re-reads and returns the winner's code.
- **Clicks are at-least-once.** A redelivered `link.clicked` double-counts; that is accepted for
  analytics. A poison message is logged and dropped so it cannot stall its partition.

```mermaid
erDiagram
    links ||--o{ link_clicks : "code (denormalized, no FK)"

    links {
        bigint id PK "snowflake"
        varchar code UK "base62(id)"
        char tenant_id
        text long_url
        char long_url_hash "sha256, unique per tenant"
        datetime created_at
    }
    link_clicks {
        bigint id PK
        varchar code
        char tenant_id
        datetime ts "UTC"
        varchar referer
        varchar ua
        char ip_hash
    }
```

---

## 8. Notification Service (sub-project B)

> **Status (2026-10-12):** code-complete against the
> [notification system spec](superpowers/specs/2026-08-19-notification-system-design.md) and covered
> by in-process tests. **Not run against real infrastructure yet** (compose, Traefik, CI images,
> `test/e2e/notification_test.go`, SMTP), and there are no k8s manifests. Open items are listed in
> the [roadmap](roadmap/2026-08-16-notification-platform-and-link-service.md).

`notification-api` accepts and records a send; delivery happens in `otp-dispatcher`, which gains a
second set of handlers that are **opt-in**: they start only when `NOTIFICATION_MYSQL_DSN` is set, so
an OTP-only deployment is unchanged.

```mermaid
sequenceDiagram
    autonumber
    participant T as Tenant
    participant N as notification-api
    participant R as Redis
    participant DB as MySQL (notification)
    participant K as Kafka
    participant D as otp-dispatcher
    participant P as Provider (SMTP / Resend / Twilio)

    T->>N: POST /v1/notifications (Bearer, optional idempotency key)
    N->>DB: load template (tenant, active, channel matches)
    alt marketing and user opted out
        N->>DB: insert notification_log (suppressed)
        N-->>T: suppressed
    else accepted
        N->>R: per-tenant and per-user rate limits
        N->>DB: insert notification_log (queued)
        Note over N,DB: id derives from the idempotency key.<br/>A duplicate insert returns the first notification.
        N->>K: publish notification.<channel>.requested
        N-->>T: queued {notification_id}
    end

    K->>D: consume
    D->>DB: load template, render (pkg/templating)
    loop until sent, permanent error, or max attempts (backoff)
        D->>P: deliver
    end
    alt delivered
        D->>DB: queued -> sent (guarded)
        D->>K: notification.sent
    else failed
        D->>DB: queued -> failed (guarded)
        D->>K: notification.failed + notification.dlq
    end
```

Why it is built this way:

- **Idempotency is in the primary key.** `notification_log.id` is derived from
  `(tenant, idempotency key)`, so a client retry hits a duplicate-key insert and gets the first
  notification back instead of sending twice. Designing this in from the start is far cheaper than
  bolting a dedup table on later.
- **Guarded state transitions.** The dispatcher updates a row only from an allowed prior state
  (`failed` only from `queued`; `sent` from `queued` or `failed`), so a redelivered Kafka message
  cannot flip a `sent` row back to `failed`. `sent` may overwrite `failed` because notification-api
  marks a row failed when its publish reports an error, and the message may still have gone out.
- **Opt-out only applies to marketing.** Suppressing a transactional message (a receipt, a login
  code) is a bug, so only `kind = marketing` consults `notification_settings`.
- **Permanent vs transient errors.** A 5yz SMTP reply is a definitive rejection and fails
  immediately; 4yz replies and network errors are retried with backoff. Retrying an unknown
  mailbox `MaxAttempts` times only delays the DLQ entry.
- **Rendered subjects are always one line.** The renderer collapses CR/LF in the subject, because a
  tenant-supplied variable containing a line break would otherwise inject mail headers.
- **Preview uses the delivery renderer.** `POST /v1/templates/:id/preview` calls the same
  `pkg/templating` code path as the dispatcher, so a preview cannot diverge from what is sent.

Pitfall still open: `{{link "url"}}` needs a shortener. `notification-api` currently wires a
`noShortener`, so such a template fails with `templating.ErrNoShortener` until the link-svc adapter
exists (blocked on how a service authenticates to link-svc on a tenant's behalf).

```mermaid
erDiagram
    templates ||--o{ template_versions : "snapshots on update"
    templates ||--o{ notification_log : "rendered from"
    notification_log ||--o{ notification_events : "engagement"

    templates {
        char id PK
        char tenant_id
        varchar name
        varchar channel "email | sms"
        varchar locale
        varchar subject
        text body
        int version "optimistic lock"
        varchar status "active | archived"
    }
    template_versions {
        char id PK
        char template_id
        int version
        varchar subject
        text body
    }
    notification_log {
        char id PK "idempotency key"
        char tenant_id
        varchar channel
        varchar recipient_masked
        char template_id
        varchar kind "transactional | marketing"
        varchar state "queued | sent | failed | suppressed"
        varchar provider
        text error
    }
    notification_settings {
        char tenant_id PK
        varchar user_ref PK
        varchar channel PK
        boolean enabled
    }
    notification_events {
        bigint id PK
        char notification_id
        varchar type "delivered | opened | clicked"
        datetime ts
    }
```

> These `templates` live in the **notification** database and are tenant-scoped. They are separate
> from the OTP `templates` in section 5, which the dashboard's Templates screen still uses.
