# Productionize worklane on k3s - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy the worklane walking skeleton (otp-api, auth-svc, otp-dispatcher + MySQL/Redis/Redpanda) to a self-hosted single-node k3s cluster, reachable at `https://api.otp.<domain>` via Cloudflare Tunnel, with images built by CI to GHCR and the dashboard on Vercel.

**Architecture:** Kustomize `base` (namespace, ConfigMap, in-cluster stateful infra as StatefulSets/Deployment, three app Deployments, Traefik Middlewares) + a `develop` overlay (GHCR image tags, host-specific `IngressRoute`, seed Job). Apps migrate their schema at startup behind an `initContainer` that waits for dependencies. Secrets are created out-of-band. A GitHub Actions matrix builds and pushes per-service images to GHCR. Host/network follows the companion infra doc (Cloudflare Tunnel as systemd in the VM).

**Tech Stack:** k3s (built-in Traefik v3), Kustomize (`kubectl kustomize`), Traefik CRDs (`IngressRoute`, `Middleware`), GitHub Actions + GHCR, Cloudflare Tunnel, Go 1.25 services (gin), Next.js dashboard on Vercel.

**Design doc:** `docs/superpowers/specs/2026-08-19-k3s-production-deploy-design.md`
**Companion (host/network):** `docs/superpowers/specs/2026-08-07-self-hosted-infra-setup.md`

## Global Constraints

- Module path `github.com/duykhanh/worklane`; Go `1.25.0`. Only Task 1 changes Go code.
- **In-cluster stateful infra:** MySQL + Redpanda are `StatefulSet` + PVC (k3s `local-path`); Redis is an ephemeral `Deployment`. One MySQL holds two databases: `otp` (via `MYSQL_DATABASE`) and `identity` (via an initdb ConfigMap).
- **Database-per-service env mapping:** otp-api & otp-dispatcher read `MYSQL_DSN` = the `OTP_MYSQL_DSN` secret key; auth-svc reads `MYSQL_DSN` = the `IDENTITY_MYSQL_DSN` secret key.
- **Secrets are never committed.** `worklane-secrets`, `worklane-jwt`, `ghcr-pull` are created out-of-band via documented `kubectl create secret` commands. `.gitignore` already excludes `deploy/compose/secrets/` and `.env`.
- **`/internal/*` is never routed** by the ingress - only `/v1` (otp-api) and `/auth` (auth-svc) get `IngressRoute` rules.
- Namespace: `worklane`. Registry: `ghcr.io/duykhanh/worklane-<svc>`.
- Manifests are validated statically with `kubectl kustomize <dir>` (must build) before any live apply; the live cluster apply + E2E is the final task, gated on the host being up.
- No em dash. Commit messages `<type>: <description>`, no co-author line. YAML: 2-space indent, no tabs.
- Placeholders `<domain>` and `<vercel-origin>` are filled at execution; keep them literal in committed files unless the real values are provided.

---

### Task 1: Unauthenticated `/healthz` on otp-api and auth-svc

k8s liveness/readiness need an endpoint outside the auth groups. Add `GET /healthz` returning `200 {"status":"ok"}` to both HTTP services. It must not touch the DB (liveness reflects the process, not its dependencies).

**Files:**
- Modify: `services/otp-api/internal/adapters/inbound/http/router.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/handlers.go`
- Test: `services/otp-api/internal/adapters/inbound/http/handlers_test.go`
- Modify: `services/auth-svc/internal/adapters/inbound/http/router.go`
- Create: `services/auth-svc/internal/adapters/inbound/http/health_handler.go`
- Test: `services/auth-svc/internal/adapters/inbound/http/handlers_test.go`

**Interfaces:**
- Produces: `GET /healthz -> 200 {"status":"ok"}` on both services, requiring no Authorization header.

- [ ] **Step 1: Write the failing otp-api test**

Append to `services/otp-api/internal/adapters/inbound/http/handlers_test.go`:

```go
func TestHealthz_NoAuth_200(t *testing.T) {
	h := newServer(&fakeSvc{}, validRepo())
	// no Authorization header on purpose
	rr := do(t, h, "GET", "/healthz", "", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 for /healthz without auth, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Fatalf("want status ok in body, got %s", rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/adapters/inbound/http/ -run TestHealthz -v`
Expected: FAIL (404 - route not registered).

- [ ] **Step 3: Add the otp-api health handler + route**

In `services/otp-api/internal/adapters/inbound/http/handlers.go`, add a method on `Handlers`:

