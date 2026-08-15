# Go & Architecture Learning Notes

A running log of the non-obvious concepts we hit while building this project, with the **why**
behind each choice. Written for someone new to Go. Newest phase at the bottom. Read it alongside the
code; every entry points at a real file.

> How to use this: when you see a `[why:go-notes#anchor]` comment in the code, the matching section
> here explains it in depth.

---

## Phase 1 - core domain (otp-api)

### 1. Packages, folders, and `internal/`

- In Go, **a folder is a package**. Every `.go` file in `services/otp-api/internal/domain/` starts
  with `package domain`. The folder name and package name usually match (not required, but do it).
- **`internal/` is a hard rule enforced by the compiler.** Code under any `.../internal/...` path can
  only be imported by packages that share the parent of that `internal/` folder. So
  `services/otp-api/internal/...` is importable only by `otp-api`, never by `otp-dispatcher`. That is
  how we get real microservice isolation inside one repository - not discipline, the compiler refuses
  to compile a violation. (See the architecture reference doc §4.)
- `pkg/` by convention holds code meant to be shared (no `internal/`), so any service can import
  `pkg/contracts/otp` and `pkg/platform/*`.

### 2. Why the domain imports *nothing* (ports & adapters)

- `internal/domain` and `internal/app` contain the business rules. They import only the standard
  library, each other, and the shared `pkg/contracts`. They must **never** import Redis, GORM, Gin,
  Kafka, etc.
- Reason: business logic that does not know about infrastructure is (a) trivially unit-testable with
  fakes, (b) unaffected when you swap Redis for something else, (c) readable without wading through
  driver details. This is the core idea of **hexagonal architecture (ports & adapters)**.
- The seam is an **interface** (a "port"). See §4.

### 3. `crypto/rand` vs `math/rand` (`code.go`)

- `math/rand` is a *pseudo*-random generator: fast, but deterministic. Given some outputs an attacker
  can predict the next ones. Fine for shuffling, **fatal for security tokens**.
- `crypto/rand` reads from the operating system's cryptographically secure RNG. Its output is
  unpredictable, which is what an OTP needs.
- We call `rand.Int(rand.Reader, max)` to get a uniform integer in `[0, max)`. It also avoids "modulo
  bias" (the subtle skew you get from `bigRandom % 10^n`), which a naive implementation would have.
- `fmt.Sprintf("%0*d", length, n)` zero-pads: the `*` takes the width from an argument (`length`), so
  a small number like `42` becomes `"000042"` for length 6.

### 4. Interfaces + dependency injection + fakes (`ports.go`, tests)

- A Go **interface** lists method signatures. Any type with those methods satisfies it automatically -
  there is no `implements` keyword ("structural typing" / duck typing).
- The application defines the interfaces it *needs* (`CodeStore`, `Counter`, `Repo`, `Publisher`,
  `Clock`) in `ports.go`. Real adapters (Redis, MySQL...) will implement them later; in tests we pass
  tiny in-memory **fakes** (`fakeStore`, `fakeCounter`, `fakeRepo`).
- This is **dependency injection**: the use case receives its dependencies (via the `Deps` struct)
  instead of constructing them. That is what lets the same `Service` run against Redis in production
  and a `map` in a test.
- Rule of thumb you will hear in Go: **"define interfaces where they are used, not where they are
  implemented."** The consumer (`app`) owns the interface; the adapter just satisfies it.

### 5. Constant-time comparison (`hash.go`)

- We store only a **salted SHA-256 hash** of the code, never the code itself. Even a leaked database
  cannot reveal active codes.
- Verifying uses `subtle.ConstantTimeCompare`, not `==`. A normal string compare returns as soon as it
  finds the first differing byte, so the *time it takes* leaks how many leading characters were
  correct - a real attacker can measure this and guess a token character by character. Constant-time
  compare always takes the same time. This "timing side channel" is a classic security bug.
- The **salt** is a per-request random value (we reuse the request id). It means two requests for the
  same code produce different hashes, defeating precomputed "rainbow table" attacks on the tiny
  6-digit space.

