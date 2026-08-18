# Dashboard Auth (M1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the dashboard's paste-an-API-key field with real email+password login backed by a new `auth-svc` that issues short-lived EdDSA JWTs, which `otp-api` verifies locally.

**Architecture:** New Go service `services/auth-svc` (hexagonal, mirrors otp-api) owns login and issues JWTs signed with an Ed25519 private key. `otp-api` gains a `Verifier` (public key) and an `authenticate` middleware that accepts **either** a JWT (humans) **or** the existing API key (machines - kept as-is for M1). The dashboard gets a working login page, an auth guard, and drops the paste-key field.

**Tech Stack:** Go 1.25, gin, GORM, golang-migrate, Redis; `golang.org/x/crypto/argon2` (passwords), `github.com/golang-jwt/jwt/v5` + stdlib `crypto/ed25519` (JWT); Next.js dashboard (zustand, TanStack Query).

**Design doc:** `docs/superpowers/specs/2026-08-19-dashboard-auth-identity-service-design.md`

## Global Constraints

- Module path: `github.com/duykhanh/worklane`. Go `1.25.0`.
- **M1 shares one database:** `auth-svc` connects to the existing `otp` MySQL database (adds a `users` table). The `identity`/`otp` database split is **M2**, not this plan.
- **M1 keeps otp-api's local API-key check** working; only *adds* the JWT path. No `api_keys`/`tenants` tables move in M1.
- Hexagon rule (enforced by `arch_test`): `internal/domain` and `internal/app` must not import `redis`, `gorm`, `sarama`, `gin`, `/adapters/`, or `/pkg/platform/`. Importing `pkg/security` from `app` is allowed.
- Immutability, small files, wrap errors with `fmt.Errorf("...: %w", err)`, table-driven tests, `go test -race ./...` green.
- No em dash. Commit messages: `<type>: <desc>`, no co-author line.
- Never log or return the OTP code, a password, or a raw JWT/key in error messages.

---

### Task 1: Password hashing (argon2id) in `pkg/security`

**Files:**
- Create: `pkg/security/password.go`
- Test: `pkg/security/password_test.go`
- Modify: `go.mod` / `go.sum` (add `golang.org/x/crypto`)

**Interfaces:**
- Produces: `security.HashPassword(plain string) (string, error)` and `security.VerifyPassword(encoded, plain string) (bool, error)`.

- [ ] **Step 1: Add the dependency**

Run: `go get golang.org/x/crypto/argon2`

- [ ] **Step 2: Write the failing test**

```go
// pkg/security/password_test.go
package security

import "testing"

func TestHashPassword_RoundTrip(t *testing.T) {
	enc, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if enc == "correct horse" {
		t.Fatal("hash must not equal plaintext")
	}
	ok, err := VerifyPassword(enc, "correct horse")
	if err != nil || !ok {
		t.Fatalf("verify correct: ok=%v err=%v", ok, err)
	}
	bad, err := VerifyPassword(enc, "wrong")
	if err != nil || bad {
		t.Fatalf("verify wrong must be false: ok=%v err=%v", bad, err)
	}
}

func TestVerifyPassword_BadFormat(t *testing.T) {
	if _, err := VerifyPassword("not-a-hash", "x"); err == nil {
		t.Fatal("want error on malformed hash")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/security/ -run TestHashPassword -v`
Expected: FAIL - `undefined: HashPassword`.

- [ ] **Step 4: Write minimal implementation**

```go
// pkg/security/password.go
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters, tuned for interactive login (~64 MiB, 1 pass, 4 lanes). They are
// encoded into every hash, so raising them later still verifies old rows.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns a self-describing argon2id string:
// $argon2id$v=19$m=65536,t=1,p=4$<b64 salt>$<b64 hash>.
func HashPassword(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("security: salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

// VerifyPassword recomputes the hash with the encoded params/salt and compares in
// constant time. Returns (false, nil) for a wrong password; error only for a malformed
// encoding.
func VerifyPassword(encoded, plain string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("security: bad password hash format")
	}
	var version, memory, iter, threads int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("security: hash version: %w", err)
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iter, &threads); err != nil {
		return false, fmt.Errorf("security: hash params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("security: hash salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("security: hash digest: %w", err)
	}
	got := argon2.IDKey([]byte(plain), salt, uint32(iter), uint32(memory), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/security/ -run TestHashPassword -v && go test ./pkg/security/ -run TestVerifyPassword -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/security/password.go pkg/security/password_test.go go.mod go.sum
git commit -m "feat(security): argon2id password hashing helpers"
```

---

### Task 2: EdDSA JWT Issuer/Verifier in `pkg/security`

**Files:**
- Create: `pkg/security/jwt.go`
- Test: `pkg/security/jwt_test.go`
- Modify: `go.mod` / `go.sum` (add `github.com/golang-jwt/jwt/v5`)

**Interfaces:**
- Produces:
  - `type Claims struct { UserID, TenantID, Email string }`
  - `security.NewIssuer(privPEM string, ttl time.Duration) (*Issuer, error)`; `(*Issuer).Issue(userID, tenantID, email string, now time.Time) (token string, expiresAt time.Time, err error)`
  - `security.NewVerifier(pubPEM string) (*Verifier, error)`; `(*Verifier).Parse(token string) (Claims, error)`

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/golang-jwt/jwt/v5`

- [ ] **Step 2: Write the failing test**

```go
// pkg/security/jwt_test.go
package security

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func testKeys(t *testing.T) (priv string, pub string) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(privKey)
	pkix, _ := x509.MarshalPKIXPublicKey(pubKey)
	priv = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	pub = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))
	return priv, pub
}

func TestJWT_IssueParse(t *testing.T) {
	priv, pub := testKeys(t)
	iss, err := NewIssuer(priv, time.Hour)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	ver, err := NewVerifier(pub)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	tok, exp, err := iss.Issue("u1", "t1", "a@b.co", time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatal("exp should be in the future")
	}
	c, err := ver.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.UserID != "u1" || c.TenantID != "t1" || c.Email != "a@b.co" {
		t.Fatalf("claims wrong: %+v", c)
	}
}

func TestJWT_Expired(t *testing.T) {
	priv, pub := testKeys(t)
	iss, _ := NewIssuer(priv, time.Hour)
	ver, _ := NewVerifier(pub)
	tok, _, _ := iss.Issue("u1", "t1", "a@b.co", time.Now().Add(-2*time.Hour))
	if _, err := ver.Parse(tok); err == nil {
		t.Fatal("expired token must fail to parse")
	}
}

