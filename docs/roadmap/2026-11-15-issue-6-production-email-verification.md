# Production email OTP and delivery history

Source: [issue #6](https://github.com/DiiKhanh/worklane/issues/6), part of
[verification milestone #3](https://github.com/DiiKhanh/worklane/issues/3).
Prerequisite [#4](https://github.com/DiiKhanh/worklane/issues/4) was closed before
execution.

## Result

**Issue #6 production verification is complete.** Real email receipt, successful
verification, reuse rejection, incorrect-code rejection and expiry rejection
were demonstrated. Request history and Resend delivery logs match the tested
request IDs. Receipt and actual-code submission outcomes were operator-confirmed;
the agent independently observed production send responses, request/history and
delivery data, Redis expiry, and HTTP 410 after expiry.

Four email sends were used under two explicit approvals. The first expiry
candidate was consumed early; the replacement had an incorrect attempt that
restarted its TTL. The final expiry check waited for the observed Redis key to
disappear before submission. That TTL extension is documented below as existing
behavior, not evidence of a strict five-minute lifetime from initial issuance.
No application code or production configuration changed.

## Environment and authorization

- Date: 2026-11-15; all recorded times are UTC.
- Production test observation window: 12:07:11-12:31:35.
- API: `https://api-otp.dikhanh.io.vn`.
- Dashboard: `https://worklane-six.vercel.app`, visibly using **Live API**.
- Local repository baseline: `f4393bf8304782984f280d5d7954a197c569d159` on `main`.
- Existing operator-designated test tenant:
  `c0b4fb4e798724addd17ea1c378a3d15`. Authentication returned that tenant and the
  operator approved using it as the separate test tenant. Six historical OTP
  requests existed before this verification; none is counted as new evidence.
- Controlled recipient is recorded only as `2***@gm.uit.edu.vn`.
  The operator controls receipt and enters OTPs directly in the production
  dashboard. Credentials, usable OTPs and message contents are excluded.
- The operator explicitly approved three production email sends and verification
  attempts: success/reuse, incorrect code and expiry, in separate requests.
  After the third code was consumed early, the operator separately approved one
  additional production email to repeat only the expiry case.
  No account provisioning, new API key, configuration change, deployment,
  template write, SMS send or external publication is authorized by that approval.
- The browser uses an isolated Playwright Chromium context. The preferred
  `chrome-devtools-axi` tool is unavailable in this session. Existing browser
  libraries were used without a system installation. No mailbox login or
  production secret object was read.

At 12:07:33, read-only Kubernetes inspection confirmed all three application
containers Ready in namespace `worklane`, with the same artifacts recorded for
[#4](2026-11-14-issue-4-production-verification.md):

| Service | Image tag | Running image digest |
| --- | --- | --- |
| auth-svc | `sha-041efd7` | `sha256:295cb62a76d5c3d6f6187b982ffc34d94be4d05770fc2cc8bb14d6d771dffb53` |
| otp-api | `sha-041efd7` | `sha256:ef1d6eb7200a094763a8280f05ed2d693ab5211c89f120d7d2ce51ef50497c6f` |
| otp-dispatcher | `sha-041efd7` | `sha256:9e36fcad357ea07258ae78cef68fe4978aff12b1f68ce3683f8626208e982a7e` |

The tag-associated Git revision is
`041efd743b3a16c07df296983b9981ee7d26c51a`. These are production observations,
not execution against the newer local checkout. Template Studio is outside this
ticket; the previously recorded backend version mismatch remains relevant to #7.

At 12:13:00, the public dashboard HTML identified build
`dIdzrqNyfr5x3rEhbk39Y`, with SHA-256
`a6708a852fd49f57e6a9a82c78697be7db2d5f9cf66c9f71a3d4e378ef50f80a`.
The latest successful GitHub Production deployment record was
[7790936824](https://api.github.com/repos/DiiKhanh/worklane/deployments/7790936824),
created at 2026-11-14 17:11:41, source
`9bff3eac88d68bd8f0f5564f53232235675a161e`, environment URL
`https://worklane-r4jypmcxu-diikhanhs-projects.vercel.app`.
That source differs from #4's owner-confirmed source `280fd11` only in
documentation. The record identifies a deployment, while the public build ID
identifies the artifact actually exercised. No independently authenticated Vercel
alias-to-source lookup was available; the newer deployment record is not claimed
as proof that the public alias points to it.

## Production cases

All sends use the dashboard Playground, Email channel and default English locale.
The browser directly observed each send response. Mail receipt and OTP entry are
performed by the operator, so operator-reported results are distinguished from
agent-observed API and dashboard state. No raw verification payload is captured.

| Case | Send response time | Request ID | Send result | Delivery evidence |
| --- | --- | --- | --- | --- |
| Success and reuse | 12:07:11.875 | `762787547c45db2f9c8a9cf66e472201` | HTTP 202 | Resend `sent`, 583 ms, created 12:07:12, no error |
| Incorrect code | 12:11:30.965 | `0d6a5565973a91d289dc267cd1d02415` | HTTP 202 | Resend `sent`, 650 ms, created 12:11:32, no error |
| Intended expiry, consumed early | 12:13:56.526 | `309de1682fe1f9b595ce50f87447440c` | HTTP 202 | Resend `sent`, 622 ms, created 12:13:57, no error |
| Natural expiry repeat | 12:23:08.721 | `e87ec3dd83d5c786b62020c9cf3d5a9a` | HTTP 202 | Resend `sent`, 630 ms, created 12:23:09, no error |

### Success and reuse

The operator confirmed receipt of the first email and reported **Verified**, then
**Expired** after submitting the same code again without a new send. The
dashboard maps HTTP 410 to its expired/unknown-code outcome, including a code
consumed by successful verification. Reuse rejection is not evidence that the
code timed out naturally.

At 12:11:26, `GET /v1/otp/requests` returned HTTP 200 and the exact first request
with channel `email`, recipient `2***@gm.uit.edu.vn`, state `verified` and
`created_at` `2026-11-15T12:07:12Z`. The dashboard OTP requests screen showed
the same full request ID and **Verified**. At 12:11:29, Delivery logs showed that
ID with provider `resend`, status **Sent**, latency **583 ms**, and no error.
Both rendered screens were visually inspected. The delivery API independently
returned the matching row. Provider acceptance alone is not proof of mailbox
receipt; receipt is established by the operator's confirmation.

### Independent rejection cases

The second request was sent only after the first case was complete. The operator
confirmed receipt and **Code mismatch** after submitting one six-digit value
known to differ from the second email's code. The operator's report arrived
before the third send. A separate third request was intended to remain untouched
until expiry, to avoid exhausting the success request's attempt budget or
extending the expiry case's TTL with a failed attempt. OTP hot state is keyed by
tenant and recipient, so sends must be sequential: a new send replaces the
previous code.

After the third send, both list APIs returned HTTP 200: the first request was
`verified`; the second and third were `sent`; all three had successful Resend
delivery logs matching the table above. The requests and delivery-log screens
rendered all three full IDs and matching states/provider latencies and were
visually inspected. Exactly three send POSTs were executed under the original
approval. The operator was instructed to verify the third email's actual code
only after 12:20:00, more than six minutes after issuance, without any intervening
send or verification attempt. The tagged backend's default TTL is five minutes;
expiry evidence must establish actual expiry rather than relying on that
default. A runtime shell inspection could not run because the application image
contains no `sh`; no production change was made to work around this.

At 12:16:03, a follow-up request/history read showed the third request already
`verified`, only about two minutes after issuance. The operator confirmed the UI
displayed **Verified**. This is another successful verification within the default
TTL, not a reproduced expiry failure. A later rejection of that consumed code
would test reuse again, not natural expiry. The operator separately approved one
additional send to repeat only the expiry case. That fourth send returned
HTTP 202 at 12:23:08.721, with its delivery evidence recorded above.

At 12:23:40, a read-only `redis-cli TTL` query for the fourth code's exact
tenant/recipient key returned **267 seconds**. This directly observes an active
expiring key and is consistent with five minutes from issuance; no code, hash,
salt or other key contents were read. The operator was instructed to retain the
fourth email and wait for an explicit ready signal before verifying.

Before the ready signal, the operator reported **Code mismatch**. At 12:26:13,
the same key's TTL was **253 seconds**, rather than the approximately 115 seconds
remaining from issuance. This is consistent with a failed verification restarting
the full TTL: the deployed Verify implementation increments attempts and saves
the record with the configured TTL on mismatch. The operator was instructed to
stop attempting verification and preserve the newest email's code. Expiry must
now be observed from the refreshed TTL, approximately 12:30:26, rather than from
the original issuance time. No additional send is needed or authorized.

This observed TTL extension is a limitation of the existing implementation;
it is not fixed or hidden by this verification. The success/reuse case was already
complete on a different request, so this incorrect attempt could not lock it.

Read-only TTL checks then returned 154 seconds at 12:27:52, 85 at 12:29:00,
16 at 12:30:09 and **-2** at 12:30:53. Redis TTL -2 means that the key no longer
exists. The countdown was consistent with natural expiration after the refreshed
deadline, without a new send or successful verification consuming that code.
Only after the final missing-key observation was the operator told to submit
the actual fourth email's code. The operator confirmed **Code expired or unknown**.

At 12:31:35.685, an independent browser submission of a synthetic six-digit
value, after the already-observed expiry, returned **HTTP 410** with
`otp: no active code`. The rendered dashboard showed **Code expired or unknown**.
This synthetic probe establishes the public API/UI missing-code path; the
operator's separate submission establishes rejection of the code actually
received. No claim is made that the agent knew or submitted the received OTP.

Expiry removes the Redis hot state. The deployed Verify implementation returns
the expired/unknown result when that state is absent; it does not update the
MySQL request to `expired`. Therefore a historical request remaining `sent` is
not evidence that its code is still usable.

The final API read at 12:31:35.801 returned HTTP 200 for requests and delivery
logs. In issuance order, the four requests were `verified`, `sent`, `verified`,
`sent`, with matching successful Resend rows and latencies from the table above.
The last `sent` request is the expired fourth code, not a successful verification.
The final requests, delivery-log and expired-outcome screens were visually
inspected. Four emails were sent in total; no further sends were performed.

## Local validation

The unchanged local application was checked separately from production. These
checks validate the local baseline, not the older deployed backend or delivery
to a real mailbox. No application or test change was needed; TDD
and new regression tests are therefore not applicable.

- `pnpm typecheck`: passed, including Next.js route-type generation.
- `pnpm test lib/api/live.test.ts`: 6 tests passed.
- `go test ./services/otp-api/internal/app -run 'TestVerify' -count=1 -v`:
  both existing verification tests passed.
- `pnpm lint`: passed.
- `make test`: full default backend suite passed, including SMS unit tests.
- `pnpm test`: 26 files, 67 tests passed; no local-storage warning.

Toolchain and isolation match the existing
[#5 local verification](2026-11-15-issue-5-automated-verification.md): Go 1.25.0,
Make 4.2.1, Zig 0.15.2 for CGO, Node 22.23.5 and pnpm 10.26.2.
No production environment file was loaded for these checks. Tagged integration
and isolated email E2E suites were not rerun for this evidence document; previous
isolated E2E evidence remains linked from #5.

## Acceptance status

- Test tenant, controlled mailbox and production-send approval: established.
- Real receipt, successful verification, matching request/history/delivery: passed,
  with operator-reported and agent-observed evidence distinguished above.
- Reused code rejection: operator-confirmed.
- Incorrect code rejection: operator-confirmed on the second request.
- Independent expiry rejection: passed on the separately approved fourth request,
  after directly observed Redis expiry and the operator's actual-code submission.
- Sanitized identifiers, timestamps and deployed artifacts: recorded, with the
  dashboard source-attribution limitation stated above.
- All selected verification criteria for #6 are satisfied. This does not verify
  strict expiry from issuance after incorrect attempts, Template Studio, stats
  reconciliation or real SMS delivery. Those claims are outside this evidence.
- No tracker update, issue closure or push has been performed. Local commit and
  review use the operator-approved starting point `f4393bf` on the current branch.
