# OTP Product MVP Implementation Plan

> **STATUS: ACTIVE (rewritten 2026-08-15).** This plan targets the current stack: a **monorepo of Gin
> microservices**, each internally **hexagonal (ports & adapters)**, with **MySQL via GORM**,
> **Redis**, **Kafka via IBM/sarama**, **Traefik** gateway, and **Kustomize** deploy. It supersedes the
> earlier go-zero + Postgres + APISIX draft. Repository structure and the production→this-project
> concept mapping live in [../../reference/architecture-and-layout.md](../../reference/architecture-and-layout.md).

> **For agentic workers:** REQUIRED SUB-SKILL: use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Build the walking skeleton from [../specs/2026-08-07-otp-mvp-scope.md](../specs/2026-08-07-otp-mvp-scope.md) -
one email OTP that flows `send → real email → verify` through **Traefik → otp-api → Kafka →
otp-dispatcher → Resend**, proven end-to-end on docker-compose, then deployed to k3s.

**Architecture:** Two independently deployable microservices in one Go module. `otp-api` (Gin, sync
HTTP) owns OTP generation/verification and publishes `otp.requested`; `otp-dispatcher` (sarama consumer,
async) sends the email. Each service is a hexagon: a **pure domain** + an **application layer** of use
cases over **ports**, with **adapters** (Gin, GORM, Redis, sarama, Resend) implementing those ports.
The domain imports no infrastructure and is 100% unit-testable with fakes. The two services share only
`pkg/platform/*` (infra libraries) and `pkg/contracts/otp` (the Kafka event schema) - never each
other's `internal/`.

**Tech Stack:** Go 1.22+, Gin (HTTP), IBM/sarama (Kafka), `redis/go-redis/v9` (Redis), GORM +
`golang-migrate` (MySQL), Resend (email), Traefik (gateway), docker-compose, testcontainers-go
(integration), Redpanda (local Kafka).

## Global Constraints

- Module path: `github.com/duykhanh/worklane`.
- **Domain purity:** `services/*/internal/domain` MUST NOT import any adapter, driver, framework, or
  `pkg/platform`. `internal/app` may import only `domain`, its own `ports.go`, and `pkg/contracts`.
  Enforced by an architecture test (Task 8).
- **Service isolation:** no service may import another service's `internal/*` (Go enforces this via the
  `internal/` directory rule). Cross-service communication is Kafka only.
- OTP plaintext code is **never** persisted or logged - only a salted **hash** is stored (Redis) with a
  TTL. Default length **6**, default TTL **5 minutes**.
- Verification uses **constant-time** comparison.
- Recipient PII (email) is **masked** in any log or audit output (`d***@gmail.com`).
- Every domain/app task is TDD: failing test first, minimal code, green, commit. Target **80%** coverage
  on `otp-api/internal/domain` + `internal/app`.
- Immutability: constructors return new values; no in-place mutation of shared structs.

---

## File Structure (target)

```
go.mod                                   # module github.com/duykhanh/worklane
Makefile
pkg/
  contracts/otp/event.go                 # RequestedEvent, SentEvent... + state string consts (shared kernel)
  platform/config/                       # typed config loader
  platform/httpserver/                   # gin engine bootstrap + middleware + error->HTTP mapping
  platform/kafka/                        # sarama producer/consumer wrapper + typed envelope
  platform/redis/                        # go-redis client + typed key builders
  platform/mysql/                        # gorm DB open + migrate runner
  platform/logger/                       # JSON logger
services/
  otp-api/
    main.go                              # composition root (Gin)
    internal/domain/                     # code, hash, state, request, errors (PURE)
    internal/app/                        # ports.go, ratelimit.go, send.go, verify.go, config.go
    internal/adapters/inbound/http/      # gin handlers, DTOs, apikey middleware
    internal/adapters/outbound/redisstore/
    internal/adapters/outbound/mysqlrepo/
    internal/adapters/outbound/kafkabus/ # publisher
  otp-dispatcher/
    main.go                              # composition root (sarama consumer)
    internal/app/                        # ports.go, handler.go, template.go
    internal/adapters/inbound/kafka/     # consumer
    internal/adapters/outbound/resendmail/
    internal/adapters/outbound/mysqlrepo/
    internal/adapters/outbound/kafkabus/ # publisher (sent/failed/dlq)
  seed/main.go                           # CLI: tenant + API key
db/otp/migrations/                       # golang-migrate .up.sql/.down.sql
deploy/
  compose/docker-compose.yml
  traefik/traefik.yml, dynamic.yml
  kustomize/base/{otp-api,otp-dispatcher}, kustomize/overlays/develop
dashboard/                               # Next.js (Phase 5)
```

---

## Phase 1 - otp-api core: domain + application (pure, TDD)

All Phase 1 code lives under `services/otp-api/internal/`. It has zero infrastructure imports.

### Task 1: Monorepo scaffolding + shared contract

**Files:** `go.mod`, `Makefile`, `pkg/contracts/otp/event.go`, `services/otp-api/internal/domain/errors.go`

- [ ] **Step 1: Init module**

```bash
go mod init github.com/duykhanh/worklane
go mod tidy
```

- [ ] **Step 2: Makefile**

