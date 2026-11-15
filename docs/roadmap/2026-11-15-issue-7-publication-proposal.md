# Approved Template Studio publication procedure

Source: [issue #7](https://github.com/DiiKhanh/worklane/issues/7).
Status: operator-approved and executed. See
[received-email evidence](2026-11-15-issue-7-received-email-verification.md).
The procedure below preserves the exact scope presented before production writes.

## Current inventory

Read-only production inspection at 2026-11-15 12:50-12:52 UTC established:

- API: `https://api-otp.dikhanh.io.vn`.
- Dashboard: `https://worklane-six.vercel.app`, with `/templates` redirecting an
  isolated unauthenticated browser to `/login`. The login screen was rendered
  and visually inspected. No authenticated dashboard evidence is claimed yet.
- Both OTP services run `14f2158340608a5caae9f7b53ffc85f0395c7834`; see the
  [upgrade evidence](2026-11-15-issue-7-backend-upgrade.md).
- Template: `OTP email`, channel `email`, locale `en`, ID
  `11111111-1111-1111-1111-111111111111`.
- Active version: v1, ID `11111111-0000-0000-0000-000000000001`, published.
- Preserved subject: `Your verification code`.
- Preserved body: `Your verification code is {{code}}. It expires in {{expiry}}.`
- Identity database: one existing tenant and one user. Email history contains
  nine requests from one tenant. These counts describe observed state, not a
  tenant-specific template boundary or a guarantee of no concurrent traffic.

Template selection is global for `(email, en)`. Every tenant sending English
email during the test publication would receive the test content, including any
new tenant created during that interval. SMS and other locales are outside the
proposed test. No new tenant, user, credential or template entity is needed.

## Exact proposed write scope

Use the existing test account/tenant designated in #6 and its controlled mailbox,
recorded only as `2***@gm.uit.edu.vn`. The operator explicitly approved the
proposal and designated the controlled mailbox on 2026-11-15. Availability to
confirm receipt must be established before publication. No password, token or
OTP is to be included in evidence.

In the dashboard, edit the existing English email template and save one new
version with exactly:

- Subject: `Worklane template verification #7`.
- Body: `Worklane template verification #7. Your verification code is {{code}}. It expires in {{expiry}}.`
- Version note: `Issue #7 controlled email verification`.

Inspect the real preview for that content before saving or publishing. Preserve
the original v1 ID and subject/body independently of the editor. If the observed
active version or content differs from this inventory, stop and refresh the
approval proposal before any write.

The original approval covered one draft version, making that version live,
republishing original v1, and **at most two email send requests** to the controlled
mailbox: one with the test publication and one after restoration. No automatic
retry, further send, deletion, cache flush, SMS send, configuration change or
credential operation is included. The added version remains in history after
restoration; no deletion is assumed.

## Execution and restoration

1. Confirm authenticated dashboard access, tenant identity, mailbox and exact
   current original version. Confirm operator availability before publication.
2. Edit, inspect the rendered preview, save the draft and make that saved version
   live through the dashboard. Record its version ID and UTC publication time.
3. Send the first email from the dashboard Playground using Email and English.
   Record request ID and timestamps. Wait for its matching delivery result before
   restoration, so the dispatcher resolves the test publication before it changes.
4. Immediately make original v1 live after that delivery result, or within
   60 seconds of the first send if delivery cannot be confirmed. Also restore v1
   on any failure after test publication. Confirm the active ID and original
   subject/body. Do not leave the test version live while waiting for the
   operator to read the mailbox.
5. Respect the recipient resend cooldown (at least 60 seconds after the first
   send, unless the observed configuration requires longer). Send the second
   email only after v1 restoration is confirmed.
6. Obtain operator confirmation of both messages: test subject/body marker in the
   first, original subject/body without the marker in the second. Redact actual
   codes and recipient details. Match request/delivery IDs and receipt order;
   provider acceptance alone does not prove receipt.
7. Record final original active ID, timestamps, receipt outcomes and any cache
   propagation delay. A stale message, missing receipt or failed restoration
   leaves the affected acceptance criterion incomplete and requires a new
   decision before additional mutation or sends.

Publish uses best-effort cache invalidation; the deployed source defaults to a
ten-minute cache TTL. Observe cache state/TTL without changing it and check
received content for both phases. Active-version metadata and Redis observations
support diagnosis but cannot replace the two receipt confirmations. A stale
message does not justify an unapproved resend or cache deletion.

## Authorization and execution

The operator approved the proposed content, shared English-email scope, retained
version history, at most two send requests to the designated controlled mailbox,
and v1 restoration before execution.

At approximately 13:02 UTC on 2026-11-15, a single login attempt using the
operator-supplied account through the production dashboard displayed
`Invalid email or password`. A read-only exact-email lookup then returned zero
matching users in production identity data. Subsequent inspection found a domain
typo: the existing test account used `.co` rather than `.com`. Login with the
correct existing account succeeded, and the operator also supplied that
correction. The account's tenant matched #6. No password reset or account
creation was needed. The password field was cleared before the failed-login
screenshot; no credentials were included in evidence.

At 13:02:41 UTC, direct read-only inspection confirmed the preserved v1 was
still active and the email template still had exactly one version. No test
draft, publication, email send or SMS send occurred during this attempt.

The first send succeeded and the operator confirmed the exact test content.
The second request, after v1 restoration, returned HTTP 429 and sent no email.
The recipient rate-limit key was subsequently observed missing, with no cooldown
marker. The operator separately approved exactly one additional send to verify
the restored original, without further publication or configuration changes.
That request succeeded and the operator confirmed original content without the
test marker. There were three send requests in total, producing two emails.
