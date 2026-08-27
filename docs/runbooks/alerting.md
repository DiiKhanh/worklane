# Alerting runbook

Two alerts, one channel (Telegram): **disk > 80%** (Grafana-managed) and **uptime**
(external monitor). Metrics already flow to Grafana Cloud via Alloy
(`deploy/k8s/base/observability/alloy-config.yaml`), so both are pure config on top.

## 1. Telegram bot (the alert channel)
1. In Telegram, talk to @BotFather -> `/newbot` -> get `<bot-token>`.
2. Send your new bot any message (so it has a chat to reply to), then read the chat id:
   ```
   curl -s "https://api.telegram.org/bot<bot-token>/getUpdates" | jq '.result[].message.chat.id'
   ```
   -> `<chat-id>` (a number; negative for a group).
3. Smoke-test the channel end-to-end:
   ```
   curl -s "https://api.telegram.org/bot<bot-token>/sendMessage" \
     -d chat_id=<chat-id> -d text="worklane alert test"
   ```
   You should get the message in Telegram. Keep `<bot-token>` + `<chat-id>` for step 2.

## 2. Grafana Cloud: disk > 80% alert (UI)
Set up in the Grafana Cloud UI (this stack does not provision Grafana as code).

**a. Contact point** — Alerts & IRM -> Alerting -> Contact points -> **Add contact point**:
- Name: `telegram`
- Integration: **Telegram**
- Bot Token: `<bot-token>` ; Chat ID: `<chat-id>`
- **Test** (sends a sample) -> Save.

**b. Notification policy** — Alerting -> Notification policies: either set the **default
policy** to the `telegram` contact point, or add a nested policy matching label
`team = worklane` -> `telegram`. (If you use a matcher, add that label on the rule in step c.)

**c. Alert rule** — Alerting -> Alert rules -> **New alert rule**:
- Query **A** (Prometheus data source, Code mode):
  ```
  100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype="ext4"}
             / node_filesystem_size_bytes{mountpoint="/",fstype="ext4"})
  ```
  > The label is `mountpoint="/"`, **not** `/host/root`. Alloy's `rootfs_path="/host/root"`
  > (see `alloy-config.yaml`) exists precisely so node-exporter strips the container prefix and
  > reports the host root as `/`. `fstype="ext4"` excludes tmpfs/overlay mounts.
- Expression **B**: Reduce A, function **Last** (collapses the series to one value).
- Alert condition: **B IS ABOVE 80**.
- Evaluate: group interval `1m`, **pending period `5m`** (fires only if disk stays >80% for 5m).
- Labels: `severity = critical` (and `team = worklane` if step b uses that matcher).
- Save. Routing goes through the notification policy from step b.

**Optional companion rules** (same contact point, same pattern):
- OTP send success rate < 95% for 10m.
- Redpanda consumer-group lag > 1000 for 10m.

## 3. Uptime ping (is it up at all)
The API now exposes a **public** `/healthz` through Traefik
(`deploy/k8s/overlays/prod/ingressroute.yaml`): `GET https://api-otp.dikhanh.io.vn/healthz`
returns `{"status":"ok"}` 200, unauthenticated, touches no MySQL/Redis.

Point a free external monitor at it (an external box, not this node - a monitor on the same
host dies with it):
- Monitor: **HTTP GET `https://api-otp.dikhanh.io.vn/healthz`**, every 60s, expect **200**.
- Options: Uptime Kuma on another box, or a hosted free pinger (UptimeRobot, Better Stack, etc.).
- Alert channel: the **same Telegram bot** (`<bot-token>` + `<chat-id>`).

## Verify (done-when)
- **Disk**: temporarily lower the rule threshold (e.g. IS ABOVE 1) -> it fires to Telegram ->
  restore to 80. Or fill the disk in a test - the threshold flip is safer.
- **Uptime**: `kubectl -n worklane scale deploy/otp-api --replicas=0` -> monitor fires **down**;
  scale back to the original replica count -> it fires **recovered**.
