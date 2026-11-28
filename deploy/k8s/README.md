# worklane on k3s - deploy runbook

Prerequisites: a running k3s cluster (built-in Traefik), `kubectl` context set, images
pushed to GHCR (see `.github/workflows/images.yml`), and the host/tunnel up
(`docs/superpowers/specs/2026-08-07-self-hosted-infra-setup.md`).

## 1. Namespace

    kubectl apply -f deploy/k8s/base/namespace.yaml

## 2. Secrets (created out-of-band - never committed)

Replace the placeholder values. `<pw>` is the MySQL root password; the DSNs embed it.

    ROOT_PW='<pw>'

    kubectl -n worklane create secret generic worklane-secrets \
      --from-literal=MYSQL_ROOT_PASSWORD="$ROOT_PW" \
      --from-literal=OTP_MYSQL_DSN="root:$ROOT_PW@tcp(mysql:3306)/otp?parseTime=true&multiStatements=true" \
      --from-literal=IDENTITY_MYSQL_DSN="root:$ROOT_PW@tcp(mysql:3306)/identity?parseTime=true&multiStatements=true" \
      --from-literal=INTERNAL_API_TOKEN="$(openssl rand -hex 32)" \
      --from-literal=RESEND_API_KEY='<resend-key>' \
      --from-literal=TWILIO_ACCOUNT_SID='<twilio-sid>' \
      --from-literal=TWILIO_AUTH_TOKEN='<twilio-token>'

For a free/internal SMS smoke, use Twilio **test credentials** for the two Twilio values above. The
base ConfigMap already sets `TWILIO_FROM="+15005550006"`, which is Twilio's magic valid sender for
test credentials. See `docs/runbooks/sms-twilio-test-credentials.md` before patching an existing
production secret.

    # JWT keypair (reuse the local dev keypair or generate a fresh one):
    kubectl -n worklane create secret generic worklane-jwt \
      --from-file=auth_priv.pem=deploy/compose/secrets/auth_priv.pem \
      --from-file=auth_pub.pem=deploy/compose/secrets/auth_pub.pem

    # GHCR pull secret (private packages):
    kubectl -n worklane create secret docker-registry ghcr-pull \
      --docker-server=ghcr.io \
      --docker-username=<github-user> \
      --docker-password=<github-pat-with-read:packages>

### Link Service secrets

Create `worklane-links` in the `worklane` namespace before applying the Link workloads.
Generate each MySQL password independently with `openssl rand -hex 24` and capture the
output without printing it.

| Key | Value |
| --- | --- |
| `LINK_DB_ROOT_PASSWORD` | Root password for the dedicated `link-mysql` instance |
| `LINK_DB_PASSWORD` | Password for the `links` database user |
| `LINK_MYSQL_DSN` | `links:<LINK_DB_PASSWORD>@tcp(link-mysql:3306)/links` |

The Link applications use the DSN for the `links` user. `link-svc` reads
`INTERNAL_API_TOKEN` from `worklane-secrets` and mounts `auth_pub.pem` from `worklane-jwt`.

## 3. Deploy / rollback (prod)

First bring-up (or any manual apply):

    kubectl apply -k deploy/k8s/overlays/prod
    kubectl -n worklane get pods -w   # mysql/redpanda/redis first, then the apps

Routine deploy of a CI-built SHA (find it in the GHCR package tags or the Actions run) -
pins all image tags to the SHA, applies, and waits for the rollout:

    deploy/scripts/deploy.sh <git-sha>
    git commit -am "deploy: prod -> <git-sha>"   # record what is running

Roll back fast to the previous ReplicaSet:

    deploy/scripts/rollback.sh

Roll back to a specific known-good SHA:

    deploy/scripts/rollback.sh <git-sha>

Before the initial Link rollout, replace both temporary Link image tags in
`overlays/prod/kustomization.yaml` with the published `sha-*` tags and matching
GHCR digests. Apply the Link resources in order (`link-mysql`, `link-svc`,
`link-dispatcher`, then middleware and routes) to preserve the running OTP and auth
image versions. `link-svc` migrates the dedicated database before serving `/healthz`.

## 4. Seed a tenant/user/api-key (on demand)

    kubectl apply -f deploy/k8s/overlays/prod/seed-job.yaml
    kubectl -n worklane logs job/seed   # prints the API key once
    kubectl -n worklane delete job/seed # clean up when done

