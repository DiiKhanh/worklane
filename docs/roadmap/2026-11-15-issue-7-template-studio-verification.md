# Template Studio production verification

Source: [issue #7](https://github.com/DiiKhanh/worklane/issues/7), part of
[verification milestone #3](https://github.com/DiiKhanh/worklane/issues/3).
Prerequisite [#6](https://github.com/DiiKhanh/worklane/issues/6) was closed at
inspection.

## Latest status

The operator subsequently approved upgrading the backend. The
[production upgrade](2026-11-15-issue-7-backend-upgrade.md) completed at
12:47 UTC with both OTP services on `14f2158`, schema version 4 clean, and the
template route returning HTTP 401 rather than 404. The deployment blocker below
is historical. Issue #7 remains incomplete pending dashboard verification,
test-content approval, mailbox receipt and restoration evidence.

## Initial result (12:39 UTC)

**Issue #7 is incomplete and blocked by the deployed backend.** At
2026-11-15 12:39:05 UTC, the public API returned HTTP 404 and plain text
`404 page not found` for `GET /v1/templates`. The control probe `GET /v1/stats`
returned HTTP 401 with `missing credentials`, demonstrating that the API was
reachable and distinguishing the absent template route from authentication.

Read-only Kubernetes inspection during 12:39-12:40 UTC confirmed that both OTP
services still run `sha-041efd7`, before Template Studio. This reproduces the
[deployment mismatch recorded for #4](2026-11-14-issue-4-production-verification.md).
The deployed revision's router has no template endpoints, and its dispatcher
predates DB-backed template resolution. A dashboard-only change cannot make
publishing control email delivery on those artifacts.

No application code, configuration, credentials or production data changed.
No template was created, edited or published; no email or SMS was sent. No
API-only custom locale was used as dashboard evidence. The issue was not edited
or closed. Existing unrelated untracked roadmap documents were left untouched.

## Environment and reproduction

- Date: 2026-11-15; all times are UTC.
- API: `https://api-otp.dikhanh.io.vn`.
- Dashboard designated by #6: `https://worklane-six.vercel.app`. No authenticated
  dashboard interaction was performed in this inspection.
- Local baseline: `14f2158340608a5caae9f7b53ffc85f0395c7834`, branch `main`.
- Tag-associated backend source:
  `041efd743b3a16c07df296983b9981ee7d26c51a`.
- All three application deployments reported one Ready replica.

| Service | Image tag | Running image digest |
| --- | --- | --- |
| auth-svc | `sha-041efd7` | `sha256:295cb62a76d5c3d6f6187b982ffc34d94be4d05770fc2cc8bb14d6d771dffb53` |
| otp-api | `sha-041efd7` | `sha256:ef1d6eb7200a094763a8280f05ed2d693ab5211c89f120d7d2ce51ef50497c6f` |
| otp-dispatcher | `sha-041efd7` | `sha256:9e36fcad357ea07258ae78cef68fe4978aff12b1f68ce3683f8626208e982a7e` |

The failure was reproduced at the public API used by the dashboard. A logged-in
browser session was unnecessary to establish the missing route: the public
404, running artifacts and deployed router agree. This does not establish any
dashboard authoring, preview, publish or mailbox-receipt acceptance criterion.

Read-only reproduction:

```sh
date -u +%FT%TZ
curl -sS -i --max-time 20 https://api-otp.dikhanh.io.vn/v1/templates
curl -sS -i --max-time 20 https://api-otp.dikhanh.io.vn/v1/stats
kubectl -n worklane get deployments auth-svc otp-api otp-dispatcher
```

Cluster access used the existing user kubeconfig. No secret objects, database
contents or mailbox messages were read. No authenticated inventory is claimed.

## Upgrade boundary

A coordinated otp-api and otp-dispatcher upgrade is required before the selected
verification can proceed. This implementation request does not authorize that
production mutation. An approval proposal must identify immutable target images,
schema state, affected data, preservation and recovery steps, and service health
checks before execution.

The current source runs migrations at otp-api startup
([main.go](../../services/otp-api/main.go)). In particular,
[migration 0004](../../db/otp/migrations/0004_template_studio.up.sql):

- Drops the legacy `templates` table and creates the versioned template schema.
  Its comment claiming the old table is empty is not a production observation;
  existing rows and migration state must be checked before approving execution.
- Seeds English email and SMS templates with fixed content. The seed is not a
  snapshot of production environment values. Compare the seed against current
  fallback content before switching dispatchers to DB-backed resolution.
- Affects shared template selection across tenants, including SMS template state,
  even though real SMS verification remains deferred. Deferral does not authorize
  disabling SMS or ignoring this migration's effect on it.

Republishing an earlier template version restores a content selection; it does
not undo schema migration or recover dropped legacy rows. These require separate
preservation and recovery planning. No deployment or migration was attempted.

## Inventory and approval still required

The dashboard creation dialog supports `en` and `vi`. English email is the
candidate because #6 proved that path. Its template ID, active version, complete
subject/body and potential production users remain unverified. Seed IDs in the
repository are not evidence of production state.

Templates are shared by `(channel, locale)`, without tenant ownership. Publishing
English email can affect every tenant sending English email. A separate test
tenant isolates request history, but cannot isolate this publication.

After the upgrade, the next approval packet must contain:

1. The observed template and original active-version IDs, exact preserved
   subject/body, affected users and a safe execution window.
2. Exact proposed test content, for example subject
   `Worklane template verification #7` and body
   `Worklane template verification #7. Your verification code is {{code}}. It expires in {{expiry}}.`
   This is a proposal only; no test content has been approved or written.
3. Dashboard steps to save a draft, inspect the rendered preview, publish the
   saved version, send to the controlled mailbox, and record receipt with OTPs
   and recipient details redacted.
4. Restoration by making the preserved original version live in the dashboard,
   followed by a separately identified email proving the original subject/body
   after cache propagation. Metadata alone cannot establish restoration.
5. The number of approved email sends, controlled mailbox, stopping conditions
   and treatment of any unreceived or stale message. If no original active
   version exists, an agreed safe final state is required before creation;
   deletion or unpublishing cannot be assumed available.

In local source, publish attempts best-effort cache invalidation, and the
dispatcher defaults to a ten-minute cache TTL. Confirm the deployed TTL and
observe delivered content after publishing and restoration; record timestamps
and any propagation delay or failure. Neither that default nor active-version
metadata proves a received message used the selected version.

## Local validation

Local checks validate the unchanged checkout, not the older deployed artifacts
or production email. No fix was needed or authorized for this deployment blocker,
so new regression tests and TDD are not applicable.

Checks ran during 12:40-12:42 UTC using the existing isolated toolchain from
[#5](2026-11-15-issue-5-automated-verification.md): Go 1.25.0, Make 4.2.1,
Zig 0.15.2 for CGO, Node 22.23.5 and pnpm 10.26.2. No production environment
file was loaded.

- `pnpm typecheck`: passed, including route-type generation.
- `pnpm test components/templates/template-detail.test.tsx components/templates/templates-view.test.tsx`:
  two files, four tests passed.
- `pnpm lint`: passed.
- `make test`: full default backend suite passed, including SMS unit tests.
- `pnpm test`: 26 files, 67 tests passed.

Tagged integration and full-stack E2E suites were not rerun. The focused UI tests
use simulated API behavior and do not prove production publishing or receipt.

## Acceptance status

- Template/locale, active-version and potential-user inventory: incomplete.
- Exact content, affected scope and restoration approval: pending inventory and
  upgrade; no production-write approval requested for an unverified template.
- Dashboard authoring, preview, publish and received test email: not executed.
- Original-version restoration and received original content: not executed.
- Delivered-content/cache-propagation verification: not executed.
- Production upgrade requires a separately reviewed and approved action plan.
  Issue #7 remains open and incomplete.
