# Solo-operator prod hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the already-deployed worklane k3s stack *operable as real production by one person* - add a CI test/smoke gate, pinned-image deploy + one-command rollback, Prometheus metrics pushed to Grafana Cloud, Telegram alerting, disk guardrails, and a nightly R2 backup with a restore drill.

**Architecture:** Layers on the existing [k3s deploy](../specs/2026-08-19-k3s-production-deploy-design.md). CI (GitHub Actions) gains a `go test` job and a compose-based E2E smoke job that gate the existing image build. Prod is pinned to `:<git-sha>` and applied by a script (`deploy.sh`), reversible by `rollback.sh`. otp-api and auth-svc expose `/metrics`; a Grafana Alloy agent scrapes app + Redpanda + node metrics and `remote_write`s them (and logs) to Grafana Cloud free tier - nothing observability-related is stored on the 15GB node. Alerts route to Telegram; a nightly `mysqldump` CronJob ships to Cloudflare R2.

**Tech Stack:** GitHub Actions + GHCR, docker-compose (CI test env, already has MySQL/Redis/Redpanda/MailHog), Go 1.25 + gin, `prometheus/client_golang`, Kustomize + Traefik CRDs, k3s, Grafana Alloy, Grafana Cloud (free), Cloudflare R2, Telegram Bot API.

**Design doc:** `docs/superpowers/specs/2026-08-25-prod-ops-hardening-design.md`
**Base spec:** `docs/superpowers/specs/2026-08-19-k3s-production-deploy-design.md`

## Global Constraints

- Module path `github.com/duykhanh/worklane`; single root `go.mod`; Go `1.25.0`. `go test ./...` covers every service.
- **Disk (15GB) is the hard constraint**, not RAM. No observability component may store data on the node (no local Prometheus TSDB / Loki). Redpanda log retention must be bounded.
- Namespace `worklane`. Registry `ghcr.io/duykhanh/worklane-<svc>`. **Prod is pinned to `:<git-sha>`; never `:latest`.**
- Secrets are created **out-of-band** and referenced by name; nothing secret is committed. `.gitignore` already excludes `.env` and `deploy/compose/secrets/` (verified: `deploy/compose/.env` is untracked).
- `/healthz` already exists on otp-api and auth-svc (unauthenticated, DB-free) - do not re-add it.
- The compose stack (`deploy/compose/docker-compose.yml`) already includes MailHog (SMTP `:1025`, API `:8025`) and defaults `EMAIL_PROVIDER=smtp`; the E2E test `test/e2e/e2e_test.go` (`//go:build e2e`) already drives send -> MailHog -> verify. Reuse it as the smoke test; do not write a new one.
- No em dash (`-` only). Commit messages `<type>: <description>`, no co-author line. YAML: 2-space indent, no tabs. Shell scripts: `set -euo pipefail`, pass `shellcheck`.
- Placeholders `<domain>`, `<vercel-origin>`, `<grafana-cloud-*>`, `<telegram-*>`, `<r2-*>` stay literal in committed files; real values are provided out-of-band at execution.
- Live-cluster verification steps are gated on the VPS/k3s being up (per the base plan). In-repo static checks (`kubectl kustomize` builds, `go test`, `shellcheck`, YAML lint) run without the cluster and must pass first.

---

### Task 1: Rename overlay `develop` -> `prod`

The overlay `deploy/k8s/overlays/develop` *is* the production deploy; the "develop" name is an operational trap under the dev=compose / prod=k3s model. Rename it and fix every reference. Pure clarity change - no infra behavior changes.

**Files:**
- Rename: `deploy/k8s/overlays/develop/` -> `deploy/k8s/overlays/prod/` (all 3 files: `kustomization.yaml`, `ingressroute.yaml`, `seed-job.yaml`)
- Modify: `deploy/k8s/README.md` (any `overlays/develop` mention)
- Grep: repo-wide for `overlays/develop`

- [ ] **Step 1: Move the directory with git**

```bash
git mv deploy/k8s/overlays/develop deploy/k8s/overlays/prod
```

- [ ] **Step 2: Fix the on-demand seed comment inside the kustomization**

In `deploy/k8s/overlays/prod/kustomization.yaml`, update the path in the comment:

```yaml
  # seed-job.yaml is applied on demand, not part of the default apply:
  #   kubectl apply -f deploy/k8s/overlays/prod/seed-job.yaml
```

- [ ] **Step 3: Find every other reference**

Run: `grep -rn "overlays/develop" . --exclude-dir=.git`
Expected: only hits are ones you are about to fix (README, docs). Update each to `overlays/prod`. Do NOT edit the two design/spec docs under `docs/superpowers/` (they are historical records); update only operational files (`deploy/k8s/README.md`, scripts).

- [ ] **Step 4: Verify the overlay still builds**