```go
// Health is an unauthenticated liveness/readiness probe. It reports only that the process
// is serving - it deliberately does not touch MySQL/Redis, so a slow dependency cannot make
// k8s kill an otherwise healthy pod.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

In `services/otp-api/internal/adapters/inbound/http/router.go`, register it on the engine root (before the `/v1` group so it stays outside `authenticate`):

```go
	h := &Handlers{svc: svc, repo: repo}

	r.GET("/healthz", h.Health)

	v1 := r.Group("/v1")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/otp-api/internal/adapters/inbound/http/ -run TestHealthz -v`
Expected: PASS.

- [ ] **Step 5: Write the failing auth-svc test**

Append to `services/auth-svc/internal/adapters/inbound/http/handlers_test.go` (this file is `package http`, internal):

```go
func TestHealthz_NoAuth_200(t *testing.T) {
	// verifier + service are unused by /healthz; nil/zero values are fine.
	r := NewRouter(fakeSvc{}, nil, "itok")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for /healthz, got %d", w.Code)
	}
}
```

(If `httptest`/`net/http` are not already imported in that file, add them.)

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./services/auth-svc/internal/adapters/inbound/http/ -run TestHealthz -v`
Expected: FAIL (404).

- [ ] **Step 7: Add the auth-svc health handler + route**

Create `services/auth-svc/internal/adapters/inbound/http/health_handler.go`:

```go
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health is an unauthenticated liveness/readiness probe: process-only, no DB/Redis touch.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

In `services/auth-svc/internal/adapters/inbound/http/router.go`, register it on the engine root (outside `/auth` and `/internal`):

```go
	h := &Handlers{svc: svc}

	r.GET("/healthz", h.Health)

	auth := r.Group("/auth")
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test -race ./services/otp-api/... ./services/auth-svc/... 2>&1 | grep -E "FAIL|ok"`
Expected: all `ok`, no `FAIL`. Then `gofmt -w` any touched files.

- [ ] **Step 9: Commit**

```bash
git add services/otp-api/internal/adapters/inbound/http/ services/auth-svc/internal/adapters/inbound/http/
git commit -m "feat(api): unauthenticated /healthz for k8s probes"
```

---

### Task 2: k8s base scaffolding - namespace + ConfigMap

Create the base directory, the namespace, and the non-secret ConfigMap. Establish a `kustomization.yaml` that grows as later tasks add resources.

**Files:**
- Create: `deploy/k8s/base/namespace.yaml`
- Create: `deploy/k8s/base/config.yaml`
- Create: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Produces: namespace `worklane`; ConfigMap `worklane-config` consumed by every app Deployment via `envFrom`.

- [ ] **Step 1: Write the namespace**

```yaml
# deploy/k8s/base/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: worklane
```

- [ ] **Step 2: Write the ConfigMap**

```yaml
# deploy/k8s/base/config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: worklane-config
data:
  REDIS_URL: "redis://redis:6379/0"
  KAFKA_BROKERS: "redpanda:9092"
  AUTH_SVC_URL: "http://auth-svc:8889"
  AUTH_TOKEN_TTL: "1h"
  EMAIL_PROVIDER: "resend"
  RESEND_FROM: "onboarding@resend.dev"
  RESEND_BASE_URL: "https://api.resend.com"
  TWILIO_FROM: "+15005550006"
  TWILIO_BASE_URL: "https://api.twilio.com"
  OTP_SMS_BODY_FMT: "Your verification code is %s. It expires in 5 minutes."
```

- [ ] **Step 3: Write the base kustomization (resources added as tasks land)**

```yaml
# deploy/k8s/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: worklane
resources:
  - namespace.yaml
  - config.yaml
```

- [ ] **Step 4: Validate it builds**

Run: `kubectl kustomize deploy/k8s/base`
Expected: prints the Namespace + ConfigMap YAML, no error.

- [ ] **Step 5: Commit**

```bash
git add deploy/k8s/base/namespace.yaml deploy/k8s/base/config.yaml deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): base namespace and non-secret ConfigMap"
```

---

### Task 3: MySQL StatefulSet + initdb + Service

One MySQL instance, PVC-backed, creating both databases (`otp` via env, `identity` via initdb).

**Files:**
- Create: `deploy/k8s/base/mysql/statefulset.yaml`
- Create: `deploy/k8s/base/mysql/svc.yaml`
- Create: `deploy/k8s/base/mysql/initdb-configmap.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Produces: Service `mysql:3306`; databases `otp` and `identity`; root password from `worklane-secrets` key `MYSQL_ROOT_PASSWORD`.

- [ ] **Step 1: Write the initdb ConfigMap**

```yaml
# deploy/k8s/base/mysql/initdb-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mysql-initdb
data:
  01-create-identity-db.sql: |
    CREATE DATABASE IF NOT EXISTS identity;
```

- [ ] **Step 2: Write the headless Service**

```yaml
# deploy/k8s/base/mysql/svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: mysql
spec:
  clusterIP: None
  selector:
    app: mysql
  ports:
    - port: 3306
      targetPort: 3306
```

- [ ] **Step 3: Write the StatefulSet**

