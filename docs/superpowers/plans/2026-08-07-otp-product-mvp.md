# OTP Product MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the walking skeleton from [2026-08-07-otp-mvp-scope.md](../specs/2026-08-07-otp-mvp-scope.md) — one email OTP that flows `send → real email → verify` through APISIX → otp-api → Kafka → dispatcher → Resend, proven end-to-end on docker-compose.

**Architecture:** Ports & adapters. A pure `internal/otp` domain core (code generation, hashing, verification, rate limiting, state) depends only on interfaces (`Clock`, `CodeStore`, `Counter`, `Repo`, `Publisher`, `EmailProvider`). Adapters (Redis, Postgres, Kafka, Resend) implement those interfaces. Two go-zero services wire them: `otp-api` (sync HTTP) publishes `otp.requested`; `dispatcher` (async consumer) sends the email. This keeps the domain 100% unit-testable with fakes and defers all infra behind boundaries.

**Tech Stack:** Go 1.22+, go-zero (HTTP service + Kafka via `go-queue`), Redis, PostgreSQL, Apache Kafka, APISIX, Resend (email), docker-compose, testcontainers-go for integration tests.

## Global Constraints

- Language: **Go 1.22+**; module path `github.com/duykhanh/otp` (adjust to real remote if different).
- Domain package `internal/otp` MUST NOT import any adapter, driver, or framework package (no `redis`, `pgx`, `kafka`, `go-zero`). Enforced by an architecture test.
- OTP plaintext code is **never** persisted or logged — only a hash is stored (Redis) with a TTL. Default length **6**, default TTL **5 minutes**.
- Verification uses **constant-time** comparison.
- Recipient PII (email) is **masked** in any log or audit output (e.g. `d***@gmail.com`).
- Every task is TDD: failing test first, minimal code, green, commit. Target **80%** coverage on `internal/otp`.
- Immutability: domain constructors return new values; no in-place mutation of shared structs.

---

## File Structure

```
go.mod
Makefile
cmd/
  otp-api/main.go          # go-zero HTTP entrypoint
  dispatcher/main.go       # Kafka consumer entrypoint
  seed/main.go             # CLI: create tenant + API key
internal/
  otp/                     # PURE domain core — no infra imports
    code.go / code_test.go             # crypto-random numeric code
    hash.go / hash_test.go             # hash + constant-time verify
    state.go / state_test.go           # request state model
    request.go / request_test.go       # OTPRequest entity
    ports.go                           # Clock, CodeStore, Counter, Repo, Publisher, EmailProvider
    ratelimit.go / ratelimit_test.go   # 4-layer rate limiting (pure, over Counter)
    service.go / service_test.go       # SendOTP / VerifyOTP orchestration
    errors.go
    arch_test.go                       # asserts domain imports no adapters
  adapter/
    redisstore/store.go                # CodeStore + Counter
    pgrepo/repo.go, migrations/*.sql   # tenants, api_keys, otp_requests, delivery_logs, templates
    kafkaev/producer.go, consumer.go   # publish/consume events
    resendmail/provider.go             # EmailProvider via Resend
  transport/
    httpapi/                           # go-zero handlers/logic for /v1/otp/*
    dispatch/                          # dispatcher business logic
deploy/
  docker-compose.yml
  apisix/apisix.yaml, apisix/routes.yaml
dashboard/                             # Next.js app (see Phase 5)
```

---

## Phase 1 — Core domain (pure, TDD)

### Task 1: Project scaffolding

**Files:**
- Create: `go.mod`, `Makefile`, `internal/otp/errors.go`

- [ ] **Step 1: Init module and tidy**

```bash
go mod init github.com/duykhanh/otp
go mod tidy
```

- [ ] **Step 2: Add Makefile**

```makefile
.PHONY: test cover
test:
	go test ./...
cover:
	go test -coverprofile=cover.out ./internal/otp/... && go tool cover -func=cover.out | tail -1
```

- [ ] **Step 3: Add domain error sentinels**

