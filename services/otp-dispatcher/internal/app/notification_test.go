package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
)

// seqSender fails with errs[i] on attempt i and succeeds once errs is exhausted.
type seqSender struct {
	errs    []error
	calls   int
	to      string
	subject string
	body    string
}

func (s *seqSender) Name() string { return "fake" }
func (s *seqSender) Send(_ context.Context, to, subject, body string) (string, error) {
	i := s.calls
	s.calls++
	s.to, s.subject, s.body = to, subject, body
	if i < len(s.errs) {
		return "", s.errs[i]
	}
	return "msg-1", nil
}

type fakeNotifTemplates struct {
	tpl   Template
	found bool
	err   error
}

func (f *fakeNotifTemplates) Find(context.Context, string, string) (Template, bool, error) {
	return f.tpl, f.found, f.err
}

type fakeNotifLog struct {
	state     string
	found     bool
	stateErr  error
	markErr   error
	sent      []NotificationOutcome
	failed    []NotificationOutcome
	gotTenant string
}

func (f *fakeNotifLog) State(_ context.Context, tenantID, _ string) (string, bool, error) {
	f.gotTenant = tenantID
	return f.state, f.found, f.stateErr
}
func (f *fakeNotifLog) MarkSent(_ context.Context, _ string, o NotificationOutcome) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.sent = append(f.sent, o)
	return nil
}
func (f *fakeNotifLog) MarkFailed(_ context.Context, _ string, o NotificationOutcome) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.failed = append(f.failed, o)
	return nil
}

type errPub struct{ err error }

func (p errPub) Publish(context.Context, string, any) error { return p.err }

type notifFixture struct {
	sender *seqSender
	tpls   *fakeNotifTemplates
	log    *fakeNotifLog
	pub    *fakePub
	slept  []time.Duration
	deps   NotificationDeps
}

func newNotifFixture() *notifFixture {
	f := &notifFixture{
		sender: &seqSender{},
		tpls:   &fakeNotifTemplates{tpl: Template{Subject: "Hi {{name}}", Body: "Order {{id}} shipped"}, found: true},
		log:    &fakeNotifLog{state: contracts.StateQueued, found: true},
		pub:    &fakePub{},
	}
	f.deps = NotificationDeps{
		Senders:   map[string]Sender{"email": f.sender},
		Templates: f.tpls, Log: f.log, Pub: f.pub,
		Clock: fixedClock{t: time.Unix(100, 0)},
		Sleep: func(_ context.Context, d time.Duration) error {
			f.slept = append(f.slept, d)
			return nil
		},
	}
	return f
}

func (f *notifFixture) handler() *NotificationHandler {
	return NewNotificationHandler(f.deps, NotificationConfig{
		SentTopic: "n.sent", FailedTopic: "n.failed", DLQTopic: "n.dlq",
		MaxAttempts: 3, BaseBackoff: time.Second,
	})
}

func notifEvent() contracts.RequestedEvent {
	return contracts.RequestedEvent{
		NotificationID: "n-1", TenantID: "t-1", Channel: "email", Recipient: "a@b.co",
		TemplateID: "tpl-1", Variables: map[string]string{"name": "An", "id": "42"}, Kind: "transactional",
	}
}

func TestNotification_RendersDeliversAndRecordsSent(t *testing.T) {
	f := newNotifFixture()
	if err := f.handler().Handle(context.Background(), notifEvent()); err != nil {
		t.Fatal(err)
	}
	if f.sender.to != "a@b.co" || f.sender.subject != "Hi An" || f.sender.body != "Order 42 shipped" {
		t.Fatalf("delivered %q / %q / %q", f.sender.to, f.sender.subject, f.sender.body)
	}
	if f.log.gotTenant != "t-1" {
		t.Fatalf("state lookup not tenant scoped: %q", f.log.gotTenant)
	}
	if len(f.log.sent) != 1 || f.log.sent[0].Provider != "fake" || f.log.sent[0].ProviderMsgID != "msg-1" {
		t.Fatalf("sent outcome: %+v", f.log.sent)
	}
	want := contracts.SentEvent{NotificationID: "n-1", TenantID: "t-1", Channel: "email", Provider: "fake"}
	if !reflect.DeepEqual(f.pub.topics, []string{"n.sent"}) || f.pub.events[0] != want {
		t.Fatalf("published %v %+v", f.pub.topics, f.pub.events)
	}
}