```yaml
# deploy/k8s/base/mysql/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql
spec:
  serviceName: mysql
  replicas: 1
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
        - name: mysql
          image: mysql:8.0
          args: ["--default-authentication-plugin=mysql_native_password"]
          env:
            - name: MYSQL_ROOT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: worklane-secrets
                  key: MYSQL_ROOT_PASSWORD
            - name: MYSQL_DATABASE
              value: otp
          ports:
            - containerPort: 3306
          volumeMounts:
            - name: data
              mountPath: /var/lib/mysql
            - name: initdb
              mountPath: /docker-entrypoint-initdb.d
          readinessProbe:
            exec:
              command: ["sh", "-c", "mysqladmin ping -h localhost -p\"$MYSQL_ROOT_PASSWORD\""]
            initialDelaySeconds: 10
            periodSeconds: 5
          livenessProbe:
            exec:
              command: ["sh", "-c", "mysqladmin ping -h localhost -p\"$MYSQL_ROOT_PASSWORD\""]
            initialDelaySeconds: 30
            periodSeconds: 10
          resources:
            requests: { cpu: "100m", memory: "512Mi" }
            limits: { cpu: "500m", memory: "1Gi" }
      volumes:
        - name: initdb
          configMap:
            name: mysql-initdb
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 2Gi
```

- [ ] **Step 4: Add to base kustomization**

Append under `resources:` in `deploy/k8s/base/kustomization.yaml`:

```yaml
  - mysql/initdb-configmap.yaml
  - mysql/svc.yaml
  - mysql/statefulset.yaml
```

- [ ] **Step 5: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 6: Commit**

```bash
git add deploy/k8s/base/mysql deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): mysql statefulset with otp+identity databases"
```

---

### Task 4: Redpanda StatefulSet + Service

Single-node Redpanda (Kafka), PVC-backed.

**Files:**
- Create: `deploy/k8s/base/redpanda/statefulset.yaml`
- Create: `deploy/k8s/base/redpanda/svc.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Produces: Service `redpanda:9092` advertising `redpanda:9092`.

- [ ] **Step 1: Write the Service**

```yaml
# deploy/k8s/base/redpanda/svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: redpanda
spec:
  clusterIP: None
  selector:
    app: redpanda
  ports:
    - port: 9092
      targetPort: 9092
```

- [ ] **Step 2: Write the StatefulSet**

```yaml
# deploy/k8s/base/redpanda/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redpanda
spec:
  serviceName: redpanda
  replicas: 1
  selector:
    matchLabels:
      app: redpanda
  template:
    metadata:
      labels:
        app: redpanda
    spec:
      containers:
        - name: redpanda
          image: redpandadata/redpanda:v23.3.3
          args:
            - redpanda
            - start
            - --smp=1
            - --overprovisioned
            - --node-id=0
            - --check=false
            - --kafka-addr=PLAINTEXT://0.0.0.0:9092
            - --advertise-kafka-addr=PLAINTEXT://redpanda:9092
          ports:
            - containerPort: 9092
          volumeMounts:
            - name: data
              mountPath: /var/lib/redpanda/data
          readinessProbe:
            exec:
              command: ["sh", "-c", "rpk cluster health | grep -q 'Healthy:.*true'"]
            initialDelaySeconds: 10
            periodSeconds: 5
          resources:
            requests: { cpu: "100m", memory: "512Mi" }
            limits: { cpu: "1", memory: "1Gi" }
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 2Gi
```

- [ ] **Step 3: Add to base kustomization**

Append under `resources:`:

```yaml
  - redpanda/svc.yaml
  - redpanda/statefulset.yaml
```

- [ ] **Step 4: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 5: Commit**

```bash
git add deploy/k8s/base/redpanda deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): single-node redpanda statefulset"
```

---

### Task 5: Redis Deployment + Service

Ephemeral Redis (all data is TTL/cache/regenerable).

**Files:**
- Create: `deploy/k8s/base/redis/deployment.yaml`
- Create: `deploy/k8s/base/redis/svc.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Produces: Service `redis:6379`.

- [ ] **Step 1: Write the Service**

```yaml
# deploy/k8s/base/redis/svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: redis
spec:
  selector:
    app: redis
  ports:
    - port: 6379
      targetPort: 6379
```

- [ ] **Step 2: Write the Deployment**

```yaml
# deploy/k8s/base/redis/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: redis:7
          ports:
            - containerPort: 6379
          readinessProbe:
            exec:
              command: ["redis-cli", "ping"]
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            requests: { cpu: "50m", memory: "64Mi" }
            limits: { cpu: "250m", memory: "256Mi" }
```

- [ ] **Step 3: Add to base kustomization**

Append under `resources:`:

```yaml
  - redis/svc.yaml
  - redis/deployment.yaml
```

- [ ] **Step 4: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 5: Commit**

```bash
git add deploy/k8s/base/redis deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): ephemeral redis deployment"
```

---

### Task 6: otp-api Deployment + Service

App Deployment with dependency-wait initContainer, `/healthz` probes, config+secret env, and the JWT public key mounted.

