# Email OTP milestone evidence and Link Service readiness

Source: [issue #10](https://github.com/DiiKhanh/worklane/issues/10), part of
[tracker #3](https://github.com/DiiKhanh/worklane/issues/3).

## Readiness conclusion

**The selected email OTP verification is satisfied under the revised scope and
ready for owner review. Link Service has not been authorized to resume.**
Production API availability, real email OTP, Template Studio publication and
restoration, tenant stats and isolated CI/E2E have successful evidence below.

**The original milestone including backup/restore remains incomplete.** No
two-database backup or isolated restore was run. The owner explicitly deferred
#9 out of the current milestone because this is a side project with one user
and backup is not a current priority. The
[owner's scope decision](https://github.com/DiiKhanh/worklane/issues/9#issuecomment-6898573531)
closed #9 as **not planned**, not as successful verification. This later decision
qualifies the backup requirement still present in #10's original body. Closed
dependencies alone are not evidence that their checks passed.

SMS and phone-number-dependent verification are also deferred. Historical
Twilio test-credential results establish integration only. No handset receipt,
phone ownership or real SMS stability is claimed. Existing SMS functionality and
automated tests remain in place.

Recommendation: review the evidence and accepted deferrals, then make a separate
decision whether to resume Link Service planning. Backup/restore should be
reconsidered before onboarding other users or retaining difficult-to-recreate
data. No parent-issue update, report publication, deployment, production mutation
or Link Service implementation is included in this local consolidation.

## Evidence register

All times are UTC. Production API: `https://api-otp.dikhanh.io.vn`; dashboard:
`https://worklane-six.vercel.app`. Production business-flow checks reused the
operator-designated test tenant `c0b4fb4e798724addd17ea1c378a3d15` and controlled
mailbox `2***@gm.uit.edu.vn`. Receipt/content and actual-code outcomes were
operator-confirmed; browser, API, request/delivery and cache observations were
agent-observed. No usable OTP, credential or sensitive message data is included.

| Check | Environment and observation time | Revision/artifact | Result and durable evidence |
| --- | --- | --- | --- |
| API availability, #4 | Production, 2026-11-14 17:03-17:05 | Backend `041efd743b3a16c07df296983b9981ee7d26c51a`; dashboard owner-confirmed source `280fd11d38f30c8f9ca5bd6e0b821a1e84311702` | Health 200; protected OTP/auth routes 401. Historical 530/1033 did not reproduce; cause unproven. [Report at 9bff3ea](https://github.com/DiiKhanh/worklane/blob/9bff3eac88d68bd8f0f5564f53232235675a161e/docs/roadmap/2026-11-14-issue-4-production-verification.md). |
| CI, protection tests and full-stack email E2E, #5 | Disposable GitHub-hosted Ubuntu runner, 2026-11-15 11:22:09-11:25:01 | `7965ac306a5befcac686bb88f80d1ada1bca33bd` | Backend build/vet/full suite, dashboard typecheck/lint/67 tests/build, both MailHog email E2E tests passed. [Run 43708589190](https://github.com/DiiKhanh/worklane/actions/runs/43708589190), [report at f4393bf](https://github.com/DiiKhanh/worklane/blob/f4393bf8304782984f280d5d7954a197c569d159/docs/roadmap/2026-11-15-issue-5-automated-verification.md). |
| Real email OTP and delivery history, #6 | Production, 2026-11-15 12:07-12:31 | Backend `041efd7`; served dashboard build `dIdzrqNyfr5x3rEhbk39Y` | Receipt, success, reuse rejection, wrong-code rejection and independently observed expiry passed. Four sends correlated with Resend logs and request history. [Report at 14f2158](https://github.com/DiiKhanh/worklane/blob/14f2158340608a5caae9f7b53ffc85f0395c7834/docs/roadmap/2026-11-15-issue-6-production-email-verification.md). |
| Upgrade and refreshed API checks, #7 | Production node `dk`, namespace `worklane`, 2026-11-15 12:45-12:47 | OTP API/dispatcher `14f2158340608a5caae9f7b53ffc85f0395c7834`; schema 4 clean | Both pinned images Ready; health 200, templates/stats/history/auth 401; Kafka lag 0. [Upgrade report at 1f2e26e](https://github.com/DiiKhanh/worklane/blob/1f2e26ec77a0858b0d35e7713f12fa58f2e6852f/docs/roadmap/2026-11-15-issue-7-backend-upgrade.md). |
| Template publication and restoration, #7 | Production dashboard and controlled mailbox, 2026-11-15 13:03-13:09 | OTP `14f2158`; served dashboard build `WDkzq-zAaeuFtWDVnCGRK` | Edit, preview, save/publish v2, receipt of test content, republish v1 and receipt of original content passed. Original v1 active; v2 superseded. [Report at a3f60f6](https://github.com/DiiKhanh/worklane/blob/a3f60f684cd0c2c4d1191aa52d17f8b36d2f6ade/docs/roadmap/2026-11-15-issue-7-received-email-verification.md). |
| Stats and Overview, #8 | Production, 2026-11-15 17:34:45.223-17:34:58.366 | Served dashboard build `HvC06s2oWRkwf6IAWQ4jk`; historical OTP attribution `14f2158` | API, rendered Overview and complete tenant history agree: sent 6, verified 2, rate 33%, failed 0, p50 622 ms; funnel 6/6/2. Both history lists had 12 rows, below the 100-row cap; stable rolling 24-hour window. [Report at 8591d92](https://github.com/DiiKhanh/worklane/blob/8591d92497a7ad031bace8366c7362a12e82be67/docs/roadmap/2026-11-15-issue-8-stats-reconciliation.md). |
| Backup/restore, #9 | Production VPS inspection only, 2026-11-15 17:32-17:33 | Baseline `a3f60f6`; OTP images `14f2158`, auth `041efd7`, MySQL 8.0.46 | **Deferred, unverified.** CronJob suspended, R2 secret absent, no verified stored object or isolated restore. Capacity and a future procedure were recorded. [Report and decision at 442fc65](https://github.com/DiiKhanh/worklane/blob/442fc659e8e191a35e9176714987cad5862d7a7d/docs/roadmap/2026-11-15-issue-9-backup-restore.md). |

The reports retain full image digests, request IDs, metric definitions and
observation limitations. In particular, #6 success request
`762787547c45db2f9c8a9cf66e472201` and #7 publication/restoration requests
`a96c07879968be73a90cfe3ab47ee944` / `572bc7a1bf64122f12637e435e3e96d7`
also appear in #8's independent history reconciliation.

## Revision reconciliation and refreshed checks

The consolidation baseline is `6ed666f3eaccd00344e320a955934601ebb90678`
on `main`. Git comparisons establish that application code, tests, workflows,
dependencies and scripts under `services/`, `pkg/`, `dashboard/`, `test/`,
`.github/`, `go.mod`, `go.sum` and `Makefile` are unchanged from the successful
#5 revision `7965ac3`. Later changes are evidence documents and the production
overlay's two OTP image pins. The clean-checkout typecheck fix in #5 is therefore
included in the later OTP source `14f2158` and current checkout.

The #5 clean typecheck failure in
[run 43708361471](https://github.com/DiiKhanh/worklane/actions/runs/43708361471)
was reproduced and fixed before successful run 43708589190. That run refreshed
all selected automated checks. It is not a passing-CI claim for this report's
commit. The deployed OTP source `14f2158` also has successful backend test,
full-stack smoke and image-build evidence in
[run 43714670395](https://github.com/DiiKhanh/worklane/actions/runs/43714670395),
2026-11-15 12:35:55-12:40:42. Both runs' revisions and completed/success job
conclusions were reread from GitHub during this consolidation.

The #7 upgrade superseded #4's missing-template 404 and older OTP artifacts.
Health/protected routes, schema and consumer health were refreshed after rollout;
real received email was refreshed through the upgraded dispatcher for both the
test version and restored original. #8 subsequently reconciled all six production
email deliveries with the new backend's stats. #6's success/wrong/reuse/expiry
exercise remains explicitly tied to the old deployment; it was not repeated in
full after upgrade. A Git comparison of the old and new sources confirms that
the Verify use case and Redis code store did not change. No fresh
post-upgrade production rejection-case execution is claimed.

Auth remains at `041efd7`; its service source is unchanged through `14f2158`.
The OTP pins are:

- API: `sha256:ad82c126d47c6bd868693ee05e218c7b77de1744ac9b514c2398dfa8ba3aa41c`.
- Dispatcher: `sha256:649e76b435c00c61dcd40ca7b1d097bf24f362f647f0dfed1c1c681d44ed8f15`.

Backend and dashboard revision labels are not assumed equal. The dashboard build
IDs in the register identify the actual artifacts served during each exercise.
GitHub Production deployment records associated with #6/#7/#8 name sources
`9bff3ea`, `1f2e26e` and `a3f60f6` respectively; those sources have unchanged
dashboard code relative to `7965ac3`. Those records do not independently bind
the public Vercel alias to a source commit. Only #4 had owner-confirmed source
attribution. #8 lacked a fresh live Kubernetes image inspection; #9's separate
17:32-17:33 inspection recorded the expected images shortly before the stats
exercise. This corroborates deployment history without asserting an atomic
artifact-to-browser attestation.

## Remaining gaps and ownership

| Gap or limitation | Milestone consequence | Owner and next action |
| --- | --- | --- |
| No proven two-database backup or isolated restore; R2 access absent and scheduler suspended | Original recovery criterion unfulfilled; explicitly removed from revised scope, not passed | Project owner: revisit #9 before other users or hard-to-recreate data; approve a fresh procedure before execution and cleanup separately. |
| Real SMS receipt and phone verification unrun | Deferred from the selected email milestone; simulated Twilio acceptance cannot replace receipt | Project owner: make a separate scope, sender/recipient and spending decision before resuming SMS verification. |
| Dashboard alias-to-source attribution limited | Served build IDs and deployment history retained; exact alias source not independently attested | Project owner/Vercel operator: supply authenticated mapping if exact source attestation is required for a later release. |
| Wrong attempts restart OTP TTL | Expiry was proved after observed Redis expiration, not as a strict five-minute lifetime from issuance | Project owner: decide separately whether absolute expiry needs remediation and regression coverage. |
| Protection tests use in-memory ports; E2E uses SMTP/MailHog | Sequential policy and full-stack email evidence passed; live Redis concurrency and real-provider E2E are not inferred | Future verification owner: expand only under a separately selected scope. |
| Historical 530/1033 cause and local-storage warning cause unproven | Current reachability and clean Node 22 checks passed; historical causes remain unknown | Operator: investigate if reproduced; do not infer a fix from a passing probe. |

There is no remaining failed or blocked required check in the **revised selected
scope**. Unrun/deferred checks above remain incomplete. This is evidence for a
readiness decision, not the decision itself. Tracker #3 and ticket #10 remain
unmodified by this work.

## Consolidation validation

On 2026-11-15 at 17:42:48 UTC, unauthenticated external curl GETs returned HTTP
200 for API `/healthz` and dashboard `/`. These are fresh reachability probes
only; no account login, OTP send, template write or production data change was
performed. Issue bodies, labels, comments and closed dependency states were
read through GitHub. In particular, #9's explicit deferral was read rather than
interpreting closure as a passed restore.

No application code changed, so TDD/new tests do not apply. Final local
`make test` and dashboard `pnpm typecheck && pnpm lint && pnpm test` could not
start because make and pnpm are absent from PATH. No fresh local passing-suite
or typecheck result is claimed. Prior successful runs above remain valid for
the unchanged application; no new CI run or external publication was requested.

Whitespace validation (`git diff --check`) and relative Markdown link checks
passed. The review baseline is the owner-selected starting commit `6ed666f`.
This report and the related SMS wording corrections are documentation only;
pre-existing untracked milestone planning files are excluded from the change.
