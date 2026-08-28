# VPS bring-up runbook (prod on self-hosted k3s)

Bringing worklane OTP live on the single-node VPS. Companion to the approved specs:
[self-hosted infra](../superpowers/specs/2026-08-07-self-hosted-infra-setup.md),
[k3s production deploy](../superpowers/specs/2026-08-19-k3s-production-deploy-design.md),
[prod-ops hardening](../superpowers/specs/2026-08-25-prod-ops-hardening-design.md).

## Target (settled at execution time)

| Topic | Value |
|-------|-------|
| Host | Personal laptop -> VMware -> Ubuntu 20.04 VM (NAT, outbound only), 4 vCPU / 6GB / 18GB disk |
| Access | SSH only via Cloudflare Tunnel (`ssh dk-vps`), tunnel `dk-vps` id `ac005b3d-...` |
| Domain | `dikhanh.io.vn` (Tenten -> Cloudflare NS) |
| API host | **`api-otp.dikhanh.io.vn`** (single-level so Cloudflare Universal SSL covers it; `api.otp.` = 2 levels would need paid ACM) |
| Expose | Cloudflare Tunnel (no open ports); edge TLS at Cloudflare, `cloudflared -> http://localhost:80` (loopback) -> k3s Traefik |
| Runtime | k3s (built-in Traefik), containerd; **cgroup v2 required** (k8s v1.36 rejects v1) |
| Email | Resend; verified domain `dikhanh.io.vn`; production sender `no-reply@dikhanh.io.vn` |
| Logs | Grafana Alloy -> Grafana Cloud (free); no local TSDB/log store (15GB disk) |
| Scope | API on k3s, dashboard on Vercel |
| Images | GHCR `ghcr.io/diikhanh/worklane-*`, **public** (anon pull, no `ghcr-pull` needed); pin `sha-<short>` |

## Progress

- [x] **Host baseline** - Ubuntu 20.04, cloudflared already running (tunnel + SSH route).
- [x] **k3s installed** - `curl -sfL https://get.k3s.io | sh -`. First start crash-looped on **cgroup v1**; fixed by adding `systemd.unified_cgroup_hierarchy=1` to `GRUB_CMDLINE_LINUX_DEFAULT`, `update-grub`, reboot. Now `cgroup2fs`, node Ready, NRestarts=0.
- [x] **kubeconfig** for user `dk` at `~/.kube/config` (`KUBECONFIG` exported in `~/.zshrc`).
- [x] **Tunnel ingress** - added `api-otp.dikhanh.io.vn -> http://localhost:80` before the catch-all in `/etc/cloudflared/config.yml`; `cloudflared tunnel route dns dk-vps api-otp.dikhanh.io.vn`; restart. Verified: `curl -sI https://api-otp.dikhanh.io.vn` -> `HTTP/2 404` (Cloudflare TLS -> tunnel -> Traefik, no app route yet).
- [x] **App deploy** - secrets (`worklane-jwt` Ed25519, `worklane-secrets`, placeholder `grafana-cloud`) + `kubectl apply -k overlays/prod` (pinned `sha-f0214ee`, host `api-otp.dikhanh.io.vn`). All 7 pods Running; disk 7.1G/18G.
- [x] **Verify** - DB split correct (identity: tenants/users/api_keys; otp: otp_requests/delivery_logs/templates); public routing 401 on bad key/login; seed printed API key; **send OTP -> 202, real email delivered via Resend, verify code -> 200**.
- [x] **Grafana Cloud** - real `grafana-cloud` secret (stack `magentagerbil3564`, region ap-southeast-1); Alloy shipping logs+metrics after fixing a `pods/log` RBAC gap (see gotchas). `forbidden count: 0`.
- [x] **Alerting** - Grafana Cloud uptime + disk alerts route to Telegram and were verified fire/recovery end-to-end.
- [x] **Deploy/rollback** - `deploy/scripts/deploy.sh` and `deploy/scripts/rollback.sh` were verified against production.
- [x] **Resend domain** - `dikhanh.io.vn` is verified, and production sends from `no-reply@dikhanh.io.vn`.
- [x] **Dashboard** - Vercel dashboard is live at `https://worklane-six.vercel.app` with live API data.
- [ ] **Later** - tracked in [vps-prod next steps](../superpowers/plans/2026-08-26-vps-prod-next-steps.md): R2 backup + restore drill, SMS production smoke via Twilio test credentials, `/v1/stats`.

## Gotchas hit

- **cgroup v1**: modern k3s/kubelet refuses cgroup v1. Ubuntu 20.04 defaults to hybrid v1; enable unified v2 via GRUB kernel param + reboot.
- **Cloudflare Universal SSL** only covers one subdomain level (`*.dikhanh.io.vn`). Use a single-label host (`api-otp`), not `api.otp`.
- **JWT keypair must be Ed25519** (auth-svc rejects RSA - see `pkg/security/jwt.go` and CI smoke keygen).
- **Public GHCR**: `imagePullSecrets: [ghcr-pull]` references a secret we don't create; kubelet logs a one-time "not found" warning then pulls anonymously. Harmless while packages are public.
- **Alloy `pods/log` RBAC**: `loki.source.kubernetes` tails logs via the API server's `pods/log` subresource; the base ClusterRole lacked it, so every tailer got `forbidden` and no logs reached Loki. Fixed in `deploy/k8s/base/observability/alloy.yaml` (added `pods/log`). Applying the role is not enough - **restart the Alloy DaemonSet** so it rebuilds its client with the new permission.
- **Loki log labels**: the log pipeline forwarded raw `discovery.kubernetes` targets, so logs landed with only `instance/job/service_name` - no `namespace`/`pod`/`container`, and `{namespace="worklane"}` matched nothing. Fixed in `deploy/k8s/base/observability/alloy-config.yaml` by adding a `discovery.relabel "logs"` that promotes `__meta_kubernetes_*` to real labels. (Metrics scrape already relabeled; only the log path was missing it.)
- **Clock/timezone confusion (not a bug)**: the VM runs UTC; the Mac is UTC+7. `17:47 UTC == 00:47 next-day local`. NTP was already synced - no skew. Check `timedatectl` before assuming drift.
- **`kubectl apply -f <base-file>` targets the wrong namespace**: base manifests carry no `namespace:` (kustomize injects `worklane` during `apply -k`). Applying such a file with a bare `kubectl apply -f` lands it in the context's `default` namespace, creating a stray copy while the real object in `worklane` stays stale. Use `kubectl apply -k overlays/prod`, or `kubectl -n worklane apply -f`, or put `namespace: worklane` in the doc.
- **Run kubectl on the VPS, not the Mac**: the k3s API (`:6443`) is not exposed off-box (tunnel only routes SSH + the API host). A `kubectl` that errors `connection to localhost:8080 refused` means you're on the Mac (or `KUBECONFIG` is unset), not the VPS.

## Deploy commands (run on the VM, in the cloned repo)

See the live session for the ordered, gated steps. Summary:

```bash
# repo + namespace
git clone https://github.com/DiiKhanh/worklane.git && cd worklane
kubectl create namespace worklane

# JWT (Ed25519), worklane-secrets, placeholder grafana-cloud - see session for exact commands

# pin image + real API host, then apply
sed -i 's/newTag: latest/newTag: sha-f0214ee/' deploy/k8s/overlays/prod/kustomization.yaml
sed -i 's/api\.otp\.<domain>/api-otp.dikhanh.io.vn/g' deploy/k8s/overlays/prod/ingressroute.yaml
kubectl apply -k deploy/k8s/overlays/prod
```