Run: `kubectl kustomize deploy/k8s/overlays/prod > /dev/null && echo OK`
Expected: `OK` (no error). If `kubectl` is unavailable locally, `kustomize build deploy/k8s/overlays/prod`.

- [ ] **Step 5: Commit**

```bash
git add -A deploy/k8s
git commit -m "refactor: rename k8s overlay develop -> prod"
```

---

### Task 2: CI test + smoke gate before image build

Today `.github/workflows/images.yml` builds and pushes with no gate. Add a `test` job (`go test ./...`) and a `smoke` job (compose up + the existing `e2e` test), and make the existing `build` matrix depend on both. A red test or red smoke must block the push.

**Files:**
- Modify: `.github/workflows/images.yml`

**Interfaces:**
- Produces: images are pushed only when `test` and `smoke` succeed. `build` job unchanged except a `needs:` clause.

- [ ] **Step 1: Add the `test` job**

Insert this job above the existing `build` job (keep `build` as-is for now):

```yaml
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.0"
      - run: go test ./...
```

- [ ] **Step 2: Add the `smoke` job (compose + e2e)**

The compose stack has MySQL/Redis/Redpanda/MailHog and defaults `EMAIL_PROVIDER=smtp`, so no real email/SMS is sent. The stack needs the JWT keypair the services mount; generate a throwaway pair in CI.

```yaml
  smoke:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.0"
      - name: Generate throwaway JWT keypair for the stack
        run: |
          mkdir -p deploy/compose/secrets
          openssl genpkey -algorithm RSA -out deploy/compose/secrets/auth_priv.pem -pkeyopt rsa_keygen_bits:2048
          openssl rsa -in deploy/compose/secrets/auth_priv.pem -pubout -out deploy/compose/secrets/auth_pub.pem
      - name: Bring up the stack
        working-directory: deploy/compose
        run: |
          cp .env.example .env 2>/dev/null || true
          docker compose up -d --wait
      - name: Wait for otp-api health
        run: |
          for i in $(seq 1 30); do
            curl -fsS http://localhost/healthz && break || sleep 2
          done
      - name: Run E2E smoke
        env:
          E2E_API_BASE: http://localhost
          E2E_MAILHOG: http://localhost:8025
          MYSQL_DSN: "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true"
        run: go test -tags e2e ./test/e2e/...
      - name: Dump logs on failure
        if: failure()
        working-directory: deploy/compose
        run: docker compose logs --no-color
      - name: Tear down
        if: always()
        working-directory: deploy/compose
        run: docker compose down -v
```

> Note: confirm at execution whether `deploy/compose/.env.example` exists and carries the CI-safe defaults (`EMAIL_PROVIDER=smtp`, Twilio magic number `+15005550006`, `MYSQL_DSN` root/secret). If it does not, create it from the committed non-secret keys in the compose file's `environment:` blocks - it must contain NO real API keys. The MySQL root password and DSN above must match the compose file's MySQL service.

- [ ] **Step 3: Gate the build on both jobs**

In the existing `build` job, add:

```yaml
  build:
    needs: [test, smoke]
    runs-on: ubuntu-latest
    strategy:
      # ... unchanged ...
```

- [ ] **Step 4: Validate the workflow syntax**

Run: `yq '.' .github/workflows/images.yml > /dev/null && echo OK` (or any YAML linter).
Expected: `OK`. If `actionlint` is available, run it.

- [ ] **Step 5: Prove the gate blocks (temporary red test)**

Add a deliberately failing test, push to a branch, confirm `build` is skipped because `test` failed, then remove it.

```bash
cat >> pkg/security/gate_check_test.go <<'EOF'
package security
import "testing"
func TestGateBlocksBuild(t *testing.T){ t.Fatal("temporary: prove CI gate blocks build") }
EOF
git add -A && git commit -m "test: temporary CI gate proof" && git push
# Observe in GitHub Actions: test FAILS, build is SKIPPED (not run).
git rm pkg/security/gate_check_test.go && git commit -m "test: remove temporary CI gate proof" && git push
```

Expected: the run with the failing test shows `build` skipped; after removal, `test` + `smoke` pass and `build` runs.

- [ ] **Step 6: Commit the workflow change**

```bash
git add .github/workflows/images.yml
git commit -m "ci: gate image build on go test + compose e2e smoke"
```

---

### Task 3: Deploy + rollback scripts (pinned SHA)

Give prod a scripted, human-gated deploy pinned to a git SHA, and a one-command rollback. The overlay's `images:` tags become script-managed instead of the hardcoded `latest`.

**Files:**
- Create: `deploy/scripts/deploy.sh`
- Create: `deploy/scripts/rollback.sh`
- Modify: `deploy/k8s/README.md` (document both commands)

