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
    MySQL[("MySQL<br/>tenants, api_keys,<br/>otp_requests, delivery_logs")]
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
    M-->>D: ok / error
    D->>DB: insert delivery_logs; state = sent|failed
    D->>K: publish otp.sent | otp.failed (+ otp.dlq on failure)

    Note over C,M: --- later ---
    C->>GW: POST /v1/otp/verify (code)
    GW->>A: forward
    A->>R: get record; attempt-lock guard
    A->>A: VerifyHash (constant-time)
    A->>R: delete code (single use)
    A->>DB: state = verified
    A-->>C: 200 verified | 401 mismatch | 410 gone | 429 locked
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

```mermaid
erDiagram
    tenants ||--o{ api_keys : "has"
    tenants ||--o{ otp_requests : "owns"
    otp_requests ||--o{ delivery_logs : "produces"

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
        varchar channel
        varchar locale
        varchar subject
        text body
    }
```

> Note: the plaintext OTP code lives **only** in Redis (as a salted hash, TTL-bound) and is never in
> MySQL. `otp_requests.recipient_masked` stores `d***@gmail.com`, not the real address.

---

## 6. Monorepo, two deployables

```mermaid
flowchart LR
    subgraph repo["worklane (one Go module)"]
        subgraph services["services/"]
            A["otp-api<br/>main.go + internal/*"]
            D["otp-dispatcher<br/>main.go + internal/*"]
            S["seed (CLI)"]
        end
        subgraph pkgs["pkg/ (shared)"]
            P1["platform/*<br/>config, mysql, redis, kafka"]
            P2["contracts/otp<br/>event schema + State"]
            P3["security<br/>api-key hash/gen"]
        end
    end

    A --> P1 & P2 & P3
    D --> P1 & P2
    S --> P1 & P3
    A -. "no code import<br/>(Kafka + schema only)" .- D
```

Services never import each other's `internal/` (Go enforces it). They meet only at the shared
`pkg/contracts` event schema and the shared MySQL schema - which is exactly what makes each one
independently deployable and, later, independently extractable into its own repo.
