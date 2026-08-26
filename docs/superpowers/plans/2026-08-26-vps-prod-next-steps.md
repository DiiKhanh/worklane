# VPS prod - next steps

Follow-ups after the initial bring-up (see [vps-bringup runbook](../../runbooks/vps-bringup.md)).
Prod is live: API at `https://api-otp.dikhanh.io.vn`, real email via Resend, logs+metrics in
Grafana Cloud. These items harden it toward the "OTP live **and stable**, operated solo" gate from
the [prod-ops hardening spec](../specs/2026-08-25-prod-ops-hardening-design.md).

Ordered by priority. Each item states its **done-when**.

## P1 - do soon (stability / stops noise)

### 1. Backup CronJob is failing nightly until R2 exists
The `mysql-backup` CronJob was applied but has no `r2-backup` secret, so it will fail every night.
**Step-by-step execution: [backup-r2 runbook](../../runbooks/backup-r2.md)** (secret keys, test job, restore drill).
Pick one:
- **Set up R2 now** (preferred): create a Cloudflare R2 bucket + S3 API token, then
  `kubectl -n worklane create secret generic r2-backup --from-literal=R2_ENDPOINT=... --from-literal=R2_BUCKET=... --from-literal=AWS_ACCESS_KEY_ID=... --from-literal=AWS_SECRET_ACCESS_KEY=...`
  (exact keys: see `deploy/k8s/base/backup/backup-cronjob.yaml` and the k8s README).
- **Or suspend it** until you get to R2: `kubectl -n worklane patch cronjob mysql-backup -p '{"spec":{"suspend":true}}'`.
- **Done-when:** either a nightly dump lands in R2 (and one **restore drill** succeeds - see hardening spec §8), or the CronJob is suspended so no failed jobs pile up.

### 2. Alerting -> Telegram (per docs/runbooks/alerting.md)
Two alerts, one channel:
- **Uptime**: external monitor (Uptime Kuma elsewhere or a hosted free pinger) hits
  `https://api-otp.dikhanh.io.vn/healthz` every 60s; alert on down/recovered.
  > Note: `/healthz` is intentionally not routed through Traefik. For an external uptime check,
  > either add a public `/healthz` IngressRoute, or monitor a routed path that returns a stable
  > code (e.g. a `401` on `/v1/otp/send` with a bad key still proves the API is up). Decide which.
- **Disk > 80%**: Grafana-managed alert rule on `node_filesystem_avail_bytes` (query in the alerting
  runbook) -> Telegram. Disk is the single most likely way this node dies.
- **Done-when:** a killed pod / a down endpoint fires a Telegram message, and recovery fires too.

## P2 - correctness of the delivery + email paths

### 3. Prove deploy.sh + rollback.sh
Prod is the only k8s env, so the way back must be tested before it is needed.
- Install `kustomize` on the VPS (deploy.sh uses `kustomize edit set image`).
- Run `deploy/scripts/deploy.sh <sha>` to a newer built SHA, confirm the running image changes, then
  `deploy/scripts/rollback.sh` and confirm it returns. Commit the overlay tag change so git records it.
- **Done-when:** a real deploy and a real rollback each move the running SHA, verified.

### 4. Verify `dikhanh.io.vn` on Resend (send to any recipient)
Today the sandbox sender `onboarding@resend.dev` only delivers to the Resend account owner.
- Add the SPF/DKIM DNS records Resend gives, on Cloudflare, and verify the domain.
- Change `RESEND_FROM` in `deploy/k8s/base/config.yaml` to a `@dikhanh.io.vn` sender; re-apply.
- **Done-when:** an OTP email is delivered to an arbitrary address, not just the account owner.

## P3 - product surface (separate tracks)

### 5. Dashboard on Vercel - DONE (2026-08-26)
Deployed `dashboard/` (Next.js, root dir `dashboard`) to Vercel at
**`https://worklane-six.vercel.app`** with `NEXT_PUBLIC_DATA_SOURCE=live` +
`NEXT_PUBLIC_API_BASE=https://api-otp.dikhanh.io.vn`. CORS origin set in
`deploy/k8s/base/ingress/middlewares.yaml` (committed) and applied. Human login
(`you@demo.co`) works over HTTPS with live data. Optional follow-up: a custom
domain (e.g. `otp.dikhanh.io.vn` CNAME to Vercel) instead of the `.vercel.app` URL.

### 6. SMS channel (Twilio)
Add `TWILIO_ACCOUNT_SID` / `TWILIO_AUTH_TOKEN` to `worklane-secrets` and a real `TWILIO_FROM` in config;
`/v1/otp/send` with `"channel":"sms"` should deliver.
- **Done-when:** an SMS OTP is delivered and verified.

## Known-harmless (no action unless it changes)

- **`ghcr-pull` not found** warning on pods: GHCR packages are public, so kubelet pulls anonymously.
  Only create the `ghcr-pull` docker-registry secret if the packages are made private.

## Explicitly skipped (owner's call)

- Rotating the Grafana Cloud token (`glc_...`) - owner opted to keep it.
