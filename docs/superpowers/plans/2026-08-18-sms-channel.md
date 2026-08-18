# SMS Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add SMS as a second OTP delivery channel end-to-end - dashboard playground -> `otp-api` -> Kafka -> `otp-dispatcher` -> Twilio - alongside the existing email channel.

**Architecture:** `otp-api` takes an explicit `channel` (default `email`), validates the recipient per channel, and publishes the channel on the existing `otp.requested` event. `otp-dispatcher` routes by channel through a `Sender` registry (`map[channel]Sender`); each sender renders its own message and wraps a provider (email = Resend/SMTP, sms = Twilio). The dashboard playground gains a shared channel selector and a country-dial-code phone input.

**Tech Stack:** Go 1.x (Gin, sarama, GORM), Kafka/Redpanda, Redis, MySQL; Next.js 16 App Router, React 19, TypeScript, Tailwind v4, shadcn base-nova, TanStack Query, RHF + Zod, Vitest + Testing Library.

## Global Constraints

- **No em dash** anywhere; use `-`. No `console.log` in dashboard code.
- **Commit messages:** conventional (`feat:`/`test:`/`chore:`), no co-author line.
- **Immutability:** new objects via spread/copy, never mutate in place.
- **Backward compatibility:** an omitted `channel` MUST behave exactly as today (email).
- **E.164 phone shape:** `^\+[1-9]\d{7,14}$`.
- **SMS template default (English):** `Your verification code is %s. It expires in 5 minutes.`
- **Phone mask format:** leading `+`, first 2 digits, `***`, last 2 digits (e.g. `+84***67`).
- **Dial-code list (curated):** `+84 VN` (default), `+1 US`, `+44 GB`, `+65 SG`, `+81 JP`, `+61 AU`, `+91 IN`, `+49 DE`, `+33 FR`, `+82 KR`.
- **Twilio adapter mirrors `resendmail`:** inject `baseURL` so tests stub it with `httptest`; non-2xx -> error.
- **Gates before "done":** `go vet ./...`, `go test -race ./...`, and (in `dashboard/`) `pnpm lint && pnpm typecheck && pnpm test` all green.
- **Work on branch** `feat/sms-channel` (create it before Task 1).
- **Dashboard is a modified Next.js:** read the relevant guide under `node_modules/next/dist/docs/` (resolved from `dashboard/`) before writing any route/layout code; keep the `dashboard/AGENTS.md` auto-block if `next dev` re-adds it.

---

## File Structure

**Backend - `otp-api`**
- `internal/domain/request.go` - MODIFY: add `ChannelSMS`, `ValidChannel`, `ValidateRecipient`, `Mask`, `maskPhone`.
- `internal/domain/errors.go` - MODIFY: add `ErrInvalidRecipient`, `ErrInvalidChannel`.
- `internal/domain/request_test.go` - MODIFY: cover new validation + masking.
- `internal/app/send.go` - MODIFY: `SendInput.Channel`, default + validate, carry channel into audit + event.
- `internal/app/usecase_test.go` - MODIFY: channel cases.
- `internal/adapters/inbound/http/dto.go` - MODIFY: `channel` field, relax `binding`.
- `internal/adapters/inbound/http/handlers.go` - MODIFY: thread channel into `SendInput`.
- `internal/adapters/inbound/http/errors.go` - MODIFY: map new sentinels to 400.
- `internal/adapters/inbound/http/handlers_test.go` - MODIFY: channel + 400 cases.
- `internal/adapters/outbound/mysqlrepo/repo.go` - MODIFY: mask by channel.

**Backend - `otp-dispatcher`**
- `internal/app/ports.go` - MODIFY: add `Sender` port.
- `internal/app/sender.go` - CREATE: `emailSender`, `smsSender`, `SMSProvider` port.
- `internal/app/handler.go` - MODIFY: registry routing.
- `internal/app/handler_test.go` - MODIFY: registry + unknown-channel tests.
- `internal/adapters/outbound/twiliosms/provider.go` - CREATE: Twilio HTTP adapter.
- `internal/adapters/outbound/twiliosms/provider_test.go` - CREATE.
- `main.go` - MODIFY: build senders + registry + Twilio config.
- `deploy/compose/*` and `.env.example` - MODIFY: Twilio env.

**Frontend - `dashboard`**
- `lib/phone.ts` - CREATE: pure dial-code + E.164 helpers.
- `lib/phone.test.ts` - CREATE.
- `lib/schemas.ts` - MODIFY: channel-aware recipient schema.
- `lib/api/source.ts` / `live.ts` / `mock.ts` - MODIFY: `channel` param.
- `lib/queries/use-otp.ts` - MODIFY: thread `channel`.
- `components/common/phone-input.tsx` - CREATE.
- `components/playground/playground.tsx` - MODIFY: shared channel state + selector.
- `components/playground/send-form.tsx` / `verify-form.tsx` - MODIFY: input by channel.
- `components/playground/send-form.test.tsx` - MODIFY.

---

## Task 1: Domain - channel, recipient validation, masking

**Files:**
- Modify: `services/otp-api/internal/domain/request.go`
- Modify: `services/otp-api/internal/domain/errors.go`
- Test: `services/otp-api/internal/domain/request_test.go`

**Interfaces:**
- Produces:
  - `domain.ChannelSMS Channel = "sms"` (alongside existing `ChannelEmail`).
  - `domain.ValidChannel(c Channel) bool`
  - `domain.ValidateRecipient(channel Channel, recipient string) error` - returns `ErrInvalidRecipient` on a format mismatch, `ErrInvalidChannel` on an unknown channel, `nil` otherwise.
  - `domain.Mask(channel Channel, recipient string) string` - email -> existing `MaskRecipient`; sms -> `+84***67` shape.
  - `domain.ErrInvalidRecipient`, `domain.ErrInvalidChannel` (sentinels).

- [ ] **Step 1: Write the failing test** (append to `request_test.go`)

