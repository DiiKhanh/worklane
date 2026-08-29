package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

type fakeSender struct {
	name        string
	id          string
	err         error
	sent        int
	lastSubject string
	lastBody    string
}

func (f *fakeSender) Name() string { return f.name }
func (f *fakeSender) Send(_ context.Context, _, subject, body string) (string, error) {
	f.sent++
	f.lastSubject = subject
	f.lastBody = body
	return f.id, f.err
}

type fakeTemplateSource struct {
	tpl   Template
	found bool
	err   error
}

func (f *fakeTemplateSource) Active(context.Context, string, string) (Template, bool, error) {
	return f.tpl, f.found, f.err
}

type fakeRepo struct {
	logs   []DeliveryLog
	states map[string]string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{states: map[string]string{}} }
func (f *fakeRepo) InsertDeliveryLog(_ context.Context, l DeliveryLog) error {
	f.logs = append(f.logs, l)
	return nil
}
func (f *fakeRepo) UpdateState(_ context.Context, id, to string) error {
	f.states[id] = to
	return nil
}

type fakePub struct {
	topics []string
	events []any
}

func (f *fakePub) Publish(_ context.Context, topic string, event any) error {
	f.topics = append(f.topics, topic)
	f.events = append(f.events, event)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newHandlerWithTemplates(senders map[string]Sender, src TemplateSource, fallback map[string]Template, expiry string) (*Handler, *fakeRepo, *fakePub) {
	repo := newFakeRepo()
	pub := &fakePub{}
	h := NewHandler(Deps{Senders: senders, Templates: src, Repo: repo, Pub: pub, Clock: fixedClock{t: time.Unix(0, 0)}}, Config{
		SentTopic: "otp.sent", FailedTopic: "otp.failed", DLQTopic: "otp.dlq",
		ExpiryText: expiry, Fallback: fallback,
	})
	return h, repo, pub
}

// newHandler keeps the pre-Template-Studio test surface: no active DB template (source
// returns found=false), so every existing test renders from the env fallback.
func newHandler(senders map[string]Sender) (*Handler, *fakeRepo, *fakePub) {
	return newHandlerWithTemplates(senders, &fakeTemplateSource{found: false},
		map[string]Template{
			"email": {Subject: "Your verification code", Body: "Your verification code is {{code}}."},
			"sms":   {Body: "Your verification code is {{code}}."},
		}, "5 minutes")
}

func evt() contracts.RequestedEvent {
	return contracts.RequestedEvent{RequestID: "r1", TenantID: "t1", Recipient: "a@b.co", Channel: "email", Code: "123456"}
}

func smsEvt() contracts.RequestedEvent {
	return contracts.RequestedEvent{RequestID: "r2", TenantID: "t1", Recipient: "+84901234567", Channel: "sms", Code: "123456"}
}

func TestHandle_RendersFromTemplateSource(t *testing.T) {
	email := &fakeSender{name: "resend", id: "e1"}
	src := &fakeTemplateSource{tpl: Template{Subject: "Code {{code}}", Body: "It is {{code}}, expires {{expiry}}."}, found: true}
	h, _, _ := newHandlerWithTemplates(map[string]Sender{"email": email}, src,
		map[string]Template{"email": {Subject: "FB", Body: "fallback {{code}}"}}, "5 minutes")
	if err := h.Handle(context.Background(), evt()); err != nil {
		t.Fatal(err)
	}
	if email.lastSubject != "Code 123456" || email.lastBody != "It is 123456, expires 5 minutes." {
		t.Fatalf("rendered from DB template expected, got %q / %q", email.lastSubject, email.lastBody)
	}
}

func TestHandle_FallsBackWhenNoActiveTemplate(t *testing.T) {
	email := &fakeSender{name: "resend", id: "e1"}
	src := &fakeTemplateSource{found: false}
	h, _, _ := newHandlerWithTemplates(map[string]Sender{"email": email}, src,
		map[string]Template{"email": {Subject: "FB", Body: "fallback {{code}}"}}, "5 minutes")
	if err := h.Handle(context.Background(), evt()); err != nil {
		t.Fatal(err)
	}
	if email.lastBody != "fallback 123456" {
		t.Fatalf("must fall back to env template, got %q", email.lastBody)
	}
}

func TestHandle_Success(t *testing.T) {
	h, repo, pub := newHandler(map[string]Sender{"email": &fakeSender{name: "smtp", id: "msg-1"}})
	if err := h.Handle(context.Background(), evt()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if repo.states["r1"] != contracts.StateSent {
		t.Fatalf("state should be sent, got %q", repo.states["r1"])
	}
	if len(repo.logs) != 1 || repo.logs[0].Status != "sent" {
		t.Fatalf("expected one sent delivery log, got %+v", repo.logs)
	}
	if repo.logs[0].Provider != "smtp" {
		t.Fatalf("delivery log must record the configured provider, got %q", repo.logs[0].Provider)
	}
	if len(pub.topics) != 1 || pub.topics[0] != "otp.sent" {
		t.Fatalf("expected publish to otp.sent, got %v", pub.topics)
	}
}

func TestHandle_Success_PublishesSentEventWithoutCode(t *testing.T) {
	h, _, pub := newHandler(map[string]Sender{"email": &fakeSender{name: "smtp", id: "msg-1"}})
	if err := h.Handle(context.Background(), evt()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	se, ok := pub.events[0].(contracts.SentEvent)
	if !ok {
		// A distinct type both fixes the msg_type discriminator and, by construction,
		// keeps the OTP code off every downstream topic.
		t.Fatalf("otp.sent payload must be a SentEvent, got %T", pub.events[0])
	}
	if se.RequestID != "r1" || se.TenantID != "t1" || se.Channel != "email" || se.Provider != "smtp" {
		t.Fatalf("sent event fields wrong: %+v", se)
	}
}

func TestHandle_Failure_PublishesFailedEventWithoutCode(t *testing.T) {
	h, _, pub := newHandler(map[string]Sender{"email": &fakeSender{name: "smtp", err: errors.New("provider down")}})
	if err := h.Handle(context.Background(), evt()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	// Both otp.failed and otp.dlq carry the same FailedEvent (no code).
	for i, topic := range []string{"otp.failed", "otp.dlq"} {
		fe, ok := pub.events[i].(contracts.FailedEvent)
		if !ok {
			t.Fatalf("%s payload must be a FailedEvent, got %T", topic, pub.events[i])
		}
		if fe.RequestID != "r1" || fe.Provider != "smtp" || fe.Error != "provider down" {
			t.Fatalf("%s failed event fields wrong: %+v", topic, fe)
		}
	}
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

func TestHandle_MailFailure_LogsFailedAndDLQ(t *testing.T) {
	h, repo, pub := newHandler(map[string]Sender{"email": &fakeSender{name: "smtp", err: errors.New("provider down")}})
	if err := h.Handle(context.Background(), evt()); err != nil {
		t.Fatalf("handle should swallow a provider failure (routed to DLQ): %v", err)
	}
	if repo.states["r1"] != contracts.StateFailed {
		t.Fatalf("state should be failed, got %q", repo.states["r1"])
	}
	if len(repo.logs) != 1 || repo.logs[0].Status != "failed" {
		t.Fatalf("expected one failed delivery log, got %+v", repo.logs)
	}
	// A provider failure fans out to both otp.failed and otp.dlq.
	if len(pub.topics) != 2 || pub.topics[0] != "otp.failed" || pub.topics[1] != "otp.dlq" {
		t.Fatalf("expected publishes to otp.failed then otp.dlq, got %v", pub.topics)
	}
}