```go
// internal/otp/errors.go
package otp

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

- [ ] **Step 4: Commit**

```bash
git add go.mod Makefile internal/otp/errors.go
git commit -m "chore: scaffold go module and domain errors"
```

### Task 2: Crypto-random code generation

**Files:**
- Create: `internal/otp/code.go`, `internal/otp/code_test.go`

**Interfaces:**
- Produces: `func GenerateCode(length int) (string, error)` — returns a zero-padded numeric string of exactly `length` digits using `crypto/rand`.

- [ ] **Step 1: Write the failing test**

```go
// internal/otp/code_test.go
package otp

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
	// 1000 draws from 1e6 space: collisions must be rare.
	if len(seen) < 990 {
		t.Fatalf("suspicious distribution, unique=%d", len(seen))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/otp/ -run TestGenerateCode -v`
Expected: FAIL — `undefined: GenerateCode`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/otp/code.go
package otp

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateCode returns a numeric OTP of exactly length digits (zero-padded),
// drawn from crypto/rand. length must be between 4 and 10.
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/otp/ -run TestGenerateCode -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/otp/code.go internal/otp/code_test.go
git commit -m "feat: crypto-random OTP code generation"
```

### Task 3: Hashing + constant-time verification

**Files:**
- Create: `internal/otp/hash.go`, `internal/otp/hash_test.go`

**Interfaces:**
- Produces: `func HashCode(code, salt string) string` (hex SHA-256 of salt+code) and `func VerifyHash(hash, code, salt string) bool` (constant-time).

- [ ] **Step 1: Write the failing test**

```go
// internal/otp/hash_test.go
package otp

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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/otp/ -run 'TestHashCode|TestVerifyHash' -v`
Expected: FAIL — `undefined: HashCode`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/otp/hash.go
package otp

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// HashCode returns the hex SHA-256 of salt+code. The salt is a per-request
// random value stored alongside the hash so two requests for the same code
// produce different hashes.
func HashCode(code, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + code))
	return hex.EncodeToString(sum[:])
}

// VerifyHash compares in constant time.
func VerifyHash(hash, code, salt string) bool {
	got := HashCode(code, salt)
	return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/otp/ -run 'TestHashCode|TestVerifyHash' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/otp/hash.go internal/otp/hash_test.go
git commit -m "feat: salted hashing with constant-time verification"
```

### Task 4: State model

**Files:**
- Create: `internal/otp/state.go`, `internal/otp/state_test.go`

**Interfaces:**
- Produces: `type State string` with consts `StateRequested`, `StateSent`, `StateFailed`, `StateVerified`, `StateExpired`; `func (s State) CanTransition(to State) bool`.

- [ ] **Step 1: Write the failing test**

```go
// internal/otp/state_test.go
package otp

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
		{StateVerified, StateSent}, {StateExpired, StateVerified},
		{StateFailed, StateSent},
	}
	for _, p := range bad {
		if p[0].CanTransition(p[1]) {
			t.Fatalf("expected %s->%s forbidden", p[0], p[1])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/otp/ -run TestState -v`
Expected: FAIL — `undefined: State`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/otp/state.go
package otp

type State string

const (
	StateRequested State = "requested"
	StateSent      State = "sent"
	StateFailed    State = "failed"
	StateVerified  State = "verified"
	StateExpired   State = "expired"
)

var transitions = map[State]map[State]bool{
	StateRequested: {StateSent: true, StateFailed: true},
	StateSent:      {StateVerified: true, StateExpired: true},
	StateFailed:    {},
	StateVerified:  {},
	StateExpired:   {},
}

func (s State) CanTransition(to State) bool {
	return transitions[s][to]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/otp/ -run TestState -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/otp/state.go internal/otp/state_test.go
git commit -m "feat: OTP request state model"
```

### Task 5: Ports (interfaces) + OTPRequest entity

**Files:**
- Create: `internal/otp/ports.go`, `internal/otp/request.go`, `internal/otp/request_test.go`

**Interfaces:**
- Produces:
  - `type Channel string` (`ChannelEmail`).
  - `type OTPRequest struct { ID, TenantID, Recipient string; Channel Channel; State State; CreatedAt time.Time }`.
  - `type Clock interface { Now() time.Time }`.
  - `type CodeStore interface { Save(ctx, key string, rec CodeRecord, ttl time.Duration) error; Get(ctx, key string) (CodeRecord, error); Delete(ctx, key string) error }` with `type CodeRecord struct { Hash, Salt string; Attempts int }`.
  - `type Counter interface { Incr(ctx, key string, ttl time.Duration) (int64, error); Exists(ctx, key string) (bool, error); Set(ctx, key string, ttl time.Duration) error }`.
  - `type Repo interface { InsertRequest(ctx, OTPRequest) error; UpdateState(ctx, id string, to State) error; InsertDeliveryLog(ctx, DeliveryLog) error; FindAPIKey(ctx, hashedKey string) (APIKey, error) }`.
  - `type Publisher interface { Publish(ctx, topic string, event any) error }`.
  - `type EmailProvider interface { Send(ctx, to, subject, body string) (providerMsgID string, err error) }`.

- [ ] **Step 1: Write the failing test**

```go
// internal/otp/request_test.go
package otp

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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/otp/ -run TestMaskRecipient -v`
Expected: FAIL — `undefined: MaskRecipient`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/otp/ports.go
package otp

import (
	"context"
	"time"
)

type Channel string

const ChannelEmail Channel = "email"

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
	InsertRequest(ctx context.Context, r OTPRequest) error
	UpdateState(ctx context.Context, id string, to State) error
	InsertDeliveryLog(ctx context.Context, l DeliveryLog) error
	FindAPIKey(ctx context.Context, hashedKey string) (APIKey, error)
}

type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
}