```go
func TestValidChannel(t *testing.T) {
	for _, c := range []Channel{ChannelEmail, ChannelSMS} {
		if !ValidChannel(c) {
			t.Fatalf("%q should be valid", c)
		}
	}
	if ValidChannel(Channel("push")) {
		t.Fatal("push is not a supported channel yet")
	}
}

func TestValidateRecipient(t *testing.T) {
	cases := []struct {
		channel   Channel
		recipient string
		wantErr   error
	}{
		{ChannelEmail, "user@example.com", nil},
		{ChannelEmail, "+84901234567", ErrInvalidRecipient},
		{ChannelSMS, "+84901234567", nil},
		{ChannelSMS, "0901234567", ErrInvalidRecipient}, // not E.164
		{ChannelSMS, "user@example.com", ErrInvalidRecipient},
		{Channel("push"), "x", ErrInvalidChannel},
	}
	for _, tc := range cases {
		if got := ValidateRecipient(tc.channel, tc.recipient); !errors.Is(got, tc.wantErr) {
			t.Fatalf("ValidateRecipient(%q,%q)=%v want %v", tc.channel, tc.recipient, got, tc.wantErr)
		}
	}
}

func TestMask_Phone(t *testing.T) {
	if got := Mask(ChannelSMS, "+84901234567"); got != "+84***67" {
		t.Fatalf("mask sms = %q want +84***67", got)
	}
	if got := Mask(ChannelEmail, "a@b.co"); got != "a***@b.co" {
		t.Fatalf("mask email = %q want a***@b.co", got)
	}
	if got := Mask(ChannelSMS, "bad"); got != "***" {
		t.Fatalf("malformed phone must mask to ***, got %q", got)
	}
}
```

Add `"errors"` to the test file imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/domain/ -run 'ValidChannel|ValidateRecipient|Mask_Phone' -v`
Expected: FAIL (undefined: `ChannelSMS`, `ValidChannel`, `ValidateRecipient`, `Mask`, `ErrInvalidRecipient`, `ErrInvalidChannel`).

- [ ] **Step 3: Add sentinels** (`errors.go`)

```go
	ErrInvalidRecipient = errors.New("otp: recipient does not match channel")
	ErrInvalidChannel   = errors.New("otp: unsupported channel")
```

- [ ] **Step 4: Implement in `request.go`**

Add to imports: `"net/mail"`, `"regexp"`. Then:

```go
const ChannelSMS Channel = "sms"

var e164Re = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// ValidChannel reports whether c is a delivery channel the platform supports.
func ValidChannel(c Channel) bool {
	return c == ChannelEmail || c == ChannelSMS
}

// ValidateRecipient checks that recipient is well-formed for its channel: an email
// address for email, an E.164 phone number for sms.
func ValidateRecipient(channel Channel, recipient string) error {
	switch channel {
	case ChannelEmail:
		if addr, err := mail.ParseAddress(recipient); err != nil || addr.Address != recipient {
			return ErrInvalidRecipient
		}
	case ChannelSMS:
		if !e164Re.MatchString(recipient) {
			return ErrInvalidRecipient
		}
	default:
		return ErrInvalidChannel
	}
	return nil
}

// Mask hides PII in a recipient for audit rows and logs, dispatching on channel.
func Mask(channel Channel, recipient string) string {
	if channel == ChannelSMS {
		return maskPhone(recipient)
	}
	return MaskRecipient(recipient)
}

