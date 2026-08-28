# SMS smoke runbook - Twilio Test Credentials

Status: **PENDING** until a production smoke creates a `provider=twilio,status=sent` delivery log.

Use this when the goal is an internal production smoke test of the SMS path without sending a real
SMS and without spending money. Twilio test credentials call the real Twilio API endpoint, validate
the request like live credentials, and return a simulated queued message, but they do not connect to
real phone numbers or charge the account.

Reference: [Twilio Test Credentials](https://www.twilio.com/docs/iam/test-credentials).

## What this proves

This proves the worklane production path:

```text
dashboard/curl -> otp-api -> Kafka otp.requested -> otp-dispatcher -> Twilio adapter -> MySQL delivery_logs
```

It does **not** prove that a handset receives an SMS. No real SMS is sent when using Twilio test
credentials, and Twilio does not trigger SMS status callbacks for test-credential sends.

## Preconditions

- Production config already has:
  - `TWILIO_FROM="+15005550006"` in `deploy/k8s/base/config.yaml`
  - `TWILIO_BASE_URL="https://api.twilio.com"` in `deploy/k8s/base/config.yaml`
- You have the Twilio **Test Account SID** and **Test Auth Token** from the Twilio Console.
- You have either a tenant API key or a dashboard JWT for the tenant you want to smoke-test.

Do not recreate `worklane-secrets`; it also contains MySQL, Resend, and internal auth values. Patch
only the Twilio keys.

## 1. Patch the production secret

```bash
kubectl -n worklane patch secret worklane-secrets --type merge -p \
  '{"stringData":{"TWILIO_ACCOUNT_SID":"<twilio-test-account-sid>","TWILIO_AUTH_TOKEN":"<twilio-test-auth-token>"}}'
```

## 2. Restart the dispatcher

Only `otp-dispatcher` reads Twilio credentials.

```bash
kubectl -n worklane rollout restart deploy/otp-dispatcher
kubectl -n worklane rollout status deploy/otp-dispatcher
```

## 3. Send a smoke SMS request

Use any syntactically valid E.164 recipient. With Twilio test credentials, it will be validated but
no SMS will be sent.

```bash
API='https://api-otp.dikhanh.io.vn'
KEY='<tenant-api-key>'

curl -sS -X POST "$API/v1/otp/send" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: sms-smoke-20260828-01" \
  -d '{"recipient":"+84901234567","channel":"sms"}'
```

Expected response:

```json
{"request_id":"<request-id>"}
```

HTTP status should be `202 Accepted`.

## 4. Verify the delivery log

```bash
curl -sS -H "Authorization: Bearer $KEY" "$API/v1/delivery-logs"
```

Done-when: the response, or the dashboard Delivery logs screen, shows the same request with:

```text
provider=twilio
status=sent
```

## Important limitation

The normal `/v1/otp/verify` step cannot be completed from a received SMS because no SMS is delivered.
Production also does not store plaintext OTP codes, only hashes. If you need to prove the full
send-and-verify loop for SMS, use a real Twilio trial/live setup with a real sender number and a
verified recipient, or temporarily capture the code from the `otp.requested` Kafka message during the
smoke window.

## Returning to real SMS later

When you want real SMS delivery, replace the test SID/token with live credentials and change
`TWILIO_FROM` to a real Twilio sender owned by the account. The magic number `+15005550006` is for
test credentials only.
