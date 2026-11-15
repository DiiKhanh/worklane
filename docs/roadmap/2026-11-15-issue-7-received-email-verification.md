# Template Studio publication and restoration through received email

Source: [issue #7](https://github.com/DiiKhanh/worklane/issues/7), part of
[verification milestone #3](https://github.com/DiiKhanh/worklane/issues/3).

## Result

**Issue #7 verification is complete.** Dashboard editing, real preview, saving
v2, publishing v2, and republishing original v1 were exercised against production.
The operator confirmed the first received email matched the approved test
subject/body and the later received email matched the original subject/body
without the test marker. The agent independently observed dashboard rendering,
API responses, version state, request history, delivery logs and cache TTL.

Receipt/content observations were operator-confirmed, not independently read
from a mailbox by the agent. Provider `sent` status alone is not used as proof
of either received content. Original v1 is live; test v2 remains superseded in
history. Two emails were sent across three explicitly approved send attempts;
the intermediate request returned HTTP 429 without creating an email.

## Environment and approved scope

- Date: 2026-11-15; all timestamps are UTC.
- Authenticated dashboard observation: approximately 13:03-13:09.
- API: `https://api-otp.dikhanh.io.vn`.
- Dashboard: `https://worklane-six.vercel.app`, visibly using **Live API**.
- Local baseline: `1f2e26ec77a0858b0d35e7713f12fa58f2e6852f` on `main`.
- Both OTP services: deployed source
  `14f2158340608a5caae9f7b53ffc85f0395c7834`, with exact image digests and
  clean schema version 4 documented in the
  [upgrade record](2026-11-15-issue-7-backend-upgrade.md).
- At 13:09:34 both OTP deployments still reported one Ready replica and the same
  pinned images.
- Separate operator-designated test tenant, matching #6:
  `c0b4fb4e798724addd17ea1c378a3d15`.
- Controlled mailbox: `2***@gm.uit.edu.vn`; the operator designated it and
  confirmed both receipt/content outcomes. No OTP, password or token is recorded.
- Before writes, production identity data contained one tenant and one user;
  nine historical email requests belonged to one tenant. Template selection is
  shared across all tenants by `(channel, locale)`, so every English email send
  during the test publication was potentially affected.
- The [approved procedure](2026-11-15-issue-7-publication-proposal.md) specified
  exact content, one new version, publication, v1 restoration, retained history
  and at most two send requests. After the second request was rejected, a
  separate approval allowed exactly one further restoration-check send.
- Existing Playwright Chromium ran in an isolated context. The preferred
  `chrome-devtools-axi` tool was unavailable. Existing browser libraries were
  used without system installation or browser access-control changes.

At 13:06:40 the public dashboard HTML identified build `WDkzq-zAaeuFtWDVnCGRK`,
SHA-256 `c380f27b28242acbfdc82212f7280ae758315a739a8bdde87a97ae8ef78784c1`.
The latest GitHub Production deployment record was
[7803688903](https://api.github.com/repos/DiiKhanh/worklane/deployments/7803688903),
created 12:51:55 for source `1f2e26ec77a0858b0d35e7713f12fa58f2e6852f`.
That record identifies a deployment; no independently authenticated Vercel
alias-to-source lookup was performed. The public build ID identifies the served
artifact observed during the exercise.

## Preserved original and dashboard authoring

The dashboard listed `OTP email`, locale `en`, template
`11111111-1111-1111-1111-111111111111`. Authenticated API inventory and editor
values confirmed its sole existing version was v1
`11111111-0000-0000-0000-000000000001`, with subject `Your verification code`
and body `Your verification code is {{code}}. It expires in {{expiry}}.`
These values matched the previously approved inventory and were preserved before
editing. Existing state called for editing rather than creating a new template.

At 13:04:32-13:04:33, the editor and real preview were exercised with the exact
approved subject `Worklane template verification #7` and body
`Worklane template verification #7. Your verification code is {{code}}. It expires in {{expiry}}.`
The preview endpoint returned HTTP 200 and the rendered preview substituted the
sample code and `5 minutes`. The screen was visually inspected.

At 13:04:49.144, saving through **Save draft** returned HTTP 201, creating v2
`4b6a26b4254ddf8e4a37aa52a9a23d67` with note
`Issue #7 controlled email verification`. The saved-draft screen and version
history were visually inspected. Saving alone left v1 active.

## Publication, delivery and restoration

| UTC time | Operation | Result |
| --- | --- | --- |
| 13:05:47.797 | Dashboard **Make v2 live** | HTTP 204; v2 active, v1 superseded. |
| 13:05:48.472 | Playground Email, English, first send | HTTP 202; request `a96c07879968be73a90cfe3ab47ee944`. |
| 13:05:49.799 | Matching delivery observed | Resend `sent`, 544 ms, delivery created 13:05:49. |
| 13:05:50.178 | Dashboard **Make v1 live** | HTTP 204; original restored approximately 2.4 seconds after v2 publication. |
| 13:06:10.333 | Restored editor and preview observed | Original subject/body visible; v1 published, v2 superseded. |
| 13:06:55.545 | Original-content send attempt | HTTP 429; no request ID or email created. |
| 13:07:30 | Read-only rate/cooldown inspection | Recipient rate-limit key absent (TTL -2), cooldown marker absent. |
| 13:08:25.124 | Separately approved extra send | HTTP 202; request `572bc7a1bf64122f12637e435e3e96d7`. |
| 13:08:26.328 | Matching restoration delivery observed | Resend `sent`, 614 ms, delivery created 13:08:26. |
| 13:09:03.809 | Final authenticated inventory | Original v1 active; v2 superseded; both request rows `sent`. |

The operator confirmed the first email had both the test subject and test body
prefix. After the extra send, the operator confirmed the newest email had subject
`Your verification code`, original verification/expiry text and no test prefix.
These confirmations establish delivered content for both selected versions.

The second attempt was made more than 60 seconds after the first but returned
429. Earlier #6 traffic was within the recipient's hourly window; quota exhaustion
is consistent with that history, but the exact counter value/error body at the
rejection was not retained. No limiter was bypassed, reset or reconfigured. No
retry occurred until the separate approval and observed missing limiter key.

An observation-script assertion initially read the editor before the restoration
refetch completed. The HTTP 204 restoration had already succeeded; waiting for
the editor/preview confirmed the original content, and independent DB reads
agreed. No duplicate publication or application fix was needed for this timing.

## Cache and final state

The template cache key was absent before the exercise, and again after v1
restoration at 13:05:56 and 13:06:40. After the approved restoration email, its
TTL was 587 seconds, consistent with the deployed ten-minute default and a fresh
cache fill. The restored original was received within that ten-minute interval,
rather than requiring the prior cached test content to expire naturally.
No stale-content receipt or cache-propagation failure was reported. No cache
delete or flush was performed manually. Metadata and TTL observations support
the two actual-content confirmations; they do not replace them.

Final dashboard reload, editor, original preview and both version states were
visually inspected. English email remains on original v1, with test v2 retained
as superseded. SMS, other locales, credentials and provider configuration were
unchanged during this publication exercise. A pre-existing cosmetic issue was
observed: the sidebar still marks Templates as `soon`; it did not prevent the
exercise and was not changed in this verification-only task.

## Local validation and acceptance

No application code or test changed. TDD/new regression tests were unnecessary
for this existing-feature verification. Separate local checks at approximately
13:09 UTC, without loading production environment files, passed:

- `make test`: full default backend suite, including SMS tests (cached results).
- `pnpm typecheck`: passed, including route-type generation.
- `pnpm lint`: passed.
- `pnpm test`: 26 files, 67 tests passed.

The existing isolated toolchain matches #5. Tagged integration/full-stack E2E
suites were not rerun; the browser and real mailbox exercise above is the selected
production verification. All #7 acceptance criteria are satisfied, distinguishing
operator receipt evidence from agent observations. No tracker comment, issue
closure, further send or deletion was performed by this verification record.