**Files:**
- Create: `deploy/k8s/base/otp-api/deployment.yaml`
- Create: `deploy/k8s/base/otp-api/svc.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Consumes: ConfigMap `worklane-config`, Secret `worklane-secrets` (key `OTP_MYSQL_DSN`, `INTERNAL_API_TOKEN`), Secret `worklane-jwt` (`auth_pub.pem`), imagePullSecret `ghcr-pull`. Image name `otp-api` (tag set by overlay).
- Produces: Service `otp-api:8888`.

- [ ] **Step 1: Write the Service**

```yaml
# deploy/k8s/base/otp-api/svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: otp-api
spec:
  selector:
    app: otp-api
  ports:
    - port: 8888
      targetPort: 8888
```

- [ ] **Step 2: Write the Deployment**

```yaml
# deploy/k8s/base/otp-api/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otp-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: otp-api
  template:
    metadata:
      labels:
        app: otp-api
    spec:
      imagePullSecrets:
        - name: ghcr-pull
      initContainers:
        - name: wait-for-deps
          image: busybox:1.36
          command:
            - sh
            - -c
            - "until nc -z mysql 3306 && nc -z redpanda 9092; do echo waiting for mysql+redpanda; sleep 2; done"
      containers:
        - name: otp-api
          image: otp-api # tag pinned by the overlay
          ports:
            - containerPort: 8888
          envFrom:
            - configMapRef:
                name: worklane-config
            - secretRef:
                name: worklane-secrets
          env:
            # otp-api reads MYSQL_DSN; map it to the otp-database DSN.
            - name: MYSQL_DSN
              valueFrom:
                secretKeyRef:
                  name: worklane-secrets
                  key: OTP_MYSQL_DSN
            - name: AUTH_JWT_PUBLIC_KEY_FILE
              value: /secrets/auth_pub.pem
          volumeMounts:
            - name: jwt
              mountPath: /secrets
              readOnly: true
          readinessProbe:
            httpGet: { path: /healthz, port: 8888 }
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet: { path: /healthz, port: 8888 }
            initialDelaySeconds: 15
            periodSeconds: 20
          resources:
            requests: { cpu: "50m", memory: "128Mi" }
            limits: { cpu: "250m", memory: "256Mi" }
      volumes:
        - name: jwt
          secret:
            secretName: worklane-jwt
```

Note: `envFrom secretRef worklane-secrets` also injects `OTP_MYSQL_DSN`, `IDENTITY_MYSQL_DSN`, `RESEND_API_KEY`, `TWILIO_*` as env vars; otp-api ignores the ones it does not read. The explicit `MYSQL_DSN` mapping is what the code consumes. `INTERNAL_API_TOKEN` arrives via `envFrom`.

- [ ] **Step 3: Add to base kustomization**

Append under `resources:`:

```yaml
  - otp-api/svc.yaml
  - otp-api/deployment.yaml
```

- [ ] **Step 4: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 5: Commit**

```bash
git add deploy/k8s/base/otp-api deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): otp-api deployment with dep-wait and healthz probes"
```

---

### Task 7: auth-svc Deployment + Service

Like otp-api, but maps `MYSQL_DSN` to the identity DB and mounts both JWT keys.

**Files:**
- Create: `deploy/k8s/base/auth-svc/deployment.yaml`
- Create: `deploy/k8s/base/auth-svc/svc.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Consumes: `worklane-config`, `worklane-secrets` (`IDENTITY_MYSQL_DSN`, `INTERNAL_API_TOKEN`), `worklane-jwt` (`auth_priv.pem` + `auth_pub.pem`), `ghcr-pull`. Image `auth-svc`.
- Produces: Service `auth-svc:8889`.

- [ ] **Step 1: Write the Service**

```yaml
# deploy/k8s/base/auth-svc/svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: auth-svc
spec:
  selector:
    app: auth-svc
  ports:
    - port: 8889
      targetPort: 8889
```

- [ ] **Step 2: Write the Deployment**

```yaml
# deploy/k8s/base/auth-svc/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auth-svc
spec:
  replicas: 1
  selector:
    matchLabels:
      app: auth-svc
  template:
    metadata:
      labels:
        app: auth-svc
    spec:
      imagePullSecrets:
        - name: ghcr-pull
      initContainers:
        - name: wait-for-deps
          image: busybox:1.36
          command:
            - sh
            - -c
            - "until nc -z mysql 3306; do echo waiting for mysql; sleep 2; done"
      containers:
        - name: auth-svc
          image: auth-svc # tag pinned by the overlay
          ports:
            - containerPort: 8889
          envFrom:
            - configMapRef:
                name: worklane-config
            - secretRef:
                name: worklane-secrets
          env:
            - name: MYSQL_DSN
              valueFrom:
                secretKeyRef:
                  name: worklane-secrets
                  key: IDENTITY_MYSQL_DSN
            - name: AUTH_JWT_PRIVATE_KEY_FILE
              value: /secrets/auth_priv.pem
            - name: AUTH_JWT_PUBLIC_KEY_FILE
              value: /secrets/auth_pub.pem
          volumeMounts:
            - name: jwt
              mountPath: /secrets
              readOnly: true
          readinessProbe:
            httpGet: { path: /healthz, port: 8889 }
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet: { path: /healthz, port: 8889 }
            initialDelaySeconds: 15
            periodSeconds: 20
          resources:
            requests: { cpu: "50m", memory: "128Mi" }
            limits: { cpu: "250m", memory: "256Mi" }
      volumes:
        - name: jwt
          secret:
            secretName: worklane-jwt
```