type EmailProvider interface {
	Send(ctx context.Context, to, subject, body string) (string, error)
}
```

```go
// internal/otp/request.go
package otp

import (
	"strings"
	"time"
)

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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/otp/ -run TestMaskRecipient -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/otp/ports.go internal/otp/request.go internal/otp/request_test.go
git commit -m "feat: domain ports and OTPRequest entity"
```

### Task 6: Four-layer rate limiting

**Files:**
- Create: `internal/otp/ratelimit.go`, `internal/otp/ratelimit_test.go`

**Interfaces:**
- Consumes: `Counter`, `Clock`.
- Produces: `type LimitConfig struct { PerRecipientMax int; PerRecipientWindow time.Duration; PerTenantMax int; PerTenantWindow time.Duration; ResendCooldown time.Duration }` and `type RateLimiter struct{...}` with `func NewRateLimiter(Counter, LimitConfig) *RateLimiter` and `func (rl *RateLimiter) CheckAndCount(ctx, tenantID, recipient string) error` returning `ErrRateLimited` / `ErrCooldown`.

- [ ] **Step 1: Write the failing test** (uses an in-memory fake Counter)

```go
// internal/otp/ratelimit_test.go
package otp

import (
	"context"
	"testing"
	"time"
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
		PerTenantMax: 100, PerTenantWindow: time.Hour,
		ResendCooldown: 0,
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
			t.Fatalf("send %d should pass: %v", i, err)
		}
	}
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != ErrRateLimited {
		t.Fatalf("4th send want ErrRateLimited, got %v", err)
	}
}

