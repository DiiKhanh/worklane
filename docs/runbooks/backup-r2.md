# Backup runbook - MySQL -> Cloudflare R2

Status: **PENDING**. Make the nightly `mysql-backup` CronJob actually work by creating the
`r2-backup` secret, running a one-off backup job, and proving a restore into a scratch MySQL. Until
that is done, either expect the CronJob to fail nightly or suspend it temporarily. Part of P1 in
[vps-prod next steps](../superpowers/plans/2026-08-26-vps-prod-next-steps.md).

The CronJob is `deploy/k8s/base/backup/backup-cronjob.yaml`: an initContainer runs
`mysqldump --databases otp identity | gzip`, the main container `aws s3 cp`s the
gzip to R2. It loads the R2 secret via `envFrom: secretRef: r2-backup`, so **every
key in that secret becomes an env var**. Schedule `0 18 * * *` UTC = 01:00 local.

## Secret contract (`r2-backup`, namespace `worklane`)

`aws-cli` reads the AWS_* vars from env automatically; the command uses `$R2_BUCKET`
and `$R2_ENDPOINT`. Exactly these 5 keys:

| Key | Value |
|-----|-------|
| `R2_ENDPOINT` | `https://<account_id>.r2.cloudflarestorage.com` (from the R2 API token screen) |
| `R2_BUCKET` | `worklane-backups` |
| `AWS_ACCESS_KEY_ID` | R2 API token Access Key ID |
| `AWS_SECRET_ACCESS_KEY` | R2 API token Secret Access Key |
| `AWS_DEFAULT_REGION` | `auto` (aws-cli v2 requires a region; R2 accepts `auto`) |

## Steps

- [ ] **1. Create R2 bucket** (Cloudflare dashboard -> R2 -> Create bucket): name
  `worklane-backups`, default location. Free tier 10GB/mo; backups are a few MB.
- [ ] **2. Create R2 API token** (R2 -> Manage R2 API Tokens -> Create): name
  `worklane-backup`, permission **Object Read & Write**, optionally scoped to the
  bucket. Copy the **Access Key ID**, **Secret Access Key**, and **S3 endpoint**
  (shown once).
- [ ] **3. Create the k8s secret** on the VPS (fill in your values):
  ```bash
  kubectl -n worklane create secret generic r2-backup \
    --from-literal=R2_ENDPOINT='https://<account_id>.r2.cloudflarestorage.com' \
    --from-literal=R2_BUCKET='worklane-backups' \
    --from-literal=AWS_ACCESS_KEY_ID='<access-key-id>' \
    --from-literal=AWS_SECRET_ACCESS_KEY='<secret-access-key>' \
    --from-literal=AWS_DEFAULT_REGION='auto'
  ```
- [ ] **4. Test now (don't wait for 01:00)** - run a one-off Job from the CronJob:
  ```bash
  kubectl -n worklane create job --from=cronjob/mysql-backup backup-test-1
  kubectl -n worklane get pods -l job-name=backup-test-1 -w   # wait Completed
  kubectl -n worklane logs job/backup-test-1 --all-containers
  ```
  Then confirm the object exists in R2 (dashboard bucket view, or `aws s3 ls`).
  **Done-when:** `worklane-YYYYMMDD.sql.gz` is in the bucket.
- [ ] **5. Restore drill** (a backup you can't restore is not a backup) - pull the
  dump back, restore into a **throwaway** MySQL, verify tables/rows exist. Do NOT
  restore over the live DB. (Spin up a temp `mysql:8.0` pod, load the gzip, run a
  couple of `SELECT COUNT(*)`.)
  **Done-when:** a restore into a scratch DB shows the expected `otp` + `identity`
  tables with data.

## Gotchas (fill in as hit)

- (none yet)