```makefile
.PHONY: test cover
test:
	go test ./...
cover:
	go test -coverprofile=cover.out ./services/otp-api/internal/... && go tool cover -func=cover.out | tail -1
```

- [ ] **Step 3: Shared event contract + state vocabulary** (used by both services)

```go
// pkg/contracts/otp/event.go
package otp

// Topic-independent event payloads exchanged over Kafka. Both otp-api (producer)
// and otp-dispatcher (consumer) marshal/unmarshal these exact shapes.
type RequestedEvent struct {
	RequestID string `json:"request_id"`
	TenantID  string `json:"tenant_id"`
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Code      string `json:"code"` // never logged
}

// State strings are the shared persistence/wire vocabulary for otp_requests.state.
const (
	StateRequested = "requested"
	StateSent      = "sent"
	StateFailed    = "failed"
	StateVerified  = "verified"
	StateExpired   = "expired"
)
```

- [ ] **Step 4: Domain error sentinels**

```go
// services/otp-api/internal/domain/errors.go
package domain

import "errors"

var (
	ErrRateLimited     = errors.New("otp: rate limited")
	ErrCooldown        = errors.New("otp: resend cooldown active")
	ErrTooManyAttempts = errors.New("otp: too many verify attempts")
	ErrNotFound        = errors.New("otp: no active code")
	ErrCodeMismatch    = errors.New("otp: code mismatch")
	ErrExpired         = errors.New("otp: code expired")
)
```

- [ ] **Step 5: Commit** `chore: scaffold monorepo, shared otp contract, domain errors`

### Task 2: Crypto-random code generation (domain)

**Files:** `services/otp-api/internal/domain/code.go` + `code_test.go`

- [ ] **Step 1: Failing test**

```go
// code_test.go
package domain

import (
	"strconv"
	"testing"
)

func TestGenerateCode_LengthAndNumeric(t *testing.T) {
	for _, n := range []int{4, 6, 8} {
		code, err := GenerateCode(n)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != n {
			t.Fatalf("want length %d, got %d (%q)", n, len(code), code)
		}
		if _, err := strconv.Atoi(code); err != nil {
			t.Fatalf("code not numeric: %q", code)
		}
	}
}

func TestGenerateCode_Distribution(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 1000; i++ {
		c, _ := GenerateCode(6)
		seen[c]++
	}
	if len(seen) < 990 {
		t.Fatalf("suspicious distribution, unique=%d", len(seen))
	}
}
```

- [ ] **Step 2:** `go test ./services/otp-api/internal/domain/ -run TestGenerateCode -v` → FAIL.
- [ ] **Step 3: Implement**

```go
// code.go
package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateCode returns a numeric OTP of exactly length digits (zero-padded), drawn
// from crypto/rand. length must be between 4 and 10. Using crypto/rand (not math/rand)
// is essential: OTPs are a security primitive and must be unpredictable.
func GenerateCode(length int) (string, error) {
	if length < 4 || length > 10 {
		return "", fmt.Errorf("otp: invalid code length %d", length)
	}
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("otp: rand: %w", err)
	}
	return fmt.Sprintf("%0*d", length, n), nil
}
```

- [ ] **Step 4:** rerun → PASS.
- [ ] **Step 5: Commit** `feat: crypto-random OTP code generation`

### Task 3: Hashing + constant-time verification (domain)

**Files:** `domain/hash.go` + `hash_test.go`

- [ ] **Step 1: Failing test**

```go
// hash_test.go
package domain

import "testing"

func TestHashCode_Deterministic(t *testing.T) {
	h1 := HashCode("123456", "salt-a")
	h2 := HashCode("123456", "salt-a")
	if h1 != h2 {
		t.Fatal("hash must be deterministic for same salt+code")
	}
	if HashCode("123456", "salt-b") == h1 {
		t.Fatal("different salt must change hash")
	}
}

func TestVerifyHash(t *testing.T) {
	h := HashCode("654321", "s")
	if !VerifyHash(h, "654321", "s") {
		t.Fatal("correct code must verify")
	}
	if VerifyHash(h, "000000", "s") {
		t.Fatal("wrong code must not verify")
	}
}
```

- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: Implement**

```go
// hash.go
package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// HashCode returns hex SHA-256 of salt+code. A per-request random salt means two
// requests for the same code produce different hashes (defends against precomputation).
func HashCode(code, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + code))
	return hex.EncodeToString(sum[:])
}

// VerifyHash compares in constant time. subtle.ConstantTimeCompare avoids leaking how
// many leading characters matched via timing - the classic OTP/token side channel.
func VerifyHash(hash, code, salt string) bool {
	got := HashCode(code, salt)
	return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1
}
```

- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: salted hashing with constant-time verification`

### Task 4: State model (domain)

**Files:** `domain/state.go` + `state_test.go`

- [ ] **Step 1: Failing test**

```go
// state_test.go
package domain

import "testing"

func TestState_Transitions(t *testing.T) {
	ok := [][2]State{
		{StateRequested, StateSent}, {StateRequested, StateFailed},
		{StateSent, StateVerified}, {StateSent, StateExpired},
	}
	for _, p := range ok {
		if !p[0].CanTransition(p[1]) {
			t.Fatalf("expected %s->%s allowed", p[0], p[1])
		}
	}
	bad := [][2]State{
		{StateVerified, StateSent}, {StateExpired, StateVerified}, {StateFailed, StateSent},
	}
	for _, p := range bad {
		if p[0].CanTransition(p[1]) {
			t.Fatalf("expected %s->%s forbidden", p[0], p[1])
		}
	}
}
```

- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: Implement** (State values reuse the shared contract vocabulary)

```go
// state.go
package domain

