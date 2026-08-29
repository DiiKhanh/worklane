package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

type fakeStore struct{ m map[string]CodeRecord }

func newFakeStore() *fakeStore { return &fakeStore{m: map[string]CodeRecord{}} }
func (f *fakeStore) Save(_ context.Context, k string, r CodeRecord, _ time.Duration) error {
	f.m[k] = r
	return nil
}
func (f *fakeStore) Get(_ context.Context, k string) (CodeRecord, error) {
	r, ok := f.m[k]
	if !ok {
		return CodeRecord{}, domain.ErrNotFound
	}
	return r, nil
}
func (f *fakeStore) Delete(_ context.Context, k string) error { delete(f.m, k); return nil }

type fakeRepo struct {
	states     map[string]string
	lastInsert Request
}

func newFakeRepo() *fakeRepo { return &fakeRepo{states: map[string]string{}} }
func (f *fakeRepo) InsertRequest(_ context.Context, r Request) error {
	f.states[r.ID] = r.State
	f.lastInsert = r
	return nil
}
func (f *fakeRepo) UpdateState(_ context.Context, id, to string) error {
	f.states[id] = to
	return nil
}
func (f *fakeRepo) ListRequests(context.Context, string, int) ([]Request, error) { return nil, nil }
func (f *fakeRepo) ListDeliveryLogs(context.Context, string, int) ([]DeliveryLog, error) {
	return nil, nil
}
func (f *fakeRepo) Stats(context.Context, string, time.Time) (Stats, error) { return Stats{}, nil }

type fakePub struct {
	events []string
	last   any
}

func (f *fakePub) Publish(_ context.Context, topic string, event any) error {
	f.events = append(f.events, topic)
	f.last = event
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newSvc() (*Service, *fakeRepo, *fakePub) {
	repo := newFakeRepo()
	pub := &fakePub{}
	svc := New(Deps{
		Store: newFakeStore(), Counter: newFakeCounter(), Repo: repo,
		Pub: pub, Clock: fixedClock{t: time.Unix(1000, 0)},
	}, Config{
		CodeLength: 6, TTL: 5 * time.Minute, MaxVerifyAttempts: 3, RequestedTopic: "otp.requested",
		Limits: LimitConfig{PerRecipientMax: 5, PerRecipientWindow: time.Hour,
			PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: 0},
	})
	return svc, repo, pub
}

func TestSend_PublishesLocaleDefaultingToEn(t *testing.T) {
	svc, _, pub := newSvc()
	if _, err := svc.Send(context.Background(), SendInput{TenantID: "t1", Recipient: "a@b.co"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	evt, ok := pub.last.(contracts.RequestedEvent)
	if !ok {
		t.Fatalf("expected RequestedEvent, got %T", pub.last)
	}
	if evt.Locale != "en" {
		t.Fatalf("empty locale must default to en, got %q", evt.Locale)
	}
}

func TestSend_PublishesGivenLocale(t *testing.T) {
	svc, _, pub := newSvc()
	if _, err := svc.Send(context.Background(), SendInput{TenantID: "t1", Recipient: "a@b.co", Locale: "vi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if evt := pub.last.(contracts.RequestedEvent); evt.Locale != "vi" {
		t.Fatalf("locale should pass through, got %q", evt.Locale)
	}
}

func TestSend_ThenVerify_Success(t *testing.T) {
	svc, repo, pub := newSvc()
	ctx := context.Background()
	res, err := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(pub.events) != 1 || pub.events[0] != "otp.requested" {
		t.Fatalf("expected otp.requested published, got %v", pub.events)
	}
	if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: res.Code}); err != nil {
		t.Fatalf("verify correct code: %v", err)
	}
	if repo.states[res.RequestID] != string(domain.StateVerified) {
		t.Fatalf("request should be verified, got %q", repo.states[res.RequestID])
	}
}

func TestSend_SMSChannel_PublishesSMSEvent(t *testing.T) {
	svc, repo, pub := newSvc()
	res, err := svc.Send(context.Background(), SendInput{
		TenantID: "t1", Recipient: "+84901234567", Channel: "sms",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.RequestID == "" {
		t.Fatal("expected a request id")
	}
	evt := pub.last.(contracts.RequestedEvent)
	if evt.Channel != "sms" || evt.Recipient != "+84901234567" {
		t.Fatalf("published event = %+v, want sms channel", evt)
	}
	if got := repo.lastInsert.Channel; got != "sms" {
		t.Fatalf("audit row channel = %q, want sms", got)
	}
}

func TestSend_PhoneUnderEmailChannel_Rejected(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Send(context.Background(), SendInput{
		TenantID: "t1", Recipient: "+84901234567", Channel: "email",
	})
	if !errors.Is(err, domain.ErrInvalidRecipient) {
		t.Fatalf("want ErrInvalidRecipient, got %v", err)
	}
}

func TestSend_OmittedChannel_DefaultsToEmail(t *testing.T) {
	svc, _, pub := newSvc()
	if _, err := svc.Send(context.Background(), SendInput{
		TenantID: "t1", Recipient: "user@example.com", // no Channel
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	evt := pub.last.(contracts.RequestedEvent)
	if evt.Channel != "email" {
		t.Fatalf("default channel should be email, got %q", evt.Channel)
	}
}

func TestVerify_WrongCode_LocksAfterMax(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	_, _ = svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	for i := 0; i < 3; i++ {
		if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}); err != domain.ErrCodeMismatch {
			t.Fatalf("attempt %d want ErrCodeMismatch, got %v", i, err)
		}
	}
	if err := svc.Verify(ctx, VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}); err != domain.ErrTooManyAttempts {
		t.Fatalf("want ErrTooManyAttempts after max, got %v", err)
	}
}

func TestVerify_NoActiveCode(t *testing.T) {
	svc, _, _ := newSvc()
	if err := svc.Verify(context.Background(), VerifyInput{TenantID: "t1", Recipient: "x@y.co", Code: "111111"}); err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound for no active code, got %v", err)
	}
}

func TestSend_RateLimited(t *testing.T) {
	repo := newFakeRepo()
	svc := New(Deps{
		Store: newFakeStore(), Counter: newFakeCounter(), Repo: repo, Pub: &fakePub{},
		Clock: fixedClock{t: time.Unix(1000, 0)},
	}, Config{
		CodeLength: 6, TTL: time.Minute, MaxVerifyAttempts: 3, RequestedTopic: "otp.requested",
		Limits: LimitConfig{PerRecipientMax: 1, PerRecipientWindow: time.Hour,
			PerTenantMax: 100, PerTenantWindow: time.Hour, ResendCooldown: 0},
	})
	ctx := context.Background()
	if _, err := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"}); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"}); err != domain.ErrRateLimited {
		t.Fatalf("second send want ErrRateLimited, got %v", err)
	}
}

func TestSend_Idempotency_Collapses(t *testing.T) {
	svc, _, pub := newSvc()
	ctx := context.Background()
	in := SendInput{TenantID: "t1", Recipient: "a@b.co", IdempotencyKey: "abc"}
	r1, err := svc.Send(ctx, in)
	if err != nil {
		t.Fatalf("send 1: %v", err)
	}
	r2, err := svc.Send(ctx, in)
	if err != nil {
		t.Fatalf("send 2: %v", err)
	}
	if r1.RequestID != r2.RequestID {
		t.Fatalf("idempotent send must return same request id (%q vs %q)", r1.RequestID, r2.RequestID)
	}
	if len(pub.events) != 1 {
		t.Fatalf("idempotent send must publish once, got %d", len(pub.events))
	}
}
