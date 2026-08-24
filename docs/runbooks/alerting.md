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
    Query:  100 * (1 - node_filesystem_avail_bytes{mountpoint="/host/root"}
                     / node_filesystem_size_bytes{mountpoint="/host/root"})
    Condition: IS ABOVE 80  (for 5m)
    Route to: the Telegram contact point.
- Optional companion rules (same contact point):
    OTP send success rate < 95% for 10m.
    Redpanda consumer group lag > 1000 for 10m.

## Uptime ping (is it up at all)
Use a free external monitor (e.g. Uptime Kuma on another box, or a hosted
free pinger). Monitor: HTTP GET https://api.otp.<domain>/healthz every 60s,
expect 200. Alert channel: the same Telegram bot.