import contracts "github.com/duykhanh/worklane/pkg/contracts/otp"

type State string

const (
	StateRequested State = contracts.StateRequested
	StateSent      State = contracts.StateSent
	StateFailed    State = contracts.StateFailed
	StateVerified  State = contracts.StateVerified
	StateExpired   State = contracts.StateExpired
)

var transitions = map[State]map[State]bool{
	StateRequested: {StateSent: true, StateFailed: true},
	StateSent:      {StateVerified: true, StateExpired: true},
	StateFailed:    {},
	StateVerified:  {},
	StateExpired:   {},
}

func (s State) CanTransition(to State) bool { return transitions[s][to] }
```

> Note: `domain` importing `pkg/contracts/otp` is allowed - `contracts` is a dependency-free shared
> kernel (plain constants/structs), not infrastructure. The arch test (Task 8) forbids adapters and
> `pkg/platform`, not `pkg/contracts`.

- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: OTP request state model`

### Task 5: Entity + recipient masking (domain)

**Files:** `domain/request.go` + `request_test.go`

- [ ] **Step 1: Failing test**

```go
// request_test.go
package domain

import "testing"

func TestMaskRecipient(t *testing.T) {
	cases := map[string]string{
		"duykhanh@gmail.com": "d***@gmail.com",
		"a@b.co":             "a***@b.co",
	}
	for in, want := range cases {
		if got := MaskRecipient(in); got != want {
			t.Fatalf("mask(%q)=%q want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: Implement**

```go
// request.go
package domain

import (
	"strings"
	"time"
)

type Channel string

const ChannelEmail Channel = "email"

type OTPRequest struct {
	ID        string
	TenantID  string
	Recipient string
	Channel   Channel
	State     State
	CreatedAt time.Time
}

