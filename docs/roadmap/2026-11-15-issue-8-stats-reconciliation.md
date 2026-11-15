# Production tenant stats and Overview reconciliation

Source: [issue #8](https://github.com/DiiKhanh/worklane/issues/8), part of
[verification milestone #3](https://github.com/DiiKhanh/worklane/issues/3).

## Result

**Issue #8 verification is complete.** The authenticated stats API, visually
inspected production Overview and complete tenant request/delivery history agree:
six successful deliveries, two verified requests, zero failed requests and
622 ms p50 provider latency. No directly blocking discrepancy was found.
No application change, production send, deployment or new regression test was
required. Existing activity from #6 and #7 supplied the controlled email evidence.

## Environment and scope

- Date: 2026-11-15; all times below are UTC.
- API: `https://api-otp.dikhanh.io.vn`.
- Dashboard: `https://worklane-six.vercel.app`, visibly using **Live API**.
- Initial local revision: `a3f60f684cd0c2c4d1191aa52d17f8b36d2f6ade`.
- Existing operator-designated test tenant:
  `c0b4fb4e798724addd17ea1c378a3d15`. The operator supplied the existing
  account; login succeeded and `/auth/me` confirmed that exact tenant in both
  observation snapshots. No account, credential or access setting changed.
- Controlled mailbox: `2***@gm.uit.edu.vn`, reused from
  [#6](2026-11-15-issue-6-production-email-verification.md) and
  [#7](2026-11-15-issue-7-received-email-verification.md). No mailbox access,
  new OTP issuance or verification attempt was needed for reconciliation.
- Existing Playwright Chromium ran in an isolated context with UTC timezone and
  a 1440 by 1100 viewport. `chrome-devtools-axi` was unavailable. Browser
  dependencies were extracted into a temporary directory without system
  installation. The browser permitted reads and the supplied-account login;
  other browser mutation requests were blocked for this observation.
- Credentials, bearer tokens, OTP values and unmasked recipient data are excluded.

The served dashboard identified build **`HvC06s2oWRkwf6IAWQ4jk`**. Its public
HTML SHA-256 was
`06be904948eab086c6b76adb986f8b5ff920db9de65d57c93b47405b024d5a17`.
The latest GitHub Production deployment record,
[7803926630](https://api.github.com/repos/DiiKhanh/worklane/deployments/7803926630),
was successful, created at 13:12:34 for source
`a3f60f684cd0c2c4d1191aa52d17f8b36d2f6ade`, with environment URL
`https://worklane-giqusqgd9-diikhanhs-projects.vercel.app`.
The deployment record is not independent proof of the public alias's source;
the build ID identifies the actual served dashboard artifact.

The last independently recorded OTP backend source is
`14f2158340608a5caae9f7b53ffc85f0395c7834`, with pinned image digests in the
[#7 upgrade record](2026-11-15-issue-7-backend-upgrade.md). The stats computation,
repository and HTTP handler files have no local changes relative to that source.
Live Kubernetes image attribution was unavailable in this session: the kubeconfig
was unreadable and noninteractive sudo required a password. That earlier backend
observation is historical attribution, not a new live image inspection.

## Baseline and observation window

The baseline read ran at **17:34:45.223-17:34:45.872**; the repeat read ran at
**17:34:57.900-17:34:58.366**. Each snapshot fetched `/auth/me`, `/v1/stats`,
`/v1/otp/requests` and `/v1/delivery-logs` sequentially through the production
API from the authenticated dashboard origin. All eight responses were HTTP 200.

The browser observed four successful stats responses at 17:34:45.429,
17:34:45.616, 17:34:53.555 and 17:34:58.126, including Overview's automatic
poll. All four had identical aggregates and all 25 hourly series points matched
the evidence below. The baseline and final request/delivery lists were identical
in IDs, states, timestamps and latencies. No observed concurrent activity or
state transition changed this comparison. These are bounded sequential reads,
not a claim of transactionally atomic cross-endpoint snapshots.

At the baseline the rolling lower boundary was approximately
**2026-11-14 17:34:45**; at the final read it was approximately
**2026-11-14 17:34:58**. Each response uses its own server `now`; these times
bound the observed client requests rather than asserting an exposed server
cutoff. No request or delivery lies near either boundary. The six in-window
requests are from 12:07-13:08 on November 15; the six older requests and their
six delivery logs are from August 25-28 and are excluded at both snapshots.
No row could age out during this observation interval.

Both unfiltered history endpoints returned **12 rows**, below their implemented
fixed limit of **100** (`defaultListLimit` in the HTTP handlers). Repository
queries apply the tenant filter, descending creation order and that limit, with
no additional history filter. Thus these responses cover all tenant history
under the implemented contract; this comparison does not assume a full page of
100 rows would be complete. The API has no history pagination parameter. A
tenant at the cap would require independent complete database evidence before
reconciling totals; increasing a client page size would not prove completeness.

## Request and delivery evidence

All six in-window requests use email. Each has exactly one matching Resend
delivery with status `sent` and no error. The four #6 and two #7 IDs match the
earlier controlled exercises. Earlier mailbox receipt observations remain
attributed to those exercises; this ticket independently rereads request and
provider-delivery history, without claiming fresh receipt evidence.

| Request ID | Request created | Current request state | Delivery created | Send latency |
| --- | --- | --- | --- | --- |
| `762787547c45db2f9c8a9cf66e472201` | 12:07:12 | verified | 12:07:12 | 583 ms |
| `0d6a5565973a91d289dc267cd1d02415` | 12:11:31 | sent | 12:11:32 | 650 ms |
| `309de1682fe1f9b595ce50f87447440c` | 12:13:56 | verified | 12:13:57 | 622 ms |
| `e87ec3dd83d5c786b62020c9cf3d5a9a` | 12:23:09 | sent | 12:23:09 | 630 ms |
| `a96c07879968be73a90cfe3ab47ee944` | 13:05:48 | sent | 13:05:49 | 544 ms |
| `572bc7a1bf64122f12637e435e3e96d7` | 13:08:25 | sent | 13:08:26 | 614 ms |

## Metric definitions and matched values

Authoritative implementation:
[stats.go](../../services/otp-api/internal/app/stats.go),
[repository reads](../../services/otp-api/internal/adapters/outbound/mysqlrepo/repo.go),
[live API mapping](../../dashboard/lib/api/live.ts), and
[Overview KPIs](../../dashboard/components/overview/kpis.tsx).

| Metric | Independent evidence calculation | Stats API | Rendered Overview |
| --- | --- | --- | --- |
| Sent 24h | Six successful delivery-log attempts, filtered by each log's creation time | `sent_today: 6` | 6 |
| Verify rate | Two verified requests / six sent-or-verified requests | `verify_rate: 0.3333333333333333` | 33% |
| Failed | Zero requests in current `failed` state | `failed: 0` | 0 |
| p50 latency | Sorted successful latencies: 544, 583, 614, **622**, 630, 650; upper middle value | `p50_latency_ms: 622` | 622 ms |
| Funnel requested | All six requests created within the window | `funnel.requested: 6` | Requested 6 |
| Funnel delivered | Four `sent` plus two `verified` requests | `funnel.sent: 6` | Delivered 6, 100% |
| Funnel verified | Two requests in current `verified` state | `funnel.verified: 2` | Verified 2, 33% |

The implemented window includes both endpoints. Successful attempts and request
lifecycles are distinct counts: retries can create multiple delivery rows per
request, and delivery creation can cross a window boundary independently of
request creation. Those cases do not occur in this observed dataset. The
`sent_today` field means rolling 24 hours, not calendar-day midnight. Latency is
provider send duration, not receipt time or end-to-end verification time.

The API emitted 25 UTC hour buckets from November 14 at 17:00 through
November 15 at 17:00, with partial edge buckets. Only these buckets are nonzero:

| UTC hour on November 15 | Pending (`requested`) | Sent | Verified | Failed |
| --- | --- | --- | --- | --- |
| 12:00 | 0 | 2 | 2 | 0 |
| 13:00 | 0 | 2 | 0 | 0 |

The series is mutually exclusive current request outcomes, attributed to request
creation hour; its `requested` field represents pending requests, whereas the
funnel's `requested` includes every lifecycle state. Verified requests contribute
to funnel delivered and verified but only to the series verified outcome.
Every other hourly point was zero. Overview's volume chart and funnel were
rendered and visually inspected alongside the KPIs. Recent activity includes
an older August row; that feed is recent history, not the 24-hour aggregate.

Historical `sent` does not prove a code remains usable. In particular, #6's
expired request remains `sent` because hot-state expiry does not update its
historical request state. The reconciliation follows implemented definitions
and does not reinterpret that request as a failed delivery.

## Local validation and acceptance

Checks used Node 22.23.5, pnpm 10.26.2, Go 1.25.0, Make 4.2.1 and Zig 0.15.2
for CGO, without loading production environment files. Existing tests were
reused; no behavior changed requiring a new TDD seam or regression test.

- Dashboard `pnpm typecheck`: passed.
- Dashboard `pnpm test lib/api/live.test.ts`: six tests passed.
- Backend `go test ./services/otp-api/internal/app -run TestComputeStats -count=1`:
  passed, including rolling-window and percentile behavior.
- Dashboard `pnpm lint`: passed.
- Dashboard full `pnpm test`: 26 files, 67 tests passed.
- Backend full `make test`: passed, including existing SMS unit tests.

All four ticket criteria are satisfied by the reused test tenant, controlled
baseline, repeated live comparison, complete history and matched values above.
No correction or corrected deployment was necessary. SMS sending and
phone-dependent verification remain deferred; historical SMS data was retained
and excluded solely because it falls outside the window. No tracker update,
issue closure, push or external publication was performed for this ticket.