// maskPhone keeps the leading '+', the first two and last two digits (e.g. +84***67).
// Anything too short or not starting with '+' returns a fixed mask so nothing leaks.
func maskPhone(p string) string {
	if len(p) < 5 || p[0] != '+' {
		return "***"
	}
	return p[:3] + "***" + p[len(p)-2:]
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/otp-api/internal/domain/ -v`
Expected: PASS (including the existing `TestMaskRecipient`).

- [ ] **Step 6: Commit**

```bash
git add services/otp-api/internal/domain/
git commit -m "feat(otp-api): domain channel validation and per-channel masking"
```

---

## Task 2: otp-api Send use case - channel through audit and event

**Files:**
- Modify: `services/otp-api/internal/app/send.go`
- Modify: `services/otp-api/internal/adapters/outbound/mysqlrepo/repo.go`
- Test: `services/otp-api/internal/app/usecase_test.go`

**Interfaces:**
- Consumes: `domain.ValidateRecipient`, `domain.ChannelEmail`, `domain.ChannelSMS` (Task 1).
- Produces: `app.SendInput.Channel string` (empty defaults to `email`). `Send` publishes `contracts.RequestedEvent.Channel` = the resolved channel and persists `app.Request.Channel` accordingly.

- [ ] **Step 1: Write the failing test**

Open `usecase_test.go`, find the existing send test to learn the fake wiring (fake store/counter/repo/publisher). Add:

```go
func TestSend_SMSChannel_PublishesSMSEvent(t *testing.T) {
	svc, fakes := newTestService(t) // reuse the existing constructor helper in this file
	res, err := svc.Send(context.Background(), app.SendInput{
		TenantID: "t1", Recipient: "+84901234567", Channel: "sms",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.RequestID == "" {
		t.Fatal("expected a request id")
	}
	evt := fakes.pub.last.(contracts.RequestedEvent)
	if evt.Channel != "sms" || evt.Recipient != "+84901234567" {
		t.Fatalf("published event = %+v, want sms channel", evt)
	}
	if got := fakes.repo.lastInsert.Channel; got != "sms" {
		t.Fatalf("audit row channel = %q, want sms", got)
	}
}

func TestSend_PhoneUnderEmailChannel_Rejected(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Send(context.Background(), app.SendInput{
		TenantID: "t1", Recipient: "+84901234567", Channel: "email",
	})
	if !errors.Is(err, domain.ErrInvalidRecipient) {
		t.Fatalf("want ErrInvalidRecipient, got %v", err)
	}
}

func TestSend_OmittedChannel_DefaultsToEmail(t *testing.T) {
	svc, fakes := newTestService(t)
	if _, err := svc.Send(context.Background(), app.SendInput{
		TenantID: "t1", Recipient: "user@example.com", // no Channel
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	evt := fakes.pub.last.(contracts.RequestedEvent)
	if evt.Channel != "email" {
		t.Fatalf("default channel should be email, got %q", evt.Channel)
	}
}
```

> Adapt `newTestService`, `fakes.pub.last`, and `fakes.repo.lastInsert` to the helpers/fakes already present in `usecase_test.go`. If the existing fake publisher/repo do not expose the last value, add a minimal field (e.g. `last any` set in `Publish`, `lastInsert app.Request` set in `InsertRequest`) - a surgical test-only change. Ensure imports include `"errors"`, `contracts "github.com/duykhanh/worklane/pkg/contracts/otp"`, and `domain`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/app/ -run 'TestSend_(SMSChannel|PhoneUnderEmail|OmittedChannel)' -v`
Expected: FAIL (`SendInput` has no `Channel`; no validation).

- [ ] **Step 3: Implement in `send.go`**

Add `Channel string` to `SendInput` (after `Recipient`). At the top of `Send`, before the idempotency block:

```go
	channel := domain.Channel(in.Channel)
	if channel == "" {
		channel = domain.ChannelEmail
	}
	if err := domain.ValidateRecipient(channel, in.Recipient); err != nil {
		return SendResult{}, err
	}
```

Replace the two hardcoded `string(domain.ChannelEmail)` occurrences (the `InsertRequest` call and the `contracts.RequestedEvent`) with `string(channel)`.

- [ ] **Step 4: Update the repo to mask by channel** (`mysqlrepo/repo.go` line 66)

```go
		RecipientMasked: domain.Mask(domain.Channel(req.Channel), req.Recipient),
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/otp-api/... -v`
Expected: PASS (new send tests + existing suite, including `mysqlrepo` tests).

- [ ] **Step 6: Commit**

```bash
git add services/otp-api/internal/app/ services/otp-api/internal/adapters/outbound/mysqlrepo/
git commit -m "feat(otp-api): carry delivery channel through send and audit"
```

---

## Task 3: otp-api HTTP - accept channel, map validation to 400

**Files:**
- Modify: `services/otp-api/internal/adapters/inbound/http/dto.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/handlers.go`
- Modify: `services/otp-api/internal/adapters/inbound/http/errors.go`
- Test: `services/otp-api/internal/adapters/inbound/http/handlers_test.go`

**Interfaces:**
- Consumes: `app.SendInput.Channel` (Task 2), `domain.ErrInvalidRecipient`, `domain.ErrInvalidChannel` (Task 1).
- Produces: `POST /v1/otp/send` accepts optional `"channel"`; a recipient/channel mismatch returns HTTP 400.

- [ ] **Step 1: Write the failing test**

Find the existing send handler test in `handlers_test.go` to reuse its router/fake-service setup. Add:

```go
func TestSend_SMSChannel_202(t *testing.T) {
	// fake service returns a request id; assert it receives channel "sms".
	// (Reuse the file's existing fakeService; extend it to capture the last SendInput.)
	r, fake := newTestRouter(t)
	w := doJSON(t, r, http.MethodPost, "/v1/otp/send",
		`{"recipient":"+84901234567","channel":"sms"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	if fake.lastInput.Channel != "sms" {
		t.Fatalf("service got channel %q, want sms", fake.lastInput.Channel)
	}
}

func TestSend_InvalidRecipient_400(t *testing.T) {
	r, _ := newTestRouter(t) // fake service returns domain.ErrInvalidRecipient for this input
	w := doJSON(t, r, http.MethodPost, "/v1/otp/send",
		`{"recipient":"+84901234567","channel":"email"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
```

> Adapt `newTestRouter`, `fake.lastInput`, and `doJSON` to the helpers already in `handlers_test.go`. To exercise the 400 path, have the fake service return `domain.ErrInvalidRecipient` (import `domain`) - the point of this task is the status mapping, not re-testing domain validation.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-api/internal/adapters/inbound/http/ -run 'TestSend_(SMSChannel|InvalidRecipient)' -v`
Expected: FAIL (DTO has no `channel`; `ErrInvalidRecipient` maps to 500).

- [ ] **Step 3: Update DTO** (`dto.go`)

```go
type sendRequest struct {
	Recipient string `json:"recipient" binding:"required"`
	Channel   string `json:"channel"`
}
```

And relax verify (recipient is a lookup key, format-agnostic):

```go
type verifyRequest struct {
	Recipient string `json:"recipient" binding:"required"`
	Code      string `json:"code" binding:"required"`
}
```

- [ ] **Step 4: Thread channel in the handler** (`handlers.go` `Send`)

```go
	res, err := h.svc.Send(c.Request.Context(), app.SendInput{
		TenantID:       c.GetString(tenantCtxKey),
		Recipient:      body.Recipient,
		Channel:        body.Channel,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
```

- [ ] **Step 5: Map the new sentinels to 400** (`errors.go` `statusFor`, add a case before `default`)

```go
	case errors.Is(err, domain.ErrInvalidRecipient), errors.Is(err, domain.ErrInvalidChannel):
		return http.StatusBadRequest // 400 - malformed request
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./services/otp-api/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add services/otp-api/internal/adapters/inbound/http/
git commit -m "feat(otp-api): accept channel on send, map recipient errors to 400"
```

---

## Task 4: otp-dispatcher - Sender port and registry routing

**Files:**
- Modify: `services/otp-dispatcher/internal/app/ports.go`
- Create: `services/otp-dispatcher/internal/app/sender.go`
- Modify: `services/otp-dispatcher/internal/app/handler.go`
- Test: `services/otp-dispatcher/internal/app/handler_test.go`

**Interfaces:**
- Produces:
  - `app.Sender` interface: `Name() string` and `Send(ctx context.Context, to, code string) (msgID string, err error)`.
  - `app.SMSProvider` interface: `Send(ctx context.Context, to, body string) (msgID string, err error)`.
  - `app.NewEmailSender(p EmailProvider, name string, tpl Template) Sender`
  - `app.NewSMSSender(p SMSProvider, name, bodyFmt string) Sender`
  - `app.Deps.Senders map[string]Sender` replaces `app.Deps.Mail`.
  - `app.Config` drops `ProviderName` and `Template` (they move into the email sender); it keeps the three topics.
- Consumes: existing `EmailProvider`, `Repo`, `Publisher`, `Clock`, `Template`.

- [ ] **Step 1: Write the failing test** (rewrite the fakes/helper in `handler_test.go`)

Replace `fakeMail` and `newHandler` with a channel-aware setup, and add routing tests:

```go
type fakeSender struct {
	name string
	id   string
	err  error
	sent int
}

func (f *fakeSender) Name() string { return f.name }
func (f *fakeSender) Send(context.Context, string, string) (string, error) {
	f.sent++
	return f.id, f.err
}

func newHandler(senders map[string]Sender) (*Handler, *fakeRepo, *fakePub) {
	repo := newFakeRepo()
	pub := &fakePub{}
	h := NewHandler(Deps{Senders: senders, Repo: repo, Pub: pub, Clock: fixedClock{t: time.Unix(0, 0)}}, Config{
		SentTopic: "otp.sent", FailedTopic: "otp.failed", DLQTopic: "otp.dlq",
	})
	return h, repo, pub
}

func smsEvt() contracts.RequestedEvent {
	return contracts.RequestedEvent{RequestID: "r2", TenantID: "t1", Recipient: "+84901234567", Channel: "sms", Code: "123456"}
}

func TestHandle_RoutesToChannelSender(t *testing.T) {
	email := &fakeSender{name: "resend", id: "e-1"}
	sms := &fakeSender{name: "twilio", id: "s-1"}
	h, repo, pub := newHandler(map[string]Sender{"email": email, "sms": sms})

	if err := h.Handle(context.Background(), smsEvt()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if sms.sent != 1 || email.sent != 0 {
		t.Fatalf("sms event must go to the sms sender only (sms=%d email=%d)", sms.sent, email.sent)
	}
	if repo.logs[0].Provider != "twilio" {
		t.Fatalf("delivery log provider = %q, want twilio", repo.logs[0].Provider)
	}
	if pub.topics[0] != "otp.sent" {
		t.Fatalf("want otp.sent, got %v", pub.topics)
	}
}

func TestHandle_UnknownChannel_DLQ(t *testing.T) {
	h, repo, pub := newHandler(map[string]Sender{"email": &fakeSender{name: "resend"}})
	bad := contracts.RequestedEvent{RequestID: "r3", TenantID: "t1", Recipient: "x", Channel: "push", Code: "1"}
	if err := h.Handle(context.Background(), bad); err != nil {
		t.Fatalf("unknown channel must be swallowed to DLQ: %v", err)
	}
	if repo.states["r3"] != contracts.StateFailed {
		t.Fatalf("state = %q, want failed", repo.states["r3"])
	}
	if len(pub.topics) != 2 || pub.topics[0] != "otp.failed" || pub.topics[1] != "otp.dlq" {
		t.Fatalf("want failed then dlq, got %v", pub.topics)
	}
}
```

Update the existing `TestHandle_Success` / `TestHandle_MailFailure_LogsFailedAndDLQ` to build senders via `map[string]Sender{"email": &fakeSender{name: "smtp", id: "msg-1"}}` (and an erroring `fakeSender` for the failure case), using `evt()` which already has `Channel: "email"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-dispatcher/internal/app/ -v`
Expected: FAIL (`Deps` has no `Senders`; `Sender` undefined).

- [ ] **Step 3: Add ports** (`ports.go`) - add below `EmailProvider`:

```go
// SMSProvider sends one text message and returns the provider's message id.
type SMSProvider interface {
	Send(ctx context.Context, to, body string) (providerMsgID string, err error)
}

// Sender renders and delivers one OTP over a specific channel. Name is recorded on the
// delivery log as the provider label.
type Sender interface {
	Name() string
	Send(ctx context.Context, to, code string) (msgID string, err error)
}
```

- [ ] **Step 4: Implement senders** (`sender.go`, new)

```go
package app

import "context"

// emailSender renders subject+body from the email Template and delivers via EmailProvider.
type emailSender struct {
	p    EmailProvider
	name string
	tpl  Template
}

// NewEmailSender builds a Sender backed by an EmailProvider (e.g. Resend or SMTP).
func NewEmailSender(p EmailProvider, name string, tpl Template) Sender {
	return &emailSender{p: p, name: name, tpl: tpl}
}

func (s *emailSender) Name() string { return s.name }
func (s *emailSender) Send(ctx context.Context, to, code string) (string, error) {
	subject, body := s.tpl.Render(code)
	return s.p.Send(ctx, to, subject, body)
}

// smsSender renders a body-only message and delivers via SMSProvider.
type smsSender struct {
	p       SMSProvider
	name    string
	bodyFmt string
}

// NewSMSSender builds a Sender backed by an SMSProvider (e.g. Twilio). bodyFmt is a
// printf format with a single %s for the code.
func NewSMSSender(p SMSProvider, name, bodyFmt string) Sender {
	return &smsSender{p: p, name: name, bodyFmt: bodyFmt}
}

func (s *smsSender) Name() string { return s.name }
func (s *smsSender) Send(ctx context.Context, to, code string) (string, error) {
	return s.p.Send(ctx, to, fmt.Sprintf(s.bodyFmt, code))
}
```

Add `"fmt"` to the imports.

- [ ] **Step 5: Rewrite the handler** (`handler.go`)

Change `Deps` and `Config`:

```go
type Config struct {
	SentTopic   string
	FailedTopic string
	DLQTopic    string
}

type Deps struct {
	Senders map[string]Sender
	Repo    Repo
	Pub     Publisher
	Clock   Clock
}
```

Replace the body of `Handle`. The render + send lines become a registry lookup; everything else (log/state/publish) is unchanged except the provider label now comes from the sender:

```go
func (h *Handler) Handle(ctx context.Context, evt contracts.RequestedEvent) error {
	sender, ok := h.d.Senders[evt.Channel]
	if !ok {
		// Unknown channel: record a failed delivery and route to the DLQ (no redelivery).
		return h.recordFailure(ctx, evt, "unknown", 0, "unsupported channel: "+evt.Channel)
	}

	start := h.d.Clock.Now()
	msgID, sendErr := sender.Send(ctx, evt.Recipient, evt.Code)
	latency := h.d.Clock.Now().Sub(start).Milliseconds()

	if sendErr != nil {
		return h.recordFailure(ctx, evt, sender.Name(), latency, sendErr.Error())
	}

	if err := h.d.Repo.InsertDeliveryLog(ctx, DeliveryLog{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Provider: sender.Name(),
		Status: contracts.StateSent, LatencyMillis: latency, Error: "",
	}); err != nil {
		return err
	}
	if err := h.d.Repo.UpdateState(ctx, evt.RequestID, contracts.StateSent); err != nil {
		return err
	}
	_ = msgID
	return h.d.Pub.Publish(ctx, h.cfg.SentTopic, evt)
}

// recordFailure logs a failed delivery, marks the request failed, and fans the event out
// to failed + DLQ. It returns nil so Kafka does not redeliver a terminal failure.
func (h *Handler) recordFailure(ctx context.Context, evt contracts.RequestedEvent, provider string, latency int64, msg string) error {
	if err := h.d.Repo.InsertDeliveryLog(ctx, DeliveryLog{
		RequestID: evt.RequestID, TenantID: evt.TenantID, Provider: provider,
		Status: contracts.StateFailed, LatencyMillis: latency, Error: msg,
	}); err != nil {
		return err
	}
	if err := h.d.Repo.UpdateState(ctx, evt.RequestID, contracts.StateFailed); err != nil {
		return err
	}
	if err := h.d.Pub.Publish(ctx, h.cfg.FailedTopic, evt); err != nil {
		return err
	}
	return h.d.Pub.Publish(ctx, h.cfg.DLQTopic, evt)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./services/otp-dispatcher/internal/app/ -v`
Expected: PASS. (`main.go` will not compile yet - fixed in Task 6. Test only the `app` package here.)

- [ ] **Step 7: Commit**

```bash
git add services/otp-dispatcher/internal/app/
git commit -m "feat(otp-dispatcher): route delivery by channel via Sender registry"
```

---

## Task 5: Twilio SMS adapter

**Files:**
- Create: `services/otp-dispatcher/internal/adapters/outbound/twiliosms/provider.go`
- Test: `services/otp-dispatcher/internal/adapters/outbound/twiliosms/provider_test.go`

**Interfaces:**
- Produces: `twiliosms.New(accountSID, authToken, from, baseURL string, hc *http.Client) *Provider` with `Send(ctx, to, body string) (string, error)` - satisfies `app.SMSProvider` (Task 4).

- [ ] **Step 1: Write the failing test** (`provider_test.go`)

```go
package twiliosms_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/twiliosms"
)

func TestSend_PostsFormAndReturnsSID(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sid":"SM123"}`))
	}))
	defer srv.Close()

	p := twiliosms.New("ACxxx", "tok", "+15005550006", srv.URL, srv.Client())
	id, err := p.Send(context.Background(), "+84901234567", "Your code is 123456")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "SM123" {
		t.Fatalf("want sid SM123, got %q", id)
	}
	if gotPath != "/2010-04-01/Accounts/ACxxx/Messages.json" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("want basic auth, got %q", gotAuth)
	}
	if !strings.Contains(gotBody, "To=%2B84901234567") || !strings.Contains(gotBody, "From=%2B15005550006") {
		t.Fatalf("form body missing To/From: %s", gotBody)
	}
}

func TestSend_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"auth failed"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := twiliosms.New("AC", "bad", "+1", srv.URL, srv.Client())
	if _, err := p.Send(context.Background(), "+84901234567", "b"); err == nil {
		t.Fatal("non-2xx must return an error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/otp-dispatcher/internal/adapters/outbound/twiliosms/ -v`
Expected: FAIL (package/`New` undefined).

- [ ] **Step 3: Implement** (`provider.go`)

```go
// Package twiliosms is otp-dispatcher's SMS adapter: it sends text messages via the
// Twilio REST API and implements the dispatcher's app.SMSProvider port.
package twiliosms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Provider talks to the Twilio Messages API. baseURL is injected (rather than hard-coded
// to https://api.twilio.com) so tests can point it at an httptest stub.
type Provider struct {
	accountSID string
	authToken  string
	from       string
	baseURL    string
	hc         *http.Client
}

func New(accountSID, authToken, from, baseURL string, hc *http.Client) *Provider {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Provider{accountSID: accountSID, authToken: authToken, from: from, baseURL: baseURL, hc: hc}
}

type sendResponse struct {
	SID string `json:"sid"`
}

// Send delivers one SMS and returns the Twilio message SID. Any non-2xx response returns
// an error so the caller records a failed delivery.
func (p *Provider) Send(ctx context.Context, to, body string) (string, error) {
	form := url.Values{"To": {to}, "From": {p.from}, "Body": {body}}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", p.baseURL, p.accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("twilio: new request: %w", err)
	}
	req.SetBasicAuth(p.accountSID, p.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("twilio: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("twilio: status %d: %s", resp.StatusCode, string(respBody))
	}
	var out sendResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("twilio: decode: %w", err)
	}
	return out.SID, nil
}
```

Add `"encoding/json"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/otp-dispatcher/internal/adapters/outbound/twiliosms/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/otp-dispatcher/internal/adapters/outbound/twiliosms/
git commit -m "feat(otp-dispatcher): Twilio SMS provider adapter"
```

---

## Task 6: Wire dispatcher composition root + config + compose

**Files:**
- Modify: `services/otp-dispatcher/main.go`
- Modify: `.env.example`
- Modify: `deploy/compose/` (the dispatcher service env)

**Interfaces:**
- Consumes: `app.NewEmailSender`, `app.NewSMSSender`, `app.Deps.Senders`, `app.Config` (Task 4); `twiliosms.New` (Task 5).

- [ ] **Step 1: Update `main.go`**

Add Twilio config reads after the Resend reads:

```go
	twilioSID := config.Env("TWILIO_ACCOUNT_SID", "")
	twilioToken := config.Env("TWILIO_AUTH_TOKEN", "")
	twilioFrom := config.Env("TWILIO_FROM", "")
	twilioBase := config.Env("TWILIO_BASE_URL", "https://api.twilio.com")
	smsBodyFmt := config.Env("OTP_SMS_BODY_FMT", "Your verification code is %s. It expires in 5 minutes.")
```

Build the SMS provider and the sender registry (replace the `handler := app.NewHandler(...)` block):

```go
	sms := twiliosms.New(twilioSID, twilioToken, twilioFrom, twilioBase, &http.Client{Timeout: 10 * time.Second})

	emailTpl := app.Template{
		Subject: config.Env("OTP_EMAIL_SUBJECT", "Your verification code"),
		BodyFmt: config.Env("OTP_EMAIL_BODY", "Your verification code is %s. It expires in 5 minutes."),
	}

	handler := app.NewHandler(app.Deps{
		Senders: map[string]app.Sender{
			"email": app.NewEmailSender(mail, providerLabel, emailTpl),
			"sms":   app.NewSMSSender(sms, "twilio", smsBodyFmt),
		},
		Repo:  repo,
		Pub:   prod,
		Clock: realClock{},
	}, app.Config{
		SentTopic:   config.Env("KAFKA_TOPIC_SENT", "otp.sent"),
		FailedTopic: config.Env("KAFKA_TOPIC_FAILED", "otp.failed"),
		DLQTopic:    config.Env("KAFKA_TOPIC_DLQ", "otp.dlq"),
	})
```

Add the import `"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/twiliosms"`.

- [ ] **Step 2: Verify the whole module builds and tests pass**

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: build ok; all tests PASS.

- [ ] **Step 3: Document env** (`.env.example`) - add:

```
# Twilio SMS (dispatcher). Use test credentials for local/dev.
TWILIO_ACCOUNT_SID=
TWILIO_AUTH_TOKEN=
TWILIO_FROM=
TWILIO_BASE_URL=https://api.twilio.com
OTP_SMS_BODY_FMT=Your verification code is %s. It expires in 5 minutes.
```

- [ ] **Step 4: Wire compose** - in the dispatcher service definition under `deploy/compose/` add the same `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM`, `TWILIO_BASE_URL`, `OTP_SMS_BODY_FMT` environment entries (match the existing `RESEND_*` style in that file - env passthrough with defaults).

- [ ] **Step 5: Commit**

```bash
git add services/otp-dispatcher/main.go .env.example deploy/compose/
git commit -m "chore(otp-dispatcher): wire Twilio SMS sender and config"
```

---

## Task 7: Dashboard - pure phone helper

**Files:**
- Create: `dashboard/lib/phone.ts`
- Test: `dashboard/lib/phone.test.ts`

**Interfaces:**
- Produces:
  - `DIAL_CODES: { code: string; label: string; iso: string }[]` (e.g. `{ code: "+84", label: "Vietnam", iso: "VN" }`), VN first.
  - `toE164(dialCode: string, national: string): string`
  - `isValidE164(value: string): boolean`
  - `parsePhone(value: string): { dialCode: string; national: string } | null`

- [ ] **Step 1: Write the failing test** (`lib/phone.test.ts`) - run from `dashboard/`

```ts
import { describe, it, expect } from "vitest";
import { DIAL_CODES, toE164, isValidE164, parsePhone } from "./phone";

describe("phone", () => {
  it("lists Vietnam first as the default", () => {
    expect(DIAL_CODES[0].code).toBe("+84");
  });

  it("composes E.164 from a dial code and national number, dropping a leading 0", () => {
    expect(toE164("+84", "0901234567")).toBe("+84901234567");
    expect(toE164("+84", "901 234 567")).toBe("+84901234567");
  });

  it("validates E.164 shape", () => {
    expect(isValidE164("+84901234567")).toBe(true);
    expect(isValidE164("0901234567")).toBe(false);
    expect(isValidE164("+0123")).toBe(false);
  });

  it("parses an E.164 value back into a known dial code and national part", () => {
    expect(parsePhone("+84901234567")).toEqual({ dialCode: "+84", national: "901234567" });
    expect(parsePhone("not-a-phone")).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (in `dashboard/`): `pnpm test -- phone`
Expected: FAIL (module not found).

- [ ] **Step 3: Implement** (`lib/phone.ts`)

```ts
export type DialCode = { code: string; label: string; iso: string };

// Curated list - sufficient for the playground, easy to extend. Vietnam is the default.
export const DIAL_CODES: DialCode[] = [
  { code: "+84", label: "Vietnam", iso: "VN" },
  { code: "+1", label: "United States", iso: "US" },
  { code: "+44", label: "United Kingdom", iso: "GB" },
  { code: "+65", label: "Singapore", iso: "SG" },
  { code: "+81", label: "Japan", iso: "JP" },
  { code: "+61", label: "Australia", iso: "AU" },
  { code: "+91", label: "India", iso: "IN" },
  { code: "+49", label: "Germany", iso: "DE" },
  { code: "+33", label: "France", iso: "FR" },
  { code: "+82", label: "South Korea", iso: "KR" },
];

const E164 = /^\+[1-9]\d{7,14}$/;

// toE164 joins a dial code and a national number into an E.164 string. It strips spaces
// and a single leading 0 (common local trunk prefix) from the national part.
export function toE164(dialCode: string, national: string): string {
  const digits = national.replace(/\D/g, "").replace(/^0+/, "");
  return `${dialCode}${digits}`;
}

export function isValidE164(value: string): boolean {
  return E164.test(value);
}

// parsePhone splits an E.164 value against the known dial codes (longest match wins so
// +1 does not shadow a longer code). Returns null if it is not valid E.164 or the dial
// code is not in the list.
export function parsePhone(value: string): { dialCode: string; national: string } | null {
  if (!isValidE164(value)) return null;
  const match = [...DIAL_CODES]
    .sort((a, b) => b.code.length - a.code.length)
    .find((d) => value.startsWith(d.code));
  if (!match) return null;
  return { dialCode: match.code, national: value.slice(match.code.length) };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (in `dashboard/`): `pnpm test -- phone`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add dashboard/lib/phone.ts dashboard/lib/phone.test.ts
git commit -m "feat(dashboard): pure phone dial-code and E.164 helpers"
```

---

## Task 8: Dashboard - thread channel through the data layer

**Files:**
- Modify: `dashboard/lib/api/source.ts`
- Modify: `dashboard/lib/api/live.ts`
- Modify: `dashboard/lib/api/mock.ts`
- Modify: `dashboard/lib/queries/use-otp.ts`
- Modify: `dashboard/lib/schemas.ts`
- Test: `dashboard/lib/api/mock.test.ts` (extend)

**Interfaces:**
- Consumes: `toE164`/`isValidE164` (Task 7) in schemas.
- Produces:
  - `DataSource.send(recipient: string, channel: string): Promise<SendResult>`
  - `DataSource.verify(recipient: string, code: string, channel?: string): Promise<VerifyResult>`
  - `useSend()` mutate arg becomes `{ recipient: string; channel: string }`.
  - `useVerify()` mutate arg becomes `{ recipient: string; code: string; channel: string }`.

- [ ] **Step 1: Write the failing test** (extend `lib/api/mock.test.ts`)

```ts
it("send records the chosen channel on the request", async () => {
  const ds = makeMockDataSource(); // reuse the file's existing constructor
  await ds.send("+84901234567", "sms");
  const requests = await ds.getRequests();
  expect(requests.some((r) => r.channel === "sms")).toBe(true);
});
```

Also update the existing mock send/verify calls in this file to pass a channel:
`ds.send("dev@worklane.io", "email")` and `ds.verify("dev@worklane.io", sent.devCode!, "email")`.

- [ ] **Step 2: Run test to verify it fails**

Run (in `dashboard/`): `pnpm test -- mock`
Expected: FAIL (send takes one argument; channel not recorded).

- [ ] **Step 3: Update the port** (`source.ts`)

```ts
  send(recipient: string, channel: string): Promise<SendResult>;
  verify(recipient: string, code: string, channel?: string): Promise<VerifyResult>;
```

- [ ] **Step 4: Update live** (`live.ts`)

```ts
  async send(recipient: string, channel: string): Promise<SendResult> {
    const res = await fetch(this.baseUrl + "/v1/otp/send", {
      method: "POST",
      headers: this.authHeaders(),
      body: JSON.stringify({ recipient, channel }),
    });
    if (!res.ok) throw new Error(`send failed: ${res.status}`);
    const body = (await res.json()) as { request_id: string };
    return { requestId: body.request_id };
  }

  async verify(recipient: string, code: string): Promise<VerifyResult> {
    // channel is accepted for interface symmetry; the backend keys verification by
    // (tenant, recipient), so it is not sent.
    const res = await fetch(this.baseUrl + "/v1/otp/verify", {
      method: "POST",
      headers: this.authHeaders(),
      body: JSON.stringify({ recipient, code }),
    });
    // ... unchanged switch ...
  }
```

- [ ] **Step 5: Update mock** (`mock.ts`)

```ts
  async send(recipient: string, channel: string): Promise<SendResult> {
    await this.latency();
    const r = rng((Date.now() ^ recipient.length) >>> 0);
    const code = String(100000 + Math.floor(r() * 900000));
    this.codes.set(recipient.toLowerCase(), code);
    const id = shortId("req", r);
    this.extraRequests = [
      ...(this.extraRequests ?? []),
      { id, recipient: mask(recipient), channel, state: "sent", createdAt: iso(Date.now()) },
    ];
    return { requestId: id, devCode: code };
  }
```

> Match the mock's real shape: reuse its existing `mask`/`iso`/`shortId` helpers and the field names on the request fixture (`id, recipient, channel, state, createdAt`). If the mock keeps its list in `this.fixtures.requests`, append there instead of a new field - follow the file's own pattern so `getRequests()` returns the new row. Keep the `verify` signature `(recipient, code, channel?)` but leave its body logic unchanged.

- [ ] **Step 6: Update the queries** (`use-otp.ts`)

```ts
  return useMutation({
    mutationFn: (vars: { recipient: string; channel: string }) =>
      getDataSource().send(vars.recipient, vars.channel),
    // ... onSuccess unchanged ...
  });
```

```ts
  return useMutation({
    mutationFn: (vars: { recipient: string; code: string; channel: string }) =>
      getDataSource().verify(vars.recipient, vars.code, vars.channel),
    // ... onSuccess unchanged ...
  });
```

- [ ] **Step 7: Channel-aware schemas** (`schemas.ts`)

```ts
import { z } from "zod";
import { isValidE164 } from "./phone";

export const CHANNELS = ["email", "sms"] as const;
export type Channel = (typeof CHANNELS)[number];

export function recipientSchema(channel: Channel) {
  return channel === "sms"
    ? z.string().refine(isValidE164, "Enter a valid phone number")
    : z.string().email("Enter a valid email address");
}

export const sendSchema = (channel: Channel) =>
  z.object({ recipient: recipientSchema(channel) });
export type SendValues = { recipient: string };

export const verifySchema = (channel: Channel) =>
  z.object({
    recipient: recipientSchema(channel),
    code: z.string().regex(/^\d{6}$/, "The code is 6 digits"),
  });
export type VerifyValues = { recipient: string; code: string };
```

> This changes `sendSchema`/`verifySchema` from constants to factories. Their only consumers are `send-form.tsx` and `verify-form.tsx`, both updated in Task 10.

- [ ] **Step 8: Run tests to verify they pass**

Run (in `dashboard/`): `pnpm test -- mock` then `pnpm typecheck`
Expected: mock test PASS. `typecheck` will report errors in `send-form.tsx`/`verify-form.tsx` (they still call the old signatures) - those are fixed in Task 10. It is acceptable for this task's commit to leave those two files failing typecheck **only if** committed together with Task 10; to keep each commit green, do Task 9 and Task 10 before the final typecheck gate.

- [ ] **Step 9: Commit**

```bash
git add dashboard/lib/
git commit -m "feat(dashboard): thread delivery channel through the data layer"
```

---

## Task 9: Dashboard - PhoneInput component

**Files:**
- Create: `dashboard/components/common/phone-input.tsx`

**Interfaces:**
- Consumes: `DIAL_CODES`, `toE164`, `parsePhone` (Task 7); `components/ui/select.tsx`, `components/ui/input.tsx` (existing).
- Produces: `PhoneInput({ value, onChange, id }: { value: string; onChange: (e164: string) => void; id?: string })` - renders a dial-code `Select` + national-number `Input`, calls `onChange` with the composed E.164 string.

- [ ] **Step 1: Read the Select API**

Open `dashboard/components/ui/select.tsx` to confirm the exported parts (e.g. `Select`, `SelectTrigger`, `SelectValue`, `SelectContent`, `SelectItem`) and their props before writing the component.

- [ ] **Step 2: Implement** (`phone-input.tsx`)

```tsx
"use client";

import { useState } from "react";
import { DIAL_CODES, toE164, parsePhone } from "@/lib/phone";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function PhoneInput({
  value,
  onChange,
  id,
}: {
  value: string;
  onChange: (e164: string) => void;
  id?: string;
}) {
  const parsed = parsePhone(value);
  const [dialCode, setDialCode] = useState(parsed?.dialCode ?? DIAL_CODES[0].code);
  const [national, setNational] = useState(parsed?.national ?? "");

  const update = (code: string, num: string) => {
    setDialCode(code);
    setNational(num);
    onChange(toE164(code, num));
  };

  return (
    <div className="flex gap-2">
      <Select value={dialCode} onValueChange={(c) => update(c, national)}>
        <SelectTrigger className="w-28 shrink-0">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {DIAL_CODES.map((d) => (
            <SelectItem key={d.iso} value={d.code}>
              {d.code} {d.iso}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        id={id}
        inputMode="tel"
        autoComplete="off"
        placeholder="901 234 567"
        value={national}
        onChange={(e) => update(dialCode, e.target.value)}
      />
    </div>
  );
}
```

> Adjust the `Select` sub-component names/props to match the actual `select.tsx` exports from Step 1.

- [ ] **Step 3: Verify it typechecks**

Run (in `dashboard/`): `pnpm typecheck`
Expected: no new errors from `phone-input.tsx` (form errors from Task 8 may remain until Task 10).

- [ ] **Step 4: Commit**

```bash
git add dashboard/components/common/phone-input.tsx
git commit -m "feat(dashboard): country-dial-code phone input"
```

---

## Task 10: Dashboard - playground channel selection (send + verify)

**Files:**
- Modify: `dashboard/components/playground/playground.tsx`
- Modify: `dashboard/components/playground/send-form.tsx`
- Modify: `dashboard/components/playground/verify-form.tsx`
- Test: `dashboard/components/playground/send-form.test.tsx`

**Interfaces:**
- Consumes: `Channel`, `sendSchema`, `verifySchema` (Task 8); `useSend`/`useVerify` new arg shapes (Task 8); `PhoneInput` (Task 9).
- Produces: a shared `channel` state in `Playground` passed to both forms; each form renders an email `Input` or `PhoneInput` per channel.

- [ ] **Step 1: Update the send-form test** (`send-form.test.tsx`)

The form now needs a `channel` prop. Update both existing renders to `<SendForm channel="email" />`. Add an SMS case:

```ts
it("shows a validation error for an invalid phone under the sms channel", async () => {
  const user = userEvent.setup();
  renderWithClient(<SendForm channel="sms" />);
  await user.type(screen.getByLabelText(/phone/i), "12345");
  await user.click(screen.getByRole("button", { name: /send code/i }));
  expect(await screen.findByText(/valid phone number/i)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (in `dashboard/`): `pnpm test -- send-form`
Expected: FAIL (`SendForm` has no `channel` prop; no phone label).

- [ ] **Step 3: Add a channel selector to the container** (`playground.tsx`)

Lift a shared `channel` state and render a segmented `email | sms` control (use the existing `Select` from `components/ui/select.tsx`, or two `Button`s with an active state - match the DS). Pass `channel` to both forms:

```tsx
const [channel, setChannel] = useState<Channel>("email");
// ...
<SendForm channel={channel} onSent={/* unchanged */} />
<VerifyForm key={verifyKey} channel={channel} initial={prefill} />
```

Import `type { Channel } from "@/lib/schemas"`. Place the selector above the two panels with a label like "Channel".

- [ ] **Step 4: Update `send-form.tsx`**

- Accept `channel: Channel` as a prop.
- Build the resolver from the factory: `resolver: zodResolver(sendSchema(channel))`.
- When `channel === "sms"` render `<PhoneInput value={watch("recipient") ?? ""} onChange={(v) => setValue("recipient", v, { shouldValidate: true })} id="send-recipient" />` with label "Recipient phone"; otherwise keep the email `Input` with label "Recipient email".
- Call `send.mutate({ recipient: values.recipient, channel })`.

Use RHF `watch`/`setValue` (add them to the `useForm` destructure) to bridge `PhoneInput`'s controlled value into the form.

- [ ] **Step 5: Update `verify-form.tsx`**

- Accept `channel: Channel`.
- `resolver: zodResolver(verifySchema(channel))`.
- Same recipient input switch as send (label "Recipient phone" vs "Recipient email").
- Call `verify.mutate({ recipient: values.recipient, code: values.code, channel })`.

- [ ] **Step 6: Run the full dashboard gate**

Run (in `dashboard/`): `pnpm lint && pnpm typecheck && pnpm test`
Expected: all PASS (send-form email + sms cases, mock channel test, phone test, and the rest of the suite).

- [ ] **Step 7: Commit**

```bash
git add dashboard/components/playground/
git commit -m "feat(dashboard): playground channel selector with phone input for SMS"
```

---

## Task 11: Full-stack verification

**Files:** none (verification only).

- [ ] **Step 1: Backend gate**

Run (repo root): `go vet ./... && go test -race ./...`
Expected: all PASS.

- [ ] **Step 2: Dashboard gate**

Run (in `dashboard/`): `pnpm lint && pnpm typecheck && pnpm test`
Expected: all PASS.

- [ ] **Step 3: Local E2E smoke (optional, needs the compose stack)**

Bring up the stack; seed a tenant/key; then:

```bash
curl -sS -X POST "$API/v1/otp/send" -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"recipient":"+15005550006","channel":"sms"}'
```

With Twilio **test credentials** set on the dispatcher (`TWILIO_ACCOUNT_SID`/`TWILIO_AUTH_TOKEN` from the test project, `TWILIO_BASE_URL=https://api.twilio.com`), `+15005550006` is Twilio's magic "valid" number and returns a queued SID without sending a real message; the delivery log should show `provider=twilio, status=sent`. Alternatively point `TWILIO_BASE_URL` at a local stub. Verify the request/delivery rows appear in the dashboard.

- [ ] **Step 4: Merge**

Follow `superpowers:finishing-a-development-branch` to integrate `feat/sms-channel`.

---

## Self-Review

**Spec coverage (vs `2026-08-18-sms-channel-design.md`):**
- §5.1 event carries real channel -> Task 2. ✅
- §5.2 domain validation + masking -> Task 1; send use case -> Task 2; DTO/handler/400 -> Task 3; repo mask -> Task 2. ✅
- §5.3 Sender port + registry + unknown-channel DLQ + SMS template -> Task 4; wiring -> Task 6. ✅
- §5.3 Twilio adapter -> Task 5. ✅
- §6.1 data layer channel -> Task 8. ✅
- §6.2 pure phone helper (import-reusable) -> Task 7. ✅
- §6.3 PhoneInput + playground channel selection + schemas -> Tasks 9, 10. ✅
- §7 error handling (400 mismatch, DLQ unknown channel, Twilio non-2xx) -> Tasks 3, 4, 5. ✅
- §8 tests, both gates -> every task + Task 11. ✅
- §9 rollout; operational cutover explicitly out of scope -> Task 11 Step 3 note. ✅

**Deferred (per spec):** live Twilio number + prod deploy; real bulk-import UI (only the pure helper is factored).

**Type consistency:** `Sender.Send(ctx, to, code)` and `SMSProvider.Send(ctx, to, body)` are distinct by design (Task 4/5). `Deps.Senders map[string]Sender` and `Config` without `ProviderName`/`Template` are used consistently in Tasks 4 and 6. `send(recipient, channel)` / `verify(recipient, code, channel?)` match across `source.ts`, `live.ts`, `mock.ts`, `use-otp.ts`, and both forms (Tasks 8, 10). `sendSchema`/`verifySchema` are factories consumed only in Task 10.