- [ ] **Step 3: Add to base kustomization**

Append under `resources:`:

```yaml
  - auth-svc/svc.yaml
  - auth-svc/deployment.yaml
```

- [ ] **Step 4: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 5: Commit**

```bash
git add deploy/k8s/base/auth-svc deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): auth-svc deployment with identity DSN and jwt keys"
```

---

### Task 8: otp-dispatcher Deployment

Kafka consumer, no Service. Waits for MySQL + Redpanda.

**Files:**
- Create: `deploy/k8s/base/otp-dispatcher/deployment.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Consumes: `worklane-config`, `worklane-secrets` (`OTP_MYSQL_DSN`, `RESEND_API_KEY`, `TWILIO_*`), `ghcr-pull`. Image `otp-dispatcher`.

- [ ] **Step 1: Write the Deployment**

```yaml
# deploy/k8s/base/otp-dispatcher/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otp-dispatcher
spec:
  replicas: 1
  selector:
    matchLabels:
      app: otp-dispatcher
  template:
    metadata:
      labels:
        app: otp-dispatcher
    spec:
      imagePullSecrets:
        - name: ghcr-pull
      initContainers:
        - name: wait-for-deps
          image: busybox:1.36
          command:
            - sh
            - -c
            - "until nc -z mysql 3306 && nc -z redpanda 9092; do echo waiting for mysql+redpanda; sleep 2; done"
      containers:
        - name: otp-dispatcher
          image: otp-dispatcher # tag pinned by the overlay
          envFrom:
            - configMapRef:
                name: worklane-config
            - secretRef:
                name: worklane-secrets
          env:
            - name: MYSQL_DSN
              valueFrom:
                secretKeyRef:
                  name: worklane-secrets
                  key: OTP_MYSQL_DSN
          livenessProbe:
            exec:
              command: ["sh", "-c", "pgrep otp-dispatcher > /dev/null"]
            initialDelaySeconds: 15
            periodSeconds: 20
          resources:
            requests: { cpu: "50m", memory: "128Mi" }
            limits: { cpu: "250m", memory: "256Mi" }
```

Note: the distroless image has no shell, so the `exec` liveness may not find `sh`/`pgrep`. If the dispatcher image lacks a shell, drop the liveness probe (Kubernetes restarts the container on process exit anyway) - decide at apply time by checking `kubectl describe`. Prefer no liveness probe over a probe that always fails.

- [ ] **Step 2: Add to base kustomization**

Append under `resources:`:

```yaml
  - otp-dispatcher/deployment.yaml
```

- [ ] **Step 3: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
git add deploy/k8s/base/otp-dispatcher deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): otp-dispatcher deployment"
```

---

### Task 9: Traefik Middlewares (base)

CORS + rate-limit middlewares, env-agnostic, so both the `IngressRoute` (overlay) and any future overlay reuse them.

**Files:**
- Create: `deploy/k8s/base/ingress/middlewares.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Produces: `Middleware/otp-cors`, `Middleware/otp-ratelimit` in namespace `worklane`.

- [ ] **Step 1: Write the middlewares**

```yaml
# deploy/k8s/base/ingress/middlewares.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: otp-ratelimit
spec:
  rateLimit:
    average: 100
    burst: 50
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: otp-cors
spec:
  headers:
    accessControlAllowMethods: ["GET", "POST", "DELETE", "OPTIONS"]
    accessControlAllowHeaders: ["authorization", "content-type", "idempotency-key"]
    accessControlAllowOriginList: ["https://<vercel-origin>"]
    accessControlMaxAge: 86400
```

- [ ] **Step 2: Add to base kustomization**

Append under `resources:`:

```yaml
  - ingress/middlewares.yaml
```

- [ ] **Step 3: Validate**

Run: `kubectl kustomize deploy/k8s/base >/dev/null && echo OK`
Expected: `OK` (Traefik CRDs build fine without the CRD installed).

- [ ] **Step 4: Commit**

```bash
git add deploy/k8s/base/ingress deploy/k8s/base/kustomization.yaml
git commit -m "feat(k8s): traefik cors and rate-limit middlewares"
```

---

### Task 10: seed Dockerfile + image target

The seed Job needs an image. Add a Dockerfile for the seed CLI (mirrors otp-api's, no migrations shipped).

**Files:**
- Create: `services/seed/Dockerfile`

**Interfaces:**
- Produces: a buildable `services/seed/Dockerfile` producing a `seed` binary entrypoint that accepts `--name/--email/--password`.

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# services/seed/Dockerfile
# Multi-stage build. Context must be the repo root (single Go module with shared pkg).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/seed ./services/seed

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/seed /seed
ENTRYPOINT ["/seed"]
```

