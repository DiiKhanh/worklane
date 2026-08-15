# Self-Hosted Infrastructure Setup (home k3s host)

- **Date:** 2026-08-07
- **Status:** Draft (for review)
- **Author:** duykhanh
- **Companion to:** [2026-08-07-otp-mvp-scope.md](./2026-08-07-otp-mvp-scope.md)

## 1. Intent

Track the **hosting/networking** for the OTP platform as its own concern, separate from the product.
The cluster runs **at home** on a personal laptop, exposed publicly **without opening any router
port**, so it can serve `https://<domain>` for a CV-showcase demo.

**Explicit caveat:** the host is a personal laptop, not a 24/7 cloud VPS. Availability follows the
laptop being on. That is acceptable for a demo/showcase; it is not an SLA product.

## 2. Environment (given)

- **Laptop** (macOS) runs **VMware** with an **Ubuntu** guest VM.
- **k3s runs natively inside the Ubuntu VM** (Linux) - no k3d/Lima needed.
- Domain registered at **Tenten**.
- **Cloudflare** in front (DNS + Tunnel), used for public exposure and SSH.
- Dashboard is hosted on **Vercel** (not on this cluster).

## 3. Target topology

```mermaid
flowchart TB
    subgraph internet["Public internet"]
        Dev["Your dev machine / phone"]
        Vercel["Vercel (Next.js dashboard)"]
    end

    subgraph cf["Cloudflare edge"]
        DNS["DNS (Tenten → Cloudflare NS)"]
        Edge["Edge TLS + Tunnel endpoint"]
    end

    subgraph laptop["Laptop (macOS) → VMware"]
        subgraph vm["Ubuntu VM (NAT, outbound only)"]
            CFD["cloudflared (Tunnel agent)"]
            subgraph k3s["k3s cluster"]
                TRAEFIK["Traefik (default ingress)"]
                API["otp-api"]
                DISP["dispatcher"]
                INFRA["Kafka / Redis / MySQL"]
            end
        end
    end

    Dev -->|https://api.otp.domain| Edge
    Vercel -->|REST calls (CORS)| Edge
    Dev -->|ssh via cloudflared access| Edge
    DNS --- Edge
    Edge <-->|outbound tunnel| CFD
    CFD -->|ingress rules| TRAEFIK
    CFD -->|ssh route| SSHD["sshd :22"]
    TRAEFIK --> API --> INFRA
    DISP --> INFRA
```

**Key idea - the tunnel is outbound.** `cloudflared` inside the VM dials **out** to Cloudflare and
holds the connection open. Cloudflare routes public requests back down that connection. Nothing
listens for inbound connections on the laptop, so **no router port-forwarding and no static IP** is
required - which is exactly why VMware **NAT** networking is sufficient.

## 4. Decisions

| Topic | Decision | Why |
|-------|----------|-----|
| VMware network mode | **NAT** | Tunnel is outbound-only; NAT gives the VM internet with zero inbound exposure. Bridged is unnecessary. |
| Public exposure | **Cloudflare Tunnel** (`cloudflared` as a systemd service in the VM) | No open ports, no static IP; edge TLS free. |
| Edge TLS | **Cloudflare** terminates TLS | Traefik stays the business gateway behind it (rate limit, CORS, routing); API-key auth is in `otp-api`. |
| SSH access | **Cloudflare Tunnel SSH route** (`cloudflared access ssh`) | Same no-port-forward path; avoids exposing sshd publicly. |
| Ingress controller | **Traefik** (k3s's built-in default - kept, not disabled) | k3s ships Traefik out of the box, so nothing extra to install; local docker-compose and k3s use the same gateway. |
| DNS | **Tenten domain → Cloudflare nameservers** | Puts DNS + Tunnel hostnames under Cloudflare. |

## 5. Hostnames (proposed)

- `api.otp.<domain>` → Cloudflare Tunnel → Traefik → `otp-api`.
- `ssh.<domain>` → Cloudflare Tunnel → `ssh://localhost:22` (via `cloudflared access`).
- Dashboard on Vercel: `otp.<domain>` (or a Vercel domain) - a CNAME managed in Cloudflare.

## 6. From-scratch setup (phased)

Each phase ends with a concrete **verify** step. Do not advance until it passes.

### Phase A - Ubuntu VM baseline
1. VMware: set the VM network adapter to **NAT**.
2. In Ubuntu: `sudo apt update && sudo apt upgrade`, install `openssh-server`, `curl`.
3. Give the VM a stable identity (hostname; optional static lease inside VMware's NAT range).
- **Verify:** `ping -c1 1.1.1.1` and `curl -I https://cloudflare.com` succeed from the VM.

### Phase B - Domain on Cloudflare
1. Create a Cloudflare account; **Add site** = your Tenten domain.
2. At **Tenten**, change the domain's **nameservers** to the two Cloudflare NS records.
- **Verify:** Cloudflare dashboard shows the domain **Active** (NS propagation can take time).

### Phase C - Cloudflare Tunnel
1. Install `cloudflared` in the VM; `cloudflared tunnel login` (authorize the domain).
2. `cloudflared tunnel create otp-home` → produces a tunnel ID + credentials file.
3. Write the tunnel config with **ingress rules** (hostname → service):
   - `api.otp.<domain>` → `http://<traefik-service>` (filled in Phase D)
   - `ssh.<domain>` → `ssh://localhost:22`
   - catch-all → `http_status:404`
4. Route DNS: `cloudflared tunnel route dns otp-home api.otp.<domain>` (and the ssh host).
5. Install cloudflared as a **systemd service** so it runs on boot.
- **Verify:** `cloudflared tunnel info otp-home` shows a healthy connection; a temporary
  `http://localhost:8080` test service is reachable at `https://api.otp.<domain>`.

### Phase D - k3s + Traefik (built-in)
1. Install k3s with defaults (Traefik included): `curl -sfL https://get.k3s.io | sh -`. No `--disable
   traefik` - we **keep** the bundled Traefik as the ingress controller.
2. Confirm `kubectl get nodes` is Ready; copy kubeconfig for local use.
3. Confirm Traefik is up: `kubectl -n kube-system get pods | grep traefik` and note its Service (a
   `LoadBalancer`/NodePort on 80/443 provided by k3s's servicelb).
4. Point the Phase-C `api.otp.<domain>` ingress rule at the **Traefik** service address (e.g. the
   node's `:80`, or a `kubectl port-forward` to the Traefik service for the MVP).
- **Verify:** a hello route (a test `IngressRoute`/`Ingress` to any upstream) answers at
  `https://api.otp.<domain>`.

### Phase E - SSH over the tunnel
1. On the client (dev machine): install `cloudflared`; add an SSH `ProxyCommand` using
   `cloudflared access ssh --hostname ssh.<domain>`.
2. `ssh user@ssh.<domain>`.
- **Verify:** you get a shell in the Ubuntu VM through Cloudflare, with **no** router port-forward.

## 7. Current blocker

SSH "works but not correctly" today. Rather than debug the partial setup, rebuild via §6 from a
clean state. When we execute, capture the exact failing symptom (client error, `cloudflared`
service logs, whether DNS resolves) so the fix targets the real cause instead of guessing.

## 8. Open questions

1. Exact subdomains to commit to (§5) - confirm `api.otp.<domain>`, `ssh.<domain>`, dashboard host.
2. Which OS user / SSH key on the Ubuntu VM is the intended login.
3. Whether the dashboard's public hostname is a custom domain via Cloudflare CNAME to Vercel, or a
   plain `*.vercel.app` for the MVP.
