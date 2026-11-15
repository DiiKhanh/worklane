# CI and isolated email OTP verification

Source: [issue #5](https://github.com/DiiKhanh/worklane/issues/5), part of
[verification milestone #3](https://github.com/DiiKhanh/worklane/issues/3).

## Result

Backend build, vet and the full default test suite passed locally. Dashboard
typechecking, lint, all 67 tests and the production build passed locally.
Existing automated tests demonstrated recipient/tenant rate limits, resend
cooldown and verification-attempt locking. The reported local-storage warning
did not reproduce with Node 22.23.5.

The latest completed CI run for the selected revision passed, including the
isolated full-stack email E2E. A **new full-stack execution during this session
remains blocked** because this host has no Docker executable. Existing CI
evidence is recorded separately and does not represent a new local E2E run.
Issue #5 remains incomplete pending that execution or an explicit decision to
accept the existing CI execution for the same revision.

## Revision, environment and boundaries

- Selected source revision: `9bff3eac88d68bd8f0f5564f53232235675a161e` on `main`.
- Local observation window: 2026-11-15 11:00-11:07 UTC.
- Host: Ubuntu 20.04.6 LTS, Linux x86_64; the same host also runs production.
  Local checks used in-process test fixtures, temporary SQLite databases and
  simulated external providers. No production database or API was targeted.
- Toolchain: Go 1.25.0, GNU Make 4.2.1, Zig 0.15.2 as the C compiler for CGO,
  Node 22.23.5, pnpm 10.26.2. Go, Make and Zig were unpacked into temporary
  user-owned directories; Node was already available outside PATH. No system
  installation, daemon, credential or access-control change was made.
- Dependencies: the existing Go module and dashboard lockfile. Local checks did
  not change source, tests, dependency manifests or lockfiles.
- The implementation-session decision was to run **only existing tests**.
  No new tests or application fixes were introduced. TDD additions therefore
  do not apply to this documentation-only change.
- No real notifications, production mutations, deployments, spending or
  external publication were performed. No CI rerun was requested. SMS delivery
  to a handset remains deferred; existing SMS unit tests were still executed.
- Local checks did not need a real account or mailbox. The existing CI smoke
  test seeds a separate random tenant/API key in its disposable identity DB
  and delivers to MailHog through SMTP. No real provider is used for that flow.
  No credentials, OTP values or sensitive payloads are recorded here.

The report commit changes documentation only. The revision above identifies
the exact application and test source checked, rather than implying that a
future report commit has already run in CI.

## CI evidence

GitHub run metadata and the smoke-job logs were read on 2026-11-15. Both runs
below report `completed/success` for `test`, `smoke` and all four image-build
jobs. The authoritative workflow is
[images.yml](../../.github/workflows/images.yml).

| Run | Commit | UTC execution window | Result |
| --- | --- | --- | --- |
| [41648898724](https://github.com/DiiKhanh/worklane/actions/runs/41648898724) | `7bac0d1a3434dafb1356369abde20ff27e8fdbe4` | 2026-11-03 13:56:42-14:01:20 | Success; only run returned for the original review date |
| [43619736197](https://github.com/DiiKhanh/worklane/actions/runs/43619736197) | `9bff3eac88d68bd8f0f5564f53232235675a161e` | 2026-11-14 17:11:20-17:15:30 | Success; latest run at inspection, exact selected revision |

The original review file referenced by #3 is not tracked in this checkout.
Issue #3 supplies the review date but no run ID. Run 41648898724 is the matching
date candidate; the exact historical association cannot be proved from that
snapshot alone. Its final result is now confirmed, rather than left pending.

The selected revision's
[backend test job](https://github.com/DiiKhanh/worklane/actions/runs/43619736197/job/132522457340)
ran `go test ./...` successfully. Its
[smoke job](https://github.com/DiiKhanh/worklane/actions/runs/43619736197/job/132522457924)
built the Compose stack, generated throwaway JWT keys, waited for readiness,
ran `go test -tags e2e ./test/e2e/...` and tore down successfully. The smoke
command completed at 2026-11-14 17:13:43 UTC with package result `ok` (2.026s).
The historical smoke command also reported `ok` (0.525s) at
2026-11-03 13:59:41 UTC.

The existing [E2E tests](../../test/e2e/e2e_test.go) assert send acceptance,
email capture in MailHog, rejection of a wrong code, acceptance of the delivered
code, and rejection without an API key. This is full-stack backend email
evidence through Traefik, not a browser/dashboard E2E or production result.
The workflow contains no dashboard check job; dashboard results below are local.

## Local checks

All commands used the selected revision, with the toolchain on PATH. Backend
CGO checks used `CGO_ENABLED=1` and `CC='zig cc'`; `make test` and `go vet`
used `GOMAXPROCS=2`. No production environment file was loaded.

| UTC time | Command | Result |
| --- | --- | --- |
| 11:02 | `cd dashboard && pnpm typecheck` | Pass |
| 11:02 | `cd dashboard && pnpm lint` | Pass |
| 11:02 | `cd dashboard && pnpm test lib/api/auth.test.ts` | 1 file, 2 tests passed |
| 11:03 | `cd dashboard && pnpm test` | 26 files, 67 tests passed; no local-storage warning |
| 11:02-11:04 | `go build ./...` | Pass |
| 11:04-11:05 | `make test` | Full default backend suite passed, including SMS unit tests |
| 11:05 | `cd dashboard && pnpm test components/shell/auth-guard.test.tsx` | 1 file, 2 tests passed; exercises `localStorage.clear()` |
| 11:05-11:06 | `cd dashboard && pnpm build` | Pass; all 12 static pages generated |
| 11:05-11:06 | `go vet ./...` | Pass |
| 11:06 | `go test -tags=e2e ./test/e2e/ -list '^TestE2E_'` | Both existing E2E tests compile and are listed; assertions were not executed |

The focused OTP command passed at 11:02 and again at 11:04 UTC:

```bash
go test ./services/otp-api/internal/app \
  -run 'TestVerify_WrongCode_LocksAfterMax|TestSend_RateLimited|TestRateLimiter' \
  -count=1 -v
```

It covers the following existing behavior:

- `TestRateLimiter_PerRecipient`: three permitted sends, then rate rejection.
- `TestRateLimiter_PerTenant`: two recipients consume the tenant quota; a third
  recipient is rejected.
- `TestRateLimiter_Cooldown`: the immediate resend is rejected.
- `TestSend_RateLimited`: the Send use case rejects a second send with a
  configured per-recipient maximum of one.
- `TestVerify_WrongCode_LocksAfterMax`: three mismatches are permitted; the next
  verification is rejected with the attempt-limit error.

These tests use in-memory store/counter ports. They demonstrate sequential
policy behavior, not live Redis expiry, concurrent requests or end-to-end HTTP
protection enforcement. The default backend suite excludes `integration` and
`e2e` build tags; their absence is not counted as a pass.

The lock test and email E2E use a fixed wrong-code fixture that can, with low
probability, equal a generated OTP. A targeted existing-test stress run
(`go test ./services/otp-api/internal/app -run '^TestVerify_WrongCode_LocksAfterMax$'
-count=1000000 -timeout=90s`) passed in 41.993s at 11:03-11:04 UTC. No failure was
reproduced. The potential fixture collision remains a limitation; no test was
changed under the existing-tests-only decision.

## Local-storage warning

The #3 snapshot mentions a `localstorage-file` warning but does not preserve the
original Node version, flags or stack trace. The focused auth tests, auth guard
tests and complete dashboard suite emitted no such warning under Node 22.23.5.
The existing [test setup](../../dashboard/test/setup.ts) checks whether
`window.localStorage.clear` exists and supplies an in-memory Storage fallback
when necessary. The auth guard tests call `localStorage.clear()` successfully.

There is no demonstrated reliability failure in this environment. That does
not establish the historical warning's cause or behavior under another Node
runtime. No warnings were suppressed and no test-setup change was made.

## Remaining blocker and completion status

`docker version` could not start (`docker: command not found`). No isolated
Docker target was supplied for a new stack. The default Compose gateway uses
host port 80 and this host runs production, so an unmodified local Compose
startup is not an isolated verification plan. No stack was started and no E2E
request was sent to this host's default API or database ports.

The existing CI workflow publishes container images after successful test and
smoke jobs. Rerunning it was not used as a workaround because external
publication requires specific approval. A new execution needs an existing
isolated Docker environment, or approval for a concrete alternative. The
existing [E2E instructions](../../test/e2e/README.md) and workflow define the
test; execution must keep SMTP/MailHog, a separate test tenant and disposable
databases, and avoid production ports and provider credentials.

- CI outcomes, URLs and commits: recorded, with the historical association
  limitation stated explicitly.
- Backend and dashboard checks: passed locally.
- Rate limits and attempt locking: demonstrated by existing automated tests,
  with infrastructure/concurrency limitations stated.
- Local-storage warning: not reproduced; no reliability failure demonstrated.
- Blocking application failures requiring fixes: none reproduced.
- New isolated full-stack email E2E: blocked; existing same-revision CI success
  recorded separately. Final ticket sign-off remains pending.
- No GitHub issue edits, closure, pushes or PR publication were performed.