- [ ] **Step 2: Verify it builds locally**

Run: `docker build -f services/seed/Dockerfile -t worklane-seed:test .`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add services/seed/Dockerfile
git commit -m "feat(seed): dockerfile for the seed cli image"
```

---

### Task 11: develop overlay - images, IngressRoute, seed Job

The environment-specific overlay: pins GHCR image tags, adds the host-specific `IngressRoute`, and a one-off seed `Job`.

**Files:**
- Create: `deploy/k8s/overlays/develop/kustomization.yaml`
- Create: `deploy/k8s/overlays/develop/ingressroute.yaml`
- Create: `deploy/k8s/overlays/develop/seed-job.yaml`

**Interfaces:**
- Consumes: base; Secret `worklane-secrets` (`IDENTITY_MYSQL_DSN`) and `ghcr-pull` for the seed Job.
- Produces: `IngressRoute/worklane-api` routing `/v1` and `/auth` for host `api.otp.<domain>`; deployable overlay with pinned images.

- [ ] **Step 1: Write the IngressRoute**

```yaml
# deploy/k8s/overlays/develop/ingressroute.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: worklane-api
spec:
  entryPoints:
    - web
  routes:
    - match: Host(`api.otp.<domain>`) && PathPrefix(`/v1`)
      kind: Rule
      services:
        - name: otp-api
          port: 8888
      middlewares:
        - name: otp-ratelimit
        - name: otp-cors
    - match: Host(`api.otp.<domain>`) && PathPrefix(`/auth`)
      kind: Rule
      services:
        - name: auth-svc
          port: 8889
      middlewares:
        - name: otp-ratelimit
        - name: otp-cors
```

- [ ] **Step 2: Write the seed Job**

```yaml
# deploy/k8s/overlays/develop/seed-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: seed
spec:
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets:
        - name: ghcr-pull
      containers:
        - name: seed
          image: seed # tag pinned by the overlay images: transformer
          args: ["--name", "demo", "--email", "you@demo.co", "--password", "changeme-now"]
          env:
            - name: MYSQL_DSN
              valueFrom:
                secretKeyRef:
                  name: worklane-secrets
                  key: IDENTITY_MYSQL_DSN
```

- [ ] **Step 3: Write the overlay kustomization (pin image tags)**

Replace `<sha>` with a real pushed tag (or `latest`) at execution:

```yaml
# deploy/k8s/overlays/develop/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: worklane
resources:
  - ../../base
  - ingressroute.yaml
  # seed-job.yaml is applied on demand, not part of the default apply:
  # kubectl apply -f deploy/k8s/overlays/develop/seed-job.yaml
images:
  - name: otp-api
    newName: ghcr.io/duykhanh/worklane-otp-api
    newTag: latest
  - name: auth-svc
    newName: ghcr.io/duykhanh/worklane-auth-svc
    newTag: latest
  - name: otp-dispatcher
    newName: ghcr.io/duykhanh/worklane-otp-dispatcher
    newTag: latest
  - name: seed
    newName: ghcr.io/duykhanh/worklane-seed
    newTag: latest
```

- [ ] **Step 4: Validate the overlay builds and images are rewritten**

Run: `kubectl kustomize deploy/k8s/overlays/develop | grep -E "image: ghcr.io/duykhanh/worklane-(otp-api|auth-svc|otp-dispatcher)"`
Expected: the three app images resolve to `ghcr.io/duykhanh/worklane-*:latest`.

- [ ] **Step 5: Commit**

```bash
git add deploy/k8s/overlays/develop
git commit -m "feat(k8s): develop overlay with GHCR images, ingressroute and seed job"
```

---

### Task 12: Deploy runbook (secrets + apply) README

A runbook documenting the out-of-band secret creation and the apply/verify flow. No secret values are committed.

**Files:**
- Create: `deploy/k8s/README.md`

**Interfaces:**
- Produces: copy-pasteable `kubectl create secret` commands and the deploy/verify sequence.

- [ ] **Step 1: Write the runbook**

```markdown
# worklane on k3s - deploy runbook

Prerequisites: a running k3s cluster (built-in Traefik), `kubectl` context set, images
pushed to GHCR (see `.github/workflows/images.yml`), and the host/tunnel up
(`docs/superpowers/specs/2026-08-07-self-hosted-infra-setup.md`).

## 1. Namespace

    kubectl apply -f deploy/k8s/base/namespace.yaml

## 2. Secrets (created out-of-band - never committed)

