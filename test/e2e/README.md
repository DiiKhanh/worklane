# End-to-end test

Exercises the full `send → real email → verify` flow against a running docker-compose stack,
through Traefik, reading the delivered code from MailHog.

## Run

From the repo root:

```bash
# 1. Bring up the stack (builds the service images on first run)
docker compose -f deploy/compose/docker-compose.yml up -d --build

# 2. Run the e2e test (it seeds its own API key via MySQL)
go test -tags=e2e ./test/e2e/ -v

# 3. Tear down when done
docker compose -f deploy/compose/docker-compose.yml down -v
```

What it asserts:
- `POST /v1/otp/send` with a seeded key → `202`, and an email is delivered (captured by MailHog).
- `POST /v1/otp/verify` with the wrong code → `401`, with the correct code → `200`.
- `POST /v1/otp/send` without a key → `401`.

## Overrides (env)

| Var | Default | Meaning |
|-----|---------|---------|
| `E2E_API_BASE` | `http://localhost` | API base (Traefik) |
| `E2E_MAILHOG` | `http://localhost:8025` | MailHog HTTP API |
| `MYSQL_DSN` | `root:secret@tcp(localhost:3306)/otp?...` | MySQL for seeding |

## Web UIs while the stack is up

| UI | URL |
|----|-----|
| Traefik dashboard | http://localhost:8090 |
| Adminer (MySQL) | http://localhost:8081 (server `mysql`, user `root`, pass `secret`) |
| RedisInsight | http://localhost:5540 (add db `redis:6379`) |
| Redpanda Console (Kafka) | http://localhost:8085 |
| MailHog (inbox) | http://localhost:8025 |