func TestNotification_SkipsRowsThatAreNotQueued(t *testing.T) {
	for _, state := range []string{contracts.StateSent, contracts.StateFailed, contracts.StateSuppressed} {
		f := newNotifFixture()
		f.log.state = state
		if err := f.handler().Handle(context.Background(), notifEvent()); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if f.sender.calls != 0 || len(f.pub.topics) != 0 || len(f.log.sent)+len(f.log.failed) != 0 {
			t.Fatalf("%s: row must be left alone", state)
		}
	}
}

func TestNotification_InvalidEvents(t *testing.T) {
	cases := map[string]func(*notifFixture, *contracts.RequestedEvent){
		"no id":      func(_ *notifFixture, e *contracts.RequestedEvent) { e.NotificationID = "" },
		"no tenant":  func(_ *notifFixture, e *contracts.RequestedEvent) { e.TenantID = "" },
		"no log row": func(f *notifFixture, _ *contracts.RequestedEvent) { f.log.found = false },
	}
	for name, mutate := range cases {
		f := newNotifFixture()
		evt := notifEvent()
		mutate(f, &evt)
		if err := f.handler().Handle(context.Background(), evt); !errors.Is(err, ErrInvalidNotification) {
			t.Fatalf("%s: got %v", name, err)
		}
		if f.sender.calls != 0 {
			t.Fatalf("%s: must not deliver", name)
		}
	}
}

func TestNotification_TerminalFailuresBeforeDelivery(t *testing.T) {
	cases := map[string]struct {
		mutate   func(*notifFixture, *contracts.RequestedEvent)
		provider string
		reason   string
	}{
		"unknown channel": {
			func(_ *notifFixture, e *contracts.RequestedEvent) { e.Channel = "push" },
			"unknown", "unsupported channel: push",
		},
		"template gone": {
			func(f *notifFixture, _ *contracts.RequestedEvent) { f.tpls.found = false },
			"fake", "template not found",
		},
		"link without shortener": {
			func(f *notifFixture, _ *contracts.RequestedEvent) { f.tpls.tpl.Body = `See {{link "https://x.co/a"}}` },
			"fake", "render: ",
		},
	}
	for name, tc := range cases {
		f := newNotifFixture()
		evt := notifEvent()
		tc.mutate(f, &evt)
		if err := f.handler().Handle(context.Background(), evt); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if f.sender.calls != 0 {
			t.Fatalf("%s: must not deliver", name)
		}
		if len(f.log.failed) != 1 || f.log.failed[0].Provider != tc.provider || !strings.HasPrefix(f.log.failed[0].Error, tc.reason) {
			t.Fatalf("%s: failed outcome %+v", name, f.log.failed)
		}
		if !reflect.DeepEqual(f.pub.topics, []string{"n.failed", "n.dlq"}) {
			t.Fatalf("%s: topics %v", name, f.pub.topics)
		}
	}
}

func TestNotification_ShortensLinksForTheTenant(t *testing.T) {
	f := newNotifFixture()
	f.tpls.tpl.Body = `See {{link "https://x.co/a"}}`
	f.deps.Shorten = func(_ context.Context, tenantID, longURL string) (string, error) {
		return "https://s/" + tenantID + "?" + longURL, nil
	}
	if err := f.handler().Handle(context.Background(), notifEvent()); err != nil {
		t.Fatal(err)
	}
	if f.sender.body != "See https://s/t-1?https://x.co/a" {
		t.Fatalf("body %q", f.sender.body)
	}
}

func TestNotification_RetriesTransientErrorsWithBackoff(t *testing.T) {
	f := newNotifFixture()
	f.sender.errs = []error{errors.New("timeout"), errors.New("timeout")}
	if err := f.handler().Handle(context.Background(), notifEvent()); err != nil {
		t.Fatal(err)
	}
	if f.sender.calls != 3 || !reflect.DeepEqual(f.slept, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatalf("calls %d slept %v", f.sender.calls, f.slept)
	}
	if len(f.log.sent) != 1 || len(f.log.failed) != 0 {
		t.Fatalf("outcomes sent=%v failed=%v", f.log.sent, f.log.failed)
	}
}