Replace the placeholder values. `<pw>` is the MySQL root password; DSNs embed it.

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
```

- [ ] **Step 2: Commit**

```bash
git add deploy/k8s/README.md
git commit -m "docs(k8s): deploy runbook with out-of-band secret creation"
```

---

### Task 13: GitHub Actions - build + push images to GHCR

CI builds each service image on push to `main` / tags and pushes to GHCR.

**Files:**
- Create: `.github/workflows/images.yml`

**Interfaces:**
- Produces: `ghcr.io/duykhanh/worklane-<svc>:{sha,latest,semver}` for `svc in {otp-api, auth-svc, otp-dispatcher, seed}`.

- [ ] **Step 1: Write the workflow**

```yaml
# .github/workflows/images.yml
name: images
on:
  push:
    branches: [main]
    tags: ["v*"]
permissions:
  contents: read
  packages: write
jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        svc: [otp-api, auth-svc, otp-dispatcher, seed]
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/duykhanh/worklane-${{ matrix.svc }}
          tags: |
            type=sha
            type=raw,value=latest,enable={{is_default_branch}}
            type=semver,pattern={{version}}
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: services/${{ matrix.svc }}/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

- [ ] **Step 2: Validate YAML locally**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/images.yml'))" && echo OK`
Expected: `OK` (well-formed YAML).

- [ ] **Step 3: Commit and push to trigger the first build**

```bash
git add .github/workflows/images.yml
git commit -m "ci: build and push service images to GHCR"
```

- [ ] **Step 4: After push, verify CI**

On GitHub: the `images` workflow runs green and 4 packages appear under
`ghcr.io/duykhanh/worklane-*`. (Make the packages accessible to the cluster: either keep
them private and use the `ghcr-pull` secret, or set them public.)

---

### Task 14: Host / network - finalize infra doc and stand up the tunnel + k3s

Operational: bring up the Ubuntu VM, Cloudflare domain, tunnel, k3s, and SSH per the companion doc; flip its status to reflect execution and pin the ingress target detail.

**Files:**
- Modify: `docs/superpowers/specs/2026-08-07-self-hosted-infra-setup.md`

**Interfaces:**
- Produces: `https://api.otp.<domain>` terminating at Cloudflare and tunneling to the node's `:80` (k3s Traefik); working SSH over the tunnel.

- [ ] **Step 1: Execute infra phases A-E**

Follow `2026-08-07-self-hosted-infra-setup.md` §6 phases A-E from a clean state, capturing the exact symptom of the noted SSH blocker if it recurs. Point the tunnel's `api.otp.<domain>` ingress rule at `http://localhost:80` (k3s servicelb -> Traefik).

- [ ] **Step 2: Verify the hop**

Run (from a dev machine): `curl -I https://api.otp.<domain>/healthz` once a hello route or otp-api is up.
Expected: reaches Traefik (200 from `/healthz` after Task 15, or 404 from Traefik before apps exist - either proves the tunnel path).

- [ ] **Step 3: Update the infra doc status**

