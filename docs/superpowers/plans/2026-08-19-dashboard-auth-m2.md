# Dashboard Auth (M2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the identity/OTP split started in M1: give `auth-svc` its own `identity` database that owns `tenants`, `users`, and `api_keys`; move all API-key ownership into `auth-svc` (list/create/revoke + `/internal/introspect`); switch `otp-api` from a local `api_keys` lookup to introspection; and drop the identity tables from the `otp` database.

**Architecture:** `auth-svc` becomes the single owner of the identity bounded context. It runs its own migrations against a separate logical database (`identity`), exposes JWT-guarded `/auth/api-keys` management endpoints, and a network-internal `POST /internal/introspect` (shared-secret guarded) that resolves an API key to a tenant. `otp-api` keeps its local JWT verification unchanged but replaces its `api_keys` table read with an HTTP call to introspect, wrapped by a short-TTL Redis cache so the machine hot path stays effectively O(1). `otp-api` sheds all identity code and tables. The dashboard's key list repoints from `/v1/api-keys` to `/auth/api-keys`.

**Tech Stack:** Go 1.25, gin, GORM, golang-migrate, Redis (`github.com/redis/go-redis/v9`), miniredis for tests, `gorm.io/driver/sqlite` for repo tests; existing `pkg/security` (argon2id, EdDSA JWT, `GenerateAPIKey`/`HashKey`); Next.js dashboard (TanStack Query, zustand).

**Design doc:** `docs/superpowers/specs/2026-08-19-dashboard-auth-identity-service-design.md` (sections 3, 4, 5, 6, 7, 8, 11 describe the M2 end state).

## Global Constraints

