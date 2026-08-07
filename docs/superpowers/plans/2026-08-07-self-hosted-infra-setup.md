# Self-Hosted Infra Setup — Execution Runbook

> **Nature of this plan:** this is **operational (ops) work**, not TDD code. Steps are shell commands run on the Ubuntu VM (or the dev machine) with an explicit **verify** after each phase. Per project convention for one-off operational work, we take the simplest direct end-to-end path — no wrappers, control planes, or custom automation. Track progress with the checkboxes.

**Goal:** Serve the OTP platform's `otp-api` at `https://api.otp.<domain>` from a home k3s cluster (Ubuntu-on-VMware), and reach the VM by SSH — both through Cloudflare Tunnel, with **no router port-forwarding**.

**Architecture:** Laptop (macOS) → VMware (NAT) → Ubuntu VM → k3s (native) → APISIX. `cloudflared` runs in the VM as a systemd service, dials out to Cloudflare, and Cloudflare routes public HTTPS + SSH back down the tunnel. Spec: [../specs/2026-08-07-self-hosted-infra-setup.md](../specs/2026-08-07-self-hosted-infra-setup.md).

**Prerequisites:** a Cloudflare account; the `<domain>` registered at Tenten; sudo on the Ubuntu VM; VMware Fusion/Workstation with the Ubuntu guest running.

## Global Constraints

- Replace `<domain>` with your real domain everywhere (e.g. `otp.example.com` as the app host under a root you own).
- VMware network adapter for the VM: **NAT** (not Bridged).
- Never open inbound ports on the home router. All ingress is via the outbound Cloudflare Tunnel.
- `sshd` must **not** be exposed to the public internet directly; SSH only via `cloudflared access`.

---

## Phase A — Ubuntu VM baseline

- [ ] **A1: Set VMware networking to NAT**

In VMware VM settings → Network Adapter → **NAT**. Boot the VM.

- [ ] **A2: Update and install basics (in the VM)**

```bash
sudo apt update && sudo apt -y upgrade
sudo apt -y install curl openssh-server ufw
sudo systemctl enable --now ssh
```

- [ ] **A3: Enable a host firewall that only allows loopback SSH**

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from 127.0.0.1 to any port 22 proto tcp
sudo ufw --force enable
```

- [ ] **A4 — Verify:** outbound internet works from the VM.

```bash
curl -I https://cloudflare.com && ping -c1 1.1.1.1
```
Expected: HTTP `200`/`301` header and a successful ping.

---

## Phase B — Domain onto Cloudflare

- [ ] **B1:** In the Cloudflare dashboard → **Add a site** → enter `<domain>` → pick the Free plan.

- [ ] **B2:** Cloudflare shows two nameservers. At **Tenten** (domain management), replace the domain's nameservers with those two.

- [ ] **B3 — Verify:** the domain becomes **Active** in Cloudflare.

```bash
dig NS <domain> +short
```
Expected: the two `*.ns.cloudflare.com` names (propagation can take minutes to hours).

---

## Phase C — Cloudflare Tunnel

- [ ] **C1: Install cloudflared (in the VM)**

```bash
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o cloudflared.deb
sudo dpkg -i cloudflared.deb
cloudflared --version
```

- [ ] **C2: Authenticate and create the tunnel**

```bash
cloudflared tunnel login          # opens a URL; authorize <domain>
cloudflared tunnel create otp-home
```
Note the printed **tunnel UUID** and the credentials file path (`~/.cloudflared/<UUID>.json`).

- [ ] **C3: Write the tunnel config**

```yaml
# ~/.cloudflared/config.yml
tunnel: <UUID>
credentials-file: /home/<user>/.cloudflared/<UUID>.json
ingress:
  - hostname: api.otp.<domain>
    service: http://localhost:9080     # APISIX (set in Phase D)
  - hostname: ssh.<domain>
    service: ssh://localhost:22
  - service: http_status:404
```

- [ ] **C4: Create DNS routes for the hostnames**

```bash
cloudflared tunnel route dns otp-home api.otp.<domain>
cloudflared tunnel route dns otp-home ssh.<domain>
```

- [ ] **C5: Install cloudflared as a service so it survives reboot**

```bash
sudo cloudflared --config /home/<user>/.cloudflared/config.yml service install
sudo systemctl enable --now cloudflared
systemctl status cloudflared --no-pager
```

- [ ] **C6 — Verify:** the tunnel is healthy and a temporary HTTP service is reachable publicly.

```bash
# temporary local service on 9080 to prove the path before APISIX exists
python3 -m http.server 9080 &   # in the VM
cloudflared tunnel info otp-home
```
From the dev machine: `curl -I https://api.otp.<domain>` → expect `200`. Stop the temp server afterward (`kill %1`).