func TestJWT_WrongKey(t *testing.T) {
	priv, _ := testKeys(t)
	_, otherPub := testKeys(t)
	iss, _ := NewIssuer(priv, time.Hour)
	ver, _ := NewVerifier(otherPub)
	tok, _, _ := iss.Issue("u1", "t1", "a@b.co", time.Now())
	if _, err := ver.Parse(tok); err == nil {
		t.Fatal("token signed by a different key must fail")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/security/ -run TestJWT -v`
Expected: FAIL - `undefined: NewIssuer`.

- [ ] **Step 4: Write minimal implementation**

```go
// pkg/security/jwt.go
package security

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the decoded, transport-free view the app layer consumes.
type Claims struct {
	UserID   string
	TenantID string
	Email    string
}

// jwtClaims is the on-the-wire shape (custom fields + standard registered claims).
type jwtClaims struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// Issuer signs tokens with an Ed25519 private key. Only auth-svc holds one.
type Issuer struct {
	key ed25519.PrivateKey
	ttl time.Duration
}

// NewIssuer parses a PKCS#8 PEM private key.
func NewIssuer(privPEM string, ttl time.Duration) (*Issuer, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return nil, errors.New("security: invalid private key PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("security: parse private key: %w", err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("security: private key is not ed25519")
	}
	return &Issuer{key: priv, ttl: ttl}, nil
}

// Issue returns a signed token and its expiry.
func (i *Issuer) Issue(userID, tenantID, email string, now time.Time) (string, time.Time, error) {
	exp := now.Add(i.ttl)
	claims := jwtClaims{
		TenantID: tenantID,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("security: sign token: %w", err)
	}
	return tok, exp, nil
}

// Verifier checks signatures with an Ed25519 public key. Resource services hold only this.
type Verifier struct{ key ed25519.PublicKey }

// NewVerifier parses a PKIX PEM public key.
func NewVerifier(pubPEM string) (*Verifier, error) {
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return nil, errors.New("security: invalid public key PEM")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("security: parse public key: %w", err)
	}
	pub, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("security: public key is not ed25519")
	}
	return &Verifier{key: pub}, nil
}

// Parse validates the signature + expiry and returns the claims. It pins the algorithm to
// EdDSA so an attacker cannot swap in "alg: none" or a weaker method.
func (v *Verifier) Parse(token string) (Claims, error) {
	var c jwtClaims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("security: unexpected signing method %v", t.Header["alg"])
		}
		return v.key, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("security: parse token: %w", err)
	}
	return Claims{UserID: c.Subject, TenantID: c.TenantID, Email: c.Email}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/security/ -run TestJWT -v`
Expected: PASS (all three).

- [ ] **Step 6: Commit**

```bash
git add pkg/security/jwt.go pkg/security/jwt_test.go go.mod go.sum
git commit -m "feat(security): EdDSA JWT issuer and verifier"
```

---

### Task 3: `users` table migration + seed provisions a user

**Files:**
- Create: `db/otp/migrations/0002_users.up.sql`, `db/otp/migrations/0002_users.down.sql`
- Modify: `services/seed/main.go`

**Interfaces:**
- Produces: a `users` table in the `otp` database; `go run ./services/seed --name <t> --email <e> --password <p>` inserts a tenant + an active user with an argon2id hash (still mints an API key as before).

- [ ] **Step 1: Write the up/down migrations**

```sql
-- db/otp/migrations/0002_users.up.sql
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

```sql
-- db/otp/migrations/0002_users.down.sql
DROP TABLE users;
```

- [ ] **Step 2: Extend the seed CLI**

In `services/seed/main.go`, add `email`/`password` flags and, after the tenant insert, insert a user. Full replacement of the flag block and inserts:

```go
	name := flag.String("name", "", "tenant name (required)")
	email := flag.String("email", "", "user email (required)")
	password := flag.String("password", "", "user password (required)")
	flag.Parse()
	if *name == "" || *email == "" || *password == "" {
		log.Fatal("seed: --name, --email and --password are required")
	}
```

After the existing `INSERT INTO api_keys ...` block, add:

```go
	pwHash, err := security.HashPassword(*password)
	if err != nil {
		log.Fatalf("seed: hash password: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO users (id, tenant_id, email, password_hash, status) VALUES (?, ?, ?, ?, 'active')",
		newID(), tenantID, *email, pwHash,
	).Error; err != nil {
		log.Fatalf("seed: insert user: %v", err)
	}
	fmt.Printf("user: %s (password set)\n", *email)
```

- [ ] **Step 3: Apply migration + seed against the running stack**

Run (compose stack up):
```bash
docker compose -f deploy/compose/docker-compose.yml restart otp-api   # applies 0002
MYSQL_DSN='root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true' \
  go run ./services/seed --name demo --email you@demo.co --password 'sup3rsecret'
```
Expected: prints `tenant_id`, an API key, and `user: you@demo.co (password set)`.

- [ ] **Step 4: Verify the row exists**

Run: `docker compose -f deploy/compose/docker-compose.yml exec -T mysql mysql -uroot -psecret otp -e "SELECT email,status FROM users;"`
Expected: one row `you@demo.co  active`.

- [ ] **Step 5: Commit**

```bash
git add db/otp/migrations/0002_users.up.sql db/otp/migrations/0002_users.down.sql services/seed/main.go
git commit -m "feat(identity): users table and seed --email/--password provisioning"
```

---

### Task 4: auth-svc domain + repo (FindUserByEmail)

**Files:**
- Create: `services/auth-svc/internal/domain/user.go`, `services/auth-svc/internal/domain/errors.go`
- Create: `services/auth-svc/internal/app/ports.go`
- Create: `services/auth-svc/internal/adapters/outbound/mysqlrepo/repo.go`
- Test: `services/auth-svc/internal/adapters/outbound/mysqlrepo/repo_test.go`

**Interfaces:**
- Produces:
  - `domain.User{ ID, TenantID, Email, PasswordHash, Status string }`
  - `domain.ErrUserNotFound`, `domain.ErrInvalidCredentials`, `domain.ErrInactive`, `domain.ErrRateLimited`
  - `app.Repo` interface with `FindUserByEmail(ctx, email string) (domain.User, error)`
  - `mysqlrepo.New(db *gorm.DB) *Repo` implementing `app.Repo`, returning `domain.ErrUserNotFound` on no row.

- [ ] **Step 1: Write domain types**

```go
// services/auth-svc/internal/domain/user.go
package domain

// User is a dashboard operator that belongs to a tenant.
type User struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Status       string
}
```

```go
// services/auth-svc/internal/domain/errors.go
package domain

import "errors"

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrInactive            = errors.New("user inactive")
	ErrRateLimited         = errors.New("too many attempts")
)
```

- [ ] **Step 2: Write the port**

```go
// services/auth-svc/internal/app/ports.go
package app

import (
	"context"
	"time"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Repo reads identity rows.
type Repo interface {
	FindUserByEmail(ctx context.Context, email string) (domain.User, error)
}

// TokenIssuer mints a signed access token. Implemented by *security.Issuer.
type TokenIssuer interface {
	Issue(userID, tenantID, email string, now time.Time) (token string, expiresAt time.Time, err error)
}

// RateLimiter caps login attempts. Allow returns false when the caller is over budget.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// Clock abstracts time for testable expiry.
type Clock interface{ Now() time.Time }
```

