# VPS prod - next steps

Follow-ups after the initial bring-up (see [vps-bringup runbook](../../runbooks/vps-bringup.md)).
Prod is live: API at `https://api-otp.dikhanh.io.vn`, dashboard at
`https://worklane-six.vercel.app`, real email via Resend from `no-reply@dikhanh.io.vn`, and
logs+metrics in Grafana Cloud. These items harden it toward the "OTP live **and stable**, operated
solo" gate from the [prod-ops hardening spec](../specs/2026-08-25-prod-ops-hardening-design.md).

Ordered by priority. Each item states its **done-when**.

## P1 - do soon (stability / stops noise)

### 1. R2 backup + restore drill - PENDING
The `mysql-backup` CronJob was applied, but the R2 setup and restore drill are still pending. Until
the `r2-backup` secret exists, the CronJob will fail each night; either create the secret and prove a
restore, or suspend the CronJob temporarily.
**Step-by-step execution: [backup-r2 runbook](../../runbooks/backup-r2.md)** (secret keys, test job, restore drill).
Pick one:
- **Set up R2 now** (preferred): create a Cloudflare R2 bucket + S3 API token, then
  `kubectl -n worklane create secret generic r2-backup --from-literal=R2_ENDPOINT=... --from-literal=R2_BUCKET=... --from-literal=AWS_ACCESS_KEY_ID=... --from-literal=AWS_SECRET_ACCESS_KEY=...`
  (exact keys: see `deploy/k8s/base/backup/backup-cronjob.yaml` and the k8s README).
- **Or suspend it** until you get to R2: `kubectl -n worklane patch cronjob mysql-backup -p '{"spec":{"suspend":true}}'`.
- **Done-when:** either a nightly dump lands in R2 (and one **restore drill** succeeds - see hardening spec §8), or the CronJob is suspended so no failed jobs pile up.

### 2. Alerting -> Telegram - DONE (2026-08-28)
Both alerts live and **verified end-to-end** (fire + recovery to Telegram), routed to one
`telegram` contact point via per-rule `notification_settings`:
- **Disk > 80%**: Grafana-managed rule `node-disk-usage>80%` on
  `node_filesystem_*{mountpoint="/",fstype="ext4"}` (note: `/`, not `/host/root` - Alloy's
  `rootfs_path` strips the prefix).
- **Uptime**: Grafana Cloud **Synthetic Monitoring** check `otp-api-healthz` probes the new
  public `/healthz` (see `deploy/k8s/overlays/prod/ingressroute.yaml`) from Singapore; rule
  `otp-api-uptime-down` fires on `probe_success < 1`.
See [alerting runbook](../../runbooks/alerting.md) for exact setup.

<details><summary>original task</summary>

Two alerts, one channel:
- **Uptime**: external monitor (Uptime Kuma elsewhere or a hosted free pinger) hits
  `https://api-otp.dikhanh.io.vn/healthz` every 60s; alert on down/recovered.
  > Note: `/healthz` is intentionally not routed through Traefik. For an external uptime check,
  > either add a public `/healthz` IngressRoute, or monitor a routed path that returns a stable
  > code (e.g. a `401` on `/v1/otp/send` with a bad key still proves the API is up). Decide which.
- **Disk > 80%**: Grafana-managed alert rule on `node_filesystem_avail_bytes` (query in the alerting
  runbook) -> Telegram. Disk is the single most likely way this node dies.
- **Done-when:** a killed pod / a down endpoint fires a Telegram message, and recovery fires too.

</details>

## P2 - correctness of the delivery + email paths

### 3. Prove deploy.sh + rollback.sh - DONE (2026-08-28)
Prod is the only k8s env, so the way back was tested before it is needed.
- `deploy/scripts/deploy.sh <sha>` moves the running image SHA and waits for rollouts.
- `deploy/scripts/rollback.sh` successfully returns the services to the previous ReplicaSet.
- **Done-when:** a real deploy and a real rollback each move the running SHA, verified.

### 4. Verify `dikhanh.io.vn` on Resend - DONE (2026-08-28)
Resend is verified for `dikhanh.io.vn`, and production sends from `no-reply@dikhanh.io.vn`.
- `RESEND_FROM` is set in `deploy/k8s/base/config.yaml`.
- **Done-when:** an OTP email is delivered to an arbitrary address, not just the Resend account owner.

## P3 - product surface (separate tracks)

### 5. Dashboard on Vercel - DONE (2026-08-26)
Deployed `dashboard/` (Next.js, root dir `dashboard`) to Vercel at
**`https://worklane-six.vercel.app`** with `NEXT_PUBLIC_DATA_SOURCE=live` +
`NEXT_PUBLIC_API_BASE=https://api-otp.dikhanh.io.vn`. CORS origin set in
`deploy/k8s/base/ingress/middlewares.yaml` (committed) and applied. Human login
(`you@demo.co`) works over HTTPS with live data. Optional follow-up: a custom
domain (e.g. `otp.dikhanh.io.vn` CNAME to Vercel) instead of the `.vercel.app` URL.

### 6. SMS channel (Twilio)
The SMS code path exists. The chosen free/internal production smoke path is Twilio test credentials,
using `TWILIO_FROM=+15005550006` and the real Twilio API URL. This exercises the API -> Kafka ->
dispatcher -> Twilio adapter -> MySQL path without sending a real SMS or charging the account.
**Step-by-step execution: [SMS Twilio test credentials runbook](../../runbooks/sms-twilio-test-credentials.md)**.
- **Done-when:** a Twilio test-credential smoke writes a `provider=twilio,status=sent` delivery log in
  production. A real handset receive-and-verify test remains a separate later live-SMS check.

### 7. `/v1/stats` for dashboard Overview - CODE READY, PENDING PROD DEPLOY
The code path adds a tenant-scoped `GET /v1/stats` endpoint and wires the dashboard live data source
to it. Deploy otp-api plus dashboard to make live Overview show real rolling 24h aggregates.
- **Done-when:** live dashboard Overview shows real tenant aggregates instead of the current
  "Metrics unavailable" state.

## Known-harmless (no action unless it changes)

- **`ghcr-pull` not found** warning on pods: GHCR packages are public, so kubelet pulls anonymously.
  Only create the `ghcr-pull` docker-registry secret if the packages are made private.

## Explicitly skipped (owner's call)

- Rotating the Grafana Cloud token (`glc_...`) - owner opted to keep it.
