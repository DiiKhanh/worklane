# Link Service MVP tickets

Date: 2026-11-15.
Status: published on GitHub after owner confirmation and publication approval.
Parent specification and tracker: [#11](https://github.com/DiiKhanh/worklane/issues/11).

GitHub issues are the authoritative tracker for subsequent ticket changes.
The work packages below preserve the approved publication baseline.

## Shared scope

The [confirmed MVP scope](2026-11-15-link-service-mvp-scope.md) is authoritative,
including its API, privacy, failure, quota and acceptance requirements. Preserve
the [glossary](../../GLOSSARY.md), [ADR 0001](../adr/0001-link-redirects-and-best-effort-analytics.md)
and existing hexagonal/service conventions. The original August design is a
baseline only where it does not conflict with the confirmed MVP.

Each implementation ticket includes relevant tests, repository checks and diff
review. Public API and browser behavior are the preferred regression boundaries;
integration tests use real isolated dependencies. The owner separately authorized
GitHub publication and the supporting documentation commit. Code implementation
and production mutations/rollout are outside this publication task.
Backup/restore, real SMS and all other deferred scope remain excluded.

## Delivery order

| Work package | GitHub issue | Outcome | Blocked by |
| --- | --- | --- | --- |
| L1 | [#12](https://github.com/DiiKhanh/worklane/issues/12) | Shared contracts, storage schema and unique code generation | None |
| L2 | [#13](https://github.com/DiiKhanh/worklane/issues/13) | Tenant management API and permanent disable | #12 |
| L3 | [#14](https://github.com/DiiKhanh/worklane/issues/14) | Public redirects, bounded cache validity and async click emission | #13 |
| L4 | [#15](https://github.com/DiiKhanh/worklane/issues/15) | Click persistence, retention and analytics API | #13 |
| L5 | [#16](https://github.com/DiiKhanh/worklane/issues/16) | Live Links dashboard | #14, #15 |
| L6 | [#17](https://github.com/DiiKhanh/worklane/issues/17) | Isolated full-stack integration and MVP verification | #16 |

L3 and L4 can proceed independently after L2. L1-L6 are local work-package
identifiers; the table maps them to published issue numbers. Native GitHub
sub-issue and dependency relationships are configured. A ticket is unblocked
only when all of its blockers are closed; the ready-for-agent label means fully
specified, not necessarily unblocked.

## L1: Define Link contracts, schema and unique code generation

Establish the shared boundaries consumed by HTTP, analytics and dashboard work
without implementing the public workflows yet.

- [ ] Define API request/response/error shapes for every confirmed route,
  including metadata separate from analytics, cursor pagination, active/disabled
  state, timestamps and retryable/unavailable errors. Follow existing repository
  error conventions; do not expand product behavior.
- [ ] Define the link.clicked contract under pkg/contracts with code, tenant,
  event timestamp, sanitized referrer origin and device category. Exclude raw
  referrer, User-Agent and IP/IP hash.
- [ ] Add service-owned MySQL migrations for immutable mappings, permanent
  disabled state, retained visit details and daily aggregates. Enforce exact
  tenant dedup and case-sensitive Base62 code identity with appropriate indexes.
- [ ] Implement the existing Snowflake-to-Base62 design with unique node IDs,
  explicit clock/sequence failure handling and no duplicate issuance.
- [ ] Verify Base62 vectors/round-trips/case, concurrent generation under the
  race detector and schema constraints against isolated MySQL.

## L2: Implement tenant Link management and permanent disable

Expose create, paginated list, metadata detail and permanent disable through
link-svc with existing JWT/API-key authentication.

- [ ] Validate exact HTTP/HTTPS destination strings and the 4,096-byte limit;
  reject credentials, control characters, localhost, literal non-public IPv4/
  IPv6 and the configured short host. Do not fetch or resolve destinations.
- [ ] Return 201 for creation and 200 for active dedup. Concurrent identical
  submissions in one tenant return one code; different tenants remain separate.
  Verify original-string equality on hash-based dedup.
- [ ] Restrict all management operations to the authenticated tenant. Foreign
  codes appear absent. Provide stable cursor pagination with 20 default and
  100 maximum entries, newest first.
- [ ] Durably disable before success; repeated disable remains successful.
  Attempt cache eviction through the cache port. Disabled-destination creation
  returns 409 LINK_DISABLED without reactivation or a replacement code.
- [ ] Apply an atomic configurable create quota, default 10/minute/tenant,
  including dedup. Return 429 with Retry-After when exceeded and 503 when quota
  storage is unavailable.
- [ ] Build short_url from configured LINK_PUBLIC_BASE and expose health using
  repository conventions. Preserve domain/app dependency rules.
- [ ] Verify these behaviors through public HTTP tests and real MySQL/Redis
  integration, including concurrent dedup and tenant isolation.

## L3: Implement public redirects and bounded async click emission

Serve public GET/HEAD while keeping Kafka out of the redirect response path and
enforcing the permanent-disable deadline under cache races and outages.

- [ ] Return 302 with the exact accepted Location for active links, 410 for
  disabled, 404 only for confirmed missing and 503 when resolution is unavailable.
  HEAD follows the same resolution without a response body or click emission.
- [ ] Cache destination, tenant, state and validity deadline. Limit cache age to
  60 seconds from authoritative read start; hits and late writes never extend
  that deadline. Apply the same rule to create warming.
- [ ] Fall back to MySQL when Redis fails. During MySQL failure, only unexpired
  cached state may serve; expired data cannot resurrect a disabled link.
- [ ] Emit only for active-link GET responses returning 302. Sanitize referrer
  to an HTTP/HTTPS origin and classify device before queueing. No raw metadata
  reaches Kafka or logs.
- [ ] Use bounded async admission/workers with bounded shutdown, safe failure
  reporting and dropped-event accounting. Kafka failure/full queue may lose
  events but cannot delay redirects waiting for Kafka.
- [ ] Send Cache-Control: no-store; avoid caching absent/disabled responses
  downstream. Define trusted ingress client-IP handling for L6 routing.
- [ ] Verify GET/HEAD/status semantics and dependency failures through HTTP.
  Reproduce concurrent read/disable/stale-refill scenarios and prove that the
  disable deadline holds, including DB outage.

## L4: Implement click persistence, retention and analytics API

Consume link.clicked in link-dispatcher and expose tenant-scoped analytics
without requiring analytics availability for metadata/copy workflows.

- [ ] Validate events and persist each aggregate increment plus any retained
  detail atomically; commit Kafka progress only after persistence. Preserve the
  accepted at-least-once duplicate-counting semantics.
- [ ] Keep daily aggregates for link lifetime. Expire detailed visits after
  30 days without reducing totals; delayed delivery must not recreate expired
  details. Bound Kafka event retention within the agreed limit.
- [ ] Return lifetime total, 14 UTC buckets including today with missing dates
  filled with zero, and the newest 20 retained visits.
- [ ] Restrict analytics to the authenticated tenant. A disabled link retains
  its analytics; a foreign tenant's code appears absent.
- [ ] Keep unavailable analytics distinct from a real zero. Preserve the
  10-second normal-operation visibility target without an outage guarantee.
- [ ] Verify Kafka-to-MySQL ingestion, transactional failure/redelivery behavior,
  UTC boundaries, late events, detail expiration, totals and ownership using
  isolated dependencies. Verify only sanitized metadata is retained.

## L5: Replace the Links fixture screen with live workflows

Connect the existing Links screen through the established DataSource/query
patterns to the confirmed API and configured public base.

- [ ] Implement create, copy, paginated list, metadata detail and analytics with
  manual refresh. Use returned short URLs rather than fictional fixture hosts.
- [ ] Show active/disabled state and confirm permanent disable before submission.
  Reflect successful disable and display disabled-dedup conflicts clearly.
- [ ] Remove fixture-only CTR, geography, message attribution and unsupported
  KPI values. Do not present sample data in the live experience.
- [ ] Render loading, empty, unavailable and retry states. Analytics errors do
  not display zero or prevent independently available metadata/copy operations.
- [ ] Scope query caches to tenant identity and prevent data from a prior tenant
  appearing after logout/login.
- [ ] Verify real browser flows, keyboard interactions, copy behavior, pagination
  and failure states; inspect the rendered UI for visual quality. Run dashboard
  typecheck, lint and relevant tests through project scripts.

## L6: Integrate the isolated stack and verify the complete MVP

Package the two services for isolated full-stack operation, extend repository
verification entry points/CI and record evidence against the confirmed scope.
This ticket prepares local/test runtime configuration, not production rollout.

- [ ] Wire images/builds, isolated MySQL database initialization, Redis, Kafka,
  both Link processes and the dashboard into the repository's existing stack
  and test conventions. Keep credentials external and avoid production services.
- [ ] Configure the isolated public redirect router with trusted-IP rate limiting
  at 100 requests/second, burst 50; verify forged forwarded headers cannot select
  a different client bucket.
- [ ] Add a full-stack browser/API check: create, copy, GET redirect, analytics
  within 10 seconds, permanent disable, 410 within 60 seconds and 409 on recreate.
  Include HEAD no-click and independent metadata availability.
- [ ] Verify cross-tenant isolation, concurrent dedup, cache-refill/disable races,
  create quota behavior and Redis/MySQL/Kafka outages plus queue saturation.
- [ ] Verify sanitization before publication and retention without aggregate
  loss. Reuse meaningful feature-level coverage rather than duplicate every test.
- [ ] Run 50 redirects/second for five minutes with a warm cache and no fault
  injection. Record environment, achieved rate, latency, errors and drop counts;
  do not claim a production SLA or exactly-once analytics.
- [ ] Run required repository checks and inspect the real dashboard. Record
  results, limitations and any failures faithfully. Keep fixes within approved
  scope; report material expansions for a separate decision.
- [ ] Confirm that no production rollout, DNS mutation, real SMS or backup/restore
  work occurred. Record remaining production rollout as a separate future step.

## Completion boundary

MVP implementation is complete only when L1-L6 acceptance and the authoritative
scope's checks pass. Drafting or closing tickets alone is not verification.
Production readiness is not inferred from isolated test results. GitHub
publication was separately authorized after scope confirmation.
