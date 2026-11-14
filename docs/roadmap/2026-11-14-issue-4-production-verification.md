# Production reachability and deployed versions

Source: [issue #4](https://github.com/DiiKhanh/worklane/issues/4), part of
[verification milestone #3](https://github.com/DiiKhanh/worklane/issues/3).

## Result

The production API was externally reachable on 2026-11-14. The earlier HTTP
530 / Cloudflare error 1033 reported in #3 did not reproduce. Its historical
cause remains **unproven**; current service start times do not establish it.
No outage remediation was applied or is proposed from this snapshot.

The running backend uses `sha-041efd7`, before Template Studio. Its missing
template route is confirmed externally. The dashboard's public build is
identified below, and the Vercel owner confirmed Production source `main` at
`280fd11`. **The verification criteria for issue #4 are satisfied.** Production
Template Studio verification cannot proceed on the current backend.

## Environment and boundaries

- Operational observation window: 2026-11-14 17:03:42-17:05:35 UTC.
- Vercel source confirmation: owner-provided evidence in the implementation
  session, recorded at 2026-11-14 17:09:33 UTC after the operational inspection.
- API: `https://api-otp.dikhanh.io.vn` through Cloudflare Tunnel and Traefik.
- Dashboard: `https://worklane-six.vercel.app` on Vercel.
- Cluster: node `dk`, namespace `worklane`; inspected with the existing user
  kubeconfig at `/home/dk/.kube/config`, API server `https://127.0.0.1:6443`.
- Repository baseline: `280fd11d38f30c8f9ca5bd6e0b821a1e84311702` on `main`.
- Only unauthenticated GETs, read-only cluster queries and GitHub metadata reads
  were used. No account, tenant, mailbox or OTP was needed for this ticket.
  No production writes, notifications, credentials, access controls or deployments
  were changed. No OTPs, credentials or sensitive response data are recorded.

## External HTTP evidence

Times are UTC on 2026-11-14. API calls used curl unless stated otherwise.
Response excerpts are limited to non-sensitive status/error bodies.

| Time | Request | Result | Interpretation |
| --- | --- | --- | --- |
| 17:03:42 | Python urllib GET `/healthz` | 403, `error code: 1010`; ray `a5a79752eec75c30-SIN` | Client-specific edge rejection; insufficient evidence of backend outage. |
| 17:03:43 | Dashboard GET `/` | 200 | Public HTML reachable; no authenticated workflow verified. |
| 17:04:06 | API GET `/healthz` | 200, `{"status":"ok"}`; ray `a5a797e6e99cce8b-SIN` | Public tunnel, route and OTP HTTP process reachable. |
| 17:04:06 | Loopback GET `/healthz`, API Host header | 200, `{"status":"ok"}` | Local Traefik route reachable independently of Cloudflare. |
| 17:04:28 | GET `/healthz`, User-Agent `curl/7.68.0` | 200; ray `a5a79872bbcf0a06-SIN` | Differential probe baseline. |
| 17:04:28 | Same request, User-Agent `Python-urllib/3.12` | 403, `error code: 1010`; ray `a5a798744bdffd8b-SIN` | Changing only User-Agent reproduced rejection. |
| 17:04:29 | Same request, User-Agent restored to `curl/7.68.0` | 200; ray `a5a798756eae55d8-SIN` | Rules out a general outage during this comparison. |
| 17:04:29 | API GET `/v1/otp/requests` | 401, `{"error":"missing credentials"}` | OTP route and authentication boundary responding. |
| 17:04:29 | API GET `/auth/me` | 401, `{"error":"missing token"}` | Auth route and authentication boundary responding. |
| 17:04:29 | API GET `/internal/introspect` | 404 | Expected: no public Traefik rule for `/internal`; no inference about internal POST handler health. |
| 17:05:34 | API GET `/healthz` | 200; ray `a5a79a112ad14fb9-SIN` | Public reachability confirmed again. |
| 17:05:35 | API GET `/v1/templates` | 404, `404 page not found` | Missing backend capability, consistent with deployed revision. |
| 17:05:35 | Auth pod GET `:8889/healthz` | 200, `{"status":"ok"}` | Auth HTTP process serving inside cluster. |
| 17:05:35 | OTP pod GET `:8888/healthz` | 200, `{"status":"ok"}` | OTP HTTP process serving inside cluster. |

The User-Agent comparison supports header-dependent edge filtering, but no
Cloudflare security-event evidence was available to identify the exact rule.
No security-policy change is justified by these probes. This 403 is separate
from the historical 530/1033.

The live IngressRoute `worklane/worklane-api` (generation 2) explicitly exposes
exact `/healthz` to `otp-api:8888`, `/v1` to `otp-api:8888`, and `/auth` to
`auth-svc:8889`. `/internal` is excluded. Health exposure was checked against
the live object rather than assumed from a generic deployment configuration.

## Operational health

At 17:04:06-17:04:45 UTC, all seven long-running workloads were Running and
1/1 Ready: auth-svc, otp-api, otp-dispatcher, MySQL, Redis, Redpanda and Alloy.
The old seed pod was Completed. App deployments had one desired, ready and
available replica each, with observed generations matching desired generations
(auth-svc 5, otp-api 5, otp-dispatcher 7).

Node `dk` was Ready with MemoryPressure, DiskPressure and PIDPressure all False.
k3s and cloudflared were active/running, with systemd NRestarts 0; active since
15:38:07 and 15:38:01 UTC respectively. App and database containers had four
historical restarts; Alloy had six. This snapshot does not establish their causes
or historical availability.

Readiness checks provide different guarantees: auth-svc and otp-api use HTTP
`/healthz`, MySQL uses `mysqladmin ping`, Redis uses `redis-cli ping`, and Redpanda
checks `rpk cluster health`. Dispatcher and Alloy have no readiness probes.
The app health handler is process-only and deliberately does not check databases
or Redis. These observations establish routing and basic operational health,
not email delivery, authenticated data access or full dependency correctness.

## Deployed versions and mismatch

At 17:04:28 UTC, the live Deployment image tags and running Pod imageIDs were:

| Service | Image tag | Running image digest |
| --- | --- | --- |
| auth-svc | `ghcr.io/diikhanh/worklane-auth-svc:sha-041efd7` | `sha256:295cb62a76d5c3d6f6187b982ffc34d94be4d05770fc2cc8bb14d6d771dffb53` |
| otp-api | `ghcr.io/diikhanh/worklane-otp-api:sha-041efd7` | `sha256:ef1d6eb7200a094763a8280f05ed2d693ab5211c89f120d7d2ce51ef50497c6f` |
| otp-dispatcher | `ghcr.io/diikhanh/worklane-otp-dispatcher:sha-041efd7` | `sha256:9e36fcad357ea07258ae78cef68fe4978aff12b1f68ce3683f8626208e982a7e` |

The tag-associated Git revision is
`041efd743b3a16c07df296983b9981ee7d26c51a`. The checked-in production overlay
agrees with those live tags. The digests identify the actual running artifacts;
the tag naming convention alone is not an independent source-provenance attestation.

At 17:05:13 UTC, the public dashboard HTML contained Next.js build identifier
`IaOCtTcnw4CuirLqgWV1p` and ETag `"a61a8684c9c4da5a8f38873f28571d87"`.
The retrieved HTML SHA-256 at 17:04:45 UTC was
`b290ab689611294f285c1bc6fc1dd3a8953de52e732243d3390e06c3a8d998e8`.

The latest GitHub Production deployment record,
[7790487495](https://api.github.com/repos/DiiKhanh/worklane/deployments/7790487495),
reports source `280fd11d38f30c8f9ca5bd6e0b821a1e84311702` and success at
2026-11-14 16:36:03 UTC. Its environment URL is
`https://worklane-2hnqquoft-diikhanhs-projects.vercel.app`.
That URL redirected to Vercel login, so its artifact could not be compared with
the public alias. The record identifies a successful deployment, but does not
prove the current alias points to it. In response to the request to inspect
Vercel's Current Production deployment, the project owner supplied Source
`main`, commit `280fd11`, title `docs(dashboard): clarify setup and playground feedback`.
This establishes the dashboard source through owner-provided evidence matching
the GitHub deployment record, rather than an independently authenticated Vercel
API inspection. The deployment URL above comes from GitHub, not an owner-supplied
URL. Authentication redirects and their transient parameters are omitted.

The verification milestone requires Template Studio backend functionality merged
in [PR #2](https://github.com/DiiKhanh/worklane/pull/2), merge commit
`a5ef6208189376b624a52f273e01be9a5a71ddff`. The running backend revision predates
that merge: its router has `/v1/stats` but no `/v1/templates`, and its dispatcher
predates DB-backed template resolution. Therefore stats availability must not be
conflated with Template Studio deployment. A coordinated backend upgrade requires
separate specific production approval and subsequent verification; it was not
performed as part of this reachability check.

## Reproduction and validation

The core read-only external probes can be repeated with:

```sh
date -u +%FT%TZ
curl -sS -i --max-time 20 https://api-otp.dikhanh.io.vn/healthz
curl -sS -i --max-time 20 https://api-otp.dikhanh.io.vn/v1/otp/requests
curl -sS -i --max-time 20 https://api-otp.dikhanh.io.vn/auth/me
curl -sS -i --max-time 20 https://api-otp.dikhanh.io.vn/v1/templates
curl -sS -i --max-time 20 -A Python-urllib/3.12 https://api-otp.dikhanh.io.vn/healthz
```

Cluster evidence came from `kubectl get` for Pods, Deployments, node conditions,
service endpoints and the live IngressRoute, using the existing user kubeconfig.
No secret objects or application data were read. Default kubectl initially tried
the root-only k3s config; selecting the existing user kubeconfig resolved that
local tooling issue without changing permissions. The historical `dk-vps` SSH
alias was absent locally; cluster access succeeded directly on the host.

No application code or tests changed, so TDD and new regression tests were not
applicable. The requested local full-suite commands were attempted during the
17:04-17:05 UTC inspection: `make test` and dashboard
`pnpm typecheck && pnpm lint && pnpm test` could
not start because make and pnpm were absent; Go and Node were also absent from
PATH. No local passing-test claim is made.

Independent existing evidence: GitHub Actions
[run 43616442665](https://github.com/DiiKhanh/worklane/actions/runs/43616442665)
for baseline `280fd11d38f30c8f9ca5bd6e0b821a1e84311702` reported completed/success
for test, smoke and all four image-build jobs at inspection. Those checks do not
verify the older production backend or this documentation change.

## Acceptance status

- External behavior and operational health: recorded with UTC timestamps.
- Historical failure: not reproduced; cause explicitly unproven.
- Outage remediation and post-remediation checks: not needed for the observed
  reachable API; no production change was applied.
- Backend artifacts and revision mismatch: identified; Template Studio unavailable.
- Dashboard public artifact: identified; Source `main` at `280fd11` confirmed
  by the Vercel owner and matched to the GitHub deployment record.
- Issue #4 verification complete. The GitHub issue was not edited or closed;
  tracker publication was not authorized by this implementation request.
