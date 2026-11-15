# Production backend upgrade for Template Studio

Source: [issue #7](https://github.com/DiiKhanh/worklane/issues/7). This supersedes
the deployment blocker in the
[initial inspection](2026-11-15-issue-7-template-studio-verification.md), while
leaving the ticket's dashboard and received-email verification incomplete.

## Result and authorization

At 2026-11-15 12:47 UTC, production otp-api and otp-dispatcher were successfully
upgraded from `041efd7` to `14f2158340608a5caae9f7b53ffc85f0395c7834`.
Both deployments had one updated Ready replica, zero container restarts, and
Available/Progressing conditions True. Startup migration completed at schema
version 4 with `dirty=0`. The public template route now requires authentication
(HTTP 401), replacing its former HTTP 404.

The operator explicitly requested the newer backend after being informed of the
deployment blocker and destructive legacy-table migration. That approval covered
the two OTP service upgrades and their startup migration. No auth-svc upgrade,
credential change, provider change, manual test-template publication or test
email/SMS send was performed. The migration created the initial published English
email and SMS templates as part of the approved upgrade.

Execution used targeted image updates rather than applying the full overlay,
avoiding unrelated live configuration changes. The repository production overlay
now records the exact deployed OTP image digests; auth-svc and seed pins remain
unchanged. No push or tracker publication was performed.

## Artifact selection

The selected images were already published by successful
[CI run 43714670395](https://github.com/DiiKhanh/worklane/actions/runs/43714670395)
for source `14f2158340608a5caae9f7b53ffc85f0395c7834`. Backend tests, full-stack
smoke and all image-build jobs succeeded. Registry config labels independently
identified that exact source revision and `DiiKhanh/worklane` repository.
No new image build or publication was needed.

| Service | Tag | Pinned amd64 image digest |
| --- | --- | --- |
| otp-api | `sha-14f2158` | `sha256:ad82c126d47c6bd868693ee05e218c7b77de1744ac9b514c2398dfa8ba3aa41c` |
| otp-dispatcher | `sha-14f2158` | `sha256:649e76b435c00c61dcd40ca7b1d097bf24f362f647f0dfed1c1c681d44ed8f15` |

These platform-manifest digests, rather than mutable tag resolution, were used
in the live deployments and match the running containers' image IDs.

## Data preservation and content comparison

Before rollout, direct read-only queries confirmed schema version 3 with
`dirty=0`, the expected legacy schema, and **zero rows** in `otp.templates`.
The zero-row result was rechecked immediately before rollout. No existing
template rows were lost when migration 0004 replaced that empty table.

A complete single-transaction OTP database dump, including schema, data,
routines and triggers, was saved before mutation. The file is private to the
operator account (mode 0600), outside the repository:

`/home/dk/.local/state/worklane/backups/otp-before-template-studio-20261115T124551Z.sql`

- Size: 7,287 bytes.
- SHA-256: `13497027b338f90ce15c1d3e2f3c52d03623636b50cd95296d6ff0f98c77a602`.
- Dump command succeeded; expected template schema and dump-completion marker
  were checked. A restore rehearsal was not performed.
- Credentials remained inside the existing MySQL container environment and were
  not printed or written into the dump command. No credential value was recorded.

The live ConfigMap had only the legacy SMS-format override among template keys;
neither deployment had template-specific environment overrides, and the shared
secret's key names contained no template overrides. The prior dispatcher defaults
and SMS override rendered the same subject/body as the new seeded templates:

- English email subject: `Your verification code`.
- Email and SMS body: `Your verification code is {{code}}. It expires in {{expiry}}.`
- Expiry substitution defaults to `5 minutes`, matching the prior literal text.

This comparison establishes expected content continuity from configuration and
source. It is not proof of mailbox receipt or SMS delivery.

## Rollout and checks

All timestamps below are UTC on 2026-11-15.

| Time | Observation |
| --- | --- |
| 12:45:51-12:45:52 | Database backup completed; schema 3 clean and legacy template table empty. |
| 12:46:10 | New otp-api logged that it was listening; rollout subsequently completed. |
| Before dispatcher update | Database showed schema 4 clean and both expected published v1 templates; public template route returned HTTP 401. |
| 12:46:41 | New dispatcher logged Resend provider selection and consumption of `otp.requested`. |
| 12:46:55 | Both rollouts complete, exact running image IDs matched pins; zero restarts. |
| 12:47:14 | Kafka consumer group `otp-dispatcher` Stable, one member, total lag 0. |
| 12:47:25-12:47:27 | Public curl probes: `/healthz` HTTP 200 with `status=ok`; templates, stats, requests, delivery logs and `/auth/me` HTTP 401. Protected routes included the dashboard origin in their CORS allow-origin header. |
| 12:47:27 | Schema 4 remained clean; one English email template, one English SMS template and two total version rows. |

A Python urllib probe at 12:47:14-12:47:16 was rejected by Cloudflare with
HTTP 403/code 1010, reproducing the user-agent-specific edge limitation recorded
for #4. The subsequent curl probes reached the application. No edge policy was
changed; urllib failure is not claimed as application success.

The production overlay rendered successfully with `kubectl kustomize`, producing
the exact two new OTP pins and preserving the auth-svc pin. Existing local tests,
typechecking and lint passed in the immediately preceding initial inspection;
application source is unchanged by this deployment record and manifest update.
No repeated test-suite run is claimed for these documentation/manifest changes.

## Recovery and remaining verification

The previous running image digests remain recorded in the initial inspection.
An application recovery can repin those images; it does not reverse migration 4.
The old dispatcher uses environment templates and the old API does not read the
legacy template table. Neither requires automatically dropping the new versioned
tables to recover the old application behavior. Schema downgrade or backup
restoration would be a separate destructive operation requiring review of any
data written since the backup. No recovery operation was needed or performed.

The seeded English email template is
`11111111-1111-1111-1111-111111111111`, active v1
`11111111-0000-0000-0000-000000000001`, observed directly in production MySQL.
Both supported dashboard locales remain `en` and `vi`; only English templates
were seeded. Actual users of the shared English email selection have not been
fully inventoried. The separate test tenant does not isolate template changes.

Completing #7 still requires authenticated dashboard inspection, a specific
test-content/scope/restoration approval, controlled mailbox receipt from the
test publication, and a second receipt after republishing the preserved original
version and accounting for cache propagation. No completed-ticket claim follows
from this successful deployment.
