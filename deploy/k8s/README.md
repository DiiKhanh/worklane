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

    # JWT keypair (reuse the local dev keypair or generate a fresh one):
    kubectl -n worklane create secret generic worklane-jwt \
      --from-file=auth_priv.pem=deploy/compose/secrets/auth_priv.pem \
      --from-file=auth_pub.pem=deploy/compose/secrets/auth_pub.pem

    # GHCR pull secret (private packages):
    kubectl -n worklane create secret docker-registry ghcr-pull \
      --docker-server=ghcr.io \
      --docker-username=<github-user> \
      --docker-password=<github-pat-with-read:packages>

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
      --from-literal=AWS_SECRET_ACCESS_KEY='<r2-secret-key>'

## Disk guardrails (host)

k3s image garbage collection - reclaim old :<git-sha> images automatically.
Edit /etc/rancher/k3s/config.yaml on the node:

    kubelet-arg:
      - "image-gc-high-threshold=80"
      - "image-gc-low-threshold=70"

Then: systemctl restart k3s

Redpanda log retention is bounded in deploy/k8s/base/redpanda/statefulset.yaml
(log_retention_ms=10m, retention_bytes=256MB) so the Kafka log cannot fill /var.
MySQL's InnoDB buffer pool is pinned to 128M in deploy/k8s/base/mysql/statefulset.yaml.

Alerting setup (Telegram + disk alert + uptime ping): see docs/runbooks/alerting.md

## Notes

- otp-api & otp-dispatcher read `MYSQL_DSN` = `OTP_MYSQL_DSN`; auth-svc reads
  `MYSQL_DSN` = `IDENTITY_MYSQL_DSN`. Both databases live on the one in-cluster MySQL.
- `/internal/introspect` has no IngressRoute; otp-api reaches auth-svc in-cluster.
- Redis is ephemeral (no PVC) - all its data is TTL/cache/regenerable.
- Before the first live apply, set the real `api.otp.<domain>` host in
  `overlays/prod/ingressroute.yaml` and the real Vercel origin in
  `base/ingress/middlewares.yaml`.