- [ ] **Step 3: Write the failing repo test**

```go
// services/auth-svc/internal/adapters/outbound/mysqlrepo/repo_test.go
package mysqlrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

func TestFindUserByEmail_NotFound(t *testing.T) {
	// A nil DB is never dialed because SQLite/testcontainers wiring is out of scope here;
	// this asserts the not-found mapping via a lightweight in-memory sqlite handle.
	r := newTestRepo(t)
	_, err := r.FindUserByEmail(context.Background(), "nobody@x.co")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestFindUserByEmail_Found(t *testing.T) {
	r := newTestRepo(t)
	seedUser(t, r, domain.User{ID: "u1", TenantID: "t1", Email: "a@b.co", PasswordHash: "h", Status: "active"})
	u, err := r.FindUserByEmail(context.Background(), "a@b.co")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if u.ID != "u1" || u.TenantID != "t1" || u.PasswordHash != "h" {
		t.Fatalf("row wrong: %+v", u)
	}
}
```

Add the test helpers in the same file, using the in-memory sqlite GORM driver (already an indirect dep via gorm) so the test needs no container:

```go
import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE users (id TEXT, tenant_id TEXT, email TEXT, password_hash TEXT, status TEXT, created_at DATETIME)`).Error; err != nil {
		t.Fatalf("schema: %v", err)
	}
	return New(db)
}

func seedUser(t *testing.T, r *Repo, u domain.User) {
	t.Helper()
	if err := r.db.Exec(`INSERT INTO users (id,tenant_id,email,password_hash,status) VALUES (?,?,?,?,?)`,
		u.ID, u.TenantID, u.Email, u.PasswordHash, u.Status).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}
```

If `gorm.io/driver/sqlite` is not yet a dependency, run `go get gorm.io/driver/sqlite` first.

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./services/auth-svc/internal/adapters/outbound/mysqlrepo/ -v`
Expected: FAIL - package `mysqlrepo` has no `New`.

- [ ] **Step 5: Write the repo**

```go
// services/auth-svc/internal/adapters/outbound/mysqlrepo/repo.go
package mysqlrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Repo implements app.Repo on MySQL/GORM.
type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type userRow struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Status       string
	CreatedAt    time.Time
}

func (userRow) TableName() string { return "users" }

// FindUserByEmail returns domain.ErrUserNotFound when no row matches.
func (r *Repo) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("mysql: find user: %w", err)
	}
	return domain.User{
		ID: row.ID, TenantID: row.TenantID, Email: row.Email,
		PasswordHash: row.PasswordHash, Status: row.Status,
	}, nil
}

var _ app.Repo = (*Repo)(nil)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./services/auth-svc/internal/adapters/outbound/mysqlrepo/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add services/auth-svc/internal/domain services/auth-svc/internal/app/ports.go services/auth-svc/internal/adapters/outbound/mysqlrepo go.mod go.sum
git commit -m "feat(auth-svc): domain, ports and user repo"
```

---

### Task 5: auth-svc Login use case

**Files:**
- Create: `services/auth-svc/internal/app/usecase.go`
- Test: `services/auth-svc/internal/app/usecase_test.go`

**Interfaces:**
- Consumes: `app.Repo`, `app.TokenIssuer`, `app.RateLimiter`, `app.Clock`; `security.VerifyPassword`.
- Produces:
  - `app.LoginResult{ Token string; ExpiresAt time.Time; User domain.User }`
  - `app.New(Deps) *Service` where `Deps{ Repo, Issuer TokenIssuer, Limiter RateLimiter, Clock }`
  - `(*Service).Login(ctx, email, password string) (LoginResult, error)`

- [ ] **Step 1: Write the failing test**

```go
// services/auth-svc/internal/app/usecase_test.go
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

type fakeRepo struct{ u domain.User; err error }

func (f fakeRepo) FindUserByEmail(context.Context, string) (domain.User, error) {
	return f.u, f.err
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(_, _, _ string, now time.Time) (string, time.Time, error) {
	return "tok", now.Add(time.Hour), nil
}

type allowLimiter struct{ ok bool }

func (a allowLimiter) Allow(context.Context, string) (bool, error) { return a.ok, nil }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0) }

func newSvc(repo Repo, allow bool) *Service {
	return New(Deps{Repo: repo, Issuer: fakeIssuer{}, Limiter: allowLimiter{ok: allow}, Clock: fixedClock{}})
}

func activeUser(t *testing.T, pw string) domain.User {
	t.Helper()
	h, _ := security.HashPassword(pw)
	return domain.User{ID: "u1", TenantID: "t1", Email: "a@b.co", PasswordHash: h, Status: "active"}
}

func TestLogin_Success(t *testing.T) {
	svc := newSvc(fakeRepo{u: activeUser(t, "pw")}, true)
	res, err := svc.Login(context.Background(), "a@b.co", "pw")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Token != "tok" || res.User.TenantID != "t1" {
		t.Fatalf("result wrong: %+v", res)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc := newSvc(fakeRepo{u: activeUser(t, "pw")}, true)
	_, err := svc.Login(context.Background(), "a@b.co", "nope")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownEmail_Generic(t *testing.T) {
	svc := newSvc(fakeRepo{err: domain.ErrUserNotFound}, true)
	_, err := svc.Login(context.Background(), "x@y.co", "pw")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown email must map to generic ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_RateLimited(t *testing.T) {
	svc := newSvc(fakeRepo{u: activeUser(t, "pw")}, false)
	_, err := svc.Login(context.Background(), "a@b.co", "pw")
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestLogin_Inactive(t *testing.T) {
	u := activeUser(t, "pw")
	u.Status = "disabled"
	svc := newSvc(fakeRepo{u: u}, true)
	_, err := svc.Login(context.Background(), "a@b.co", "pw")
	if !errors.Is(err, domain.ErrInactive) {
		t.Fatalf("want ErrInactive, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/auth-svc/internal/app/ -v`
Expected: FAIL - `undefined: New`.

- [ ] **Step 3: Write the use case**

