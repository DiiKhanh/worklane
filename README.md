# Worklane

Worklane is a tenant-scoped OTP messaging platform, with a Template Studio and a Link Service. The OTP API creates and verifies one-time passcodes; the dispatcher sends delivery messages and records outcomes. Template Studio manages versioned email and SMS templates used by OTP delivery. The Link Service provides tenant-scoped short links, public redirects, and click analytics.

The existing dispatcher sends email through SMTP (MailHog locally) or Resend. A Twilio SMS adapter is present, but real SMS and phone verification remain deferred. The broader general-purpose Notification Service described in the design documents is not implemented. The Link Service MVP includes a live Links dashboard and an opt-in local Compose stack. Its scope is tracked in [issue #11](https://github.com/DiiKhanh/worklane/issues/11).

## Repository layout

| Path | Contents |
| --- | --- |
| `services/otp-api` | Synchronous OTP HTTP API, verification, template management, and stats |
| `services/otp-dispatcher` | Kafka consumer that renders templates, delivers OTP messages, and records delivery outcomes |
| `services/auth-svc` | Login, API-key management, and identity introspection |
| `services/link-svc` | Link management and public redirect HTTP service |
| `services/link-dispatcher` | Asynchronous click-event persistence and analytics worker |
| `services/seed` | CLI for provisioning a tenant, user, and API key |
| `pkg/` | Shared platform packages, event contracts, ID generation, security, and template rendering |
| `dashboard/` | Next.js dashboard for OTP operations, Template Studio, and the Link Service |
| `db/` | Identity, OTP, and Link Service SQL migrations |
| `deploy/compose` | Local OTP stack and opt-in Link Service profile with MySQL, Redis, Redpanda, and Traefik |
| `deploy/k8s` | Kubernetes base manifests and production overlay |
| `docs/` | Architecture, feature behavior, operations, learning notes, roadmap, and decisions |

The backend is a single Go module. Each service has its own `main.go` and private `internal/` packages; shared code lives in `pkg/`.

## Prerequisites

- Go 1.25 or newer
- Docker with the Docker Compose plugin
- OpenSSL, to generate the local Ed25519 JWT key pair required by the Compose auth services
- Node.js and pnpm 10.26.2 for the dashboard (`dashboard/package.json` pins pnpm)

## Local development

### Start the local backend stack

From the repository root, generate the local auth keys and start the Compose stack:

```sh
mkdir -p deploy/compose/secrets
openssl genpkey -algorithm Ed25519 -out deploy/compose/secrets/auth_priv.pem
openssl pkey -in deploy/compose/secrets/auth_priv.pem -pubout -out deploy/compose/secrets/auth_pub.pem
docker compose -f deploy/compose/docker-compose.yml up -d --build
```

The stack exposes the gateway at `http://localhost`, Traefik's dashboard at `http://localhost:8090`, Adminer at `http://localhost:8081`, RedisInsight at `http://localhost:5540`, Redpanda Console at `http://localhost:8085`, and MailHog at `http://localhost:8025`. The [isolated Link Service stack](docs/verification/link-service-mvp.md) documents the `links` profile, local credentials, and end-to-end checks.

Stop the stack while keeping its data volumes:

```sh
docker compose -f deploy/compose/docker-compose.yml down
```

### Run the dashboard

From `dashboard/`:

```sh
pnpm install
pnpm dev
```

The dashboard uses mock data by default. To connect it to the local backend, set `NEXT_PUBLIC_DATA_SOURCE=live` and `NEXT_PUBLIC_API_BASE=http://localhost` in `dashboard/.env.local`, then restart the dev server. The base URL must reach the shared gateway for both `/auth/*` and `/v1/*`; the login page always uses the real auth service, even in mock mode.

## Configuration

`.env.example` lists names for running Go binaries directly: `MYSQL_DSN`, `REDIS_URL`, `KAFKA_BROKERS`, `HTTP_ADDR`, `OTP_CODE_LENGTH`, `OTP_TTL`, `OTP_MAX_ATTEMPTS`, `OTP_LIMIT_RECIPIENT`, `OTP_LIMIT_TENANT`, `OTP_RESEND_COOLDOWN`, `EMAIL_PROVIDER`, `SMTP_HOST`, `SMTP_PORT`, `RESEND_API_KEY`, `RESEND_FROM`, `RESEND_BASE_URL`, `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM`, `TWILIO_BASE_URL`, and `OTP_SMS_BODY_FMT`. Compose has local defaults in `deploy/compose/docker-compose.yml`; it reads selected overrides from a root `.env` file. Dashboard configuration uses `NEXT_PUBLIC_DATA_SOURCE` and `NEXT_PUBLIC_API_BASE` in `dashboard/.env.local`.

Keep credentials and private keys out of source control. The local Compose key files belong under the ignored `deploy/compose/secrets/` directory.

## Tests, lint, and typecheck

Run backend tests from the repository root using the Makefile targets:

```sh
make test
make test-link
make test-link-integration
make test-link-schema
```

The Link Service integration and schema tests use isolated dependencies through Testcontainers and require Docker. The dashboard commands run from `dashboard/`:

```sh
pnpm test
pnpm lint
pnpm typecheck
```

The root Makefile does not define Go lint or typecheck targets.

## Documentation

- [Glossary](GLOSSARY.md) - domain terms, including Link Service language
- [Architecture](docs/architecture.md) and [repository layout reference](docs/reference/architecture-and-layout.md)
- [Template Studio design](docs/superpowers/specs/2026-08-29-template-studio-design.md)
- [Link Service feature docs](docs/features/link-foundations.md), [management](docs/features/link-management.md), [redirects](docs/features/link-redirects.md), and [analytics](docs/features/link-analytics.md)
- [Link Service isolated stack and MVP verification](docs/verification/link-service-mvp.md)
- [Link Service MVP scope](docs/roadmap/2026-11-15-link-service-mvp-scope.md) and [tracker issue #11](https://github.com/DiiKhanh/worklane/issues/11)
- [Notification platform roadmap](docs/roadmap/2026-08-16-notification-platform-and-link-service.md) and [notification system design](docs/superpowers/specs/2026-08-19-notification-system-design.md)
- [Operations runbooks](docs/runbooks/)
- [Architecture decision records](docs/adr/), including [ADR 0001](docs/adr/0001-link-redirects-and-best-effort-analytics.md) on redirect availability and best-effort click analytics