---

## Phase D — k3s + APISIX

- [ ] **D1: Install k3s with Traefik disabled (APISIX is our ingress)**

```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
sudo k3s kubectl get nodes
```

- [ ] **D2: Make kubeconfig usable**

```bash
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $(id -u):$(id -g) ~/.kube/config
kubectl get nodes    # after installing kubectl, or use: k3s kubectl
```

- [ ] **D3: Install APISIX via Helm**

```bash
helm repo add apisix https://charts.apiseven.com
helm repo update
kubectl create namespace apisix
helm install apisix apisix/apisix -n apisix \
  --set service.type=NodePort \
  --set service.http.nodePort=30980
```

- [ ] **D4: Point the tunnel at APISIX**

Update `~/.cloudflared/config.yml` `api.otp.<domain>` service to the APISIX address reachable from the host (e.g. `http://localhost:30980` if the NodePort is bound, or the APISIX ClusterIP via a `kubectl port-forward` for the MVP). Restart:

```bash
sudo systemctl restart cloudflared
```

- [ ] **D5 — Verify:** a hello route through APISIX answers publicly.

Create a test APISIX route to any upstream (or the OTP `otp-api` service once deployed), then from the dev machine:
```bash
curl -i https://api.otp.<domain>/v1/otp/send   # expect 401 from APISIX key-auth (no key)
```
Expected: a response **from APISIX** (401 without an API key proves the full path Cloudflare→tunnel→APISIX works).

---

## Phase E — SSH over the tunnel

- [ ] **E1: Install cloudflared on the dev machine (macOS)**

```bash
brew install cloudflared
```

- [ ] **E2: Add an SSH ProxyCommand for the tunnel host**

```sshconfig
# ~/.ssh/config  (dev machine)
Host otp-home
  HostName ssh.<domain>
  User <vm-user>
  ProxyCommand cloudflared access ssh --hostname %h
```

- [ ] **E3 — Verify:** SSH in with no router port-forward.

```bash
ssh otp-home
```
Expected: a shell on the Ubuntu VM, reached entirely through Cloudflare.

---

## Phase F — Deploy the OTP stack (hand-off to the product plan)

- [ ] **F1:** Package `otp-api` + `dispatcher` (+ Redis/Postgres/Kafka charts) as Helm releases and install into k3s. This is the deploy step referenced by the product plan; do it only after the product's docker-compose e2e is green.
- [ ] **F2 — Verify:** the full success criteria from the MVP scope doc §5 hold against `https://api.otp.<domain>`.

---

## Fixing the current broken SSH

Today SSH "works but not correctly." Do **not** patch the old setup — rebuild via Phases A–E. If SSH still fails at **E3**, capture the real symptom before changing anything:

- [ ] On the dev machine: `ssh -v otp-home 2>&1 | head -40` — does `cloudflared access` connect, or does DNS/hostname fail?
- [ ] In the VM: `journalctl -u cloudflared -n 100 --no-pager` — is the tunnel healthy, is the `ssh.<domain>` ingress rule present?
- [ ] In the VM: `sudo systemctl status ssh` and `sudo ss -tlnp | grep :22` — is `sshd` listening on localhost.
- [ ] In Cloudflare: is `ssh.<domain>` a CNAME to the tunnel, and is there a matching **Cloudflare Access** app if Access is enforced.

Match the symptom to the phase that owns it (DNS→B, tunnel/ingress→C, sshd→A/E) and fix only that.

---

## Self-Review

**Spec coverage (vs 2026-08-07-self-hosted-infra-setup.md):**
- Ubuntu-on-VMware NAT baseline → Phase A. ✅
- Tenten domain onto Cloudflare NS → Phase B. ✅
- Cloudflare Tunnel (outbound, systemd, ingress for api + ssh) → Phase C. ✅
- k3s with Traefik disabled + APISIX ingress → Phase D. ✅
- SSH over tunnel via `cloudflared access` → Phase E. ✅
- Deploy hand-off + success criteria → Phase F. ✅
- Current-blocker diagnosis → dedicated section. ✅

**Placeholder scan:** `<domain>`, `<UUID>`, `<user>`, `<vm-user>` are intentional per-environment values, each explained. No unresolved TODOs.

**Consistency:** hostnames (`api.otp.<domain>`, `ssh.<domain>`) and the tunnel name `otp-home` are used identically across phases. The tunnel's `api.otp` upstream is intentionally staged: a temporary `:9080` service in Phase C proves the path, then D4 repoints it to the APISIX NodePort `:30980`.
```