**Interfaces:**
- Consumes: `deploy/k8s/overlays/prod` (Task 1).
- Produces: `deploy.sh <git-sha>` sets all four image tags to `<git-sha>` and `kubectl apply -k`s the prod overlay; `rollback.sh [deploy|<git-sha>]` reverts.

- [ ] **Step 1: Write `deploy.sh`**

```bash
#!/usr/bin/env bash
# Deploy a pinned image SHA to prod. Usage: deploy/scripts/deploy.sh <git-sha>
set -euo pipefail

SHA="${1:?usage: deploy.sh <git-sha>}"
OVERLAY="deploy/k8s/overlays/prod"
SVCS=(otp-api auth-svc otp-dispatcher seed)

cd "$(git rev-parse --show-toplevel)"

for svc in "${SVCS[@]}"; do
  ( cd "$OVERLAY" && kustomize edit set image "$svc=ghcr.io/duykhanh/worklane-$svc:$SHA" )
done

echo ">> Applying prod overlay pinned to $SHA"
kubectl apply -k "$OVERLAY"
kubectl -n worklane rollout status deploy/otp-api --timeout=120s
kubectl -n worklane rollout status deploy/auth-svc --timeout=120s
kubectl -n worklane rollout status deploy/otp-dispatcher --timeout=120s

echo ">> Deployed $SHA. Commit the overlay change so git records what is running:"
echo "   git add $OVERLAY/kustomization.yaml && git commit -m 'deploy: prod -> $SHA'"
```

> `kustomize edit set image` requires the standalone `kustomize` CLI. If only `kubectl` is present, replace the loop with `sed -i` edits of the `newTag:` lines - but prefer installing `kustomize`.

- [ ] **Step 2: Write `rollback.sh`**

```bash
#!/usr/bin/env bash
# Roll prod back. Usage:
#   rollback.sh              -> kubectl rollout undo (last-known-good, fastest)
#   rollback.sh <git-sha>    -> re-deploy a specific previous SHA
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

if [[ $# -eq 0 ]]; then
  for d in otp-api auth-svc otp-dispatcher; do
    echo ">> rollout undo deploy/$d"
    kubectl -n worklane rollout undo "deploy/$d"
    kubectl -n worklane rollout status "deploy/$d" --timeout=120s
  done
  echo ">> Rolled back to previous ReplicaSet. Reconcile the overlay tag in git to match."
else
  exec deploy/scripts/deploy.sh "$1"
fi
```

- [ ] **Step 3: Make executable and lint**

```bash
chmod +x deploy/scripts/deploy.sh deploy/scripts/rollback.sh
shellcheck deploy/scripts/deploy.sh deploy/scripts/rollback.sh
```

Expected: `shellcheck` clean (no warnings).

- [ ] **Step 4: Static-validate the overlay still builds after a scripted tag set**

Without a cluster you can still prove the kustomize edit works:

```bash
( cd deploy/k8s/overlays/prod && kustomize edit set image otp-api=ghcr.io/duykhanh/worklane-otp-api:testsha )
kubectl kustomize deploy/k8s/overlays/prod | grep -q "worklane-otp-api:testsha" && echo OK
git checkout deploy/k8s/overlays/prod/kustomization.yaml   # revert the test edit
```

Expected: `OK`.

- [ ] **Step 5: Document in the k8s README**

Add a "Deploy / rollback" section to `deploy/k8s/README.md`:

```markdown
## Deploy / rollback (prod)

Deploy a CI-built SHA (find it in the GHCR package tags or the Actions run):
    deploy/scripts/deploy.sh <git-sha>
    git commit -am "deploy: prod -> <git-sha>"   # record what is running

Roll back fast to the previous ReplicaSet:
    deploy/scripts/rollback.sh

Roll back to a specific known-good SHA:
    deploy/scripts/rollback.sh <git-sha>
```

- [ ] **Step 6: Live verification (gated on cluster up)**

Deploy a SHA, confirm the running image, roll back, confirm it changed both ways:

```bash
kubectl -n worklane get deploy otp-api -o jsonpath='{.spec.template.spec.containers[0].image}'; echo
deploy/scripts/deploy.sh <sha-A>   # -> image shows :<sha-A>
deploy/scripts/rollback.sh         # -> rollout undo; image reverts
```

Expected: image string changes to `<sha-A>` after deploy and reverts after rollback.

- [ ] **Step 7: Commit**

```bash
git add deploy/scripts deploy/k8s/README.md
git commit -m "feat: scripted pinned-SHA deploy + one-command rollback"
```

---

### Task 4: Prometheus `/metrics` on otp-api and auth-svc

Both HTTP services need an unauthenticated `/metrics` endpoint and request instrumentation so Grafana Cloud can chart signal #1 (send success rate) and signal #2 (p95 latency). Build one shared middleware in `pkg/platform/metrics` (the repo already groups shared code under `pkg/platform/*`), used by both routers.