### 6. Sentinel errors (`errors.go`)

- Go handles errors as values, not exceptions. We declare package-level `error` values
  (`ErrRateLimited`, `ErrCodeMismatch`, ...) once and return them.
- Callers compare with `errors.Is(err, domain.ErrCodeMismatch)` (or `==` for these simple ones). The
  HTTP adapter will map each sentinel to a status code (429/401/410) in one place. Returning typed
  sentinels instead of `errors.New("...")` strings everywhere keeps that mapping reliable.

### 7. A named string type for state (`state.go`)

- `type State string` creates a distinct type whose underlying type is `string`. You get a real type
  (methods, type safety) while the value is still just a string on the wire and in MySQL.
- We attach the state machine as a method: `func (s State) CanTransition(to State) bool`. Methods can
  only be defined in the same package that declares the type - that is why the FSM lives next to the
  `State` definition.
- The allowed transitions are a `map[State]map[State]bool`. A missing entry reads as `false` (Go
  returns the zero value for absent map keys), so "not listed" automatically means "forbidden".

### 8. Application layer vs pure domain (`ratelimit.go`, `send.go`, `verify.go`)

- **Pure domain** = zero I/O, pure functions/values: `GenerateCode`, `HashCode`, `State`,
  `MaskRecipient`. You can test them with no setup at all.
- **Application layer** = *orchestration* that calls ports: rate limiting (needs the `Counter`), and
  the `Send`/`Verify` use cases (need store/repo/publisher/clock). It depends on interfaces, still no
  concrete infrastructure.
- Why split them: it keeps the innermost rules dependency-free and makes the "what talks to Redis"
  boundary obvious. Rate limiting *looks* like a rule but it reads/writes counters, so it belongs in
  `app`, not `domain`.

### 9. Table-driven tests & TDD rhythm

- Idiomatic Go tests loop over a slice/map of cases (see `TestGenerateCode`, `TestMaskRecipient`).
  One test function, many inputs - easy to add a case.
- We work **test-first (TDD)**: write the test, run it and *watch it fail for the right reason*
  (`undefined: GenerateCode`), then write the minimal code to pass. Watching it fail proves the test
  actually exercises the new behavior. `t.Fatalf` stops the test immediately with a message.

### 10. Idempotency (`send.go`)

