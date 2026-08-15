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