**Files:**
- Create: `pkg/platform/metrics/metrics.go`
- Test: `pkg/platform/metrics/metrics_test.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/router.go`
- Modify: `services/auth-svc/internal/adapters/inbound/http/router.go`
- Modify: `go.mod` / `go.sum` (add `prometheus/client_golang`)

**Interfaces:**
- Produces:
  - `metrics.Middleware() gin.HandlerFunc` - records `http_requests_total{method,path,status}` and `http_request_duration_seconds{method,path}` (histogram).
  - `metrics.Handler() gin.HandlerFunc` - serves the Prometheus exposition format.
- Consumed by: both `NewRouter` functions (registered before the auth groups, so `/metrics` needs no auth).

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/prometheus/client_golang@latest
```

- [ ] **Step 2: Write the failing test**

Create `pkg/platform/metrics/metrics_test.go`:

```go
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMetricsExposedAndUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/metrics", Handler())
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })

	// generate one request so the counter is non-zero
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ping", nil))

	rr := httptest.NewRecorder()
	// deliberately no Authorization header
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 on /metrics, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "http_requests_total") {
		t.Fatalf("want http_requests_total in metrics body, got:\n%s", rr.Body.String())
	}
}
```

- [ ] **Step 3: Run it to see it fail**

Run: `go test ./pkg/platform/metrics/...`
Expected: FAIL (package/functions not defined).

- [ ] **Step 4: Implement `metrics.go`**

Create `pkg/platform/metrics/metrics.go`:

```go
// Package metrics provides a gin middleware and /metrics handler exposing
// Prometheus request counters and a latency histogram. Shared by otp-api and
// auth-svc so both export the same signal names (send rate, p95 latency).
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	reqTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method, route and status.",
	}, []string{"method", "path", "status"})

	reqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

// Middleware records count + latency. It uses the matched route template
// (c.FullPath()) as the "path" label so high-cardinality path params do not
// explode the metric series.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		reqDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
		reqTotal.WithLabelValues(c.Request.Method, path, strconv.Itoa(c.Writer.Status())).Inc()
	}
}

// Handler serves the Prometheus exposition format. Unauthenticated by design.
func Handler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}
```

- [ ] **Step 5: Run the test to see it pass**

Run: `go test ./pkg/platform/metrics/...`
Expected: PASS.

- [ ] **Step 6: Wire into otp-api router**

In `services/otp-api/internal/adapters/inbound/http/router.go`, add the import
`"github.com/duykhanh/worklane/pkg/platform/metrics"` and, right after `r.Use(gin.Recovery())`:

```go
	r.Use(metrics.Middleware())
	r.GET("/metrics", metrics.Handler())
```

(Place `/metrics` beside the existing `r.GET("/healthz", h.Health)` line - both are unauthenticated, outside the `/v1` group.)

- [ ] **Step 7: Wire into auth-svc router**

Apply the identical change in `services/auth-svc/internal/adapters/inbound/http/router.go` (after `r.Use(gin.Recovery())`, `/metrics` beside `/healthz`).

- [ ] **Step 8: Run the full suite**

Run: `go test ./...`
Expected: PASS (existing tests plus the new metrics test). If any router construction test asserts an exact route count, update it to include `/metrics`.

- [ ] **Step 9: Commit**

```bash
git add pkg/platform/metrics go.mod go.sum services/otp-api services/auth-svc
git commit -m "feat: expose Prometheus /metrics on otp-api and auth-svc"
```

---

### Task 5: Resource guardrails - Redpanda retention + k3s image GC

Defend the 15GB disk. Bound the Redpanda log (OTP events are consumed in seconds) and enable kubelet image garbage collection so old `:<git-sha>` images are reclaimed. Redpanda already caps memory at `1Gi` in its StatefulSet; add retention.

**Files:**
- Modify: `deploy/k8s/base/redpanda/statefulset.yaml`
- Modify: `deploy/k8s/base/mysql/statefulset.yaml`
- Modify: `deploy/k8s/README.md` (document the k3s image-GC host config)

**Interfaces:**
- Consumes: existing Redpanda + MySQL StatefulSets.
- Produces: bounded Redpanda log; a pinned-small MySQL buffer pool; documented host-level image GC.

- [ ] **Step 1: Add a retention postStart hook to Redpanda**

Redpanda cluster config is set via `rpk cluster config set`. Add a `lifecycle.postStart` that waits for the admin API then bounds the log to 10 minutes / 256MB. In `deploy/k8s/base/redpanda/statefulset.yaml`, inside the `redpanda` container spec (sibling of `args:`), add:

```yaml
          lifecycle:
            postStart:
              exec:
                command:
                  - sh
                  - -c
                  - |
                    for i in $(seq 1 30); do
                      rpk cluster config set log_retention_ms 600000 && \
                      rpk cluster config set retention_bytes 268435456 && exit 0
                      sleep 2
                    done
                    echo "redpanda: failed to set retention (non-fatal)" >&2