```go
// services/auth-svc/internal/app/usecase.go
package app

import (
	"context"
	"time"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Deps are the injected ports.
type Deps struct {
	Repo    Repo
	Issuer  TokenIssuer
	Limiter RateLimiter
	Clock   Clock
}

// Service is the auth application layer.
type Service struct{ d Deps }

func New(d Deps) *Service { return &Service{d: d} }

// LoginResult is what a successful login returns.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      domain.User
}

// Login validates credentials and mints a token. Every credential failure (unknown email,
// wrong password) collapses to ErrInvalidCredentials so the API cannot be used to probe
// which emails exist.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	allowed, err := s.d.Limiter.Allow(ctx, "login:"+email)
	if err != nil {
		return LoginResult{}, err
	}
	if !allowed {
		return LoginResult{}, domain.ErrRateLimited
	}

	u, err := s.d.Repo.FindUserByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if u.Status != "active" {
		return LoginResult{}, domain.ErrInactive
	}
	ok, err := security.VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		return LoginResult{}, domain.ErrInvalidCredentials
	}

	tok, exp, err := s.d.Issuer.Issue(u.ID, u.TenantID, u.Email, s.d.Clock.Now())
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: tok, ExpiresAt: exp, User: u}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/auth-svc/internal/app/ -v`
Expected: PASS (all five).

- [ ] **Step 5: Commit**

```bash
git add services/auth-svc/internal/app/usecase.go services/auth-svc/internal/app/usecase_test.go
git commit -m "feat(auth-svc): login use case with generic-credential errors"
```

---

### Task 6: Redis fixed-window rate limiter for login

**Files:**
- Create: `services/auth-svc/internal/adapters/outbound/ratelimit/limiter.go`
- Test: `services/auth-svc/internal/adapters/outbound/ratelimit/limiter_test.go`

**Interfaces:**
- Consumes: `*redis.Client` (`github.com/redis/go-redis/v9`).
- Produces: `ratelimit.New(rc *redis.Client, max int, window time.Duration) *Limiter` implementing `app.RateLimiter` (`Allow(ctx, key) (bool, error)`).

- [ ] **Step 1: Write the failing test (miniredis)**

```go
// services/auth-svc/internal/adapters/outbound/ratelimit/limiter_test.go
package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAllow_BlocksAfterMax(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := New(rc, 2, time.Minute)

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		ok, err := lim.Allow(ctx, "login:a@b.co")
		if err != nil || !ok {
			t.Fatalf("attempt %d should be allowed: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := lim.Allow(ctx, "login:a@b.co")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("third attempt within window must be blocked")
	}
}
```

If `github.com/alicebob/miniredis/v2` is not present, run `go get github.com/alicebob/miniredis/v2`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/auth-svc/internal/adapters/outbound/ratelimit/ -v`
Expected: FAIL - `undefined: New`.

- [ ] **Step 3: Write the limiter**

```go
// services/auth-svc/internal/adapters/outbound/ratelimit/limiter.go
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter is a fixed-window counter: INCR a per-key counter, set the TTL on first hit,
// and reject once it exceeds max. Simple and good enough for login throttling.
type Limiter struct {
	rc     *redis.Client
	max    int
	window time.Duration
}

func New(rc *redis.Client, max int, window time.Duration) *Limiter {
	return &Limiter{rc: rc, max: max, window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	n, err := l.rc.Incr(ctx, "rl:"+key).Result()
	if err != nil {
		return false, fmt.Errorf("ratelimit: incr: %w", err)
	}
	if n == 1 {
		if err := l.rc.Expire(ctx, "rl:"+key, l.window).Err(); err != nil {
			return false, fmt.Errorf("ratelimit: expire: %w", err)
		}
	}
	return n <= int64(l.max), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/auth-svc/internal/adapters/outbound/ratelimit/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/auth-svc/internal/adapters/outbound/ratelimit go.mod go.sum
git commit -m "feat(auth-svc): redis fixed-window login rate limiter"
```

---

### Task 7: auth-svc HTTP layer (login, me, JWT middleware)

**Files:**
- Create: `services/auth-svc/internal/adapters/inbound/http/router.go`, `handlers.go`, `dto.go`, `middleware.go`, `errors.go`
- Test: `services/auth-svc/internal/adapters/inbound/http/handlers_test.go`

**Interfaces:**
- Consumes: `AuthService` (satisfied by `*app.Service`), `*security.Verifier`.
- Produces: `http.NewRouter(svc AuthService, verifier *security.Verifier) *gin.Engine` serving `POST /auth/login`, `GET /auth/me`.

- [ ] **Step 1: Write DTOs, error mapping, middleware, router, handlers**

```go
// services/auth-svc/internal/adapters/inbound/http/dto.go
package http

import "time"

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type userDTO struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      userDTO   `json:"user"`
}
```

```go
// services/auth-svc/internal/adapters/inbound/http/errors.go
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
	case errors.Is(err, domain.ErrInactive):
		c.JSON(http.StatusForbidden, gin.H{"error": "user inactive"})
	case errors.Is(err, domain.ErrRateLimited):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, try again later"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
```

```go
// services/auth-svc/internal/adapters/inbound/http/middleware.go
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
)

const (
	userCtxKey   = "user_id"
	tenantCtxKey = "tenant_id"
	emailCtxKey  = "email"
)

// jwtAuth verifies a Bearer JWT with the public key and stashes the claims.
func jwtAuth(v *security.Verifier) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := v.Parse(strings.TrimSpace(strings.TrimPrefix(auth, prefix)))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(userCtxKey, claims.UserID)
		c.Set(tenantCtxKey, claims.TenantID)
		c.Set(emailCtxKey, claims.Email)
		c.Next()
	}
}
```

```go
// services/auth-svc/internal/adapters/inbound/http/handlers.go
package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
)

// AuthService is the inbound port this adapter needs.
type AuthService interface {
	Login(ctx context.Context, email, password string) (app.LoginResult, error)
}

type Handlers struct{ svc AuthService }

func (h *Handlers) Login(c *gin.Context) {
	var body loginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.Login(c.Request.Context(), body.Email, body.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		Token:     res.Token,
		ExpiresAt: res.ExpiresAt,
		User:      userDTO{ID: res.User.ID, Email: res.User.Email, TenantID: res.User.TenantID},
	})
}

// Me echoes the authenticated identity from the verified JWT (no DB hit needed).
func (h *Handlers) Me(c *gin.Context) {
	c.JSON(http.StatusOK, userDTO{
		ID:       c.GetString(userCtxKey),
		Email:    c.GetString(emailCtxKey),
		TenantID: c.GetString(tenantCtxKey),
	})
}
```

```go
// services/auth-svc/internal/adapters/inbound/http/router.go
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
)

func NewRouter(svc AuthService, verifier *security.Verifier) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := &Handlers{svc: svc}

	auth := r.Group("/auth")
	auth.POST("/login", h.Login)
	auth.GET("/me", jwtAuth(verifier), h.Me)
	return r
}
```

- [ ] **Step 2: Write the failing handler test**

```go
// services/auth-svc/internal/adapters/inbound/http/handlers_test.go
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

type fakeSvc struct {
	res app.LoginResult
	err error
}

func (f fakeSvc) Login(context.Context, string, string) (app.LoginResult, error) {
	return f.res, f.err
}