In `docs/superpowers/specs/2026-08-07-self-hosted-infra-setup.md`, change `Status: Draft (for review)` to reflect execution (e.g. `Status: Executed for develop`), resolve the §8 open questions with the actual subdomains / SSH user / dashboard hostname chosen, and note in §6 Phase D that the tunnel targets `http://localhost:80` -> Traefik.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-08-07-self-hosted-infra-setup.md
git commit -m "docs(infra): finalize host/network setup for develop"
```

---

### Task 15: Cluster bring-up + end-to-end verification + dashboard on Vercel

The integration task: on the live cluster, create secrets, apply the overlay, seed, and prove both OTP paths over HTTPS, then point the Vercel dashboard at the live API.

**Files:** none (operational; uses `deploy/k8s/README.md`).

**Interfaces:**
- Consumes: everything above (images in GHCR, manifests, host/tunnel up).

- [ ] **Step 1: Replace placeholders with real values**

In `deploy/k8s/base/ingress/middlewares.yaml` set the real `<vercel-origin>`; in
`deploy/k8s/overlays/develop/ingressroute.yaml` set the real `api.otp.<domain>` host; in the
overlay `images:` set the pushed tags (or keep `latest`). Commit these value edits:

```bash
git add deploy/k8s
git commit -m "chore(k8s): set develop domain, vercel origin and image tags"
```

- [ ] **Step 2: Create the secrets** (runbook §2)

Run the three `kubectl create secret` commands from `deploy/k8s/README.md` with real values.
Verify: `kubectl -n worklane get secret worklane-secrets worklane-jwt ghcr-pull`.

- [ ] **Step 3: Apply and wait**

Run:
```bash
kubectl apply -k deploy/k8s/overlays/develop
kubectl -n worklane rollout status deploy/otp-api deploy/auth-svc deploy/otp-dispatcher --timeout=180s
kubectl -n worklane get pods
```
Expected: all pods `Running`/`Ready`; no `ImagePullBackOff` (fix `ghcr-pull` if so), no `CrashLoopBackOff` (check `kubectl logs` - usually a missing secret key or a dep not ready).

- [ ] **Step 4: Verify the database split**

Run:
```bash
kubectl -n worklane exec sts/mysql -- sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" identity -e "SHOW TABLES;"'
kubectl -n worklane exec sts/mysql -- sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" otp -e "SHOW TABLES;"'
```
Expected: `identity` has `api_keys/tenants/users`; `otp` has `delivery_logs/otp_requests/templates`.

- [ ] **Step 5: Seed + machine path**

Run:
```bash
kubectl apply -f deploy/k8s/overlays/develop/seed-job.yaml
kubectl -n worklane logs job/seed        # capture $KEY
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $KEY" https://api.otp.<domain>/v1/otp/requests
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer bogus" https://api.otp.<domain>/v1/otp/requests
curl -s -o /dev/null -w "%{http_code}\n" -X POST https://api.otp.<domain>/internal/introspect -d '{}'
```
Expected: valid key `200`, bogus `401`, internal `404`. Then send a real OTP and confirm delivery:
```bash
curl -s -X POST https://api.otp.<domain>/v1/otp/send -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' -d '{"recipient":"<your-email>","channel":"email"}'
```
Expected: `202` and a real email arrives; `/v1/otp/verify` with the code returns `200`.

- [ ] **Step 6: Human path**

Run:
```bash
TOKEN=$(curl -s -X POST https://api.otp.<domain>/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"you@demo.co","password":"changeme-now"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -s -H "Authorization: Bearer $TOKEN" https://api.otp.<domain>/auth/api-keys
```
Expected: a JWT, then the seeded key listed.

- [ ] **Step 7: Restart-safety soak**

Run:
```bash
kubectl -n worklane delete pod -l app=otp-api
kubectl -n worklane rollout status deploy/otp-api --timeout=120s
kubectl -n worklane delete pod mysql-0 && kubectl -n worklane rollout status sts/mysql --timeout=180s
```
Expected: otp-api rejoins and re-migrates idempotently; MySQL data survives (PVC) and the API still works.

- [ ] **Step 8: Dashboard on Vercel**

In the Vercel project for `dashboard/`, set `NEXT_PUBLIC_DATA_SOURCE=live` and
`NEXT_PUBLIC_API_BASE=https://api.otp.<domain>`, deploy, and confirm the Vercel origin matches
`otp-cors`'s allow-list. Log in with the seeded user and confirm live screens load without CORS
errors (browser devtools network tab shows `200`s, no CORS block).

- [ ] **Step 9: Update the design doc status**

In `docs/superpowers/specs/2026-08-19-k3s-production-deploy-design.md`, mark the deploy shipped
(and, since the roadmap gate is "email + SMS live in production, stable", note the gate's status).

```bash
git add docs/superpowers/specs/2026-08-19-k3s-production-deploy-design.md
git commit -m "docs: mark k3s production deploy shipped"
```

---

## Self-Review

**Spec coverage (design §§1-13):**
- Host/network (§11) -> Task 14. App workloads (§7) -> Tasks 6-8 (+ `/healthz` §7 -> Task 1). Stateful infra (§6) -> Tasks 3-5. Config/Secrets (§5) -> Task 2 (ConfigMap) + Task 12 (secrets runbook). Ingress (§8) -> Task 9 (middlewares) + Task 11 (IngressRoute). Kustomize layout (§4) -> Tasks 2-11. Image pipeline + CI (§9) -> Task 10 (seed Dockerfile) + Task 13. Dashboard (§10) -> Task 15 Step 8. Bring-up/verify (§12) + success criteria (§13) -> Task 15.
- Refinement vs spec §4: the host-specific `IngressRoute` lives in `overlays/develop` (not `base/ingress/`) because its `Host()` match is environment-specific; env-agnostic `Middleware`s stay in base. Noted here so the two are not seen as inconsistent.

**Placeholder scan:** `<domain>` and `<vercel-origin>` are intentional config placeholders, resolved in Task 15 Step 1 before the live apply; all manifest bodies are complete. No TBD/TODO.

**Type/name consistency:** Service names (`mysql`, `redis`, `redpanda`, `otp-api:8888`, `auth-svc:8889`) match the ConfigMap URLs (`REDIS_URL`, `KAFKA_BROKERS`, `AUTH_SVC_URL`) and the env mapping (`OTP_MYSQL_DSN`->otp-api/dispatcher, `IDENTITY_MYSQL_DSN`->auth-svc). Image logical names in base (`otp-api`, `auth-svc`, `otp-dispatcher`, `seed`) match the overlay `images:` transformer entries. Secret names (`worklane-secrets`, `worklane-jwt`, `ghcr-pull`) are consistent across Deployments, the runbook, and the seed Job. `/healthz` port matches each service's container port.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-19-k3s-production-deploy.md`.