func TestRateLimiter_Cooldown(t *testing.T) {
	rl := NewRateLimiter(newFakeCounter(), LimitConfig{
		PerRecipientMax: 10, PerRecipientWindow: time.Hour,
		PerTenantMax: 100, PerTenantWindow: time.Hour,
		ResendCooldown: time.Minute,
	})
	ctx := context.Background()
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != nil {
		t.Fatalf("first send should pass: %v", err)
	}
	if err := rl.CheckAndCount(ctx, "t1", "a@b.co"); err != ErrCooldown {
		t.Fatalf("immediate resend want ErrCooldown, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/otp/ -run TestRateLimiter -v`
Expected: FAIL — `undefined: NewRateLimiter`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/otp/ratelimit.go
package otp

import (
	"context"
	"fmt"
	"time"
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

func NewRateLimiter(c Counter, cfg LimitConfig) *RateLimiter {
	return &RateLimiter{c: c, cfg: cfg}
}

// CheckAndCount enforces cooldown, per-recipient, and per-tenant limits, then
// records the send. Order matters: cooldown is checked before counters so a
// blocked resend does not consume quota.
func (rl *RateLimiter) CheckAndCount(ctx context.Context, tenantID, recipient string) error {
	cdKey := fmt.Sprintf("otp:cd:%s:%s", tenantID, recipient)
	if rl.cfg.ResendCooldown > 0 {
		exists, err := rl.c.Exists(ctx, cdKey)
		if err != nil {
			return err
		}
		if exists {
			return ErrCooldown
		}
	}

	rKey := fmt.Sprintf("otp:rl:rcpt:%s:%s", tenantID, recipient)
	n, err := rl.c.Incr(ctx, rKey, rl.cfg.PerRecipientWindow)
	if err != nil {
		return err
	}
	if int(n) > rl.cfg.PerRecipientMax {
		return ErrRateLimited
	}

	tKey := fmt.Sprintf("otp:rl:tenant:%s", tenantID)
	tn, err := rl.c.Incr(ctx, tKey, rl.cfg.PerTenantWindow)
	if err != nil {
		return err
	}
	if int(tn) > rl.cfg.PerTenantMax {
		return ErrRateLimited
	}

	if rl.cfg.ResendCooldown > 0 {
		if err := rl.c.Set(ctx, cdKey, rl.cfg.ResendCooldown); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/otp/ -run TestRateLimiter -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/otp/ratelimit.go internal/otp/ratelimit_test.go
git commit -m "feat: multi-layer rate limiting (recipient, tenant, cooldown)"
```

### Task 7: Domain service — SendOTP & VerifyOTP (incl. verify-attempt lock + idempotency)

**Files:**
- Create: `internal/otp/service.go`, `internal/otp/service_test.go`

**Interfaces:**
- Consumes: `CodeStore`, `Counter`, `Repo`, `Publisher`, `Clock`, `RateLimiter`.
- Produces:
  - `type Config struct { CodeLength int; TTL time.Duration; MaxVerifyAttempts int; Limits LimitConfig }`.
  - `type Service struct{...}` + `func NewService(deps Deps, cfg Config) *Service` where `type Deps struct { Store CodeStore; Counter Counter; Repo Repo; Pub Publisher; Clock Clock }`.
  - `func (s *Service) Send(ctx, in SendInput) (SendResult, error)` with `SendInput{TenantID, Recipient, IdempotencyKey string}` and `SendResult{RequestID, Code string}` (Code returned to the caller only so the dispatcher can template it; never logged).
  - `func (s *Service) Verify(ctx, in VerifyInput) error` with `VerifyInput{TenantID, Recipient, Code string}` returning nil / `ErrCodeMismatch` / `ErrExpired` / `ErrTooManyAttempts` / `ErrNotFound`.

- [ ] **Step 1: Write the failing test** (fakes for every port)

```go
// internal/otp/service_test.go
package otp

import (
	"context"
	"testing"
	"time"
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
		return CodeRecord{}, ErrNotFound
	}
	return r, nil
}
func (f *fakeStore) Delete(_ context.Context, k string) error { delete(f.m, k); return nil }

type fakeRepo struct{ states map[string]State }

func newFakeRepo() *fakeRepo { return &fakeRepo{states: map[string]State{}} }
func (f *fakeRepo) InsertRequest(_ context.Context, r OTPRequest) error {
	f.states[r.ID] = r.State
	return nil
}
func (f *fakeRepo) UpdateState(_ context.Context, id string, to State) error {
	f.states[id] = to
	return nil
}
func (f *fakeRepo) InsertDeliveryLog(context.Context, DeliveryLog) error { return nil }
func (f *fakeRepo) FindAPIKey(context.Context, string) (APIKey, error)   { return APIKey{}, nil }

type fakePub struct{ events []string }

func (f *fakePub) Publish(_ context.Context, topic string, _ any) error {
	f.events = append(f.events, topic)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newService() (*Service, *fakeStore, *fakePub) {
	store := newFakeStore()
	pub := &fakePub{}
	svc := NewService(Deps{
		Store: store, Counter: newFakeCounter(), Repo: newFakeRepo(),
		Pub: pub, Clock: fixedClock{t: time.Unix(1000, 0)},
	}, Config{
		CodeLength: 6, TTL: 5 * time.Minute, MaxVerifyAttempts: 3,
		Limits: LimitConfig{PerRecipientMax: 5, PerRecipientWindow: time.Hour,
			PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: 0},
	})
	return svc, store, pub
}

func TestSend_ThenVerify_Success(t *testing.T) {
	svc, _, pub := newService()
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
	svc, _, _ := newService()
	ctx := context.Background()
	res, _ := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	_ = res
	for i := 0; i < 3; i++ {
		if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}); err != ErrCodeMismatch {
			t.Fatalf("attempt %d want ErrCodeMismatch, got %v", i, err)
		}
	}
	if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}); err != ErrTooManyAttempts {
		t.Fatalf("want ErrTooManyAttempts after max, got %v", err)
	}
}

func TestSend_Idempotency_Collapses(t *testing.T) {
	svc, _, pub := newService()
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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/otp/ -run 'TestSend|TestVerify' -v`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/otp/service.go
package otp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Config struct {
	CodeLength        int
	TTL               time.Duration
	MaxVerifyAttempts int
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

func NewService(d Deps, cfg Config) *Service {
	return &Service{d: d, cfg: cfg, rl: NewRateLimiter(d.Counter, cfg.Limits)}
}

type SendInput struct {
	TenantID       string
	Recipient      string
	IdempotencyKey string
}
type SendResult struct {
	RequestID string
	Code      string
}
type VerifyInput struct {
	TenantID  string
	Recipient string
	Code      string
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
			return SendResult{RequestID: rec.Salt, Code: ""}, nil // Salt doubles as request id below
		}
		if err := s.d.Counter.Set(ctx, idemKey, s.cfg.TTL); err != nil {
			return SendResult{}, err
		}
	}

	if err := s.rl.CheckAndCount(ctx, in.TenantID, in.Recipient); err != nil {
		return SendResult{}, err
	}

	code, err := GenerateCode(s.cfg.CodeLength)
	if err != nil {
		return SendResult{}, err
	}
	requestID := newID()
	salt := requestID // reuse request id as salt; unique per request
	rec := CodeRecord{Hash: HashCode(code, salt), Salt: salt, Attempts: 0}
	if err := s.d.Store.Save(ctx, codeKey(in.TenantID, in.Recipient), rec, s.cfg.TTL); err != nil {
		return SendResult{}, err
	}

	req := OTPRequest{
		ID: requestID, TenantID: in.TenantID, Recipient: in.Recipient,
		Channel: ChannelEmail, State: StateRequested, CreatedAt: s.d.Clock.Now(),
	}
	if err := s.d.Repo.InsertRequest(ctx, req); err != nil {
		return SendResult{}, err
	}
	event := map[string]string{
		"request_id": requestID, "tenant_id": in.TenantID,
		"recipient": in.Recipient, "channel": string(ChannelEmail), "code": code,
	}
	if err := s.d.Pub.Publish(ctx, "otp.requested", event); err != nil {
		return SendResult{}, err
	}
	return SendResult{RequestID: requestID, Code: code}, nil
}

func (s *Service) Verify(ctx context.Context, in VerifyInput) error {
	key := codeKey(in.TenantID, in.Recipient)
	rec, err := s.d.Store.Get(ctx, key)
	if err != nil {
		return err // ErrNotFound when expired/absent
	}
	if rec.Attempts >= s.cfg.MaxVerifyAttempts {
		return ErrTooManyAttempts
	}
	if !VerifyHash(rec.Hash, in.Code, rec.Salt) {
		rec.Attempts++
		_ = s.d.Store.Save(ctx, key, rec, s.cfg.TTL)
		if rec.Attempts >= s.cfg.MaxVerifyAttempts {
			return ErrTooManyAttempts
		}
		return ErrCodeMismatch
	}
	_ = s.d.Store.Delete(ctx, key)
	return s.d.Repo.UpdateState(ctx, rec.Salt, StateVerified)
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

> Note for the implementer: the idempotency branch above returns the prior request id via the stored `Salt` (which equals the request id). If the executing engineer finds this coupling unclear, store an explicit `RequestID` field on `CodeRecord` instead and return it — update `ports.go` and the fakes accordingly. Keep the behavior asserted by `TestSend_Idempotency_Collapses`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/otp/ -run 'TestSend|TestVerify' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/otp/service.go internal/otp/service_test.go
git commit -m "feat: OTP domain service (send/verify, attempt lock, idempotency)"
```

### Task 8: Architecture test (domain purity) + coverage gate

**Files:**
- Create: `internal/otp/arch_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/otp/arch_test.go
package otp_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainHasNoInfraImports(t *testing.T) {
	forbidden := []string{"redis", "pgx", "kafka", "zeromicro", "go-zero", "resend"}
	fset := token.NewFileSet()
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			for _, bad := range forbidden {
				if strings.Contains(imp.Path.Value, bad) {
					t.Fatalf("%s imports forbidden %s", e.Name(), imp.Path.Value)
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it passes** (domain is already pure)

Run: `go test ./internal/otp/ -run TestDomainHasNoInfraImports -v`
Expected: PASS.

- [ ] **Step 3: Check coverage meets 80%**

Run: `make cover`
Expected: total coverage for `internal/otp` ≥ 80%. If below, add table tests for `GenerateCode` bounds and `Verify` `ErrNotFound`/`ErrExpired` paths.

- [ ] **Step 4: Commit**

```bash
git add internal/otp/arch_test.go
git commit -m "test: enforce domain purity and coverage gate"
```

---

## Phase 2 — Adapters (integration-tested with testcontainers)

### Task 9: Postgres schema + repo

**Files:**
- Create: `internal/adapter/pgrepo/migrations/0001_init.sql`, `internal/adapter/pgrepo/repo.go`, `internal/adapter/pgrepo/repo_test.go`

**Interfaces:**
- Consumes: `otp.Repo`.
- Produces: `func New(pool *pgxpool.Pool) *Repo` implementing `otp.Repo`.

- [ ] **Step 1: Write the migration**

```sql
-- internal/adapter/pgrepo/migrations/0001_init.sql
CREATE TABLE tenants (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE api_keys (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  hashed_key TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE otp_requests (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  recipient_masked TEXT NOT NULL,
  channel TEXT NOT NULL,
  state TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE delivery_logs (
  id BIGSERIAL PRIMARY KEY,
  request_id UUID NOT NULL,
  provider TEXT NOT NULL,
  status TEXT NOT NULL,
  latency_ms BIGINT NOT NULL,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE templates (
  id UUID PRIMARY KEY,
  channel TEXT NOT NULL,
  locale TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL
);
```

- [ ] **Step 2: Write the failing integration test** (testcontainers Postgres)

```go
// internal/adapter/pgrepo/repo_test.go
//go:build integration
package pgrepo_test
// Spin up a postgres testcontainer, run 0001_init.sql, then:
//   - insert a tenant + api key, FindAPIKey by hashed_key returns it
//   - InsertRequest then UpdateState to 'verified' updates the row
// Assert each. (Full container boilerplate written here by the implementer
// following testcontainers-go postgres module docs.)
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test -tags=integration ./internal/adapter/pgrepo/ -v`
Expected: FAIL — repo not implemented.

- [ ] **Step 4: Implement `Repo`** using `pgxpool` with parameterized queries for `InsertRequest`, `UpdateState`, `InsertDeliveryLog`, `FindAPIKey`. Store `recipient_masked` via `otp.MaskRecipient`.

- [ ] **Step 5: Run to verify it passes**

Run: `go test -tags=integration ./internal/adapter/pgrepo/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/pgrepo/
git commit -m "feat: postgres schema and repo adapter"
```

### Task 10: Redis adapter (CodeStore + Counter)

**Files:**
- Create: `internal/adapter/redisstore/store.go`, `internal/adapter/redisstore/store_test.go`

**Interfaces:**
- Produces: `func New(client *redis.Client) *Store` implementing both `otp.CodeStore` and `otp.Counter`. `Incr` uses `INCR` + `EXPIRE` on first hit; `Set` uses `SET key 1 EX ttl`; `Exists` uses `EXISTS`; `Save` stores the `CodeRecord` as a JSON string with `SET ... EX`.

- [ ] **Step 1:** Write failing integration test (testcontainers Redis): `Incr` returns increasing values and TTL is set; `Save`/`Get` round-trips a `CodeRecord`; `Get` on a missing key returns `otp.ErrNotFound`.
- [ ] **Step 2:** Run `go test -tags=integration ./internal/adapter/redisstore/ -v` → FAIL.
- [ ] **Step 3:** Implement `Store` with `github.com/redis/go-redis/v9`, JSON-encoding `CodeRecord`, mapping redis.Nil to `otp.ErrNotFound`.
- [ ] **Step 4:** Run test → PASS.
- [ ] **Step 5:** Commit `feat: redis code store and counter adapter`.

### Task 11: Kafka producer + consumer

**Files:**
- Create: `internal/adapter/kafkaev/producer.go`, `internal/adapter/kafkaev/consumer.go`, `internal/adapter/kafkaev/producer_test.go`

**Interfaces:**
- Produces: `func NewProducer(brokers []string) *Producer` implementing `otp.Publisher` (JSON-encodes the event, keys by `request_id`); `func NewConsumer(brokers []string, topic, group string, handle func(ctx, []byte) error) *Consumer` with `Start(ctx)`.

- [ ] **Step 1:** Write failing integration test (testcontainers Kafka/Redpanda): publish to `otp.requested`, consumer receives the same bytes.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement with `github.com/segmentio/kafka-go`.
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5:** Commit `feat: kafka producer and consumer adapter`.

### Task 12: Resend email provider

**Files:**
- Create: `internal/adapter/resendmail/provider.go`, `internal/adapter/resendmail/provider_test.go`

**Interfaces:**
- Produces: `func New(apiKey, from string, httpClient *http.Client) *Provider` implementing `otp.EmailProvider`. `Send` POSTs to `https://api.resend.com/emails` with bearer auth; returns the Resend message id.

- [ ] **Step 1:** Write failing unit test with an `httptest.Server` stub asserting the request body/headers and returning a fake id.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement the HTTP call (inject base URL so the test points at the stub).
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5:** Commit `feat: resend email provider adapter`.

---

## Phase 3 — Services

### Task 13: otp-api (go-zero) — send/verify + API-key auth

**Files:**
- Create: `otp-api.api` (go-zero API DSL), generate with `goctl api go`, then hand-write `internal/transport/httpapi/sendlogic.go`, `verifylogic.go`, `internal/transport/httpapi/apikeymiddleware.go`, `cmd/otp-api/main.go`.

**Interfaces:**
- Consumes: `otp.Service`, `otp.Repo` (for `FindAPIKey`).
- Produces: HTTP `POST /v1/otp/send` → `202 {request_id}`; `POST /v1/otp/verify` → `200 {status:"verified"}` or `4xx`; plus three tenant-scoped read endpoints for the dashboard: `GET /v1/api-keys`, `GET /v1/otp/requests`, `GET /v1/delivery-logs`. Middleware resolves `Authorization: Bearer <api-key>` → hashes it → `Repo.FindAPIKey` → injects `tenant_id` into context.

- [ ] **Step 1:** Write the `.api` DSL (routes, request/response types) and an httptest-level test for `sendlogic` that injects a fake `otp.Service` and asserts a valid request yields `request_id` and a rate-limited one yields `429`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** `goctl api go -api otp-api.api -dir .`; implement the two logic files delegating to `otp.Service`; implement API-key middleware; map domain errors → HTTP codes (`ErrRateLimited/ErrCooldown`→429, `ErrCodeMismatch`→401, `ErrExpired/ErrNotFound`→410/404, `ErrTooManyAttempts`→429).
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5:** Commit `feat: otp-api http service with api-key auth`.

> The three read endpoints require adding list methods to the `otp.Repo` port and `pgrepo`: `ListAPIKeys(ctx, tenantID) ([]APIKey, error)`, `ListRequests(ctx, tenantID string, limit int) ([]OTPRequest, error)`, `ListDeliveryLogs(ctx, tenantID string, limit int) ([]DeliveryLog, error)`. Add these to `ports.go` (Task 5), implement in `pgrepo` (Task 9), and update the fakes. Do this as the first step of Task 13.

### Task 14: dispatcher — consume otp.requested, send email, log delivery

**Files:**
- Create: `internal/transport/dispatch/handler.go`, `internal/transport/dispatch/handler_test.go`, `cmd/dispatcher/main.go`

**Interfaces:**
- Consumes: `otp.EmailProvider`, `otp.Repo`, `otp.Publisher`.
- Produces: `func NewHandler(mail otp.EmailProvider, repo otp.Repo, pub otp.Publisher, tmpl Template) *Handler`; `func (h *Handler) Handle(ctx, raw []byte) error` — decodes the event, renders the email from `tmpl`, sends, writes a `DeliveryLog`, updates request state to `sent`/`failed`, publishes `otp.sent`/`otp.failed`; on send error publishes to `otp.dlq` (no drainer in MVP).

- [ ] **Step 1:** Write failing unit test with fake mail/repo/pub: a valid event triggers `mail.Send`, a `DeliveryLog` insert, state→`sent`, and an `otp.sent` publish; a failing `mail.Send` yields state→`failed` and an `otp.dlq` publish.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement `Handle` + a minimal `Template{Subject, BodyFmt}` renderer (`fmt.Sprintf(BodyFmt, code)`).
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5:** Commit `feat: dispatcher email delivery handler`.

---

## Phase 4 — Compose + end-to-end

### Task 15: docker-compose stack + APISIX gateway

**Files:**
- Create: `deploy/docker-compose.yml`, `deploy/apisix/apisix.yaml`, `deploy/apisix/routes.yaml`, `.env.example`

- [ ] **Step 1:** Write `docker-compose.yml` with services: `postgres`, `redis`, `redpanda` (Kafka API), `apisix`, `otp-api`, `dispatcher`. Wire env (`RESEND_API_KEY`, brokers, DSNs).
- [ ] **Step 2:** Configure APISIX standalone routes: route `/v1/otp/*` → `otp-api:8888`, enable `key-auth` plugin and `limit-count` (edge rate limit), and a `cors` plugin allowing the Vercel origin.
- [ ] **Step 3:** `docker compose up -d` then verify `curl localhost:9080/v1/otp/send` without a key returns `401` from APISIX.
- [ ] **Step 4:** Commit `chore: docker-compose stack with apisix gateway`.

### Task 16: End-to-end send→verify test

**Files:**
- Create: `test/e2e/e2e_test.go` (build tag `e2e`), `test/e2e/README.md`

- [ ] **Step 1:** Write an e2e test that (against a running compose stack + a seeded key from Task 17): calls `POST /v1/otp/send`, reads the delivered code from a **test email inbox** (use Resend's sandbox or a MailHog SMTP fallback for e2e), then calls `POST /v1/otp/verify` and asserts `200`. Also assert a wrong code returns `401` and exceeding attempts returns `429`.
- [ ] **Step 2:** Run `go test -tags=e2e ./test/e2e/ -v` → FAIL (until stack + seed exist).
- [ ] **Step 3:** Document the run steps in `test/e2e/README.md` (compose up, seed, run).
- [ ] **Step 4:** Once green, Commit `test: end-to-end send/verify through apisix`.

> **Delivery caveat:** for automated e2e, prefer a MailHog SMTP container so the test can read the code programmatically; keep Resend for real/manual verification. This keeps the pipeline deterministic without a real inbox.

---

## Phase 5 — Seed CLI + Dashboard

### Task 17: Seed CLI (tenant + API key)

**Files:**
- Create: `cmd/seed/main.go`, `cmd/seed/main_test.go`

**Interfaces:**
- Produces: `otp-seed --name "<tenant>"` prints a freshly generated **plaintext API key once** and stores only its hash (`otp.HashCode(key, "")` or a dedicated key-hash function) plus a new tenant row.

- [ ] **Step 1:** Write a unit test for the key generator: `GenerateAPIKey()` returns a 32+ char URL-safe token, and its stored hash verifies.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement `GenerateAPIKey` (crypto/rand, base64url) and a `main` that inserts tenant + api_key via `pgrepo`.
- [ ] **Step 4:** Run → PASS; manually run `go run ./cmd/seed --name demo` against compose Postgres and capture the key.
- [ ] **Step 5:** Commit `feat: seed CLI for tenant and api key`.

### Task 18: Next.js dashboard — 3 read-only screens

**Files:**
- Create: `dashboard/` (Next.js App Router, TS), from a shadcn dashboard block. Key files: `app/(dash)/api-keys/page.tsx`, `app/(dash)/history/page.tsx`, `app/(dash)/deliveries/page.tsx`, `lib/api.ts`, `lib/queries.ts`, `store/ui.ts`.

**Interfaces:**
- Consumes: `otp-api` read endpoints. **New read endpoints required in otp-api** (add to Task 13 scope if not present): `GET /v1/api-keys`, `GET /v1/otp/requests`, `GET /v1/delivery-logs` — all tenant-scoped by API key.

- [ ] **Step 1:** `npx create-next-app@latest dashboard --ts --app`; `npx shadcn@latest init`; add a dashboard block; add TanStack Query provider, Zustand store, TanStack Table, RHF+Zod.
- [ ] **Step 2:** `lib/api.ts` — a typed fetch wrapper reading `NEXT_PUBLIC_API_BASE` and the API key; `lib/queries.ts` — `useApiKeys`, `useRequests`, `useDeliveries` with `refetchInterval: 5000` on deliveries.
- [ ] **Step 3:** Build the three pages as TanStack Table views bound to the queries; server data stays in React Query, only filters/token live in Zustand (`store/ui.ts`).
- [ ] **Step 4:** Add a component test (Vitest + Testing Library) asserting the deliveries table renders rows from a mocked query and polls (advance timers, assert refetch).
- [ ] **Step 5:** Run `npm test` → PASS; `npm run build` → succeeds.
- [ ] **Step 6:** Commit `feat: next.js dashboard with three read-only screens`.

---

## Self-Review

**Spec coverage (vs 2026-08-07-otp-mvp-scope.md):**
- Email via Resend → Task 12. ✅
- otp-api + dispatcher (no worker/cron) → Tasks 13, 14. ✅
- APISIX (TLS off here / edge auth + rate limit + CORS) → Task 15. ✅
- Kafka topics requested/sent/failed (+dlq publish, no drainer) → Tasks 11, 14. ✅
- Redis hash+TTL, counters, cooldown, attempts → Tasks 6, 7, 10. ✅
- Postgres tables → Task 9. ✅
- Core domain (gen, hash-only, constant-time, 4-layer limit, attempt lock, idempotency, state) → Tasks 2–8. ✅
- Tenant/API key seed CLI → Task 17. ✅
- Dashboard 3 read-only screens (React Query polling, Zustand, RHF+Zod, TanStack Table) → Task 18. ✅
- Testing: unit (Phase 1), integration testcontainers (Phase 2), e2e send→verify (Task 16), 80% domain coverage (Task 8). ✅

**Gaps closed during review:** dashboard requires three GET read endpoints not in the original otp-api task — noted explicitly in Task 18 and must be folded into Task 13.

**Type consistency:** `CodeRecord{Hash,Salt,Attempts}`, `otp.Service` method signatures, and port interfaces are used identically across Tasks 5–14.
```