- Module path: `github.com/duykhanh/worklane`. Go `1.25.0`.
- **M2 is a clean cutover, not a data migration.** The stack is pre-production (no live tenant data), so the identity tables are recreated fresh in the `identity` DB and dropped from `otp`; re-seed after the split. Any bring-up in this plan assumes `docker compose ... down -v` first.
- **Database-per-service:** `auth-svc` connects ONLY to `identity`; `otp-api`/`otp-dispatcher` connect ONLY to `otp`. No cross-database FK or JOIN. `otp_requests.tenant_id` / `delivery_logs.tenant_id` are soft references (id only).
- **`auth-svc` now owns identity migrations** (`db/identity/migrations`) and runs them at startup; `otp-api` keeps owning `db/otp/migrations`. Migrations are forward-only and shipped into the image (golang-migrate reads the `.sql` files at runtime).
- Hexagon rule (enforced by each service's `arch_test`): `internal/domain` and `internal/app` must not import `redis`, `gorm`, `sarama`, `gin`, `resend`, `/adapters/`, or `/pkg/platform/`. Importing `pkg/security` from `app` IS allowed.
- `/internal/*` is never routed through Traefik (Traefik routes only `/v1` and `/auth`); it is reachable only over the Docker network and is additionally guarded by `X-Internal-Token: <INTERNAL_API_TOKEN>`.
- Never log or return a password, an OTP code, a raw JWT, or a raw/plaintext API key in error messages. Plaintext API keys are shown exactly once (on create) and never re-read.
- Immutability; small focused files; wrap errors with `fmt.Errorf("...: %w", err)`; table-driven tests; `go test -race ./...` green; `gofmt`/`goimports` clean.
- No em dash. Commit messages: `<type>: <description>`, no co-author line.

---

### Task 1: `identity` database + auth-svc-owned migrations

Stand up the separate `identity` database and make `auth-svc` create its schema. This is the foundation every later auth-svc task builds on.

**Files:**
- Create: `db/identity/migrations/0001_init.up.sql`
- Create: `db/identity/migrations/0001_init.down.sql`
- Create: `deploy/compose/initdb/01-create-identity-db.sql`
- Modify: `services/auth-svc/main.go` (run migrations at startup)
- Modify: `services/auth-svc/Dockerfile` (ship migrations + set `MIGRATIONS_DIR`)
- Modify: `deploy/compose/docker-compose.yml` (auth-svc DSN -> `identity`; mount initdb; fix `depends_on`)

**Interfaces:**
- Produces: an `identity` database containing `tenants`, `users`, `api_keys`; `auth-svc` applies `db/identity/migrations` on boot via `mysql.Migrate` (existing `pkg/platform/mysql.Migrate(dsn, dir string) error`).

- [ ] **Step 1: Write the identity up-migration**

The three identity tables, copied verbatim from otp-api's `0001_init` (tenants, api_keys) and `0002_users` (users) so shapes are unchanged:

```sql
-- db/identity/migrations/0001_init.up.sql
CREATE TABLE tenants (
  id CHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
  id            CHAR(36) PRIMARY KEY,
  tenant_id     CHAR(36) NOT NULL,
  email         VARCHAR(255) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  status        VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_users_tenant (tenant_id)
);

CREATE TABLE api_keys (
  id CHAR(36) PRIMARY KEY,
  tenant_id CHAR(36) NOT NULL,
  hashed_key VARCHAR(128) NOT NULL UNIQUE,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_api_keys_tenant (tenant_id)
);
```

- [ ] **Step 2: Write the identity down-migration**

```sql
-- db/identity/migrations/0001_init.down.sql
DROP TABLE api_keys;
DROP TABLE users;
DROP TABLE tenants;
```

- [ ] **Step 3: Write the MySQL init script that creates the database**

`MYSQL_DATABASE: otp` creates `otp` on first init; this adds `identity`. Init scripts run only when the data directory is empty (fresh volume), which is why this plan does a `down -v` cutover.

```sql
-- deploy/compose/initdb/01-create-identity-db.sql
CREATE DATABASE IF NOT EXISTS identity;
```

- [ ] **Step 4: Make auth-svc run its migrations at startup**

In `services/auth-svc/main.go`, add a `migrationsDir` config read and a `mysql.Migrate` call BEFORE `mysql.Open`. Insert the config line next to the other `config.Env` reads:

```go
	migrationsDir := config.Env("MIGRATIONS_DIR", "db/identity/migrations")
```

Then, immediately before `db, err := mysql.Open(dsn)`:

```go
	// auth-svc owns the identity schema; apply it before opening the pool so the
	// service always comes up against an up-to-date database.
	if err := mysql.Migrate(dsn, migrationsDir); err != nil {
		log.Fatalf("auth-svc: migrate: %v", err)
	}
```

Also change the default DSN in `main.go` from `.../otp` to `.../identity`:

```go
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/identity?parseTime=true&multiStatements=true")
```

- [ ] **Step 5: Ship migrations into the auth-svc image**

Replace `services/auth-svc/Dockerfile` with (mirrors otp-api's Dockerfile, identity migrations path):

```dockerfile
# Multi-stage build. Context must be the repo root (single Go module with shared pkg).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/auth-svc ./services/auth-svc

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/auth-svc /auth-svc
# Migrations are read at startup (golang-migrate needs the .sql files at runtime).
COPY --from=build /src/db/identity/migrations /db/identity/migrations
ENV MIGRATIONS_DIR=/db/identity/migrations
EXPOSE 8889
ENTRYPOINT ["/auth-svc"]
```

- [ ] **Step 6: Update compose (DSN, initdb mount, depends_on)**

In `deploy/compose/docker-compose.yml`:

1. Under `mysql:`, add the initdb volume mount:

```yaml
    volumes:
      - "./initdb:/docker-entrypoint-initdb.d:ro"
```

2. Under `auth-svc:`, change `MYSQL_DSN` to the identity database:

```yaml
      MYSQL_DSN: "root:secret@tcp(mysql:3306)/identity?parseTime=true&multiStatements=true"
```

3. Under `auth-svc:`, replace the `depends_on` block (it no longer needs otp-api, since auth-svc now owns its own schema):

```yaml
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
```

- [ ] **Step 7: Verify the split boots and creates the schema**

Run:
```bash
docker compose -f deploy/compose/docker-compose.yml down -v
docker compose -f deploy/compose/docker-compose.yml up -d --build mysql redis auth-svc
sleep 15
docker compose -f deploy/compose/docker-compose.yml exec -T mysql \
  mysql -uroot -psecret identity -e "SHOW TABLES;"
```
Expected: `api_keys`, `tenants`, `users` listed in the `identity` database; `auth-svc` logs `listening on :8889` with no migrate error.

- [ ] **Step 8: Commit**

```bash
git add db/identity/migrations deploy/compose/initdb services/auth-svc/main.go services/auth-svc/Dockerfile deploy/compose/docker-compose.yml
git commit -m "feat(identity): dedicated identity database owned by auth-svc"
```

---

### Task 2: Drop identity tables from the `otp` database

`otp-api` no longer owns identity. Add a forward-only migration that removes `tenants`, `api_keys`, and `users` from the `otp` database. Migration history (`0001`, `0002`) stays intact; `0003` is the drop.

**Files:**
- Create: `db/otp/migrations/0003_drop_identity.up.sql`
- Create: `db/otp/migrations/0003_drop_identity.down.sql`

**Interfaces:**
- Produces: the `otp` database contains only `otp_requests`, `delivery_logs`, `templates` after migration.

- [ ] **Step 1: Write the up-migration**

```sql
-- db/otp/migrations/0003_drop_identity.up.sql
-- Identity moves to auth-svc's `identity` database (M2). otp-api keeps only OTP tables.
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS tenants;
```

- [ ] **Step 2: Write the down-migration**

Recreate the three tables so the migration is reversible (shapes identical to `0001`/`0002`):

```sql
-- db/otp/migrations/0003_drop_identity.down.sql
CREATE TABLE tenants (
  id CHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE api_keys (
  id CHAR(36) PRIMARY KEY,
  tenant_id CHAR(36) NOT NULL,
  hashed_key VARCHAR(128) NOT NULL UNIQUE,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_api_keys_tenant (tenant_id)
);

CREATE TABLE users (
  id            CHAR(36) PRIMARY KEY,
  tenant_id     CHAR(36) NOT NULL,
  email         VARCHAR(255) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  status        VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_users_tenant (tenant_id)
);
```

- [ ] **Step 3: Verify the migration applies**

Run (otp-api applies migrations on boot):
```bash
docker compose -f deploy/compose/docker-compose.yml up -d --build otp-api
sleep 12
docker compose -f deploy/compose/docker-compose.yml exec -T mysql \
  mysql -uroot -psecret otp -e "SHOW TABLES;"
```
Expected: only `delivery_logs`, `otp_requests`, `templates` in the `otp` database (no `tenants`/`api_keys`/`users`).

- [ ] **Step 4: Commit**

```bash
git add db/otp/migrations/0003_drop_identity.up.sql db/otp/migrations/0003_drop_identity.down.sql
git commit -m "feat(otp-api): drop identity tables from otp database"
```

---

### Task 3: auth-svc api_keys domain + repo

Give `auth-svc` a domain `APIKey`, the repo methods to look one up by hash and to manage the collection, all tenant-scoped. Repo tests use in-memory SQLite (the pattern already used by `mysqlrepo/repo_test.go`).

**Files:**
- Create: `services/auth-svc/internal/domain/apikey.go`
- Modify: `services/auth-svc/internal/domain/errors.go`
- Modify: `services/auth-svc/internal/app/ports.go`
- Modify: `services/auth-svc/internal/adapters/outbound/mysqlrepo/repo.go`
- Test: `services/auth-svc/internal/adapters/outbound/mysqlrepo/apikey_repo_test.go`

**Interfaces:**
- Consumes: `security.HashKey` (existing).
- Produces:
  - `domain.APIKey{ ID, TenantID, Status string; CreatedAt time.Time }`
  - `domain.ErrAPIKeyNotFound`
  - `app.Repo` gains: `FindAPIKeyByHash(ctx, hashedKey string) (domain.APIKey, error)`, `InsertAPIKey(ctx, tenantID, hashedKey string) (id string, err error)`, `ListAPIKeys(ctx, tenantID string) ([]domain.APIKey, error)`, `RevokeAPIKey(ctx, tenantID, id string) error`
  - `*mysqlrepo.Repo` implements all four, tenant-scoped, returning `domain.ErrAPIKeyNotFound` when a lookup/revoke matches no row.

- [ ] **Step 1: Write the domain type**

```go
// services/auth-svc/internal/domain/apikey.go
package domain

import "time"

// APIKey is a machine credential belonging to a tenant. The plaintext is never stored;
// only its hash (see pkg/security.HashKey) lives in the database.
type APIKey struct {
	ID        string
	TenantID  string
	Status    string
	CreatedAt time.Time
}
```

- [ ] **Step 2: Add the not-found error**

In `services/auth-svc/internal/domain/errors.go`, add `ErrAPIKeyNotFound` to the existing `var (...)` block:

```go
	ErrAPIKeyNotFound = errors.New("api key not found")
```

- [ ] **Step 3: Extend the Repo port**

In `services/auth-svc/internal/app/ports.go`, replace the `Repo` interface with:

```go
// Repo reads and writes identity rows (users and api keys).
type Repo interface {
	FindUserByEmail(ctx context.Context, email string) (domain.User, error)
	FindAPIKeyByHash(ctx context.Context, hashedKey string) (domain.APIKey, error)
	InsertAPIKey(ctx context.Context, tenantID, hashedKey string) (id string, err error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, tenantID, id string) error
}
```

- [ ] **Step 4: Write the failing repo test**

```go
// services/auth-svc/internal/adapters/outbound/mysqlrepo/apikey_repo_test.go
package mysqlrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// newTestRepo/seedUser live in repo_test.go; here we add an api_keys table to that schema.
func withAPIKeys(t *testing.T) *Repo {
	t.Helper()
	r := newTestRepo(t)
	if err := r.db.Exec(`CREATE TABLE api_keys (id TEXT, tenant_id TEXT, hashed_key TEXT, status TEXT, created_at DATETIME)`).Error; err != nil {
		t.Fatalf("schema: %v", err)
	}
	return r
}

func TestInsertAndFindAPIKeyByHash(t *testing.T) {
	r := withAPIKeys(t)
	ctx := context.Background()

	id, err := r.InsertAPIKey(ctx, "t1", "hash-abc")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == "" {
		t.Fatal("insert must return a non-empty id")
	}

	ak, err := r.FindAPIKeyByHash(ctx, "hash-abc")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if ak.ID != id || ak.TenantID != "t1" || ak.Status != "active" {
		t.Fatalf("row wrong: %+v", ak)
	}
}

func TestFindAPIKeyByHash_NotFound(t *testing.T) {
	r := withAPIKeys(t)
	_, err := r.FindAPIKeyByHash(context.Background(), "missing")
	if !errors.Is(err, domain.ErrAPIKeyNotFound) {
		t.Fatalf("want ErrAPIKeyNotFound, got %v", err)
	}
}

func TestListAPIKeys_TenantScoped(t *testing.T) {
	r := withAPIKeys(t)
	ctx := context.Background()
	_, _ = r.InsertAPIKey(ctx, "t1", "h1")
	_, _ = r.InsertAPIKey(ctx, "t1", "h2")
	_, _ = r.InsertAPIKey(ctx, "t2", "h3")

	keys, err := r.ListAPIKeys(ctx, "t1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("want 2 keys for t1, got %d", len(keys))
	}
}

func TestRevokeAPIKey_TenantScoped(t *testing.T) {
	r := withAPIKeys(t)
	ctx := context.Background()
	id, _ := r.InsertAPIKey(ctx, "t1", "h1")

	// A different tenant cannot revoke it.
	if err := r.RevokeAPIKey(ctx, "t2", id); !errors.Is(err, domain.ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant revoke must be not-found, got %v", err)
	}
	// The owner can.
	if err := r.RevokeAPIKey(ctx, "t1", id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ak, _ := r.FindAPIKeyByHash(ctx, "h1")
	if ak.Status != "revoked" {
		t.Fatalf("status after revoke = %q, want revoked", ak.Status)
	}
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./services/auth-svc/internal/adapters/outbound/mysqlrepo/ -run APIKey -v`
Expected: FAIL - `r.InsertAPIKey undefined`.

- [ ] **Step 6: Implement the repo methods**

Append to `services/auth-svc/internal/adapters/outbound/mysqlrepo/repo.go`. Add imports `crypto/rand` and `encoding/hex` to the existing import block, then:

```go
type apiKeyRow struct {
	ID        string
	TenantID  string
	HashedKey string
	Status    string
	CreatedAt time.Time
}

func (apiKeyRow) TableName() string { return "api_keys" }

// newID returns a 128-bit random hex id (same shape the seed CLI uses).
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// FindAPIKeyByHash looks a key up by its stored hash. Returns domain.ErrAPIKeyNotFound
// when no row matches.
func (r *Repo) FindAPIKeyByHash(ctx context.Context, hashedKey string) (domain.APIKey, error) {
	var row apiKeyRow
	err := r.db.WithContext(ctx).Where("hashed_key = ?", hashedKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("mysql: find api key: %w", err)
	}
	return domain.APIKey{ID: row.ID, TenantID: row.TenantID, Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

// InsertAPIKey stores a new active key hash for a tenant and returns its generated id.
func (r *Repo) InsertAPIKey(ctx context.Context, tenantID, hashedKey string) (string, error) {
	id := newID()
	err := r.db.WithContext(ctx).Exec(
		"INSERT INTO api_keys (id, tenant_id, hashed_key, status) VALUES (?, ?, ?, 'active')",
		id, tenantID, hashedKey,
	).Error
	if err != nil {
		return "", fmt.Errorf("mysql: insert api key: %w", err)
	}
	return id, nil
}

// ListAPIKeys returns a tenant's keys, newest first.
func (r *Repo) ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error) {
	var rows []apiKeyRow
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mysql: list api keys: %w", err)
	}
	out := make([]domain.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.APIKey{ID: row.ID, TenantID: row.TenantID, Status: row.Status, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

// RevokeAPIKey sets status='revoked' for a key the tenant owns. Scoping the WHERE by
// tenant_id makes cross-tenant revocation impossible. Returns domain.ErrAPIKeyNotFound
// when nothing matched.
func (r *Repo) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	res := r.db.WithContext(ctx).Model(&apiKeyRow{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("status", "revoked")
	if res.Error != nil {
		return fmt.Errorf("mysql: revoke api key: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrAPIKeyNotFound
	}
	return nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./services/auth-svc/internal/adapters/outbound/mysqlrepo/ -v`
Expected: PASS (user tests + all four api-key tests).

- [ ] **Step 8: Commit**

```bash
git add services/auth-svc/internal/domain/apikey.go services/auth-svc/internal/domain/errors.go services/auth-svc/internal/app/ports.go services/auth-svc/internal/adapters/outbound/mysqlrepo/repo.go services/auth-svc/internal/adapters/outbound/mysqlrepo/apikey_repo_test.go
git commit -m "feat(auth-svc): api_keys domain and tenant-scoped repo"
```

---

### Task 4: auth-svc use cases (Introspect, CreateAPIKey, ListAPIKeys, RevokeAPIKey)

Add the application-layer operations. `Introspect` is the read the machine path depends on; the others back the dashboard management endpoints.

**Files:**
- Modify: `services/auth-svc/internal/app/usecase.go`
- Test: `services/auth-svc/internal/app/apikey_usecase_test.go`

**Interfaces:**
- Consumes: `app.Repo` (Task 3), `security.GenerateAPIKey`/`security.HashKey`.
- Produces on `*app.Service`:
  - `Introspect(ctx, plaintextKey string) (tenantID string, active bool, err error)`
  - `CreateAPIKey(ctx, tenantID string) (plaintext, id string, err error)`
  - `ListAPIKeys(ctx, tenantID string) ([]domain.APIKey, error)`
  - `RevokeAPIKey(ctx, tenantID, id string) error`

- [ ] **Step 1: Write the failing test**

```go
// services/auth-svc/internal/app/apikey_usecase_test.go
package app

import (
	"context"
	"errors"
	"testing"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// keyRepo is a fake app.Repo focused on api-key behavior. FindUserByEmail is unused here.
type keyRepo struct {
	byHash   map[string]domain.APIKey
	inserted []string // hashes inserted
	revoked  map[string]bool
}

func newKeyRepo() *keyRepo {
	return &keyRepo{byHash: map[string]domain.APIKey{}, revoked: map[string]bool{}}
}

func (k *keyRepo) FindUserByEmail(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}
func (k *keyRepo) FindAPIKeyByHash(_ context.Context, h string) (domain.APIKey, error) {
	ak, ok := k.byHash[h]
	if !ok {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	return ak, nil
}
func (k *keyRepo) InsertAPIKey(_ context.Context, tenantID, h string) (string, error) {
	k.inserted = append(k.inserted, h)
	k.byHash[h] = domain.APIKey{ID: "k1", TenantID: tenantID, Status: "active"}
	return "k1", nil
}
func (k *keyRepo) ListAPIKeys(_ context.Context, tenantID string) ([]domain.APIKey, error) {
	out := []domain.APIKey{}
	for _, ak := range k.byHash {
		if ak.TenantID == tenantID {
			out = append(out, ak)
		}
	}
	return out, nil
}
func (k *keyRepo) RevokeAPIKey(_ context.Context, _, id string) error {
	k.revoked[id] = true
	return nil
}

func keySvc(repo Repo) *Service {
	return New(Deps{Repo: repo, Issuer: fakeIssuer{}, Limiter: allowLimiter{ok: true}, Clock: fixedClock{}})
}

func TestIntrospect_ActiveKey(t *testing.T) {
	repo := newKeyRepo()
	repo.byHash[security.HashKey("plain-key")] = domain.APIKey{ID: "k1", TenantID: "t1", Status: "active"}
	svc := keySvc(repo)

	tenant, active, err := svc.Introspect(context.Background(), "plain-key")
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if !active || tenant != "t1" {
		t.Fatalf("want active t1, got active=%v tenant=%q", active, tenant)
	}
}

func TestIntrospect_UnknownKey(t *testing.T) {
	svc := keySvc(newKeyRepo())
	_, active, err := svc.Introspect(context.Background(), "nope")
	if err != nil {
		t.Fatalf("unknown key must not error: %v", err)
	}
	if active {
		t.Fatal("unknown key must be inactive")
	}
}

func TestIntrospect_RevokedKey(t *testing.T) {
	repo := newKeyRepo()
	repo.byHash[security.HashKey("plain-key")] = domain.APIKey{ID: "k1", TenantID: "t1", Status: "revoked"}
	svc := keySvc(repo)

	_, active, err := svc.Introspect(context.Background(), "plain-key")
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if active {
		t.Fatal("revoked key must be inactive")
	}
}

func TestCreateAPIKey_ReturnsPlaintextAndStoresHash(t *testing.T) {
	repo := newKeyRepo()
	svc := keySvc(repo)

	plain, id, err := svc.CreateAPIKey(context.Background(), "t1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if plain == "" || id == "" {
		t.Fatal("create must return plaintext and id")
	}
	// The stored value must be the HASH of the plaintext, never the plaintext itself.
	if len(repo.inserted) != 1 || repo.inserted[0] != security.HashKey(plain) {
		t.Fatalf("stored hash mismatch: %+v", repo.inserted)
	}
	if repo.inserted[0] == plain {
		t.Fatal("must store the hash, not the plaintext key")
	}
}

func TestRevokeAPIKey_Delegates(t *testing.T) {
	repo := newKeyRepo()
	svc := keySvc(repo)
	if err := svc.RevokeAPIKey(context.Background(), "t1", "k1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if !repo.revoked["k1"] {
		t.Fatal("revoke did not reach the repo")
	}
}

var _ = errors.Is // keep errors import if unused after edits
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/auth-svc/internal/app/ -run 'Introspect|CreateAPIKey|RevokeAPIKey' -v`
Expected: FAIL - `svc.Introspect undefined`.

- [ ] **Step 3: Implement the use cases**

Append to `services/auth-svc/internal/app/usecase.go` (add `"errors"` and the `pkg/security` import if not already present - `security` is already imported for `VerifyPassword`):

```go
// Introspect resolves a plaintext API key to its tenant. It returns active=false (with a
// nil error) for an unknown or revoked key so the caller can answer a clean {active:false}
// - a missing key is not an operational failure. A non-nil error means a real lookup fault.
func (s *Service) Introspect(ctx context.Context, plaintextKey string) (string, bool, error) {
	ak, err := s.d.Repo.FindAPIKeyByHash(ctx, security.HashKey(plaintextKey))
	if errors.Is(err, domain.ErrAPIKeyNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if ak.Status != "active" {
		return "", false, nil
	}
	return ak.TenantID, true, nil
}

// CreateAPIKey mints a new key for a tenant. The plaintext is returned to the caller ONCE
// and never persisted; only its hash is stored.
func (s *Service) CreateAPIKey(ctx context.Context, tenantID string) (string, string, error) {
	plain, err := security.GenerateAPIKey()
	if err != nil {
		return "", "", err
	}
	id, err := s.d.Repo.InsertAPIKey(ctx, tenantID, security.HashKey(plain))
	if err != nil {
		return "", "", err
	}
	return plain, id, nil
}

// ListAPIKeys returns a tenant's keys (no secrets, just metadata).
func (s *Service) ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error) {
	return s.d.Repo.ListAPIKeys(ctx, tenantID)
}

// RevokeAPIKey revokes a key the tenant owns.
func (s *Service) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	return s.d.Repo.RevokeAPIKey(ctx, tenantID, id)
}
```

Remove the trailing `var _ = errors.Is` line from the test if `errors` is already used there (it is, via `errors.Is` in other files but not this one - keep the guard only if `go vet` complains about an unused import; otherwise delete it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/auth-svc/internal/app/ -v`
Expected: PASS (login tests + all api-key use-case tests).

- [ ] **Step 5: Commit**

```bash
git add services/auth-svc/internal/app/usecase.go services/auth-svc/internal/app/apikey_usecase_test.go
git commit -m "feat(auth-svc): introspect and api-key management use cases"
```

---

### Task 5: auth-svc `/internal/introspect` endpoint

Expose introspection over HTTP for otp-api, guarded by a shared internal token. This route lives OUTSIDE `/auth` so Traefik (which routes only `/auth` and `/v1`) never exposes it.

**Files:**
- Modify: `services/auth-svc/internal/adapters/inbound/http/middleware.go` (add `internalAuth`)
- Modify: `services/auth-svc/internal/adapters/inbound/http/dto.go` (introspect DTOs)
- Create: `services/auth-svc/internal/adapters/inbound/http/introspect_handler.go`
- Modify: `services/auth-svc/internal/adapters/inbound/http/handlers.go` (extend `AuthService` port)
- Modify: `services/auth-svc/internal/adapters/inbound/http/router.go` (wire route + pass token)
- Test: `services/auth-svc/internal/adapters/inbound/http/introspect_handler_test.go`

**Interfaces:**
- Consumes: `AuthService` (extended with `Introspect`).
- Produces: `POST /internal/introspect` `{ "token": "<api key>" }` with header `X-Internal-Token: <secret>` -> `200 {"active":bool,"tenant_id":string}`; `401` on a bad/missing internal token. `NewRouter(svc AuthService, verifier *security.Verifier, internalToken string) *gin.Engine`.

- [ ] **Step 1: Extend the AuthService port**

In `services/auth-svc/internal/adapters/inbound/http/handlers.go`, replace the `AuthService` interface with the full M2 surface (login + introspect + key management, used across this and the next task):

```go
// AuthService is the inbound port this adapter needs.
type AuthService interface {
	Login(ctx context.Context, email, password string) (app.LoginResult, error)
	Introspect(ctx context.Context, plaintextKey string) (tenantID string, active bool, err error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error)
	CreateAPIKey(ctx context.Context, tenantID string) (plaintext, id string, err error)
	RevokeAPIKey(ctx context.Context, tenantID, id string) error
}
```

Add the domain import to `handlers.go`:

```go
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
```

- [ ] **Step 2: Add the internal-token middleware**

Append to `services/auth-svc/internal/adapters/inbound/http/middleware.go`:

```go
// internalAuth guards service-to-service routes with a shared secret. Constant-time compare
// avoids leaking the token via timing. The token is required (empty config is rejected at
// startup, so an empty header can never match).
func internalAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Internal-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}
```

Add `"crypto/subtle"` to the `middleware.go` import block.

- [ ] **Step 3: Add introspect DTOs**

Append to `services/auth-svc/internal/adapters/inbound/http/dto.go`:

```go
type introspectRequest struct {
	Token string `json:"token" binding:"required"`
}

type introspectResponse struct {
	Active   bool   `json:"active"`
	TenantID string `json:"tenant_id,omitempty"`
}
```

- [ ] **Step 4: Write the introspect handler**

```go
// services/auth-svc/internal/adapters/inbound/http/introspect_handler.go
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Introspect validates an API key and reports {active, tenant_id}. An unknown or revoked
// key is a normal 200 with active:false, not an error - the caller decides the HTTP status
// for its own client.
func (h *Handlers) Introspect(c *gin.Context) {
	var body introspectRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, active, err := h.svc.Introspect(c.Request.Context(), body.Token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, introspectResponse{Active: active, TenantID: tenantID})
}
```

- [ ] **Step 5: Wire the route and thread the token**

Replace `services/auth-svc/internal/adapters/inbound/http/router.go` with:

```go
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
)

// NewRouter builds the auth-svc HTTP handler.
//   - /auth/login is public.
//   - /auth/me and /auth/api-keys sit behind JWT verification (public key).
//   - /internal/introspect is NOT routed by Traefik and is guarded by a shared secret;
//     otp-api calls it over the Docker network to resolve API keys.
func NewRouter(svc AuthService, verifier *security.Verifier, internalToken string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := &Handlers{svc: svc}

	auth := r.Group("/auth")
	auth.POST("/login", h.Login)
	auth.GET("/me", jwtAuth(verifier), h.Me)

	internal := r.Group("/internal", internalAuth(internalToken))
	internal.POST("/introspect", h.Introspect)

	return r
}
```

- [ ] **Step 6: Write the failing handler test**

```go
// services/auth-svc/internal/adapters/inbound/http/introspect_handler_test.go
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// introspectSvc extends the fake with an Introspect result. It embeds the login fake shape
// so it satisfies the full AuthService interface; unused methods return zero values.
type introspectSvc struct {
	tenant string
	active bool
	err    error
}

func (s introspectSvc) Login(context.Context, string, string) (app.LoginResult, error) {
	return app.LoginResult{}, nil
}
func (s introspectSvc) Introspect(context.Context, string) (string, bool, error) {
	return s.tenant, s.active, s.err
}
func (s introspectSvc) ListAPIKeys(context.Context, string) ([]domain.APIKey, error) {
	return nil, nil
}
func (s introspectSvc) CreateAPIKey(context.Context, string) (string, string, error) {
	return "", "", nil
}
func (s introspectSvc) RevokeAPIKey(context.Context, string, string) error { return nil }

const internalTok = "secret-internal"

func TestIntrospect_ActiveWithToken(t *testing.T) {
	r := NewRouter(introspectSvc{tenant: "t1", active: true}, nil, internalTok)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/internal/introspect", strings.NewReader(`{"token":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", internalTok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	var got introspectResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.Active || got.TenantID != "t1" {
		t.Fatalf("body wrong: %+v", got)
	}
}

func TestIntrospect_MissingToken401(t *testing.T) {
	r := NewRouter(introspectSvc{tenant: "t1", active: true}, nil, internalTok)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/internal/introspect", strings.NewReader(`{"token":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	// no X-Internal-Token header
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
```

Note: the existing `handlers_test.go` `fakeSvc` (login-only) will no longer satisfy the widened `AuthService`. Update `fakeSvc` in `handlers_test.go` to add the three no-op methods (`Introspect`, `ListAPIKeys`, `CreateAPIKey`, `RevokeAPIKey`) so that file still compiles:

```go
func (f fakeSvc) Introspect(context.Context, string) (string, bool, error) { return "", false, nil }
func (f fakeSvc) ListAPIKeys(context.Context, string) ([]domain.APIKey, error) { return nil, nil }
func (f fakeSvc) CreateAPIKey(context.Context, string) (string, string, error) { return "", "", nil }
func (f fakeSvc) RevokeAPIKey(context.Context, string, string) error { return nil }
```

and add the `domain` import to `handlers_test.go`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./services/auth-svc/internal/adapters/inbound/http/ -v`
Expected: PASS (login/me tests still green, both introspect tests green).

- [ ] **Step 8: Commit**

```bash
git add services/auth-svc/internal/adapters/inbound/http/
git commit -m "feat(auth-svc): internal introspect endpoint with shared-secret guard"
```

---

### Task 6: auth-svc `/auth/api-keys` management endpoints

JWT-guarded list/create/revoke, tenant scoped from the JWT claims. Create returns the plaintext key exactly once.

**Files:**
- Create: `services/auth-svc/internal/adapters/inbound/http/apikey_handlers.go`
- Modify: `services/auth-svc/internal/adapters/inbound/http/dto.go` (api-key DTOs)
- Modify: `services/auth-svc/internal/adapters/inbound/http/errors.go` (map `ErrAPIKeyNotFound`)
- Modify: `services/auth-svc/internal/adapters/inbound/http/router.go` (wire routes)
- Test: `services/auth-svc/internal/adapters/inbound/http/apikey_handlers_test.go`

**Interfaces:**
- Consumes: `AuthService` (already extended in Task 5), `jwtAuth` (sets `tenantCtxKey`).
- Produces (all under `jwtAuth`):
  - `GET /auth/api-keys` -> `200 [{id,tenant_id,status,created_at}]`
  - `POST /auth/api-keys` -> `201 {id,key}` (`key` is plaintext, shown once)
  - `DELETE /auth/api-keys/:id` -> `204`; `404` if not owned/not found

- [ ] **Step 1: Add api-key DTOs**

Append to `services/auth-svc/internal/adapters/inbound/http/dto.go`:

```go
type apiKeyDTO struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type createKeyResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"` // plaintext, returned once
}
```

- [ ] **Step 2: Map the not-found error**

In `services/auth-svc/internal/adapters/inbound/http/errors.go`, add a case to `writeError`'s switch (before `default`):

```go
	case errors.Is(err, domain.ErrAPIKeyNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
```

- [ ] **Step 3: Write the handlers**

```go
// services/auth-svc/internal/adapters/inbound/http/apikey_handlers.go
package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ListAPIKeys returns the calling tenant's keys (metadata only, no secrets).
func (h *Handlers) ListAPIKeys(c *gin.Context) {
	keys, err := h.svc.ListAPIKeys(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]apiKeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, apiKeyDTO{
			ID: k.ID, TenantID: k.TenantID, Status: k.Status,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, out)
}

// CreateAPIKey mints a key for the calling tenant and returns the plaintext exactly once.
func (h *Handlers) CreateAPIKey(c *gin.Context) {
	plain, id, err := h.svc.CreateAPIKey(c.Request.Context(), c.GetString(tenantCtxKey))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, createKeyResponse{ID: id, Key: plain})
}

// RevokeAPIKey revokes a key the calling tenant owns.
func (h *Handlers) RevokeAPIKey(c *gin.Context) {
	if err := h.svc.RevokeAPIKey(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 4: Wire the routes**

In `services/auth-svc/internal/adapters/inbound/http/router.go`, extend the `auth` group so the api-key routes sit behind `jwtAuth`:

```go
	auth := r.Group("/auth")
	auth.POST("/login", h.Login)
	auth.GET("/me", jwtAuth(verifier), h.Me)

	keys := auth.Group("/api-keys", jwtAuth(verifier))
	keys.GET("", h.ListAPIKeys)
	keys.POST("", h.CreateAPIKey)
	keys.DELETE("/:id", h.RevokeAPIKey)
```

- [ ] **Step 5: Write the failing handler test**

This test issues a real JWT so `jwtAuth` resolves a tenant. It reuses `testKeys` from `pkg/security` via the exported `securitytest` helper if present; otherwise generate a keypair inline (mirror `pkg/security/jwt_test.go`'s `testKeys`).

```go
// services/auth-svc/internal/adapters/inbound/http/apikey_handlers_test.go
package http

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

func issueTestJWT(t *testing.T, tenantID string) (token string, verifier *security.Verifier) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(privKey)
	pkix, _ := x509.MarshalPKIXPublicKey(pubKey)
	priv := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	pub := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))

	iss, _ := security.NewIssuer(priv, time.Hour)
	ver, _ := security.NewVerifier(pub)
	tok, _, _ := iss.Issue("u1", tenantID, "a@b.co", time.Now())
	return tok, ver
}

// keySvc is an AuthService fake tracking create/revoke and returning a fixed list.
type apiKeySvc struct {
	created  bool
	revokeID string
}

func (s *apiKeySvc) Login(context.Context, string, string) (interface{ any }, error) { panic("unused") }

func TestCreateAPIKey_Returns201WithPlaintext(t *testing.T) {
	tok, ver := issueTestJWT(t, "t1")
	svc := &fullSvc{createPlain: "plain-123", createID: "k1"}
	r := NewRouter(svc, ver, "itok")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}
	var got createKeyResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Key != "plain-123" || got.ID != "k1" {
		t.Fatalf("body wrong: %+v", got)
	}
	if svc.createdTenant != "t1" {
		t.Fatalf("create tenant = %q, want t1 (from JWT)", svc.createdTenant)
	}
}

func TestListAPIKeys_Returns200(t *testing.T) {
	tok, ver := issueTestJWT(t, "t1")
	svc := &fullSvc{list: []domain.APIKey{{ID: "k1", TenantID: "t1", Status: "active"}}}
	r := NewRouter(svc, ver, "itok")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got []apiKeyDTO
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "k1" {
		t.Fatalf("body wrong: %+v", got)
	}
}

func TestRevokeAPIKey_Returns204(t *testing.T) {
	tok, ver := issueTestJWT(t, "t1")
	svc := &fullSvc{}
	r := NewRouter(svc, ver, "itok")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/auth/api-keys/k1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if svc.revokedID != "k1" || svc.revokedTenant != "t1" {
		t.Fatalf("revoke got tenant=%q id=%q", svc.revokedTenant, svc.revokedID)
	}
}

func TestAPIKeys_RequireJWT(t *testing.T) {
	_, ver := issueTestJWT(t, "t1")
	r := NewRouter(&fullSvc{}, ver, "itok")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/api-keys", nil) // no Authorization
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without JWT", w.Code)
	}
}
```

Add a single `fullSvc` fake (satisfying the whole `AuthService`) to this test file - delete the abandoned `apiKeySvc` sketch above and use only `fullSvc`:

```go
type fullSvc struct {
	createPlain   string
	createID      string
	createdTenant string
	list          []domain.APIKey
	revokedTenant string
	revokedID     string
}

func (s *fullSvc) Login(context.Context, string, string) (app.LoginResult, error) {
	return app.LoginResult{}, nil
}
func (s *fullSvc) Introspect(context.Context, string) (string, bool, error) { return "", false, nil }
func (s *fullSvc) ListAPIKeys(_ context.Context, _ string) ([]domain.APIKey, error) {
	return s.list, nil
}
func (s *fullSvc) CreateAPIKey(_ context.Context, tenantID string) (string, string, error) {
	s.createdTenant = tenantID
	return s.createPlain, s.createID, nil
}
func (s *fullSvc) RevokeAPIKey(_ context.Context, tenantID, id string) error {
	s.revokedTenant, s.revokedID = tenantID, id
	return nil
}
```

Add the `app` import (`github.com/duykhanh/worklane/services/auth-svc/internal/app`) to this test file for `app.LoginResult`.

- [ ] **Step 6: Run test to verify it fails, then implement passes**

Run: `go test ./services/auth-svc/internal/adapters/inbound/http/ -v`
Expected: after Steps 1-4 are in place, PASS across all handler tests.

- [ ] **Step 7: Commit**

```bash
git add services/auth-svc/internal/adapters/inbound/http/
git commit -m "feat(auth-svc): jwt-guarded api-key list/create/revoke endpoints"
```

---

### Task 7: auth-svc main.go wiring for introspect token

Feed the internal token into the router so the endpoints from Tasks 5-6 are actually mounted.

**Files:**
- Modify: `services/auth-svc/main.go`
- Modify: `deploy/compose/docker-compose.yml` (auth-svc `INTERNAL_API_TOKEN`)

**Interfaces:**
- Consumes: `authhttp.NewRouter(svc, verifier, internalToken)` (Task 5).

- [ ] **Step 1: Read and require the internal token, pass it to the router**

In `services/auth-svc/main.go`, after the `httpAddr` config read, add:

```go
	internalToken := config.EnvOrFile("INTERNAL_API_TOKEN", "")
	if internalToken == "" {
		log.Fatal("auth-svc: INTERNAL_API_TOKEN is required")
	}
```

Then change the server construction to pass the token:

```go
	srv := &http.Server{Addr: httpAddr, Handler: authhttp.NewRouter(svc, verifier, internalToken), ReadHeaderTimeout: 5 * time.Second}
```

- [ ] **Step 2: Add the env in compose**

Under `auth-svc:` `environment:` in `deploy/compose/docker-compose.yml`:

```yaml
      INTERNAL_API_TOKEN: "${INTERNAL_API_TOKEN:-dev-internal-token}"
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./services/auth-svc/...`
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add services/auth-svc/main.go deploy/compose/docker-compose.yml
git commit -m "feat(auth-svc): require and wire INTERNAL_API_TOKEN"
```

---

### Task 8: otp-api Introspector port + identity client + Redis cache

Give otp-api an outbound port for API-key resolution, an HTTP client that calls auth-svc's introspect endpoint, and a short-TTL Redis cache decorator so the machine path stays fast.

**Files:**
- Modify: `services/otp-api/internal/app/ports.go` (add `Introspector` + `ErrInvalidAPIKey`; remove `FindAPIKey`/`ListAPIKeys`/`APIKey`)
- Create: `services/otp-api/internal/adapters/outbound/identity/client.go`
- Create: `services/otp-api/internal/adapters/outbound/identity/cache.go`
- Test: `services/otp-api/internal/adapters/outbound/identity/client_test.go`
- Test: `services/otp-api/internal/adapters/outbound/identity/cache_test.go`

**Interfaces:**
- Produces:
  - `app.Introspector` interface: `Introspect(ctx context.Context, apiKey string) (tenantID string, err error)`
  - `app.ErrInvalidAPIKey` (sentinel; returned when the key is unknown/revoked)
  - `identity.NewClient(baseURL, internalToken string) *Client` implementing `app.Introspector`
  - `identity.NewCached(inner app.Introspector, rc *redis.Client, ttl time.Duration) *Cached` implementing `app.Introspector`

- [ ] **Step 1: Edit the app port**

In `services/otp-api/internal/app/ports.go`:

1. Delete the `APIKey` struct (lines defining `// APIKey is the resolved...` and the struct).
2. In the `Repo` interface, delete the `FindAPIKey(...)` and `ListAPIKeys(...)` lines. The `Repo` interface becomes:

```go
type Repo interface {
	InsertRequest(ctx context.Context, r Request) error
	UpdateState(ctx context.Context, id, to string) error
	ListRequests(ctx context.Context, tenantID string, limit int) ([]Request, error)
	ListDeliveryLogs(ctx context.Context, tenantID string, limit int) ([]DeliveryLog, error)
}
```

3. Add the introspector port + sentinel. Add `"errors"` to the import block, then:

```go
// ErrInvalidAPIKey means the presented API key is unknown or revoked (a client error,
// mapped to 401). It is distinct from a transport failure calling the identity service
// (mapped to 503), so the middleware can react differently to each.
var ErrInvalidAPIKey = errors.New("invalid api key")

// Introspector resolves a machine API key to its tenant by asking the identity service.
// Implemented by the identity HTTP client (optionally wrapped in a Redis cache).
type Introspector interface {
	Introspect(ctx context.Context, apiKey string) (tenantID string, err error)
}
```

- [ ] **Step 2: Write the failing client test**

```go
// services/otp-api/internal/adapters/outbound/identity/client_test.go
package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

func TestClient_Introspect_Active(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "itok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":true,"tenant_id":"t1"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "itok")
	tenant, err := c.Introspect(context.Background(), "some-key")
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if tenant != "t1" {
		t.Fatalf("tenant = %q, want t1", tenant)
	}
}

func TestClient_Introspect_Inactive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"active":false}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "itok")
	_, err := c.Introspect(context.Background(), "bad-key")
	if !errors.Is(err, app.ErrInvalidAPIKey) {
		t.Fatalf("want ErrInvalidAPIKey, got %v", err)
	}
}

func TestClient_Introspect_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "itok")
	_, err := c.Introspect(context.Background(), "k")
	if err == nil || errors.Is(err, app.ErrInvalidAPIKey) {
		t.Fatalf("server error must be a transport error, got %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/adapters/outbound/identity/ -v`
Expected: FAIL - no package / `NewClient` undefined.

- [ ] **Step 4: Write the client**

```go
// services/otp-api/internal/adapters/outbound/identity/client.go
//
// Package identity is otp-api's outbound adapter for the identity service: it resolves a
// machine API key to a tenant via auth-svc's network-internal /internal/introspect
// endpoint. It implements the app.Introspector port.
package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// Client calls auth-svc's introspect endpoint over the internal network.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, internalToken string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   internalToken,
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

type introspectReq struct {
	Token string `json:"token"`
}

type introspectResp struct {
	Active   bool   `json:"active"`
	TenantID string `json:"tenant_id"`
}

// Introspect returns the tenant id for an active key, app.ErrInvalidAPIKey for an
// unknown/revoked key, or a wrapped transport error if the identity service is unreachable
// or misbehaves.
func (c *Client) Introspect(ctx context.Context, apiKey string) (string, error) {
	body, err := json.Marshal(introspectReq{Token: apiKey})
	if err != nil {
		return "", fmt.Errorf("identity: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/introspect", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("identity: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)

	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("identity: introspect call: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("identity: introspect status %d", res.StatusCode)
	}
	var out introspectResp
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("identity: decode: %w", err)
	}
	if !out.Active {
		return "", app.ErrInvalidAPIKey
	}
	return out.TenantID, nil
}

var _ app.Introspector = (*Client)(nil)
```

- [ ] **Step 5: Write the failing cache test**

```go
// services/otp-api/internal/adapters/outbound/identity/cache_test.go
package identity

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// countingIntrospector records how many times the inner introspector is hit.
type countingIntrospector struct {
	tenant string
	calls  int
}

func (c *countingIntrospector) Introspect(context.Context, string) (string, error) {
	c.calls++
	return c.tenant, nil
}

func TestCached_HitsInnerOnceThenCaches(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	inner := &countingIntrospector{tenant: "t1"}
	cached := NewCached(inner, rc, time.Minute)

	for i := 0; i < 3; i++ {
		tenant, err := cached.Introspect(context.Background(), "same-key")
		if err != nil || tenant != "t1" {
			t.Fatalf("call %d: tenant=%q err=%v", i, tenant, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("inner called %d times, want 1 (cache should absorb the rest)", inner.calls)
	}
}
```

If `github.com/alicebob/miniredis/v2` is not present, run `go get github.com/alicebob/miniredis/v2` (it was added for auth-svc's limiter in M1).

- [ ] **Step 6: Write the cache decorator**

```go
// services/otp-api/internal/adapters/outbound/identity/cache.go
package identity

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// Cached wraps an Introspector with a short-TTL positive cache in Redis. Only successful
// resolutions are cached (keyed by the key's hash, never the plaintext); unknown/revoked
// keys and transport errors always fall through to the inner call. The TTL bounds how long
// a revoked key can keep working - a conscious freshness/latency tradeoff.
type Cached struct {
	inner app.Introspector
	rc    *redis.Client
	ttl   time.Duration
}

func NewCached(inner app.Introspector, rc *redis.Client, ttl time.Duration) *Cached {
	return &Cached{inner: inner, rc: rc, ttl: ttl}
}

func (c *Cached) Introspect(ctx context.Context, apiKey string) (string, error) {
	cacheKey := "introspect:" + security.HashKey(apiKey)
	if tenant, err := c.rc.Get(ctx, cacheKey).Result(); err == nil && tenant != "" {
		return tenant, nil
	}
	tenant, err := c.inner.Introspect(ctx, apiKey)
	if err != nil {
		return "", err
	}
	// Best-effort cache write; a Redis hiccup must not fail a valid auth.
	_ = c.rc.Set(ctx, cacheKey, tenant, c.ttl).Err()
	return tenant, nil
}

var _ app.Introspector = (*Cached)(nil)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./services/otp-api/internal/adapters/outbound/identity/ -v`
Expected: PASS (client + cache).

- [ ] **Step 8: Commit**

```bash
git add services/otp-api/internal/app/ports.go services/otp-api/internal/adapters/outbound/identity/
git commit -m "feat(otp-api): identity introspection client with redis cache"
```

---

### Task 9: otp-api authenticate via introspection; remove local api_keys

Switch the middleware's opaque-key branch to the introspector, drop the now-dead `api_keys` code (repo methods, handler, route, DTO), and update the affected tests and composition root. After this task otp-api has no `api_keys` table dependency.

**Files:**
- Modify: `services/otp-api/internal/adapters/inbound/http/middleware.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/router.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/handlers.go` (remove `ListAPIKeys`)
- Modify: `services/otp-api/internal/adapters/inbound/http/dto.go` (remove `apiKeyDTO`)
- Modify: `services/otp-api/internal/adapters/outbound/mysqlrepo/repo.go` (remove `FindAPIKey`/`ListAPIKeys`/`apiKeyRow`)
- Modify: `services/otp-api/internal/adapters/outbound/mysqlrepo/repo_test.go` (remove api-key repo tests)
- Modify: `services/otp-api/internal/adapters/inbound/http/middleware_test.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/handlers_test.go`
- Modify: `services/otp-api/internal/app/usecase_test.go` (drop `FindAPIKey`/`ListAPIKeys` from `fakeRepo`)
- Modify: `services/otp-api/main.go`
- Modify: `deploy/compose/docker-compose.yml` (otp-api `AUTH_SVC_URL`, `INTERNAL_API_TOKEN`, depends_on)

**Interfaces:**
- Consumes: `app.Introspector`, `app.ErrInvalidAPIKey` (Task 8), `*security.Verifier`.
- Produces: `authenticate(v *security.Verifier, intro app.Introspector) gin.HandlerFunc`; `NewRouter(svc OTPService, repo app.Repo, verifier *security.Verifier, intro app.Introspector) *gin.Engine`.

- [ ] **Step 1: Rewrite the middleware**

Replace `services/otp-api/internal/adapters/inbound/http/middleware.go` with:

```go
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// tenantCtxKey is where the resolved tenant id is stashed for handlers to read.
const tenantCtxKey = "tenant_id"

// authenticate resolves a Bearer token that is either a user JWT (dashboard) or a tenant
// API key (machine). A JWT has three dot-separated parts and is verified locally with the
// public key. Anything else is an opaque API key, resolved by asking the identity service
// (introspection, Redis-cached). Both paths set tenant_id for downstream handlers.
func authenticate(v *security.Verifier, intro app.Introspector) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing credentials"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))

		if strings.Count(token, ".") == 2 {
			claims, err := v.Parse(token)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			c.Set(tenantCtxKey, claims.TenantID)
			c.Next()
			return
		}

		tenantID, err := intro.Introspect(c.Request.Context(), token)
		switch {
		case errors.Is(err, app.ErrInvalidAPIKey):
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		case err != nil:
			// The identity service is unreachable or errored: fail closed but distinctly.
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "auth temporarily unavailable"})
			return
		}
		c.Set(tenantCtxKey, tenantID)
		c.Next()
	}
}
```

- [ ] **Step 2: Update the router (signature + drop /v1/api-keys)**

Replace `services/otp-api/internal/adapters/inbound/http/router.go` with:

```go
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// NewRouter builds the otp-api HTTP handler: all /v1 routes sit behind the authenticate
// middleware (user JWT or tenant API key via introspection), so every endpoint is
// tenant-scoped.
func NewRouter(svc OTPService, repo app.Repo, verifier *security.Verifier, intro app.Introspector) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := &Handlers{svc: svc, repo: repo}

	v1 := r.Group("/v1")
	v1.Use(authenticate(verifier, intro))
	{
		v1.POST("/otp/send", h.Send)
		v1.POST("/otp/verify", h.Verify)
		v1.GET("/otp/requests", h.ListRequests)
		v1.GET("/delivery-logs", h.ListDeliveryLogs)
	}
	return r
}
```

- [ ] **Step 3: Remove the ListAPIKeys handler and DTO**

In `services/otp-api/internal/adapters/inbound/http/handlers.go`, delete the entire `func (h *Handlers) ListAPIKeys(...)` method.

In `services/otp-api/internal/adapters/inbound/http/dto.go`, delete the `apiKeyDTO` struct.

- [ ] **Step 4: Remove the repo api-key code**

In `services/otp-api/internal/adapters/outbound/mysqlrepo/repo.go`, delete the `apiKeyRow` struct + its `TableName()`, and the `FindAPIKey` and `ListAPIKeys` methods.

In `services/otp-api/internal/adapters/outbound/mysqlrepo/repo_test.go`, delete `TestRepo_FindAPIKey` (and any api-key seeding helper it uses that nothing else references).

- [ ] **Step 5: Update the interface fakes in tests**

- `services/otp-api/internal/app/usecase_test.go`: delete the `fakeRepo` methods `FindAPIKey` and `ListAPIKeys`.
- `services/otp-api/internal/adapters/inbound/http/handlers_test.go`: delete the `fakeRepo` methods `FindAPIKey` and `ListAPIKeys`, delete `TestListAPIKeys_Returns200`, and update any `NewRouter(...)` calls in this file to the new 4-arg signature, passing a stub introspector (see the middleware test's stub below) for the extra argument.
- `services/otp-api/internal/adapters/inbound/http/middleware_test.go`: rewrite so the opaque-key path uses an introspector stub instead of `stubRepo.FindAPIKey`:

```go
// stubIntrospector stands in for the identity client.
type stubIntrospector struct {
	tenant string
	err    error
}

func (s stubIntrospector) Introspect(context.Context, string) (string, error) {
	return s.tenant, s.err
}

func testRouter(v *security.Verifier, intro app.Introspector) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/probe", authenticate(v, intro), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(tenantCtxKey))
	})
	return r
}

func TestAuthenticate_ValidAPIKey(t *testing.T) {
	r := testRouter(nil, stubIntrospector{tenant: "tenant-key"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer opaque-key-value")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "tenant-key" {
		t.Fatalf("code=%d body=%q, want 200 tenant-key", w.Code, w.Body.String())
	}
}

func TestAuthenticate_InvalidAPIKey_401(t *testing.T) {
	r := testRouter(nil, stubIntrospector{err: app.ErrInvalidAPIKey})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer bad")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d, want 401", w.Code)
	}
}

func TestAuthenticate_IntrospectDown_503(t *testing.T) {
	r := testRouter(nil, stubIntrospector{err: errors.New("connection refused")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d, want 503", w.Code)
	}
}
```

Keep the existing valid-JWT test in this file (it already builds a real verifier/issuer); update its `testRouter` call to pass a `stubIntrospector{}` as the second argument. Ensure the imports include `"errors"`, `"context"`, and `app`.

- [ ] **Step 6: Wire the composition root**

In `services/otp-api/main.go`:

1. Add config reads near the others:

```go
	authSvcURL := config.Env("AUTH_SVC_URL", "http://localhost:8889")
	internalToken := config.EnvOrFile("INTERNAL_API_TOKEN", "")
	introspectTTL := config.EnvDuration("INTROSPECT_CACHE_TTL", time.Minute)
```

2. After `verifier` is built, add a fail-fast check and build the introspector (add the identity import `identity "github.com/duykhanh/worklane/services/otp-api/internal/adapters/outbound/identity"`):

```go
	if internalToken == "" {
		log.Fatal("otp-api: INTERNAL_API_TOKEN is required")
	}
	introspector := identity.NewCached(identity.NewClient(authSvcURL, internalToken), rc, introspectTTL)
```

3. Update the router construction:

```go
		Handler:           otphttp.NewRouter(svc, repo, verifier, introspector),
```

- [ ] **Step 7: Update compose (otp-api env + depends_on)**

Under `otp-api:` `environment:` in `deploy/compose/docker-compose.yml`, add:

```yaml
      AUTH_SVC_URL: "http://auth-svc:8889"
      INTERNAL_API_TOKEN: "${INTERNAL_API_TOKEN:-dev-internal-token}"
```

Under `otp-api:` `depends_on:`, add auth-svc so the introspection target is up (keep the existing mysql/redis/redpanda entries):

```yaml
      auth-svc: { condition: service_started }
```

- [ ] **Step 8: Run the full otp-api test suite**

Run: `go test -race ./services/otp-api/...`
Expected: PASS (middleware JWT + API-key + 503, handlers without api-keys, repo without api-keys, arch test green). Then `go build ./services/otp-api/...`.

- [ ] **Step 9: Commit**

```bash
git add services/otp-api deploy/compose/docker-compose.yml
git commit -m "feat(otp-api): authenticate api keys via introspection, drop local api_keys"
```

---

### Task 10: seed CLI targets the identity database

The seed inserts `tenants`, `users`, and `api_keys` - all now in `identity`. Point its default DSN there and update the doc comment.

**Files:**
- Modify: `services/seed/main.go`

**Interfaces:**
- Produces: `go run ./services/seed --name <t> --email <e> --password <p>` writes tenant + user + api key into the `identity` database.

- [ ] **Step 1: Change the default DSN and doc comment**

In `services/seed/main.go`, change the DSN default from `.../otp` to `.../identity`:

```go
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/identity?parseTime=true&multiStatements=true")
```

Update the package doc comment's example line to reflect the identity DB:

```go
//	MYSQL_DSN='root:secret@tcp(localhost:3306)/identity?parseTime=true&multiStatements=true' \
//	  go run ./services/seed --name demo --email you@demo.co --password 'sup3rsecret'
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./services/seed/...`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add services/seed/main.go
git commit -m "feat(seed): provision tenant/user/api-key into identity database"
```

---

### Task 11: dashboard lists keys via `/auth/api-keys`

`/v1/api-keys` is gone. Repoint the live data source's `listApiKeys` to the identity endpoint. (Create/revoke UI stays deferred - the page's "New key" button is already disabled and labeled "coming in Phase 2".)

**Files:**
- Modify: `dashboard/lib/api/live.ts`
- Test: `dashboard/lib/api/live.test.ts` (create if absent; otherwise extend)

**Interfaces:**
- Consumes: `/auth/api-keys` (JWT-guarded; the live source already sends `Authorization: Bearer <JWT>`), returning `[{id,tenant_id,status,created_at}]`.
- Produces: `LiveDataSource.listApiKeys()` mapping that shape to `ApiKey[]`.

- [ ] **Step 1: Repoint and map the endpoint**

In `dashboard/lib/api/live.ts`, replace the `listApiKeys` method body so it calls `/auth/api-keys` and reads `created_at`:

```ts
  async listApiKeys(): Promise<ApiKey[]> {
    const rows = await this.get<
      { id: string; tenant_id: string; status: string; created_at?: string }[]
    >("/auth/api-keys");
    return rows.map((k) => ({
      id: k.id,
      tenantId: k.tenant_id,
      status: k.status === "revoked" ? "revoked" : "active",
      createdAt: k.created_at ?? "",
    }));
  }
```

- [ ] **Step 2: Write/extend the test**

Add a test asserting the path and mapping (mirror the existing dashboard test style; if `live.test.ts` does not exist, create it):

```ts
// dashboard/lib/api/live.test.ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { LiveDataSource } from "./live";

afterEach(() => vi.restoreAllMocks());

describe("LiveDataSource.listApiKeys", () => {
  it("calls /auth/api-keys and maps the response", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      expect(url).toBe("http://x/auth/api-keys");
      return new Response(
        JSON.stringify([
          { id: "k1", tenant_id: "t1", status: "active", created_at: "2026-08-19T00:00:00Z" },
        ]),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    const keys = await ds.listApiKeys();

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(keys).toEqual([
      { id: "k1", tenantId: "t1", status: "active", createdAt: "2026-08-19T00:00:00Z" },
    ]);
  });
});
```

- [ ] **Step 3: Run the dashboard test**

Run: `cd dashboard && pnpm test -- live.test.ts`
Expected: PASS. Then `pnpm test` to confirm no regression in the existing api suite.

- [ ] **Step 4: Commit**

```bash
git add dashboard/lib/api/live.ts dashboard/lib/api/live.test.ts
git commit -m "feat(dashboard): list api keys from identity /auth/api-keys"
```

---

### Task 12: End-to-end verification on the compose stack + docs

Prove the whole split works against the real stack: JWT (human) and API-key-via-introspection (machine) both resolve to a tenant, and identity lives only in the `identity` DB. Then update the design/architecture docs to mark M2 done.

**Files:**
- Modify: `docs/superpowers/specs/2026-08-19-dashboard-auth-identity-service-design.md` (mark M2 complete)
- Modify: `docs/architecture.md` (reflect identity DB + introspection, if it describes the DB/auth layout)

- [ ] **Step 1: Clean cutover bring-up**

Run:
```bash
docker compose -f deploy/compose/docker-compose.yml down -v
docker compose -f deploy/compose/docker-compose.yml up -d --build
sleep 25
```
Expected: all services healthy; `auth-svc` and `otp-api` logs show no migrate errors.

- [ ] **Step 2: Confirm the database split**

Run:
```bash
docker compose -f deploy/compose/docker-compose.yml exec -T mysql mysql -uroot -psecret identity -e "SHOW TABLES;"
docker compose -f deploy/compose/docker-compose.yml exec -T mysql mysql -uroot -psecret otp -e "SHOW TABLES;"
```
Expected: `identity` has `api_keys`, `tenants`, `users`; `otp` has only `delivery_logs`, `otp_requests`, `templates`.

- [ ] **Step 3: Seed a tenant/user/key into identity**

Run:
```bash
MYSQL_DSN='root:secret@tcp(localhost:3306)/identity?parseTime=true&multiStatements=true' \
  go run ./services/seed --name demo --email you@demo.co --password 'sup3rsecret'
```
Expected: prints `tenant_id`, `user: you@demo.co (password set)`, and an API key. Save the API key as `$KEY`.

- [ ] **Step 4: Machine path (API key -> introspection)**

Run (through Traefik on :80):
```bash
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $KEY" http://localhost/v1/otp/requests
```
Expected: `200`. Run it again immediately and confirm otp-api logs show only ONE introspect round-trip for the pair (the second is served from the Redis cache). A bogus key returns `401`:
```bash
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer not-a-real-key" http://localhost/v1/otp/requests
```
Expected: `401`.

- [ ] **Step 5: Human path (login -> JWT -> api-keys)**

Run:
```bash
TOKEN=$(curl -s -X POST http://localhost/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"you@demo.co","password":"sup3rsecret"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -s -H "Authorization: Bearer $TOKEN" http://localhost/auth/api-keys
```
Expected: login returns a token; `/auth/api-keys` returns the seeded key's metadata (`status:"active"`). Verify create + revoke:
```bash
NEWKEY=$(curl -s -X POST http://localhost/auth/api-keys -H "Authorization: Bearer $TOKEN")
echo "$NEWKEY"   # {"id":"...","key":"..."} - plaintext shown once
KID=$(echo "$NEWKEY" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -s -o /dev/null -w "%{http_code}\n" -X DELETE http://localhost/auth/api-keys/$KID -H "Authorization: Bearer $TOKEN"
```
Expected: create returns `201` JSON with `id`+`key`; delete returns `204`.

- [ ] **Step 6: Confirm introspect is not exposed via Traefik**

Run:
```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost/internal/introspect -H 'Content-Type: application/json' -d '{"token":"x"}'
```
Expected: `404` (Traefik has no `/internal` route) - the endpoint is reachable only on the Docker network.

- [ ] **Step 7: Update the docs**

In `docs/superpowers/specs/2026-08-19-dashboard-auth-identity-service-design.md` section 6, mark M2 as delivered (e.g. append `**Status: M1 shipped; M2 shipped (this plan).**`). If `docs/architecture.md` describes the database or auth topology, update it to show the `identity` database owned by auth-svc and otp-api's API-key introspection path. Keep edits factual and minimal.

- [ ] **Step 8: Commit**

```bash
git add docs/superpowers/specs/2026-08-19-dashboard-auth-identity-service-design.md docs/architecture.md
git commit -m "docs: mark identity/OTP split (M2) complete"
```

---

## Self-Review

**Spec coverage (design doc section 6 M2 = "move api_keys fully into auth-svc; otp-api switches API-key validation to introspection; remove otp-api's local identity tables" + user's added "split identity DB"):**
- Split identity DB -> Task 1 (identity DB + auth-svc migrations), Task 2 (drop from otp).
- Move api_keys into auth-svc -> Task 3 (repo), Task 4 (use cases), Task 6 (`/auth/api-keys`), Task 9 (removal from otp-api), Task 10 (seed).
- Introspection -> Task 5 (`/internal/introspect`), Task 7 (token wiring), Task 8 (otp-api client+cache), Task 9 (middleware switch).
- Dashboard keeps working -> Task 11 (repoint list).
- Design section 4 endpoints (login/me unchanged; api-keys GET/POST/DELETE; introspect) -> Tasks 5, 6. Section 5 otp-api unified middleware + config -> Tasks 8, 9. Section 8 seed -> Task 10. Section 11 infra (two DBs, env, Traefik) -> Tasks 1, 7, 9, 12.
- E2E verification -> Task 12.

**Placeholder scan:** No TBD/TODO; every code step has real code; test bodies are concrete.

**Type consistency:** `Introspect` returns `(tenantID string, active bool, err error)` on the auth-svc `Service`/`AuthService` (Tasks 4, 5); the otp-api `Introspector` port returns `(tenantID string, err error)` with `app.ErrInvalidAPIKey` (Tasks 8, 9) - deliberately different shapes on the two sides of the network boundary, bridged by the HTTP client in Task 8. `Repo` in auth-svc uses `domain.APIKey`; `NewRouter(svc, verifier, internalToken)` (auth-svc, Task 5) and `NewRouter(svc, repo, verifier, intro)` (otp-api, Task 9) signatures are used consistently in their tests and composition roots. `security.HashKey`/`GenerateAPIKey` names match `pkg/security/apikey.go`.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-19-dashboard-auth-m2.md`.