## 5. Verify

    # DB split
    kubectl -n worklane exec sts/mysql -- sh -c \
      'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" identity -e "SHOW TABLES;"'
    # Machine path (through the tunnel):
    curl -s -o /dev/null -w "%{http_code}\n" \
      -H "Authorization: Bearer <key>" https://api.otp.<domain>/v1/otp/requests   # 200
    # Human path:
    curl -s -X POST https://api.otp.<domain>/auth/login \
      -H 'Content-Type: application/json' \
      -d '{"email":"you@demo.co","password":"changeme-now"}'                      # token
    # Internal endpoint must NOT be routed:
    curl -s -o /dev/null -w "%{http_code}\n" \
      -X POST https://api.otp.<domain>/internal/introspect -d '{}'               # 404

### Link production smoke

After opening the routes, run `deploy/scripts/smoke-links.sh` manually with
`LINK_SMOKE_API_KEY` exported from a dedicated smoke tenant. The script needs `curl` and
`jq`. Its defaults are `LINK_SMOKE_API_BASE=https://api-otp.dikhanh.io.vn` and
`LINK_SMOKE_PUBLIC_BASE=https://link.dikhanh.io.vn`. Optionally export
`LINK_SMOKE_OTHER_API_KEY` from a different tenant for a read-only isolation check.

Record the printed code with the rollout evidence. Each successful run leaves one
disabled link and one recorded click in the smoke tenant. The detailed visit expires
after 30 days and the aggregate remains. Production verification uses this manual
smoke script. The Compose E2E and load suites are for disposable environments.

## Grafana Cloud secret (out-of-band)

From the Grafana Cloud stack: copy the Prometheus remote_write URL + user id,
the Loki push URL + user id, and an access-policy token, then:

    kubectl -n worklane create secret generic grafana-cloud \
      --from-literal=PROM_URL='<grafana-cloud-prom-url>' \
      --from-literal=PROM_USER='<grafana-cloud-prom-user>' \
      --from-literal=LOKI_URL='<grafana-cloud-loki-url>' \
      --from-literal=LOKI_USER='<grafana-cloud-loki-user>' \
      --from-literal=TOKEN='<grafana-cloud-token>'

## R2 backup secret (out-of-band)

Create an R2 bucket + an S3-compatible API token, then:

    kubectl -n worklane create secret generic r2-backup \
      --from-literal=R2_ENDPOINT='<r2-s3-endpoint>' \
      --from-literal=R2_BUCKET='<r2-bucket>' \
      --from-literal=AWS_ACCESS_KEY_ID='<r2-access-key>' \
      --from-literal=AWS_SECRET_ACCESS_KEY='<r2-secret-key>' \
      --from-literal=AWS_DEFAULT_REGION='auto'

## Disk guardrails (host)

k3s image garbage collection - reclaim old :<git-sha> images automatically.
Edit /etc/rancher/k3s/config.yaml on the node:

    kubelet-arg:
      - "image-gc-high-threshold=80"
      - "image-gc-low-threshold=70"

Then: systemctl restart k3s

Redpanda defaults to 10 minutes and 256 MB per partition
(`base/redpanda/statefulset.yaml`). `link-dispatcher` configures `link.clicked` to
retain events for up to seven days, subject to the same 256 MB cap for its single
partition. Both MySQL instances use a 128M InnoDB buffer pool.

Alerting setup (Telegram + disk alert + uptime ping): see docs/runbooks/alerting.md

## Notes

- otp-api & otp-dispatcher read `MYSQL_DSN` = `OTP_MYSQL_DSN`; auth-svc reads
  `MYSQL_DSN` = `IDENTITY_MYSQL_DSN`. Both databases live on the one in-cluster MySQL.
- `/internal/introspect` has no IngressRoute; otp-api reaches auth-svc in-cluster.
- Redis is ephemeral (no PVC) - all its data is TTL/cache/regenerable.
- Before the first live apply, set the real `api.otp.<domain>` host in
  `overlays/prod/ingressroute.yaml` and the real Vercel origin in
  `base/ingress/middlewares.yaml`.

## Link routing

- `worklane-links-api` takes `/v1/links` and `/v1/links/*` on `api-otp.dikhanh.io.vn`
  ahead of the generic OTP `/v1` route. It uses `otp-ratelimit` and `otp-cors`, which
  exposes `Retry-After` to the dashboard.
- `worklane-links-public` on `link.dikhanh.io.vn` serves `GET /healthz` and matches
  `GET` and `HEAD` on one-segment alphanumeric paths of 1 to 11 characters, excluding
  `/metrics`. The redirect rule needs Traefik v3 and limits requests per
  `CF-Connecting-IP` (100 per second, burst 50). Confirm the Cloudflare Tunnel is the
  sole inbound path to Traefik before exposing this route so that the header can be trusted.
- Alloy scrapes `link-svc` on the declared port 8890 inside the cluster. See the
  [deployment topology](../../docs/architecture.md#7-link-service-on-k3s).
