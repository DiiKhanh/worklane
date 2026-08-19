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

## 3. Deploy

    kubectl apply -k deploy/k8s/overlays/develop
    kubectl -n worklane get pods -w   # mysql/redpanda/redis first, then the apps

## 4. Seed a tenant/user/api-key (on demand)

    kubectl apply -f deploy/k8s/overlays/develop/seed-job.yaml
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

## Notes

- otp-api & otp-dispatcher read `MYSQL_DSN` = `OTP_MYSQL_DSN`; auth-svc reads
  `MYSQL_DSN` = `IDENTITY_MYSQL_DSN`. Both databases live on the one in-cluster MySQL.
- `/internal/introspect` has no IngressRoute; otp-api reaches auth-svc in-cluster.
- Redis is ephemeral (no PVC) - all its data is TTL/cache/regenerable.
- Before the first live apply, set the real `api.otp.<domain>` host in
  `overlays/develop/ingressroute.yaml` and the real Vercel origin in
  `base/ingress/middlewares.yaml`.