- Networks retry. If a client's `POST /send` times out and it retries, you must not send two emails.
  An **Idempotency-Key** lets the client say "this is the same logical request." We store the key in
  Redis with a TTL; a repeat within the window returns the original request id and does **not**
  re-publish. This is a standard distributed-systems pattern (Stripe's API works this way).

### 11. The verify-attempt lock, and why the guard is *before* the hash check (`verify.go`)

- To resist brute force we cap wrong guesses (`MaxVerifyAttempts`). Subtlety we discovered from a
  failing test: "3 attempts" should mean the 3rd wrong guess still returns *mismatch* (it was a valid
  try), and only the **next** call is locked.
- Crucially the lock guard runs **before** the hash comparison. So once locked, even the *correct*
  code is rejected - an attacker cannot slip a lucky final guess past the cap. Order of checks is a
  security property, not a style choice.
- This is a case where **the test drove a real design decision** (off-by-one in the lock semantics).
  If we had written the code first we might never have questioned it.

### 12. Executable architecture rules (`arch/arch_test.go`)

- Rules that live only in a README rot. We encode "domain and app must not import infrastructure" as a
  **test** that parses the source files' imports and fails if it sees `redis`, `gorm`, `gin`,
  `sarama`, `/adapters/`, etc.
- `go/parser` + `go/token` are standard-library tools for reading Go source as data (an AST). Here we
  only need `parser.ImportsOnly` - fast, no full type-checking.
- Now the hexagon boundary is enforced by CI. A teammate (or future you) who accidentally couples the
  domain to a driver gets a red build instead of a silent architecture erosion.

### 13. Coverage as a gate

- `make cover` runs the domain/app tests with `-coverprofile` and prints the percent. We target 80%.
  Right now: domain 93.8%, overall 85.9%. Coverage is a floor, not a goal - 100% of trivial getters
  proves little; the point is that the *rules* (limits, lock, idempotency, transitions) are all
  exercised.

---

## Phase 2 - adapters (talking to real infrastructure)

### 14. Build tags: separating fast unit tests from slow integration tests

- The first line `//go:build integration` (with a blank line after) is a **build constraint**. Files
  with it compile *only* when you pass `-tags=integration`. So `go test ./...` (what CI runs on every
  save) stays fast and needs no Docker; `go test -tags=integration ./...` runs the heavy ones.
- Why: adapter tests spin up real MySQL/Redis/Kafka - each takes seconds. You do not want that on
  every keystroke, but you *do* want it before merging. Tags give you both.

### 15. testcontainers: real infrastructure, disposable, per-test

- Instead of mocking Redis/MySQL/Kafka (which proves nothing about real SQL/commands), we boot the
  **actual** service in a throwaway Docker container, run the adapter against it, then destroy it.
- `tcmysql.Run(ctx, "mysql:8.0", ...)` starts a container; `ctr.ConnectionString(ctx, ...)` gives a
  DSN; `t.Cleanup(func(){ ctr.Terminate(ctx) })` guarantees teardown even if the test fails.
- This is the honest way to test an adapter: it catches real bugs (wrong column name, a Redis command
  that behaves unexpectedly, a migration that will not apply) that a mock would hide.

### 16. Structural typing proves an adapter satisfies a port - at compile time

- `var _ app.Repo = (*mysqlrepo.Repo)(nil)` is a throwaway variable whose only job is to make the
  compiler check that `*Repo` implements `app.Repo`. If a method is missing or has the wrong
  signature, the build fails - you find out immediately, not at runtime.
- Note the adapter never says `implements app.Repo`. Go has no such keyword: having the right methods
  *is* implementing the interface. The `var _ =` line is how you assert that on purpose.
- The Kafka `Producer` is a neat case: it lives in `pkg/` and cannot import `app` (the `internal/`
  rule forbids it), yet it still satisfies `app.Publisher` just by having a matching `Publish` method.
  Structural typing lets a shared library satisfy a service-private interface without any coupling.

### 17. GORM basics and why migrations, not AutoMigrate

- A GORM "model" is a struct whose fields map to columns; `func (T) TableName() string` pins the
  table name. `db.WithContext(ctx).Create(&row)` / `.Where(...).First(&row)` / `.Model(...).Update(...)`
  are the CRUD verbs. GORM builds parameterized SQL, so no SQL-injection risk from `?` placeholders.
- We keep these row structs **private** to the adapter and map them to `app` types at the boundary.
  The app never sees a GORM struct - swapping ORMs later touches only this package.
- We drive schema with explicit SQL files via `golang-migrate`, **not** GORM's `AutoMigrate`.
  AutoMigrate infers schema from structs and silently alters tables - fine for a toy, dangerous in
  production (no review, no down-migration, surprising column changes). Real migrations are versioned,
  reviewable, and reversible.

### 18. Redis rolling window: INCR then EXPIRE only on the first hit (`redisstore/store.go`)

- Rate limiting uses `INCR key` (atomic increment, returns the new value). We call `EXPIRE key ttl`
  **only when the value is 1** - i.e. the first request in a window. That fixes the window to start at
  the first request and roll off as a whole. If we re-set the TTL on every INCR, a steady stream of
  requests would keep pushing expiry out and the limit would never reset.
- Missing key: go-redis returns a sentinel `redis.Nil`. The adapter translates it to
  `domain.ErrNotFound` so the app layer never learns that Redis exists.

### 19. Kafka: typed envelope, partition keys, and at-least-once (`pkg/platform/kafka`)

- Every message is wrapped in an `Envelope{MsgType, Data}`. `MsgType` (a discriminator like
  `"otp.RequestedEvent"`) lets a consumer that handles several event types switch before decoding.
  This mirrors the production convention.
- **Partition key:** events with a `PartitionKey()` method (our `RequestedEvent` returns its
  RequestID) are keyed so all messages for one request go to the same partition and stay ordered.
  Kafka only guarantees order *within* a partition.
- **Consumer group + at-least-once:** the handler runs, and only on success do we `MarkMessage`
  (advance the offset). If the handler fails we do **not** mark, so the message is redelivered. This
  is "at-least-once" delivery - the reason every consumer must be **idempotent** (safe to process the
  same message twice). It is a core distributed-systems tradeoff: at-least-once (possible duplicates)
  vs at-most-once (possible loss); we choose no-loss and handle duplicates.

### 20. Testing an HTTP client with httptest (`resendmail/provider_test.go`)

- To test code that calls an external API (Resend) without hitting the network, we start a local
  `httptest.NewServer` that plays the API, and inject its URL into the adapter (`baseURL`). The test
  asserts the request we send (method, path, `Authorization` header, body) and controls the response.
- Injecting `baseURL` and the `*http.Client` is dependency injection again - the same trick that made
  the domain testable, applied to an outbound HTTP call.

---

## Phase 3 - services (Gin + the composition root)

### 21. The inbound port is defined where it is used (`http/handlers.go`)

- The HTTP adapter declares `type OTPService interface { Send...; Verify... }` and depends on *that*,
  not on the concrete `*app.Service`. In `main.go` we pass the real service; in tests we pass a
  `fakeSvc`. Same inversion as the outbound ports, now on the driving side.
- This is why the handler test needs no Redis/DB/Kafka at all - it swaps the whole use case for a fake
  and checks only the HTTP translation (status codes, JSON shape).

### 22. Gin middleware and request-scoped context (`http/middleware.go`)

- A middleware is a function run before the handler. `apiKeyAuth` reads the `Authorization` header,
  hashes the key, looks up the tenant, and either `c.AbortWithStatusJSON(401, ...)` (stop here) or
  `c.Set("tenant_id", ...)` + `c.Next()` (continue). Handlers later read `c.GetString("tenant_id")`.
- Passing the tenant via the request context (not a global) keeps each request isolated - vital when
  many requests run concurrently. We do auth in the service, not the gateway, because Traefik has no
  key-auth plugin; bonus: the logic is visible and unit-tested.

### 23. One place maps errors to HTTP status (`http/errors.go`)

- `statusFor(err)` is the single switch translating domain sentinels to codes: rate/cooldown/attempts
  -> 429, mismatch -> 401, not-found/expired -> 410, anything else -> 500. Handlers just call
  `writeError(c, err)`.
- Centralizing this means the use cases stay transport-ignorant (they return `domain.ErrRateLimited`,
  not `429`) and there is exactly one place to audit the mapping. Note we scrub the message on 500 so
  an unexpected internal error never leaks its detail to the client.

### 24. The composition root (`services/*/main.go`)

- This is the only file that imports concrete adapters *and* the app together. It reads config, opens
  MySQL/Redis/Kafka, constructs the adapters, injects them into `app.New(...)`, and starts the server.
  Everywhere else depends on interfaces; here we finally pick the real implementations.
- Because wiring lives in one auditable place, you can read `main.go` top-to-bottom and see the whole
  system's shape. No hidden globals, no service locator.
- **Graceful shutdown:** we run the server in a goroutine and block on an OS signal (`SIGINT`/`SIGTERM`).
  On signal, `srv.Shutdown(ctx)` stops accepting new requests and lets in-flight ones finish within a
  timeout. This is standard for a production service - a hard `os.Exit` would cut active requests.

### 25. Two services, two repos - on purpose

- `otp-dispatcher` has its *own* `mysqlrepo` (just `InsertDeliveryLog` + `UpdateState`), separate from
  otp-api's. They cannot share it: Go's `internal/` rule forbids one service importing another's
  internals, and that is exactly the microservice boundary we want.
- The small duplication (a row struct, a `TableName`) is the price of independence. What they *do*
  share is the database schema (one set of migrations) and the Kafka event contract - the two seams
  that are meant to be shared. This is the pragmatic "shared database" MVP simplification; a stricter
  design would give each service its own tables.