func TestNotification_ExhaustedRetriesFailAndRouteToDLQ(t *testing.T) {
	f := newNotifFixture()
	f.sender.errs = []error{errors.New("e1"), errors.New("e2"), errors.New(strings.Repeat("x", 2000))}
	if err := f.handler().Handle(context.Background(), notifEvent()); err != nil {
		t.Fatal(err)
	}
	if f.sender.calls != 3 || len(f.slept) != 2 {
		t.Fatalf("calls %d slept %v", f.sender.calls, f.slept)
	}
	if len(f.log.failed) != 1 || len(f.log.failed[0].Error) != maxErrorRunes {
		t.Fatalf("failed outcome: %+v", f.log.failed)
	}
	if !reflect.DeepEqual(f.pub.topics, []string{"n.failed", "n.dlq"}) {
		t.Fatalf("topics %v", f.pub.topics)
	}
	evt, ok := f.pub.events[1].(contracts.FailedEvent)
	if !ok || evt.NotificationID != "n-1" || evt.Provider != "fake" {
		t.Fatalf("dlq event %+v", f.pub.events[1])
	}
}

func TestNotification_PermanentErrorIsNotRetried(t *testing.T) {
	f := newNotifFixture()
	f.sender.errs = []error{Permanent(errors.New("bad recipient"))}
	if err := f.handler().Handle(context.Background(), notifEvent()); err != nil {
		t.Fatal(err)
	}
	if f.sender.calls != 1 || len(f.slept) != 0 {
		t.Fatalf("calls %d slept %v", f.sender.calls, f.slept)
	}
	if len(f.log.failed) != 1 || f.log.failed[0].Error != "bad recipient" {
		t.Fatalf("failed outcome: %+v", f.log.failed)
	}
}

func TestNotification_ShutdownDuringBackoffLeavesRowQueued(t *testing.T) {
	f := newNotifFixture()
	f.sender.errs = []error{errors.New("timeout")}
	ctx, cancel := context.WithCancel(context.Background())
	f.deps.Sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	if err := f.handler().Handle(ctx, notifEvent()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if len(f.log.failed)+len(f.log.sent) != 0 || len(f.pub.topics) != 0 {
		t.Fatal("an interrupted delivery must not be recorded")
	}
}

func TestNotification_InfrastructureErrorsAreReturned(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]func(*notifFixture){
		"state lookup":    func(f *notifFixture) { f.log.stateErr = boom },
		"template lookup": func(f *notifFixture) { f.tpls.err = boom },
		"mark sent":       func(f *notifFixture) { f.log.markErr = boom },
		"mark failed": func(f *notifFixture) {
			f.log.markErr = boom
			f.sender.errs = []error{Permanent(errors.New("rejected"))}
		},
		"publish sent": func(f *notifFixture) { f.deps.Pub = errPub{err: boom} },
		"publish failed": func(f *notifFixture) {
			f.deps.Pub = errPub{err: boom}
			f.sender.errs = []error{Permanent(errors.New("rejected"))}
		},
	}
	for name, mutate := range cases {
		f := newNotifFixture()
		mutate(f)
		if err := f.handler().Handle(context.Background(), notifEvent()); !errors.Is(err, boom) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestNotification_MaxAttemptsFloorsAtOne(t *testing.T) {
	f := newNotifFixture()
	f.sender.errs = []error{errors.New("e1")}
	h := NewNotificationHandler(f.deps, NotificationConfig{FailedTopic: "n.failed", DLQTopic: "n.dlq"})
	if err := h.Handle(context.Background(), notifEvent()); err != nil {
		t.Fatal(err)
	}
	if f.sender.calls != 1 || len(f.log.failed) != 1 {
		t.Fatalf("calls %d failed %v", f.sender.calls, f.log.failed)
	}
}

func TestPermanentErrorKeepsMessageAndUnwraps(t *testing.T) {
	inner := errors.New("resend: status 422: nope")
	err := Permanent(inner)
	if err.Error() != inner.Error() || !errors.Is(err, inner) {
		t.Fatalf("got %q", err)
	}
}