// MaskRecipient hides the local part of an email for logs/audit.
func MaskRecipient(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
```

- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: OTPRequest entity and recipient masking`

### Task 6: Ports (application layer)

**Files:** `services/otp-api/internal/app/ports.go`

Outbound ports the use cases depend on. These are the seams every adapter implements and every fake
substitutes in tests. `EmailProvider` is intentionally absent here - email is otp-dispatcher's concern.

- [ ] **Step 1: Write** `ports.go`

```go
// app/ports.go
package app

import (
	"context"
	"time"
)

type CodeRecord struct {
	Hash     string
	Salt     string
	Attempts int
}

type DeliveryLog struct {
	RequestID     string
	Provider      string
	Status        string
	LatencyMillis int64
	Error         string
}

type APIKey struct {
	ID       string
	TenantID string
	Status   string
}

type Clock interface{ Now() time.Time }

type CodeStore interface {
	Save(ctx context.Context, key string, rec CodeRecord, ttl time.Duration) error
	Get(ctx context.Context, key string) (CodeRecord, error)
	Delete(ctx context.Context, key string) error
}

type Counter interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Exists(ctx context.Context, key string) (bool, error)
	Set(ctx context.Context, key string, ttl time.Duration) error
}

type Repo interface {
	InsertRequest(ctx context.Context, r Request) error
	UpdateState(ctx context.Context, id, to string) error
	FindAPIKey(ctx context.Context, hashedKey string) (APIKey, error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error)
	ListRequests(ctx context.Context, tenantID string, limit int) ([]Request, error)
	ListDeliveryLogs(ctx context.Context, tenantID string, limit int) ([]DeliveryLog, error)
}

type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
}

// Request is the app-layer view of an OTP request row (decoupled from domain.OTPRequest
// so the persistence shape can evolve independently).
type Request struct {
	ID        string
	TenantID  string
	Recipient string
	Channel   string
	State     string
	CreatedAt time.Time
}
```

- [ ] **Step 2: Commit** `feat: otp-api application ports`

### Task 7: Rate limiting (application)

**Files:** `app/ratelimit.go` + `ratelimit_test.go`

Rate limiting orchestrates over the `Counter` port, so it belongs in the application layer, not the
pure domain. It returns `domain.ErrRateLimited` / `domain.ErrCooldown`.

- [ ] **Step 1: Failing test** (in-memory fake Counter)

```go
// ratelimit_test.go
package app

import (
	"context"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

type fakeCounter struct {
	counts map[string]int64
	marks  map[string]bool
}

func newFakeCounter() *fakeCounter {
	return &fakeCounter{counts: map[string]int64{}, marks: map[string]bool{}}
}
func (f *fakeCounter) Incr(_ context.Context, k string, _ time.Duration) (int64, error) {
	f.counts[k]++
	return f.counts[k], nil
}
func (f *fakeCounter) Exists(_ context.Context, k string) (bool, error) { return f.marks[k], nil }
func (f *fakeCounter) Set(_ context.Context, k string, _ time.Duration) error {
	f.marks[k] = true
	return nil
}

func TestRateLimiter_PerRecipient(t *testing.T) {
	rl := NewRateLimiter(newFakeCounter(), LimitConfig{
		PerRecipientMax: 3, PerRecipientWindow: time.Hour,
		PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: 0,
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
			t.Fatalf("send %d should pass: %v", i, err)
		}
	}
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != domain.ErrRateLimited {
		t.Fatalf("4th send want ErrRateLimited, got %v", err)
	}
}

func TestRateLimiter_Cooldown(t *testing.T) {
	rl := NewRateLimiter(newFakeCounter(), LimitConfig{
		PerRecipientMax: 10, PerRecipientWindow: time.Hour,
		PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: time.Minute,
	})
	ctx := context.Background()
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
		t.Fatalf("first send should pass: %v", err)
	}
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != domain.ErrCooldown {
		t.Fatalf("immediate resend want ErrCooldown, got %v", err)
	}
}
```

- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: Implement**

```go
// ratelimit.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

type LimitConfig struct {
	PerRecipientMax    int
	PerRecipientWindow time.Duration
	PerTenantMax       int
	PerTenantWindow    time.Duration
	ResendCooldown     time.Duration
}

type RateLimiter struct {
	c   Counter
	cfg LimitConfig
}

func NewRateLimiter(c Counter, cfg LimitConfig) *RateLimiter { return &RateLimiter{c: c, cfg: cfg} }

// CheckAndCount enforces cooldown, per-recipient, then per-tenant limits, then records
// the send. Order matters: cooldown is checked before the counters so a blocked resend
// does not consume quota.
func (rl *RateLimiter) CheckAndCount(ctx context.Context, tenantID, recipient string) error {
	cdKey := fmt.Sprintf("otp:cd:%s:%s", tenantID, recipient)
	if rl.cfg.ResendCooldown > 0 {
		exists, err := rl.c.Exists(ctx, cdKey)
		if err != nil {
			return err
		}
		if exists {
			return domain.ErrCooldown
		}
	}

	rKey := fmt.Sprintf("otp:rl:rcpt:%s:%s", tenantID, recipient)
	n, err := rl.c.Incr(ctx, rKey, rl.cfg.PerRecipientWindow)
	if err != nil {
		return err
	}
	if int(n) > rl.cfg.PerRecipientMax {
		return domain.ErrRateLimited
	}

	tKey := fmt.Sprintf("otp:rl:tenant:%s", tenantID)
	tn, err := rl.c.Incr(ctx, tKey, rl.cfg.PerTenantWindow)
	if err != nil {
		return err
	}
	if int(tn) > rl.cfg.PerTenantMax {
		return domain.ErrRateLimited
	}

	if rl.cfg.ResendCooldown > 0 {
		if err := rl.c.Set(ctx, cdKey, rl.cfg.ResendCooldown); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: multi-layer rate limiting (recipient, tenant, cooldown)`

### Task 8: Use cases - Send & Verify + arch test

**Files:** `app/config.go`, `app/send.go`, `app/verify.go`, `app/usecase_test.go`, `app/arch_test.go`

- [ ] **Step 1: Failing test** (fakes for every port; same behavior the old plan asserted)

```go
// usecase_test.go
package app

import (
	"context"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

type fakeStore struct{ m map[string]CodeRecord }

func newFakeStore() *fakeStore { return &fakeStore{m: map[string]CodeRecord{}} }
func (f *fakeStore) Save(_ context.Context, k string, r CodeRecord, _ time.Duration) error {
	f.m[k] = r
	return nil
}
func (f *fakeStore) Get(_ context.Context, k string) (CodeRecord, error) {
	r, ok := f.m[k]
	if !ok {
		return CodeRecord{}, domain.ErrNotFound
	}
	return r, nil
}
func (f *fakeStore) Delete(_ context.Context, k string) error { delete(f.m, k); return nil }

type fakeRepo struct{ states map[string]string }

func newFakeRepo() *fakeRepo { return &fakeRepo{states: map[string]string{}} }
func (f *fakeRepo) InsertRequest(_ context.Context, r Request) error { f.states[r.ID] = r.State; return nil }
func (f *fakeRepo) UpdateState(_ context.Context, id, to string) error { f.states[id] = to; return nil }
func (f *fakeRepo) FindAPIKey(context.Context, string) (APIKey, error) { return APIKey{}, nil }
func (f *fakeRepo) ListAPIKeys(context.Context, string) ([]APIKey, error) { return nil, nil }
func (f *fakeRepo) ListRequests(context.Context, string, int) ([]Request, error) { return nil, nil }
func (f *fakeRepo) ListDeliveryLogs(context.Context, string, int) ([]DeliveryLog, error) { return nil, nil }

type fakePub struct{ events []string }

func (f *fakePub) Publish(_ context.Context, topic string, _ any) error {
	f.events = append(f.events, topic)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newSvc() (*Service, *fakePub) {
	pub := &fakePub{}
	svc := New(Deps{
		Store: newFakeStore(), Counter: newFakeCounter(), Repo: newFakeRepo(),
		Pub: pub, Clock: fixedClock{t: time.Unix(1000, 0)},
	}, Config{
		CodeLength: 6, TTL: 5 * time.Minute, MaxVerifyAttempts: 3, RequestedTopic: "otp.requested",
		Limits: LimitConfig{PerRecipientMax: 5, PerRecipientWindow: time.Hour,
			PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: 0},
	})
	return svc, pub
}

func TestSend_ThenVerify_Success(t *testing.T) {
	svc, pub := newSvc()
	ctx := context.Background()
	res, err := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(pub.events) != 1 || pub.events[0] != "otp.requested" {
		t.Fatalf("expected otp.requested published, got %v", pub.events)
	}
	if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: res.Code}); err != nil {
		t.Fatalf("verify correct code: %v", err)
	}
}

func TestVerify_WrongCode_LocksAfterMax(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	_, _ = svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	for i := 0; i < 3; i++ {
		if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}); err != domain.ErrCodeMismatch {
			t.Fatalf("attempt %d want ErrCodeMismatch, got %v", i, err)
		}
	}
	if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}); err != domain.ErrTooManyAttempts {
		t.Fatalf("want ErrTooManyAttempts after max, got %v", err)
	}
}

func TestSend_Idempotency_Collapses(t *testing.T) {
	svc, pub := newSvc()
	ctx := context.Background()
	in := SendInput{TenantID: "t1", Recipient: "a@b.co", IdempotencyKey: "abc"}
	r1, _ := svc.Send(ctx, in)
	r2, _ := svc.Send(ctx, in)
	if r1.RequestID != r2.RequestID {
		t.Fatalf("idempotent send must return same request id")
	}
	if len(pub.events) != 1 {
		t.Fatalf("idempotent send must publish once, got %d", len(pub.events))
	}
}
```

- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: Implement** the use cases

```go
// config.go
package app

import "time"

type Config struct {
	CodeLength        int
	TTL               time.Duration
	MaxVerifyAttempts int
	RequestedTopic    string
	Limits            LimitConfig
}

type Deps struct {
	Store   CodeStore
	Counter Counter
	Repo    Repo
	Pub     Publisher
	Clock   Clock
}

type Service struct {
	d   Deps
	cfg Config
	rl  *RateLimiter
}

func New(d Deps, cfg Config) *Service {
	return &Service{d: d, cfg: cfg, rl: NewRateLimiter(d.Counter, cfg.Limits)}
}
```

```go
// send.go
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

type SendInput struct {
	TenantID, Recipient, IdempotencyKey string
}
type SendResult struct {
	RequestID, Code string
}

func codeKey(tenantID, recipient string) string {
	return fmt.Sprintf("otp:code:%s:%s", tenantID, recipient)
}

func (s *Service) Send(ctx context.Context, in SendInput) (SendResult, error) {
	// Idempotency: a repeated key returns the prior request without re-publishing.
	if in.IdempotencyKey != "" {
		idemKey := fmt.Sprintf("otp:idem:%s:%s", in.TenantID, in.IdempotencyKey)
		if exists, err := s.d.Counter.Exists(ctx, idemKey); err != nil {
			return SendResult{}, err
		} else if exists {
			rec, err := s.d.Store.Get(ctx, codeKey(in.TenantID, in.Recipient))
			if err != nil {
				return SendResult{}, err
			}
			return SendResult{RequestID: rec.Salt}, nil // salt == request id (see below)
		}
		if err := s.d.Counter.Set(ctx, idemKey, s.cfg.TTL); err != nil {
			return SendResult{}, err
		}
	}

	if err := s.rl.CheckAndCount(ctx, in.TenantID, in.Recipient); err != nil {
		return SendResult{}, err
	}

	code, err := domain.GenerateCode(s.cfg.CodeLength)
	if err != nil {
		return SendResult{}, err
	}
	requestID := newID()
	salt := requestID // reuse request id as the per-request salt (unique per request)
	rec := CodeRecord{Hash: domain.HashCode(code, salt), Salt: salt}
	if err := s.d.Store.Save(ctx, codeKey(in.TenantID, in.Recipient), rec, s.cfg.TTL); err != nil {
		return SendResult{}, err
	}

	if err := s.d.Repo.InsertRequest(ctx, Request{
		ID: requestID, TenantID: in.TenantID, Recipient: in.Recipient,
		Channel: string(domain.ChannelEmail), State: contracts.StateRequested, CreatedAt: s.d.Clock.Now(),
	}); err != nil {
		return SendResult{}, err
	}
	evt := contracts.RequestedEvent{
		RequestID: requestID, TenantID: in.TenantID, Recipient: in.Recipient,
		Channel: string(domain.ChannelEmail), Code: code,
	}
	if err := s.d.Pub.Publish(ctx, s.cfg.RequestedTopic, evt); err != nil {
		return SendResult{}, err
	}
	return SendResult{RequestID: requestID, Code: code}, nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

```go
// verify.go
package app

import (
	"context"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

type VerifyInput struct {
	TenantID, Recipient, Code string
}

func (s *Service) Verify(ctx context.Context, in VerifyInput) error {
	key := codeKey(in.TenantID, in.Recipient)
	rec, err := s.d.Store.Get(ctx, key)
	if err != nil {
		return err // ErrNotFound when expired/absent
	}
	if rec.Attempts >= s.cfg.MaxVerifyAttempts {
		return domain.ErrTooManyAttempts
	}
	if !domain.VerifyHash(rec.Hash, in.Code, rec.Salt) {
		rec.Attempts++
		_ = s.d.Store.Save(ctx, key, rec, s.cfg.TTL)
		if rec.Attempts >= s.cfg.MaxVerifyAttempts {
			return domain.ErrTooManyAttempts
		}
		return domain.ErrCodeMismatch
	}
	_ = s.d.Store.Delete(ctx, key)
	return s.d.Repo.UpdateState(ctx, rec.Salt, contracts.StateVerified)
}
```

- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Architecture test** (`arch_test.go`, package `app_test` at the domain root)

```go
// services/otp-api/internal/arch_test.go
package internal_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Domain must import no infrastructure; app must import no adapter/platform.
func TestPurity(t *testing.T) {
	forbidden := map[string][]string{
		"domain": {"redis", "gorm", "sarama", "gin", "resend", "/adapters/", "/pkg/platform/"},
		"app":    {"redis", "gorm", "sarama", "gin", "resend", "/adapters/", "/pkg/platform/"},
	}
	fset := token.NewFileSet()
	for pkg, bad := range forbidden {
		entries, _ := os.ReadDir(pkg)
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(pkg, e.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				for _, b := range bad {
					if strings.Contains(imp.Path.Value, b) {
						t.Fatalf("%s/%s imports forbidden %s", pkg, e.Name(), imp.Path.Value)
					}
				}
			}
		}
	}
}
```

- [ ] **Step 6:** `make cover` → ≥ 80% on `services/otp-api/internal/...`. If below, add table tests for
  `GenerateCode` bounds and `Verify` `ErrNotFound` path.
- [ ] **Step 7: Commit** `feat: otp-api send/verify use cases + purity/coverage gate`

> **Idempotency note (unchanged from the proven design):** the idempotency branch returns the prior
> request id via the stored `Salt` (which equals the request id). If this coupling reads unclear during
> implementation, add an explicit `RequestID` field to `CodeRecord` and return it - keep the behavior
> asserted by `TestSend_Idempotency_Collapses`.

---

## Phase 2 - Adapters (integration-tested with testcontainers)

Adapters live under each service's `internal/adapters/outbound/` and implement the app ports. Shared
client construction (GORM DB, redis client, sarama config) lives in `pkg/platform/*`.

### Task 9: MySQL migrations + GORM repo (otp-api)

**Files:** `db/otp/migrations/0001_init.up.sql` (+ `.down.sql`), `pkg/platform/mysql/mysql.go`,
`services/otp-api/internal/adapters/outbound/mysqlrepo/repo.go` + `repo_test.go`

- [ ] **Step 1: Migration**

```sql
-- db/otp/migrations/0001_init.up.sql
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
CREATE TABLE otp_requests (
  id CHAR(36) PRIMARY KEY,
  tenant_id CHAR(36) NOT NULL,
  recipient_masked VARCHAR(255) NOT NULL,
  channel VARCHAR(16) NOT NULL,
  state VARCHAR(16) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_requests_tenant (tenant_id, created_at)
);
CREATE TABLE delivery_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  request_id CHAR(36) NOT NULL,
  tenant_id CHAR(36) NOT NULL,
  provider VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  latency_ms BIGINT NOT NULL,
  error TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_logs_tenant (tenant_id, created_at)
);
CREATE TABLE templates (
  id CHAR(36) PRIMARY KEY,
  channel VARCHAR(16) NOT NULL,
  locale VARCHAR(16) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  body TEXT NOT NULL
);
```

- [ ] **Step 2:** `pkg/platform/mysql` - `Open(dsn) (*gorm.DB, error)` (GORM + `gorm.io/driver/mysql`) and
  `Migrate(dsn, dir)` running `golang-migrate` on `db/otp/migrations`.
- [ ] **Step 3: Failing integration test** (`//go:build integration`, testcontainers MySQL): run
  migrations, then insert a tenant + api key and assert `FindAPIKey` returns it; `InsertRequest` then
  `UpdateState('verified')` updates the row; `recipient_masked` is stored via `domain.MaskRecipient`.
- [ ] **Step 4: Implement** `mysqlrepo.New(db *gorm.DB) *Repo` with GORM model structs (table-mapped) and
  methods for `InsertRequest`, `UpdateState`, `FindAPIKey`, `ListAPIKeys`, `ListRequests`,
  `ListDeliveryLogs`, `InsertDeliveryLog`. GORM uses parameterized queries by default (no SQL injection).
- [ ] **Step 5:** `go test -tags=integration ./services/otp-api/internal/adapters/outbound/mysqlrepo/ -v` → PASS.
- [ ] **Step 6: Commit** `feat: mysql migrations and gorm repo (otp-api)`

### Task 10: Redis adapter - CodeStore + Counter (otp-api)

**Files:** `pkg/platform/redis/redis.go` (client + typed key builders), `services/otp-api/internal/adapters/outbound/redisstore/store.go` + `store_test.go`

- [ ] **Step 1:** Failing integration test (testcontainers Redis): `Incr` returns increasing values and
  sets a TTL on first hit; `Save`/`Get` round-trips a `CodeRecord` (JSON); `Get` on a missing key returns
  `domain.ErrNotFound`; `Exists`/`Set` behave for cooldown markers.
- [ ] **Step 2:** run `-tags=integration` → FAIL.
- [ ] **Step 3:** Implement `redisstore.New(client *redis.Client) *Store` (`redis/go-redis/v9`) mapping
  `redis.Nil` → `domain.ErrNotFound`; `Incr` = `INCR` then `EXPIRE` when the value is 1.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: redis code store and counter adapter`

### Task 11: Kafka producer + consumer wrapper (sarama)

**Files:** `pkg/platform/kafka/producer.go`, `consumer.go`, `envelope.go`, `producer_test.go`

Mirror the production convention: a typed envelope with a discriminator, config-driven topics, and a
fire-and-forget publish using `context.WithoutCancel`.

- [ ] **Step 1:** Failing integration test (testcontainers Redpanda): publish a `RequestedEvent` to
  `otp.requested`; a consumer group receives the same bytes and decodes it.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement with `github.com/IBM/sarama`:
  - `Producer` (sync producer) implementing `app.Publisher`: JSON-encodes `{msg_type, data}`, keys by
    `request_id`.
  - `Consumer` (consumer group) with `Start(ctx, handler func(ctx, []byte) error)`.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: sarama kafka producer/consumer wrapper with typed envelope`

### Task 12: Resend email provider (otp-dispatcher)

**Files:** `services/otp-dispatcher/internal/adapters/outbound/resendmail/provider.go` + `provider_test.go`

- [ ] **Step 1:** Failing unit test with an `httptest.Server` stub asserting request body/headers and
  returning a fake message id.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement `New(apiKey, from string, baseURL string, hc *http.Client) *Provider`
  implementing the dispatcher's `EmailProvider` port; `Send` POSTs to `{baseURL}/emails` with bearer
  auth and returns the Resend message id. Inject `baseURL` so the test points at the stub.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: resend email provider adapter`

---

## Phase 3 - Services (Gin + sarama consumer)

### Task 13: otp-api - Gin HTTP + API-key middleware + read endpoints

**Files:** `pkg/platform/httpserver/`, `services/otp-api/internal/adapters/inbound/http/` (`router.go`,
`send.go`, `verify.go`, `reads.go`, `apikey.go`, `dto.go`, `errors.go`, `handlers_test.go`),
`services/otp-api/main.go`

**Produces:** `POST /v1/otp/send` → `202 {request_id}`; `POST /v1/otp/verify` → `200 {status:"verified"}`
or `4xx`; read endpoints `GET /v1/api-keys`, `GET /v1/otp/requests`, `GET /v1/delivery-logs` (all
tenant-scoped). API-key middleware resolves `Authorization: Bearer <key>` → hashes it →
`Repo.FindAPIKey` → injects `tenant_id` into the Gin context.

- [ ] **Step 1:** `httptest`-level test injecting a fake `*app.Service`: a valid `send` yields `202` +
  `request_id`; a rate-limited one yields `429`; missing/invalid key yields `401`.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement handlers delegating to `app.Service`; a central error mapper translates
  domain errors → HTTP: `ErrRateLimited`/`ErrCooldown`/`ErrTooManyAttempts` → `429`, `ErrCodeMismatch`
  → `401`, `ErrNotFound`/`ErrExpired` → `410`. Wire everything in `main.go` (composition root):
  construct GORM/redis/sarama clients from config, build the adapters, inject into `app.New(...)`, mount
  the Gin router via `pkg/platform/httpserver`.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: otp-api gin service with api-key auth and read endpoints`

### Task 14: otp-dispatcher - consume otp.requested, send email, log delivery

**Files:** `services/otp-dispatcher/internal/app/` (`ports.go`, `handler.go`, `template.go`,
`handler_test.go`), `services/otp-dispatcher/internal/adapters/inbound/kafka/consumer.go`,
`services/otp-dispatcher/main.go`

**Dispatcher ports:** `EmailProvider` (Send), `Repo` (InsertDeliveryLog + UpdateState), `Publisher`
(sent/failed/dlq).

- [ ] **Step 1:** Failing unit test with fake mail/repo/pub: a valid `RequestedEvent` triggers
  `mail.Send`, a `DeliveryLog` insert, state → `sent`, and an `otp.sent` publish; a failing `mail.Send`
  yields state → `failed`, an `otp.failed` publish, and a publish to `otp.dlq` (no drainer in MVP).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement `Handle(ctx, raw []byte) error` - decode the shared `contracts.RequestedEvent`,
  render the email from a minimal `Template{Subject, BodyFmt}` (`fmt.Sprintf(BodyFmt, code)`), send,
  write the delivery log, update state, publish. Wire the sarama consumer + adapters in `main.go`.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** `feat: dispatcher email delivery handler`

---

## Phase 4 - Compose + end-to-end (Traefik)

### Task 15: docker-compose stack + Traefik gateway

**Files:** `deploy/compose/docker-compose.yml`, `deploy/traefik/traefik.yml`, `deploy/traefik/dynamic.yml`,
`.env.example`, `services/*/Dockerfile`

- [ ] **Step 1:** `docker-compose.yml` with services: `mysql`, `redis`, `redpanda` (Kafka API),
  `traefik`, `otp-api`, `otp-dispatcher`. Wire env (`RESEND_API_KEY`, brokers, DSNs, topics).
- [ ] **Step 2:** Traefik config - static (`traefik.yml`: entrypoints, docker provider) + dynamic
  (`dynamic.yml` or Docker labels): a router for `/v1/*` → `otp-api`, a **rate-limit** middleware (edge),
  and a **CORS** middleware (`headers`) allowing the Vercel origin. TLS is left to Cloudflare in prod;
  local is plain HTTP on `:80`. API-key auth is **not** at Traefik - it is enforced in `otp-api`.
- [ ] **Step 3:** `docker compose up -d`, then `curl -i localhost/v1/otp/send` (no key) → `401` from
  `otp-api` through Traefik; with a seeded key (Task 17) → `202`.
- [ ] **Step 4: Commit** `chore: docker-compose stack with traefik gateway`

### Task 16: End-to-end send→verify test

**Files:** `test/e2e/e2e_test.go` (build tag `e2e`), `test/e2e/README.md`

- [ ] **Step 1:** e2e test against a running compose stack + a seeded key (Task 17): `POST /v1/otp/send`,
  read the delivered code from a **MailHog** SMTP container (deterministic; Resend stays for
  real/manual verification), then `POST /v1/otp/verify` → `200`. Also assert wrong code → `401` and
  exceeding attempts → `429`.
- [ ] **Step 2:** `go test -tags=e2e ./test/e2e/ -v` → FAIL until stack + seed exist.
- [ ] **Step 3:** Document run steps in `test/e2e/README.md`.
- [ ] **Step 4:** Once green, Commit `test: end-to-end send/verify through traefik`.

> **Delivery caveat:** for automated e2e prefer a MailHog container so the test reads the code
> programmatically; keep Resend for real verification. Deterministic pipeline without a real inbox.

---

## Phase 5 - Seed CLI + Dashboard

### Task 17: Seed CLI (tenant + API key)

**Files:** `services/seed/main.go`, `services/seed/apikey.go` + `apikey_test.go`

- [ ] **Step 1:** Unit test for the key generator: `GenerateAPIKey()` returns a 32+ char URL-safe token,
  and its stored hash verifies (`domain.HashCode(key, "")` or a dedicated key-hash helper).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement `GenerateAPIKey` (crypto/rand, base64url) and a `main` that inserts a tenant +
  api_key via `mysqlrepo`. Prints the **plaintext key once**; stores only the hash.
- [ ] **Step 4:** run → PASS; then `go run ./services/seed --name demo` against compose MySQL; capture the key.
- [ ] **Step 5: Commit** `feat: seed CLI for tenant and api key`

### Task 18: Next.js dashboard - 3 read-only screens

**Files:** `dashboard/` (Next.js App Router, TS) from a shadcn dashboard block. Key files:
`app/(dash)/api-keys/page.tsx`, `app/(dash)/history/page.tsx`, `app/(dash)/deliveries/page.tsx`,
`lib/api.ts`, `lib/queries.ts`, `store/ui.ts`.

Consumes the `otp-api` read endpoints from Task 13.

- [ ] **Step 1:** `create-next-app` (TS, App Router); `shadcn init` + a dashboard block; add TanStack
  Query provider, Zustand, TanStack Table, RHF+Zod.
- [ ] **Step 2:** `lib/api.ts` - typed fetch wrapper reading `NEXT_PUBLIC_API_BASE` + the API key;
  `lib/queries.ts` - `useApiKeys`, `useRequests`, `useDeliveries` with `refetchInterval: 5000` on
  deliveries.
- [ ] **Step 3:** Three TanStack Table pages bound to the queries; server data stays in React Query,
  only filters/token live in Zustand (`store/ui.ts`).
- [ ] **Step 4:** Component test (Vitest + Testing Library): deliveries table renders rows from a mocked
  query and polls (advance timers, assert refetch).
- [ ] **Step 5:** `npm test` → PASS; `npm run build` → succeeds.
- [ ] **Step 6: Commit** `feat: next.js dashboard with three read-only screens`

---

## Self-Review

**Spec coverage (vs 2026-08-07-otp-mvp-scope.md):**
- Email via Resend → Task 12. ✅
- otp-api (Gin) + otp-dispatcher (sarama), no worker/cron → Tasks 13, 14. ✅
- Traefik gateway (edge rate limit + CORS; auth in otp-api; TLS at Cloudflare) → Task 15. ✅
- Kafka topics requested/sent/failed (+dlq publish, no drainer) → Tasks 11, 14. ✅
- Redis hash+TTL, counters, cooldown, attempts → Tasks 7, 8, 10. ✅
- MySQL via GORM + golang-migrate → Task 9. ✅
- Core domain (gen, hash-only, constant-time, 4-layer limit, attempt lock, idempotency, state) → Tasks 2–8. ✅
- Tenant/API key seed CLI → Task 17. ✅
- Dashboard 3 read-only screens (React Query polling, Zustand, RHF+Zod, TanStack Table) → Task 18. ✅
- Testing: unit (Phase 1), integration testcontainers (Phase 2), e2e send→verify (Task 16), 80% coverage (Task 8). ✅

**Architecture fidelity:** two independently deployable microservices, each hexagonal; domain/app import
no infra (enforced by Task 8); services share only `pkg/contracts` + `pkg/platform`, never each other's
`internal/`. Matches [../../reference/architecture-and-layout.md](../../reference/architecture-and-layout.md).

**Deferred (per scope):** worker/cron DLQ drainer + cleanup (fast-follow); SMS/Twilio failover, LGTM
observability, dashboard auth, HMAC signing (Phase 2+).
```