func TestLogin_OK(t *testing.T) {
	svc := fakeSvc{res: app.LoginResult{
		Token: "tok", ExpiresAt: time.Unix(3600, 0),
		User: domain.User{ID: "u1", Email: "a@b.co", TenantID: "t1"},
	}}
	// verifier can be nil here: /auth/login does not use it.
	r := NewRouter(svc, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"a@b.co","password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got loginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Token != "tok" || got.User.TenantID != "t1" {
		t.Fatalf("body wrong: %+v", got)
	}
}

func TestLogin_BadCredentials401(t *testing.T) {
	r := NewRouter(fakeSvc{err: domain.ErrInvalidCredentials}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"a@b.co","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails, then passes**

Run: `go test ./services/auth-svc/internal/adapters/inbound/http/ -v`
Expected: first FAIL (no package), then PASS after Step 1 files are in place. (If you wrote Step 1 before Step 2, it should PASS now.)

- [ ] **Step 4: Commit**

```bash
git add services/auth-svc/internal/adapters/inbound/http
git commit -m "feat(auth-svc): http login and me endpoints"
```

---

### Task 8: auth-svc composition root + Dockerfile

**Files:**
- Create: `services/auth-svc/main.go`
- Create: `services/auth-svc/Dockerfile`

**Interfaces:**
- Consumes: everything above. Reads env `MYSQL_DSN`, `REDIS_URL`, `AUTH_JWT_PRIVATE_KEY`, `AUTH_JWT_PUBLIC_KEY`, `AUTH_TOKEN_TTL` (default `1h`), `HTTP_ADDR` (default `:8889`), `LOGIN_RATE_MAX` (default 10), `LOGIN_RATE_WINDOW` (default `15m`).

- [ ] **Step 1: Write main.go**

```go
// services/auth-svc/main.go
//
// Command auth-svc is the identity service: it authenticates dashboard users
// (email+password) and issues short-lived EdDSA JWTs. Composition root only.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	redisplatform "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/pkg/security"
	authhttp "github.com/duykhanh/worklane/services/auth-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/auth-svc/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/auth-svc/internal/adapters/outbound/ratelimit"
	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
	redisURL := config.Env("REDIS_URL", "redis://localhost:6379/0")
	priv := config.Env("AUTH_JWT_PRIVATE_KEY", "")
	pub := config.Env("AUTH_JWT_PUBLIC_KEY", "")
	ttl := config.EnvDuration("AUTH_TOKEN_TTL", time.Hour)
	httpAddr := config.Env("HTTP_ADDR", ":8889")

	if priv == "" || pub == "" {
		log.Fatal("auth-svc: AUTH_JWT_PRIVATE_KEY and AUTH_JWT_PUBLIC_KEY are required")
	}
	issuer, err := security.NewIssuer(priv, ttl)
	if err != nil {
		log.Fatalf("auth-svc: issuer: %v", err)
	}
	verifier, err := security.NewVerifier(pub)
	if err != nil {
		log.Fatalf("auth-svc: verifier: %v", err)
	}

	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("auth-svc: mysql: %v", err)
	}
	rc, err := redisplatform.Open(redisURL)
	if err != nil {
		log.Fatalf("auth-svc: redis: %v", err)
	}

	limiter := ratelimit.New(rc, config.EnvInt("LOGIN_RATE_MAX", 10), config.EnvDuration("LOGIN_RATE_WINDOW", 15*time.Minute))
	svc := app.New(app.Deps{Repo: mysqlrepo.New(db), Issuer: issuer, Limiter: limiter, Clock: realClock{}})

	srv := &http.Server{Addr: httpAddr, Handler: authhttp.NewRouter(svc, verifier), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("auth-svc: listening on %s", httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("auth-svc: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("auth-svc: shutdown: %v", err)
	}
	log.Println("auth-svc: stopped")
}
```

Note: `redisplatform.Open` must exist (otp-api uses it). Reuse the same signature. If it does not, mirror `pkg/platform/redis` usage from otp-api's main.go.

- [ ] **Step 2: Write the Dockerfile (copy otp-dispatcher's, change the target)**

Read `services/otp-dispatcher/Dockerfile` and create `services/auth-svc/Dockerfile` identical except the build target path `./services/auth-svc`. (Keep the same base images and build flags so the stack stays consistent.)

- [ ] **Step 3: Build to verify it compiles**

Run: `go build ./services/auth-svc/...`
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add services/auth-svc/main.go services/auth-svc/Dockerfile
git commit -m "feat(auth-svc): composition root and Dockerfile"
```

---

### Task 9: auth-svc architecture test

**Files:**
- Create: `services/auth-svc/internal/arch/arch_test.go`

- [ ] **Step 1: Write the arch test (copy otp-api's, same forbidden list)**

```go
// services/auth-svc/internal/arch/arch_test.go
package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainAndAppAreInfraFree(t *testing.T) {
	forbidden := []string{"redis", "gorm", "sarama", "gin", "/adapters/", "/pkg/platform/"}
	layers := []string{"../domain", "../app"}

	fset := token.NewFileSet()
	for _, dir := range layers {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for _, imp := range f.Imports {
				for _, bad := range forbidden {
					if strings.Contains(imp.Path.Value, bad) {
						t.Errorf("%s imports forbidden %s", name, imp.Path.Value)
					}
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./services/auth-svc/... -v`
Expected: PASS across all auth-svc packages.

- [ ] **Step 3: Commit**

```bash
git add services/auth-svc/internal/arch/arch_test.go
git commit -m "test(auth-svc): enforce hexagon boundary"
```

---

### Task 10: otp-api `authenticate` middleware accepts JWT or API key

**Files:**
- Modify: `services/otp-api/internal/adapters/inbound/http/middleware.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/router.go`
- Test: `services/otp-api/internal/adapters/inbound/http/middleware_test.go` (create)

**Interfaces:**
- Consumes: `*security.Verifier`, existing `app.Repo` (`FindAPIKey`).
- Produces: `authenticate(v *security.Verifier, repo app.Repo) gin.HandlerFunc`; `NewRouter(svc OTPService, repo app.Repo, verifier *security.Verifier) *gin.Engine`.

- [ ] **Step 1: Write the failing test**

```go
// services/otp-api/internal/adapters/inbound/http/middleware_test.go
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// stubRepo must satisfy the FULL app.Repo interface (open
// services/otp-api/internal/app/ports.go and implement exactly its method set; stub
// unused methods to return zero values). Only FindAPIKey is exercised here.
type stubRepo struct{ ak app.APIKey; err error }

func (s stubRepo) FindAPIKey(context.Context, string) (app.APIKey, error) { return s.ak, s.err }

// The remaining app.Repo methods are unused here; embed to satisfy the interface.
func (stubRepo) InsertRequest(context.Context, app.Request) error       { return nil }
func (stubRepo) UpdateState(context.Context, string, string) error      { return nil }
func (stubRepo) ListRequests(context.Context, string, int) ([]app.Request, error) { return nil, nil }
func (stubRepo) ListDeliveryLogs(context.Context, string, int) ([]app.DeliveryLog, error) { return nil, nil }
func (stubRepo) ListAPIKeys(context.Context, string) ([]app.APIKey, error) { return nil, nil }

func testRouter(v *security.Verifier, repo app.Repo) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/probe", authenticate(v, repo), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(tenantCtxKey))
	})
	return r
}

func TestAuthenticate_ValidJWT(t *testing.T) {
	priv, pub := security.TestKeyPair(t) // helper added below
	iss, _ := security.NewIssuer(priv, time.Hour)
	ver, _ := security.NewVerifier(pub)
	tok, _, _ := iss.Issue("u1", "tenant-jwt", "a@b.co", time.Now())

	r := testRouter(ver, stubRepo{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "tenant-jwt" {
		t.Fatalf("jwt path: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticate_ValidAPIKey(t *testing.T) {
	_, pub := security.TestKeyPair(t)
	ver, _ := security.NewVerifier(pub)
	repo := stubRepo{ak: app.APIKey{TenantID: "tenant-key", Status: "active"}}

	r := testRouter(ver, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer opaque-key-no-dots")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "tenant-key" {
		t.Fatalf("api-key path: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticate_Garbage401(t *testing.T) {
	_, pub := security.TestKeyPair(t)
	ver, _ := security.NewVerifier(pub)
	r := testRouter(ver, stubRepo{err: context.DeadlineExceeded})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer nonsense")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}
```

Also create a shared test-key helper in `pkg/security` so cross-package tests can build a keypair. Create `pkg/security/testkeys.go`:

```go
// pkg/security/testkeys.go
package security

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// TestKeyPair returns a fresh Ed25519 (privPEM, pubPEM) for tests in other packages.
func TestKeyPair(t *testing.T) (string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(priv)
	pkix, _ := x509.MarshalPKIXPublicKey(pub)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))
}
```

Note: importing `testing` in a non-`_test.go` file is acceptable for a dedicated test-helper file; keep it in `pkg/security` so cross-package tests can use it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/adapters/inbound/http/ -run TestAuthenticate -v`
Expected: FAIL - `undefined: authenticate`.

- [ ] **Step 3: Rewrite the middleware**

Replace the body of `services/otp-api/internal/adapters/inbound/http/middleware.go` with:

```go
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

const tenantCtxKey = "tenant_id"

// authenticate resolves a Bearer token that is either a user JWT (dashboard) or a tenant
// API key (machine). A JWT has three dot-separated parts; anything else is treated as an
// opaque API key and looked up by hash. Both paths set tenant_id for downstream handlers.
func authenticate(v *security.Verifier, repo app.Repo) gin.HandlerFunc {
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

		ak, err := repo.FindAPIKey(c.Request.Context(), security.HashKey(token))
		if err != nil || ak.Status != "active" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set(tenantCtxKey, ak.TenantID)
		c.Next()
	}
}
```

- [ ] **Step 4: Thread the verifier through the router**

In `services/otp-api/internal/adapters/inbound/http/router.go`, change the signature and the middleware call:

```go
func NewRouter(svc OTPService, repo app.Repo, verifier *security.Verifier) *gin.Engine {
	// ...unchanged setup...
	v1 := r.Group("/v1")
	v1.Use(authenticate(verifier, repo))
	// ...unchanged routes...
}
```
Add `"github.com/duykhanh/worklane/pkg/security"` to the router imports.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/otp-api/internal/adapters/inbound/http/ -v`
Expected: PASS (new middleware tests + existing handler tests). Fix any existing `NewRouter(...)` callers in this package's tests to pass a verifier built from `security.TestKeyPair`.

- [ ] **Step 6: Commit**

```bash
git add services/otp-api/internal/adapters/inbound/http pkg/security/testkeys.go
git commit -m "feat(otp-api): accept user JWT or API key in one auth middleware"
```

---

### Task 11: otp-api composition root wires the verifier

**Files:**
- Modify: `services/otp-api/main.go`

- [ ] **Step 1: Build the verifier and pass it to NewRouter**

In `services/otp-api/main.go`, after building `repo`/`store`, add:

```go
	pub := config.Env("AUTH_JWT_PUBLIC_KEY", "")
	if pub == "" {
		log.Fatal("otp-api: AUTH_JWT_PUBLIC_KEY is required")
	}
	verifier, err := security.NewVerifier(pub)
	if err != nil {
		log.Fatalf("otp-api: verifier: %v", err)
	}
```

Change the server handler line to:

```go
		Handler:           otphttp.NewRouter(svc, repo, verifier),
```

Add `"github.com/duykhanh/worklane/pkg/security"` to imports.

- [ ] **Step 2: Build to verify it compiles**

Run: `go build ./services/otp-api/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add services/otp-api/main.go
git commit -m "feat(otp-api): load JWT public key at startup"
```

---

### Task 12: Compose + Traefik wiring for auth-svc

**Files:**
- Modify: `deploy/compose/docker-compose.yml`
- Modify: `deploy/traefik/dynamic.yml`
- Modify: `deploy/compose/.env` (dev keys; gitignored) and `.env.example`

**Interfaces:**
- Produces: `auth-svc` reachable at `http://localhost/auth/*` via Traefik; otp-api + auth-svc share the Ed25519 keypair via env.

- [ ] **Step 1: Generate a dev Ed25519 keypair**

Run:
```bash
openssl genpkey -algorithm ed25519 -out /tmp/auth_priv.pem
openssl pkey -in /tmp/auth_priv.pem -pubout -out /tmp/auth_pub.pem
```
The app reads the key **content** (not a path) from `AUTH_JWT_PRIVATE_KEY` /
`AUTH_JWT_PUBLIC_KEY`. PEM is multi-line, and the cleanest way to carry a multi-line value
into a container is a **YAML block scalar** directly in `docker-compose.yml` (the compose
`.env` file does not preserve newlines). So the PEM bodies are pasted inline in the compose
`environment:` blocks in Step 2 - not stored in `deploy/compose/.env`.

Print the two PEMs to copy their exact contents into Step 2:
```bash
cat /tmp/auth_priv.pem   # -> AUTH_JWT_PRIVATE_KEY (auth-svc only)
cat /tmp/auth_pub.pem    # -> AUTH_JWT_PUBLIC_KEY (auth-svc AND otp-api)
```
These are **dev-only** keys; production supplies real keys via its own secret manager.

- [ ] **Step 2: Add the auth-svc service to docker-compose.yml**

```yaml
  auth-svc:
    build:
      context: ../..
      dockerfile: services/auth-svc/Dockerfile
    environment:
      MYSQL_DSN: "root:secret@tcp(mysql:3306)/otp?parseTime=true&multiStatements=true"
      REDIS_URL: "redis://redis:6379/0"
      HTTP_ADDR: ":8889"
      AUTH_TOKEN_TTL: "1h"
      AUTH_JWT_PRIVATE_KEY: |
        -----BEGIN PRIVATE KEY-----
        <priv PEM body>
        -----END PRIVATE KEY-----
      AUTH_JWT_PUBLIC_KEY: |
        -----BEGIN PUBLIC KEY-----
        <pub PEM body>
        -----END PUBLIC KEY-----
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
      otp-api: { condition: service_started } # otp-api owns migrations incl. users table
```

Add the public key to the existing `otp-api` service environment:

```yaml
      AUTH_JWT_PUBLIC_KEY: |
        -----BEGIN PUBLIC KEY-----
        <pub PEM body>
        -----END PUBLIC KEY-----
```

- [ ] **Step 3: Route /auth through Traefik**

In `deploy/traefik/dynamic.yml`, add a router + service:

```yaml
  routers:
    authsvc:
      rule: "PathPrefix(`/auth`)"
      entryPoints: [web]
      service: authsvc
      middlewares: [otp-cors]
    # ...existing otpapi router unchanged...
  services:
    authsvc:
      loadBalancer:
        servers:
          - url: "http://auth-svc:8889"
    # ...existing otpapi service unchanged...
```

- [ ] **Step 4: Bring the stack up and smoke-test login over HTTP**

Run:
```bash
cd deploy/compose && docker compose up -d --build auth-svc otp-api
# seed a user (Task 3) if not already present, then:
curl -s -X POST http://localhost/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"you@demo.co","password":"sup3rsecret"}'
```
Expected: `200` with `{"token":"...","expires_at":"...","user":{...}}`.

- [ ] **Step 5: Verify the JWT authorizes /v1**

Run:
```bash
TOKEN=$(curl -s -X POST http://localhost/auth/login -H 'Content-Type: application/json' -d '{"email":"you@demo.co","password":"sup3rsecret"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -s http://localhost/v1/otp/requests -H "Authorization: Bearer $TOKEN"
```
Expected: `200` with the tenant's requests (JSON array), proving otp-api verified the JWT locally.

- [ ] **Step 6: Commit**

```bash
git add deploy/compose/docker-compose.yml deploy/traefik/dynamic.yml deploy/compose/.env.example
git commit -m "feat(deploy): run auth-svc behind Traefik, share JWT keypair"
```
(Do not commit `deploy/compose/.env` - it is gitignored.)

---

### Task 13: Dashboard auth client

**Files:**
- Create: `dashboard/lib/api/auth.ts`
- Test: `dashboard/lib/api/auth.test.ts`

**Interfaces:**
- Produces: `login(email, password): Promise<{token: string; expiresAt: string; user: {id: string; email: string; tenantId: string}}>` and `me(token): Promise<{id: string; email: string; tenantId: string}>`, both hitting `${NEXT_PUBLIC_API_BASE}/auth/*`.

- [ ] **Step 1: Write the failing test (mock fetch)**

```ts
// dashboard/lib/api/auth.test.ts
import { describe, it, expect, vi } from "vitest";
import { login } from "./auth";

describe("auth client", () => {
  it("posts credentials and returns the token", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        token: "tok",
        expires_at: "2026-01-01T00:00:00Z",
        user: { id: "u1", email: "a@b.co", tenant_id: "t1" },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const res = await login("a@b.co", "pw");
    expect(res.token).toBe("tok");
    expect(res.user.tenantId).toBe("t1");
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/auth/login"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("throws on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 401 }));
    await expect(login("a@b.co", "x")).rejects.toThrow();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run lib/api/auth.test.ts`
Expected: FAIL - cannot find `./auth`.

- [ ] **Step 3: Write the client**

```ts
// dashboard/lib/api/auth.ts
const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost";

export type AuthUser = { id: string; email: string; tenantId: string };
export type LoginResult = { token: string; expiresAt: string; user: AuthUser };

export async function login(email: string, password: string): Promise<LoginResult> {
  const res = await fetch(`${BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw new Error(res.status === 401 ? "Invalid email or password" : `Login failed (${res.status})`);
  }
  const body = await res.json();
  return {
    token: body.token,
    expiresAt: body.expires_at,
    user: { id: body.user.id, email: body.user.email, tenantId: body.user.tenant_id },
  };
}

export async function me(token: string): Promise<AuthUser> {
  const res = await fetch(`${BASE}/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`me failed (${res.status})`);
  const u = await res.json();
  return { id: u.id, email: u.email, tenantId: u.tenant_id };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run lib/api/auth.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add dashboard/lib/api/auth.ts dashboard/lib/api/auth.test.ts
git commit -m "feat(dashboard): auth API client (login, me)"
```

---

### Task 14: Dashboard real login view

**Files:**
- Modify: `dashboard/components/login/login-view.tsx`
- Modify: `dashboard/components/login/login-view.test.tsx`

**Interfaces:**
- Consumes: `login()` from Task 13, `useUIStore().setToken`, the `worklane-token` localStorage key (Task 15 defines the constant; use the literal string `"worklane-token"` here).

- [ ] **Step 1: Rewrite the login view as a working email+password form**

```tsx
// dashboard/components/login/login-view.tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { LogIn } from "lucide-react";
import { login } from "@/lib/api/auth";
import { useUIStore } from "@/lib/store/ui";
import { Panel } from "@/components/common/panel";
import { LogoMark } from "@/components/shell/logo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LoginView() {
  const router = useRouter();
  const setToken = useUIStore((s) => s.setToken);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const res = await login(email, password);
      setToken(res.token);
      localStorage.setItem("worklane-token", res.token);
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-background p-6">
      <div className="grid w-[380px] gap-5">
        <span className="flex items-center justify-center gap-2">
          <LogoMark className="size-6" />
          <span className="text-[15px] font-semibold tracking-tight">worklane</span>
        </span>
        <Panel title="Sign in" description="Use your worklane operator account.">
          <form onSubmit={onSubmit} className="grid gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="login-email">Email</Label>
              <Input id="login-email" type="email" autoComplete="username"
                value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@company.com" />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="login-password">Password</Label>
              <Input id="login-password" type="password" autoComplete="current-password"
                value={password} onChange={(e) => setPassword(e.target.value)} />
            </div>
            {error && <p role="alert" className="m-0 text-xs text-[var(--state-failed)]">{error}</p>}
            <Button type="submit" className="w-full" disabled={busy}>
              <LogIn className="size-4" />
              {busy ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </Panel>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Update the test to assert real login behavior**

```tsx
// dashboard/components/login/login-view.test.tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { LoginView } from "./login-view";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/lib/api/auth", () => ({
  login: vi.fn().mockResolvedValue({ token: "tok", expiresAt: "", user: { id: "u1", email: "a@b.co", tenantId: "t1" } }),
}));

describe("LoginView", () => {
  beforeEach(() => { push.mockClear(); localStorage.clear(); });

  it("logs in, stores the token, and redirects", async () => {
    render(<LoginView />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "a@b.co" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "pw" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/"));
    expect(localStorage.getItem("worklane-token")).toBe("tok");
  });
});
```

- [ ] **Step 3: Run the test**

Run: `cd dashboard && npx vitest run components/login/login-view.test.tsx`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add dashboard/components/login/login-view.tsx dashboard/components/login/login-view.test.tsx
git commit -m "feat(dashboard): working email+password login view"
```

---

### Task 15: Token store rename, auth guard, drop paste-key field, real sidebar identity

**Files:**
- Modify: `dashboard/components/shell/api-key-field.tsx` (delete) and its usage in the shell header
- Modify: `dashboard/app/(app)/layout.tsx` (add guard)
- Modify: the sidebar footer component that renders "Acme Inc / tnt_a1b2c3d4"
- Modify: the TanStack Query provider to redirect to `/login` on 401
- Test: `dashboard/components/shell/auth-guard.test.tsx` (create)

**Interfaces:**
- Consumes: `useUIStore().token/setToken`, `me()` from Task 13.
- Produces: `AuthGuard` component that redirects to `/login` when no token; a query error handler that clears the token and redirects on 401.

- [ ] **Step 1: Remove the paste-key field**

Delete `dashboard/components/shell/api-key-field.tsx` and remove its `<ApiKeyField />` usage + import from the shell header component (search: `grep -rn ApiKeyField dashboard/components dashboard/app`).

- [ ] **Step 2: Add an AuthGuard and mount it in the app layout**

```tsx
// dashboard/components/shell/auth-guard.tsx
"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useUIStore } from "@/lib/store/ui";

const STORAGE_KEY = "worklane-token";

/** Redirects to /login when there is no token; rehydrates the store from localStorage. */
export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const token = useUIStore((s) => s.token);
  const setToken = useUIStore((s) => s.setToken);

  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && !token) setToken(saved);
    if (!saved && !token) router.replace("/login");
  }, [token, setToken, router]);

  if (!token) return null;
  return <>{children}</>;
}
```

Wrap the app-group layout's children with `<AuthGuard>` in `dashboard/app/(app)/layout.tsx`.

- [ ] **Step 3: Redirect to /login on any API 401**

In the providers file that creates the TanStack `QueryClient`, attach a `QueryCache` error handler:

```tsx
import { QueryCache, QueryClient } from "@tanstack/react-query";

const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error) => {
      if (error instanceof Error && /\b401\b/.test(error.message)) {
        localStorage.removeItem("worklane-token");
        useUIStore.getState().setToken("");
        if (typeof window !== "undefined") window.location.assign("/login");
      }
    },
  }),
});
```
(The live data source already throws `GET ... failed: 401`, so matching `401` in the message is sufficient.)

- [ ] **Step 4: Show the real signed-in identity in the sidebar**

In the sidebar footer component, replace the hardcoded org/tenant with a `me()` lookup:

```tsx
"use client";
import { useQuery } from "@tanstack/react-query";
import { me } from "@/lib/api/auth";
import { useUIStore } from "@/lib/store/ui";

export function SidebarIdentity() {
  const token = useUIStore((s) => s.token);
  const { data } = useQuery({
    queryKey: ["me", token],
    queryFn: () => me(token),
    enabled: !!token,
  });
  return (
    <div className="grid">
      <span className="text-sm font-medium">{data?.email ?? "..."}</span>
      <span className="text-xs text-muted-foreground">{data?.tenantId ?? ""}</span>
    </div>
  );
}
```
Wire this component where the old "Acme Inc / tnt_a1b2c3d4" markup lived.

- [ ] **Step 5: Write the guard test**

```tsx
// dashboard/components/shell/auth-guard.test.tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { AuthGuard } from "./auth-guard";
import { useUIStore } from "@/lib/store/ui";

const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));

describe("AuthGuard", () => {
  beforeEach(() => { replace.mockClear(); localStorage.clear(); useUIStore.getState().setToken(""); });

  it("redirects to /login when there is no token", () => {
    render(<AuthGuard><div>secret</div></AuthGuard>);
    expect(replace).toHaveBeenCalledWith("/login");
  });

  it("renders children when a token is present", () => {
    useUIStore.getState().setToken("tok");
    const { getByText } = render(<AuthGuard><div>secret</div></AuthGuard>);
    expect(getByText("secret")).toBeTruthy();
  });
});
```

- [ ] **Step 6: Run dashboard tests + typecheck**

Run: `cd dashboard && npx vitest run && npx tsc --noEmit`
Expected: PASS, no type errors.

- [ ] **Step 7: Commit**

```bash
git add dashboard/components dashboard/app
git commit -m "feat(dashboard): auth guard, real identity, drop paste-key field"
```

---

### Task 16: End-to-end verification in the compose stack

**Files:** none (verification only).

- [ ] **Step 1: Bring up the full stack**

Run: `cd deploy/compose && docker compose up -d --build`
Expected: all services healthy incl. `auth-svc`.

- [ ] **Step 2: Provision a user (if not already)**

Run:
```bash
MYSQL_DSN='root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true' \
  go run ./services/seed --name demo --email you@demo.co --password 'sup3rsecret'
```

- [ ] **Step 3: Log in through the dashboard UI**

Open `http://localhost:3000` -> redirected to `/login` -> sign in with `you@demo.co` / `sup3rsecret`.
Expected: redirect to the dashboard; **OTP requests** and **Delivery logs** load; the sidebar shows `you@demo.co`; there is **no** "Paste API key" field in the header.

- [ ] **Step 4: Confirm the machine path still works (M1 keeps API keys)**

Run (using an API key printed by seed):
```bash
curl -s -X POST http://localhost/v1/otp/send -H "Authorization: Bearer <API_KEY>" \
  -H 'Content-Type: application/json' -d '{"recipient":"you@demo.co","channel":"email"}'
```
Expected: `202` with a `request_id` - the API-key path is intact.

- [ ] **Step 5: Full test suite green**

Run: `go test -race ./... && cd dashboard && npx vitest run && npx tsc --noEmit`
Expected: all pass.

- [ ] **Step 6: Commit any final fixups**

```bash
git commit -am "test: M1 auth end-to-end verified" --allow-empty
```

---

## Out of scope for this plan (deferred to M2)

Moving `api_keys`/`tenants` into a separate `identity` database; otp-api validating API keys via `POST /internal/introspect` instead of a local lookup; Redis-cached introspection; removing otp-api's identity tables; API-key management endpoints (`/auth/api-keys` create/list/revoke) and the dashboard API-keys page rewiring. Password reset, roles, invites, refresh tokens remain out of scope entirely (see spec section 12).
