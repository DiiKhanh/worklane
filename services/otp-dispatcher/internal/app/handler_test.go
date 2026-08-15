package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

type fakeMail struct {
	id  string
	err error
}

func (f *fakeMail) Send(context.Context, string, string, string) (string, error) {
	return f.id, f.err
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

type fakePub struct{ topics []string }

func (f *fakePub) Publish(_ context.Context, topic string, _ any) error {
	f.topics = append(f.topics, topic)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newHandler(mail EmailProvider) (*Handler, *fakeRepo, *fakePub) {
	repo := newFakeRepo()
	pub := &fakePub{}
	h := NewHandler(Deps{Mail: mail, Repo: repo, Pub: pub, Clock: fixedClock{t: time.Unix(0, 0)}}, Config{
		SentTopic: "otp.sent", FailedTopic: "otp.failed", DLQTopic: "otp.dlq", ProviderName: "smtp",
		Template: Template{Subject: "Code", BodyFmt: "Your code is %s"},
	})
	return h, repo, pub
}

func evt() contracts.RequestedEvent {
	return contracts.RequestedEvent{RequestID: "r1", TenantID: "t1", Recipient: "a@b.co", Channel: "email", Code: "123456"}
}

func TestHandle_Success(t *testing.T) {
	h, repo, pub := newHandler(&fakeMail{id: "msg-1"})
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

func TestHandle_MailFailure_LogsFailedAndDLQ(t *testing.T) {
	h, repo, pub := newHandler(&fakeMail{err: errors.New("provider down")})
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