```

> Rationale: `postStart` runs asynchronously after the container starts; the retry loop tolerates the admin API not being ready yet, and a final failure is non-fatal (retention defaults are looser but the pod still serves). This keeps retention declarative inside the StatefulSet without a separate Job.

- [ ] **Step 2: Pin the MySQL buffer pool small**

Keep MySQL's RAM bounded on the shared node. In `deploy/k8s/base/mysql/statefulset.yaml`, add an explicit small buffer pool to the `mysql` container (as `args`, sibling of the image):

```yaml
          args:
            - --innodb-buffer-pool-size=128M
```

> MySQL 8's default is already 128M, but pinning it makes the RAM budget explicit and prevents a surprise if the image default changes. Do not raise it - the schema is tiny and the node is shared.

- [ ] **Step 3: Validate the manifests build**

Run: `kubectl kustomize deploy/k8s/base > /dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 4: Document k3s image GC (host config)**

Image GC is a kubelet/k3s host setting, not a manifest. Add to `deploy/k8s/README.md`:

```markdown
## Disk guardrails (host)

k3s image garbage collection - reclaim old :<git-sha> images automatically.
Edit /etc/rancher/k3s/config.yaml on the node:

    kubelet-arg:
      - "image-gc-high-threshold=80"
      - "image-gc-low-threshold=70"

Then: systemctl restart k3s

Redpanda log retention is bounded in deploy/k8s/base/redpanda/statefulset.yaml
(log_retention_ms=10m, retention_bytes=256MB) so the Kafka log cannot fill /var.
```

- [ ] **Step 5: Live verification (gated on cluster up)**

```bash
kubectl -n worklane exec statefulset/redpanda -- rpk cluster config get log_retention_ms
kubectl -n worklane exec statefulset/mysql -- mysql -uroot -p"$MYSQL_ROOT_PASSWORD" \
  -e "SELECT @@innodb_buffer_pool_size;"
```

Expected: `log_retention_ms` = `600000`; buffer pool = `134217728` (128M). Also confirm the node stays below the GC high threshold: `df -h /` shows disk use trending down after several deploys (old images reclaimed).

- [ ] **Step 6: Commit**

```bash
git add deploy/k8s/base/redpanda/statefulset.yaml deploy/k8s/base/mysql/statefulset.yaml deploy/k8s/README.md
git commit -m "feat: bound Redpanda retention, pin MySQL buffer pool, document k3s image GC"
```

---

### Task 6: Grafana Alloy -> Grafana Cloud (metrics + logs)

Run a single Grafana Alloy agent in-cluster that scrapes the app `/metrics`, Redpanda's metrics, and node metrics, and `remote_write`s them (plus pod logs) to Grafana Cloud free tier. No local TSDB - nothing lands on the 15GB disk.

**Files:**
- Create: `deploy/k8s/base/observability/alloy-config.yaml` (ConfigMap with Alloy `config.alloy`)
- Create: `deploy/k8s/base/observability/alloy.yaml` (DaemonSet + ServiceAccount + RBAC)
- Modify: `deploy/k8s/base/kustomization.yaml` (add the observability resources)
- Modify: `deploy/k8s/README.md` (out-of-band `grafana-cloud` secret creation)

**Interfaces:**
- Consumes: `/metrics` from otp-api/auth-svc (Task 4); Redpanda metrics port `:9644/metrics`.
- Produces: metrics + logs in Grafana Cloud; a `grafana-cloud` Secret referenced by the DaemonSet.

- [ ] **Step 1: Document the out-of-band secret**

Add to `deploy/k8s/README.md` (mirrors the base spec's out-of-band secret style):

```markdown
## Grafana Cloud secret (out-of-band)

From the Grafana Cloud stack: copy the Prometheus remote_write URL + user id,
the Loki push URL + user id, and an access-policy token, then:

    kubectl -n worklane create secret generic grafana-cloud \
      --from-literal=PROM_URL='<grafana-cloud-prom-url>' \
      --from-literal=PROM_USER='<grafana-cloud-prom-user>' \
      --from-literal=LOKI_URL='<grafana-cloud-loki-url>' \
      --from-literal=LOKI_USER='<grafana-cloud-loki-user>' \
      --from-literal=TOKEN='<grafana-cloud-token>'
```

- [ ] **Step 2: Write the Alloy config ConfigMap**

Create `deploy/k8s/base/observability/alloy-config.yaml`. This scrapes app metrics (pods with the `metrics: enabled` annotation), node metrics, Redpanda, and tails pod logs, sending all to Grafana Cloud via env-substituted secrets.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alloy-config
data:
  config.alloy: |
    prometheus.remote_write "gc" {
      endpoint {
        url = sys.env("PROM_URL")
        basic_auth {
          username = sys.env("PROM_USER")
          password = sys.env("TOKEN")
        }
      }
    }

    // App services: scrape /metrics on otp-api and auth-svc.
    discovery.kubernetes "pods" { role = "pod" }
    discovery.relabel "app" {
      targets = discovery.kubernetes.pods.targets
      rule {
        source_labels = ["__meta_kubernetes_namespace"]
        regex         = "worklane"
        action        = "keep"
      }
      rule {
        source_labels = ["__meta_kubernetes_pod_label_app"]
        regex         = "otp-api|auth-svc"
        action        = "keep"
      }
    }
    prometheus.scrape "app" {
      targets    = discovery.relabel.app.output
      forward_to = [prometheus.remote_write.gc.receiver]
    }

    // Redpanda self-metrics (consumer lag / topic bytes) on :9644.
    discovery.relabel "redpanda" {
      targets = discovery.kubernetes.pods.targets
      rule {
        source_labels = ["__meta_kubernetes_pod_label_app"]
        regex         = "redpanda"
        action        = "keep"
      }
      rule { target_label = "__address__" replacement = "redpanda:9644" }
    }
    prometheus.scrape "redpanda" {
      targets     = discovery.relabel.redpanda.output
      metrics_path = "/metrics"
      forward_to  = [prometheus.remote_write.gc.receiver]
    }

    // Node metrics (disk %, signal #4).
    prometheus.exporter.unix "node" { }
    prometheus.scrape "node" {
      targets    = prometheus.exporter.unix.node.targets
      forward_to = [prometheus.remote_write.gc.receiver]
    }

    // Pod logs -> Loki.
    loki.write "gc" {
      endpoint {
        url = sys.env("LOKI_URL")
        basic_auth {
          username = sys.env("LOKI_USER")
          password = sys.env("TOKEN")
        }
      }
    }
    discovery.kubernetes "logpods" { role = "pod" }
    loki.source.kubernetes "pods" {
      targets    = discovery.kubernetes.logpods.targets
      forward_to = [loki.write.gc.receiver]
    }
```

> Alloy's River/config syntax and component names track the Alloy version pinned in the DaemonSet - verify against that version's docs at execution and adjust field names if the release differs. The intent per component is fixed: app scrape, Redpanda scrape, node metrics, log tail, all `remote_write` to Grafana Cloud.

- [ ] **Step 3: Write the Alloy DaemonSet + RBAC**

Create `deploy/k8s/base/observability/alloy.yaml`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alloy
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alloy
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes", "nodes/proxy", "services", "endpoints"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: alloy
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: alloy
subjects:
  - kind: ServiceAccount
    name: alloy
    namespace: worklane
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: alloy
spec:
  selector:
    matchLabels: { app: alloy }
  template:
    metadata:
      labels: { app: alloy }
    spec:
      serviceAccountName: alloy
      containers:
        - name: alloy
          image: grafana/alloy:v1.5.1
          args:
            - run
            - /etc/alloy/config.alloy
            - --storage.path=/tmp/alloy
          envFrom:
            - secretRef:
                name: grafana-cloud
          volumeMounts:
            - name: config
              mountPath: /etc/alloy
          resources:
            requests: { cpu: "50m", memory: "100Mi" }
            limits:   { cpu: "300m", memory: "256Mi" }
      volumes:
        - name: config
          configMap:
            name: alloy-config
```

> `--storage.path=/tmp/alloy` keeps Alloy's WAL on emptyDir/tmp, bounded - it must never spool unbounded onto the node (design pitfall). Confirm the pinned `grafana/alloy` tag exists at execution.

- [ ] **Step 4: Register the resources in base kustomization**

Add to `deploy/k8s/base/kustomization.yaml` `resources:`:

```yaml
  - observability/alloy-config.yaml
  - observability/alloy.yaml
```

- [ ] **Step 5: Validate the build**

Run: `kubectl kustomize deploy/k8s/base > /dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 6: Live verification (gated on cluster up + secret created)**

After creating the `grafana-cloud` secret and applying:

```bash
kubectl -n worklane rollout status ds/alloy
kubectl -n worklane logs ds/alloy | grep -iE "remote_write|error" | head
```

Expected: Alloy running, no auth errors. In Grafana Cloud Explore, `http_requests_total` and `node_filesystem_avail_bytes` return data; the 4 signals (send success rate, p95 latency, Redpanda lag, disk %) are chartable.

- [ ] **Step 7: Commit**

```bash
git add deploy/k8s/base/observability deploy/k8s/base/kustomization.yaml deploy/k8s/README.md
git commit -m "feat: Grafana Alloy agent -> Grafana Cloud (metrics + logs, no local TSDB)"
```

---

### Task 7: Alerting to Telegram + external uptime ping

Wire alerts to Telegram: a Grafana Cloud alert rule on disk > 80% (the top outage cause), and an external uptime monitor pinging `/healthz`. Both are configured outside the repo; commit only the runbook so the setup is reproducible.

**Files:**
- Create: `docs/runbooks/alerting.md`
- Modify: `deploy/k8s/README.md` (link to the runbook)

**Interfaces:**
- Consumes: node disk metrics in Grafana Cloud (Task 6); `https://api.otp.<domain>/healthz` (base spec).
- Produces: a Telegram contact point + a disk alert + an uptime monitor.

- [ ] **Step 1: Write the Telegram bot setup runbook**

Create `docs/runbooks/alerting.md`:

```markdown
# Alerting runbook

## Telegram bot (alert channel)
1. Talk to @BotFather -> /newbot -> get <telegram-bot-token>.
2. Send the bot any message, then read your chat id:
   curl "https://api.telegram.org/bot<telegram-bot-token>/getUpdates"
   -> result[].message.chat.id = <telegram-chat-id>
3. Test:
   curl "https://api.telegram.org/bot<telegram-bot-token>/sendMessage" \
     -d chat_id=<telegram-chat-id> -d text="worklane alert test"

## Grafana Cloud: disk > 80% alert
- Contact point: Telegram, using <telegram-bot-token> + <telegram-chat-id>.
- Alert rule (Grafana-managed):
    Query:  100 * (1 - node_filesystem_avail_bytes{mountpoint="/"}
                     / node_filesystem_size_bytes{mountpoint="/"})
    Condition: IS ABOVE 80  (for 5m)
    Route to: the Telegram contact point.
- Optional companion rules (same contact point):
    OTP send success rate < 95% for 10m.
    Redpanda consumer group lag > 1000 for 10m.

## Uptime ping (is it up at all)
Use a free external monitor (e.g. Uptime Kuma on another box, or a hosted
free pinger). Monitor: HTTP GET https://api.otp.<domain>/healthz every 60s,
expect 200. Alert channel: the same Telegram bot.
```

- [ ] **Step 2: Link the runbook from the k8s README**

Add to `deploy/k8s/README.md`: `Alerting setup: see docs/runbooks/alerting.md`.

- [ ] **Step 3: Live verification (gated on Grafana Cloud + monitor set up)**

- Fill the disk toward the threshold on a scratch path (or temporarily lower the rule to 1%) and confirm a Telegram message arrives; restore the threshold.
- `kubectl -n worklane delete pod -l app=otp-api` and confirm the uptime monitor fires a "down" then "recovered" Telegram message.

Expected: both a disk alert and an uptime down/recovered alert land in Telegram.

- [ ] **Step 4: Commit**

```bash
git add docs/runbooks/alerting.md deploy/k8s/README.md
git commit -m "docs: alerting runbook (Telegram + disk alert + uptime ping)"
```

---

### Task 8: Nightly MySQL backup to Cloudflare R2 + restore drill

A low-criticality nightly `mysqldump` of both databases to Cloudflare R2. The real deliverable is a proven restore drill - a backup you have never restored is not a backup.

**Files:**
- Create: `deploy/k8s/base/backup/backup-cronjob.yaml`
- Create: `deploy/scripts/restore-drill.sh`
- Modify: `deploy/k8s/base/kustomization.yaml`
- Modify: `deploy/k8s/README.md` (out-of-band `r2-backup` secret + restore-drill note)

**Interfaces:**
- Consumes: in-cluster MySQL (`mysql:3306`), the `worklane-secrets` root password (base spec).
- Produces: `worklane-YYYYmmdd.sql.gz` objects in R2; a `restore-drill.sh` that verifies one.

- [ ] **Step 1: Document the out-of-band R2 secret**

Add to `deploy/k8s/README.md`:

```markdown
## R2 backup secret (out-of-band)

Create an R2 bucket + an S3-compatible API token, then:

    kubectl -n worklane create secret generic r2-backup \
      --from-literal=R2_ENDPOINT='<r2-s3-endpoint>' \
      --from-literal=R2_BUCKET='<r2-bucket>' \
      --from-literal=AWS_ACCESS_KEY_ID='<r2-access-key>' \
      --from-literal=AWS_SECRET_ACCESS_KEY='<r2-secret-key>'
```

- [ ] **Step 2: Write the backup CronJob**

Create `deploy/k8s/base/backup/backup-cronjob.yaml`. It dumps both DBs, gzips, and uploads via the AWS CLI (R2 is S3-compatible).

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: mysql-backup
spec:
  schedule: "0 18 * * *"   # 01:00 Asia/Ho_Chi_Minh (UTC+7)
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 1
  failedJobsHistoryLimit: 2
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: backup
              image: amazon/aws-cli:2.15.30
              command:
                - sh
                - -c
                - |
                  set -eu
                  apk add --no-cache mysql-client >/dev/null 2>&1 || \
                    (yum install -y mariadb >/dev/null 2>&1 || true)
                  TS=$(date +%Y%m%d)
                  DUMP=/tmp/worklane-$TS.sql.gz
                  mysqldump -h mysql -uroot -p"$MYSQL_ROOT_PASSWORD" \
                    --databases otp identity | gzip > "$DUMP"
                  aws s3 cp "$DUMP" "s3://$R2_BUCKET/worklane-$TS.sql.gz" \
                    --endpoint-url "$R2_ENDPOINT"
              env:
                - name: MYSQL_ROOT_PASSWORD
                  valueFrom:
                    secretKeyRef: { name: worklane-secrets, key: MYSQL_ROOT_PASSWORD }
              envFrom:
                - secretRef: { name: r2-backup }
```

> The `amazon/aws-cli` image lacks `mysqldump`; the inline install is best-effort across base images. If it proves flaky at execution, switch to a small image that bundles both (e.g. a `mysql:8.0`-based image with `aws-cli` added, or run `mysqldump` in one container and `aws s3 cp` in a second). Keep whichever is simplest and verified.

- [ ] **Step 3: Write the restore-drill script**

Create `deploy/scripts/restore-drill.sh`:

```bash
#!/usr/bin/env bash
# Prove the latest R2 backup restores. Requires: aws cli + a scratch MySQL
# (docker) locally, and the r2-backup env vars exported.
set -euo pipefail

: "${R2_ENDPOINT:?}" "${R2_BUCKET:?}"
LATEST=$(aws s3 ls "s3://$R2_BUCKET/" --endpoint-url "$R2_ENDPOINT" \
  | awk '{print $4}' | sort | tail -1)
echo ">> latest backup: $LATEST"
aws s3 cp "s3://$R2_BUCKET/$LATEST" "/tmp/$LATEST" --endpoint-url "$R2_ENDPOINT"

docker run -d --rm --name restore-drill -e MYSQL_ROOT_PASSWORD=secret \
  -p 3307:3306 mysql:8.0
until docker exec restore-drill mysqladmin ping -psecret --silent 2>/dev/null; do sleep 2; done

gunzip -c "/tmp/$LATEST" | docker exec -i restore-drill mysql -uroot -psecret
echo ">> row counts after restore:"
docker exec restore-drill mysql -uroot -psecret -e \
  "SELECT 'tenants', COUNT(*) FROM identity.tenants
   UNION SELECT 'otp_requests', COUNT(*) FROM otp.otp_requests;"
docker stop restore-drill
echo ">> restore drill OK"
```

- [ ] **Step 4: Register the CronJob + lint the script**

```bash
# add backup/backup-cronjob.yaml to deploy/k8s/base/kustomization.yaml resources:
kubectl kustomize deploy/k8s/base > /dev/null && echo OK
chmod +x deploy/scripts/restore-drill.sh
shellcheck deploy/scripts/restore-drill.sh
```

Expected: `OK` and `shellcheck` clean.

- [ ] **Step 5: Live verification (gated on cluster up + secret created)**

```bash
kubectl -n worklane create job --from=cronjob/mysql-backup backup-now
kubectl -n worklane wait --for=condition=complete job/backup-now --timeout=180s
aws s3 ls "s3://<r2-bucket>/" --endpoint-url "<r2-s3-endpoint>"   # object present
R2_ENDPOINT=<...> R2_BUCKET=<...> AWS_ACCESS_KEY_ID=<...> AWS_SECRET_ACCESS_KEY=<...> \
  deploy/scripts/restore-drill.sh                                  # prints row counts, "restore drill OK"
```

Expected: a `worklane-<date>.sql.gz` object exists in R2 and the drill restores it and prints non-error row counts.

- [ ] **Step 6: Commit**

```bash
git add deploy/k8s/base/backup deploy/scripts/restore-drill.sh deploy/k8s/base/kustomization.yaml deploy/k8s/README.md
git commit -m "feat: nightly MySQL backup to R2 + restore drill"
```

---

## Notes for the implementer

- **Order matters loosely:** Tasks 1-4 are in-repo and cluster-independent (do them first, fully verified locally). Tasks 5-8 have a final live-verification step gated on the VPS/k3s being up and the relevant out-of-band secret existing; their in-repo parts (manifests build, scripts lint) are still done and committed first.
- **Phase 2 (ArgoCD) is intentionally NOT in this plan** - see the design doc's Phase 2 sketch. It becomes its own spec + plan once this layer is stable and (ideally) the disk is resized.
- **Every out-of-band secret** (`grafana-cloud`, `r2-backup`, plus the Telegram token used only in Grafana/monitor config) is created with `kubectl create secret` per the README; none is committed.
