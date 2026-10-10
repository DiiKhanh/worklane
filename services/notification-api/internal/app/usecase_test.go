package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

var errBoom = errors.New("boom")

const (
	tenantA    = "tenant-a"
	tenantB    = "tenant-b"
	emailTopic = "notification.email.requested"
	smsTopic   = "notification.sms.requested"
)

var now = time.Date(2026, 10, 10, 15, 30, 0, 0, time.UTC)

type fakeTemplates struct {
	rows      map[string]domain.Template
	versions  []domain.TemplateVersion
	findErr   error
	insertErr error
	updateErr error
}

func (f *fakeTemplates) Insert(_ context.Context, t domain.Template) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.rows[t.ID] = t
	return nil
}
func (f *fakeTemplates) Find(_ context.Context, tenantID, id string) (domain.Template, error) {
	if f.findErr != nil {
		return domain.Template{}, f.findErr
	}
	t, ok := f.rows[id]
	if !ok || t.TenantID != tenantID {
		return domain.Template{}, domain.ErrNotFound
	}
	return t, nil
}
func (f *fakeTemplates) List(_ context.Context, tenantID string) ([]domain.Template, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	var out []domain.Template
	for _, t := range f.rows {
		if t.TenantID == tenantID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeTemplates) Update(_ context.Context, next domain.Template, prior domain.TemplateVersion) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.rows[next.ID].Version != prior.Version {
		return domain.ErrConflict
	}
	f.rows[next.ID] = next
	f.versions = append(f.versions, prior)
	return nil
}

type fakeLog struct {
	rows      map[string]domain.Notification
	events    map[string][]domain.Event
	insertErr error
	findErr   error
	eventsErr error
	markErr   error
	listLimit int
}

func (f *fakeLog) Insert(_ context.Context, n domain.Notification) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	if _, dup := f.rows[n.ID]; dup {
		return domain.ErrAlreadyExists
	}
	f.rows[n.ID] = n
	return nil
}
func (f *fakeLog) Find(_ context.Context, tenantID, id string) (domain.Notification, error) {
	if f.findErr != nil {
		return domain.Notification{}, f.findErr
	}
	n, ok := f.rows[id]
	if !ok || n.TenantID != tenantID {
		return domain.Notification{}, domain.ErrNotFound
	}
	return n, nil
}
func (f *fakeLog) List(_ context.Context, tenantID string, limit int) ([]domain.Notification, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	f.listLimit = limit
	var out []domain.Notification
	for _, n := range f.rows {
		if n.TenantID == tenantID {
			out = append(out, n)
		}
	}
	return out, nil
}
func (f *fakeLog) Events(_ context.Context, id string) ([]domain.Event, error) {
	return f.events[id], f.eventsErr
}
func (f *fakeLog) MarkFailed(_ context.Context, id, reason string, at time.Time) error {
	if f.markErr != nil {
		return f.markErr
	}
	n := f.rows[id]
	n.State, n.Error, n.UpdatedAt = domain.StateFailed, reason, at
	f.rows[id] = n
	return nil
}

type fakeSettings struct {
	rows      map[string][]domain.Setting // by tenant + "|" + user_ref
	listErr   error
	upsertErr error
	listN     int
}

func (f *fakeSettings) List(_ context.Context, tenantID, userRef string) ([]domain.Setting, error) {
	f.listN++
	return f.rows[tenantID+"|"+userRef], f.listErr
}
func (f *fakeSettings) Upsert(_ context.Context, tenantID, userRef string, s domain.Setting) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	key := tenantID + "|" + userRef
	kept := []domain.Setting{s}
	for _, row := range f.rows[key] {
		if row.Channel != s.Channel {
			kept = append(kept, row)
		}
	}
	f.rows[key] = kept
	return nil
}

type fakeCounter struct {
	counts map[string]int64
	ttls   map[string]time.Duration
	err    error
}

func (f *fakeCounter) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.counts[key]++
	f.ttls[key] = ttl
	return f.counts[key], nil
}

type published struct {
	topic string
	event any
}

type fakePub struct {
	sent []published
	err  error
}

func (f *fakePub) Publish(_ context.Context, topic string, event any) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, published{topic, event})
	return nil
}

type fakeShortener struct {
	calls []string // tenant + " " + url
	err   error
}

func (f *fakeShortener) Shorten(_ context.Context, tenantID, longURL string) (string, error) {
	f.calls = append(f.calls, tenantID+" "+longURL)
	if f.err != nil {
		return "", f.err
	}
	return "https://l.test/abc", nil
}

type seqIDs struct{ n int }

func (s *seqIDs) New() string { s.n++; return fmt.Sprintf("id-%d", s.n) }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return now }

type harness struct {
	svc       *Service
	templates *fakeTemplates
	log       *fakeLog
	settings  *fakeSettings
	counter   *fakeCounter
	pub       *fakePub
	shortener *fakeShortener
}

// newHarness seeds one active email template ("tpl-email") and one sms template
// ("tpl-sms") for tenant A, with limits high enough not to trip unless a test lowers them.
func newHarness(t *testing.T, edit ...func(*Config)) *harness {
	t.Helper()
	h := &harness{
		templates: &fakeTemplates{rows: map[string]domain.Template{
			"tpl-email": {
				ID: "tpl-email", TenantID: tenantA, Name: "Welcome", Channel: domain.ChannelEmail, Locale: "en",
				Subject: "Hi {{name}}", Body: `Hello {{name}}, open {{link "https://example.com/start"}}`,
				Version: 1, Status: domain.TemplateActive, CreatedAt: now, UpdatedAt: now,
			},
			"tpl-sms": {
				ID: "tpl-sms", TenantID: tenantA, Name: "Ping", Channel: domain.ChannelSMS, Locale: "en",
				Body: "Hi {{name}}", Version: 1, Status: domain.TemplateActive, CreatedAt: now, UpdatedAt: now,
			},
		}},
		log:       &fakeLog{rows: map[string]domain.Notification{}, events: map[string][]domain.Event{}},
		settings:  &fakeSettings{rows: map[string][]domain.Setting{}},
		counter:   &fakeCounter{counts: map[string]int64{}, ttls: map[string]time.Duration{}},
		pub:       &fakePub{},
		shortener: &fakeShortener{},
	}
	cfg := Config{
		EmailTopic: emailTopic, SMSTopic: smsTopic,
		TenantLimitMax: 100, TenantLimitWindow: time.Minute,
		UserLimitMax: 100, UserLimitWindow: time.Hour,
		ListLimit: 50,
	}
	for _, e := range edit {
		e(&cfg)
	}
	h.svc = New(Deps{
		Templates: h.templates, Log: h.log, Settings: h.settings, Counter: h.counter,
		Pub: h.pub, Shortener: h.shortener, IDs: &seqIDs{}, Clock: fixedClock{},
	}, cfg)
	return h
}

func emailSend() SendInput {
	return SendInput{
		TenantID: tenantA, Channel: domain.ChannelEmail, Recipient: "duy@gmail.com",
		TemplateID: "tpl-email", Variables: map[string]string{"name": "Duy"},
		Kind: domain.KindTransactional, UserRef: "user-1",
	}
}

func TestSend_QueuesAndPublishesToTheChannelTopic(t *testing.T) {
	h := newHarness(t)
	res, err := h.svc.Send(context.Background(), emailSend())
	if err != nil {
		t.Fatal(err)
	}
	if res != (SendResult{NotificationID: "id-1", State: domain.StateQueued}) {
		t.Fatalf("result = %+v", res)
	}

	row := h.log.rows["id-1"]
	want := domain.Notification{
		ID: "id-1", TenantID: tenantA, Channel: domain.ChannelEmail, RecipientMasked: "d***@gmail.com",
		TemplateID: "tpl-email", Kind: domain.KindTransactional, State: domain.StateQueued,
		CreatedAt: now, UpdatedAt: now,
	}
	if row != want {
		t.Fatalf("log row = %+v\nwant %+v", row, want)
	}

	if len(h.pub.sent) != 1 || h.pub.sent[0].topic != emailTopic {
		t.Fatalf("published = %+v", h.pub.sent)
	}
	evt := h.pub.sent[0].event.(contracts.RequestedEvent)
	wantEvt := contracts.RequestedEvent{
		NotificationID: "id-1", TenantID: tenantA, Channel: domain.ChannelEmail, Recipient: "duy@gmail.com",
		TemplateID: "tpl-email", Variables: map[string]string{"name": "Duy"},
		Kind: domain.KindTransactional, UserRef: "user-1",
	}
	if !reflect.DeepEqual(evt, wantEvt) {
		t.Fatalf("event = %+v\nwant %+v", evt, wantEvt)
	}
}

func TestSend_SMSGoesToTheSMSTopicWithEmptyVariablesMap(t *testing.T) {
	h := newHarness(t)
	in := SendInput{
		TenantID: tenantA, Channel: domain.ChannelSMS, Recipient: "+84901234567",
		TemplateID: "tpl-sms", Kind: domain.KindTransactional,
	}
	if _, err := h.svc.Send(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if h.pub.sent[0].topic != smsTopic {
		t.Fatalf("topic = %s", h.pub.sent[0].topic)
	}
	evt := h.pub.sent[0].event.(contracts.RequestedEvent)
	if evt.Variables == nil {
		t.Fatal("nil variables must be published as an empty map, not null")
	}
	if h.log.rows["id-1"].RecipientMasked != "+84***67" {
		t.Fatalf("masked = %q", h.log.rows["id-1"].RecipientMasked)
	}
}

func TestSend_Validation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*SendInput)
		err  error
	}{
		{"bad channel", func(in *SendInput) { in.Channel = "push" }, domain.ErrInvalidChannel},
		{"bad kind", func(in *SendInput) { in.Kind = "promo" }, domain.ErrInvalidKind},
		{"bad recipient", func(in *SendInput) { in.Recipient = "not-an-email" }, domain.ErrInvalidRecipient},
		{"long user_ref", func(in *SendInput) { in.UserRef = strings.Repeat("u", 256) }, domain.ErrInvalidUserRef},
		{"long idempotency key", func(in *SendInput) { in.IdempotencyKey = strings.Repeat("k", 256) }, domain.ErrInvalidIdempotencyKey},
		{"no template", func(in *SendInput) { in.TemplateID = "" }, domain.ErrTemplateRequired},
		{"unknown template", func(in *SendInput) { in.TemplateID = "nope" }, domain.ErrNotFound},
		{"other tenant's template", func(in *SendInput) { in.TenantID = tenantB }, domain.ErrNotFound},
		{"template of another channel", func(in *SendInput) { in.TemplateID = "tpl-sms" }, domain.ErrChannelMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			in := emailSend()
			tt.edit(&in)
			if _, err := h.svc.Send(context.Background(), in); !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if len(h.log.rows) != 0 || len(h.pub.sent) != 0 || len(h.counter.counts) != 0 {
				t.Fatal("a rejected send must not log, publish or consume quota")
			}
		})
	}
}

func TestSend_ArchivedTemplateIsRejected(t *testing.T) {
	h := newHarness(t)
	tpl := h.templates.rows["tpl-email"]
	tpl.Status = domain.TemplateArchived
	h.templates.rows["tpl-email"] = tpl
	if _, err := h.svc.Send(context.Background(), emailSend()); !errors.Is(err, domain.ErrTemplateArchived) {
		t.Fatalf("err = %v", err)
	}
}

func TestSend_MarketingToOptedOutUserIsSuppressed(t *testing.T) {
	h := newHarness(t)
	h.settings.rows[tenantA+"|user-1"] = []domain.Setting{{Channel: domain.ChannelEmail, Enabled: false}}
	in := emailSend()
	in.Kind = domain.KindMarketing

	res, err := h.svc.Send(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != domain.StateSuppressed || res.Duplicate {
		t.Fatalf("result = %+v", res)
	}
	if h.log.rows[res.NotificationID].State != domain.StateSuppressed {
		t.Fatal("a suppressed send must be logged as suppressed")
	}
	if len(h.pub.sent) != 0 {
		t.Fatal("a suppressed send must never be published")
	}
	if len(h.counter.counts) != 0 {
		t.Fatal("a suppressed send must not consume rate-limit quota")
	}
}

func TestSend_TransactionalBypassesOptOut(t *testing.T) {
	h := newHarness(t)
	h.settings.rows[tenantA+"|user-1"] = []domain.Setting{{Channel: domain.ChannelEmail, Enabled: false}}
	res, err := h.svc.Send(context.Background(), emailSend())
	if err != nil {
		t.Fatal(err)
	}
	if res.State != domain.StateQueued || len(h.pub.sent) != 1 {
		t.Fatalf("transactional must deliver despite opt-out: %+v", res)
	}
	if h.settings.listN != 0 {
		t.Fatal("transactional sends must not even read preferences")
	}
}

func TestSend_MarketingOptOutIsPerChannelUserAndTenant(t *testing.T) {
	tests := []struct {
		name string
		rows map[string][]domain.Setting
		edit func(*SendInput)
	}{
		{"other channel disabled", map[string][]domain.Setting{tenantA + "|user-1": {{Channel: domain.ChannelSMS, Enabled: false}}}, nil},
		{"explicitly enabled", map[string][]domain.Setting{tenantA + "|user-1": {{Channel: domain.ChannelEmail, Enabled: true}}}, nil},
		{"other user disabled", map[string][]domain.Setting{tenantA + "|user-2": {{Channel: domain.ChannelEmail, Enabled: false}}}, nil},
		{"other tenant disabled", map[string][]domain.Setting{tenantB + "|user-1": {{Channel: domain.ChannelEmail, Enabled: false}}}, nil},
		{"no user_ref", map[string][]domain.Setting{tenantA + "|": {{Channel: domain.ChannelEmail, Enabled: false}}}, func(in *SendInput) { in.UserRef = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.settings.rows = tt.rows
			in := emailSend()
			in.Kind = domain.KindMarketing
			if tt.edit != nil {
				tt.edit(&in)
			}
			res, err := h.svc.Send(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if res.State != domain.StateQueued || len(h.pub.sent) != 1 {
				t.Fatalf("expected a queued, published send, got %+v", res)
			}
		})
	}
}

func TestSend_PreferenceLookupFailureFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.settings.listErr = errBoom
	in := emailSend()
	in.Kind = domain.KindMarketing
	if _, err := h.svc.Send(context.Background(), in); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if len(h.pub.sent) != 0 || len(h.log.rows) != 0 {
		t.Fatal("marketing must not be sent when the opt-out state is unknown")
	}
}

func TestSend_TenantRateLimit(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.TenantLimitMax = 2 })
	for i := 0; i < 2; i++ {
		in := emailSend()
		in.UserRef = fmt.Sprintf("user-%d", i)
		if _, err := h.svc.Send(context.Background(), in); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	in := emailSend()
	in.UserRef = "user-9"
	if _, err := h.svc.Send(context.Background(), in); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("err = %v, want rate limited", err)
	}
	if len(h.pub.sent) != 2 || len(h.log.rows) != 2 {
		t.Fatal("a rate-limited send must not be logged or published")
	}
	if h.counter.ttls["notif:rl:tenant:"+tenantA] != time.Minute {
		t.Fatalf("tenant window ttl = %v", h.counter.ttls)
	}
}

func TestSend_UserRateLimitIsPerUserAndSkippedWithoutUserRef(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.UserLimitMax = 1 })
	if _, err := h.svc.Send(context.Background(), emailSend()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Send(context.Background(), emailSend()); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("second send to the same user: err = %v", err)
	}
	other := emailSend()
	other.UserRef = "user-2"
	if _, err := h.svc.Send(context.Background(), other); err != nil {
		t.Fatalf("another user must have their own window: %v", err)
	}
	anon := emailSend()
	anon.UserRef = ""
	for i := 0; i < 3; i++ {
		if _, err := h.svc.Send(context.Background(), anon); err != nil {
			t.Fatalf("send without user_ref must skip the user limit: %v", err)
		}
	}
	if h.counter.ttls["notif:rl:user:"+tenantA+":user-1"] != time.Hour {
		t.Fatalf("user window ttl = %v", h.counter.ttls)
	}
}

func TestSend_CounterFailureIsAnError(t *testing.T) {
	h := newHarness(t)
	h.counter.err = errBoom
	if _, err := h.svc.Send(context.Background(), emailSend()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if len(h.pub.sent) != 0 {
		t.Fatal("must not publish when the limiter is down")
	}
}

func TestSend_IdempotencyKeyDedups(t *testing.T) {
	h := newHarness(t)
	in := emailSend()
	in.IdempotencyKey = "order-42"

	first, err := h.svc.Send(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if first.NotificationID != domain.IDFromIdempotencyKey(tenantA, "order-42") || first.Duplicate {
		t.Fatalf("first = %+v", first)
	}

	// The dispatcher has delivered it in the meantime.
	row := h.log.rows[first.NotificationID]
	row.State = domain.StateSent
	h.log.rows[first.NotificationID] = row

	second, err := h.svc.Send(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want := SendResult{NotificationID: first.NotificationID, State: domain.StateSent, Duplicate: true}
	if second != want {
		t.Fatalf("second = %+v, want %+v", second, want)
	}
	if len(h.pub.sent) != 1 || len(h.log.rows) != 1 {
		t.Fatalf("a duplicate must not publish or log again: %d published, %d rows", len(h.pub.sent), len(h.log.rows))
	}
}

func TestSend_WithoutIdempotencyKeyEverySendIsNew(t *testing.T) {
	h := newHarness(t)
	a, _ := h.svc.Send(context.Background(), emailSend())
	b, err := h.svc.Send(context.Background(), emailSend())
	if err != nil {
		t.Fatal(err)
	}
	if a.NotificationID == b.NotificationID || len(h.pub.sent) != 2 {
		t.Fatalf("expected two distinct notifications, got %+v and %+v", a, b)
	}
}

func TestSend_SuppressedDuplicateReturnsTheFirstNotification(t *testing.T) {
	h := newHarness(t)
	in := emailSend()
	in.Kind = domain.KindMarketing
	in.IdempotencyKey = "promo-1"
	first, err := h.svc.Send(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// The user opts out, then the tenant retries the same request.
	h.settings.rows[tenantA+"|user-1"] = []domain.Setting{{Channel: domain.ChannelEmail, Enabled: false}}
	second, err := h.svc.Send(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if second.NotificationID != first.NotificationID || !second.Duplicate || second.State != domain.StateQueued {
		t.Fatalf("second = %+v", second)
	}
}

func TestSend_InsertFailure(t *testing.T) {
	h := newHarness(t)
	h.log.insertErr = errBoom
	if _, err := h.svc.Send(context.Background(), emailSend()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if len(h.pub.sent) != 0 {
		t.Fatal("must not publish a notification that was not logged")
	}
}

func TestSend_DuplicateLookupFailure(t *testing.T) {
	h := newHarness(t)
	in := emailSend()
	in.IdempotencyKey = "order-42"
	if _, err := h.svc.Send(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	h.log.findErr = errBoom
	if _, err := h.svc.Send(context.Background(), in); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestSend_PublishFailureMarksTheRowFailed(t *testing.T) {
	h := newHarness(t)
	h.pub.err = errBoom
	in := emailSend()
	in.IdempotencyKey = "order-42"
	if _, err := h.svc.Send(context.Background(), in); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	row := h.log.rows[domain.IDFromIdempotencyKey(tenantA, "order-42")]
	if row.State != domain.StateFailed || row.Error != "publish failed" {
		t.Fatalf("row = %+v", row)
	}

	// A retry with the same key must surface the failure, not look accepted.
	h.pub.err = nil
	res, err := h.svc.Send(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Duplicate || res.State != domain.StateFailed {
		t.Fatalf("retry = %+v", res)
	}
}

func TestSend_PublishFailureStillReturnsThePublishErrorWhenMarkFails(t *testing.T) {
	h := newHarness(t)
	h.pub.err = errBoom
	h.log.markErr = errors.New("db down")
	if _, err := h.svc.Send(context.Background(), emailSend()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestSend_TemplateLookupFailure(t *testing.T) {
	h := newHarness(t)
	h.templates.findErr = errBoom
	if _, err := h.svc.Send(context.Background(), emailSend()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateTemplate(t *testing.T) {
	h := newHarness(t)
	got, err := h.svc.CreateTemplate(context.Background(), CreateTemplateInput{
		TenantID: tenantA, Name: "Receipt", Channel: domain.ChannelEmail, Locale: "en",
		Subject: "Order {{order_id}}", Body: "Thanks {{name}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Template{
		ID: "id-1", TenantID: tenantA, Name: "Receipt", Channel: domain.ChannelEmail, Locale: "en",
		Subject: "Order {{order_id}}", Body: "Thanks {{name}}",
		Version: 1, Status: domain.TemplateActive, CreatedAt: now, UpdatedAt: now,
	}
	if got != want || h.templates.rows["id-1"] != want {
		t.Fatalf("template = %+v\nwant %+v", got, want)
	}
}

func TestCreateTemplate_Validation(t *testing.T) {
	base := CreateTemplateInput{
		TenantID: tenantA, Name: "Receipt", Channel: domain.ChannelEmail, Locale: "en", Subject: "Hi", Body: "Hello",
	}
	tests := []struct {
		name string
		edit func(*CreateTemplateInput)
		err  error
	}{
		{"bad channel", func(in *CreateTemplateInput) { in.Channel = "push" }, domain.ErrInvalidChannel},
		{"blank name", func(in *CreateTemplateInput) { in.Name = "" }, domain.ErrInvalidTemplate},
		{"email without subject", func(in *CreateTemplateInput) { in.Subject = "" }, domain.ErrInvalidTemplate},
		{"malformed token", func(in *CreateTemplateInput) { in.Body = "Hi {{ first name }}" }, domain.ErrInvalidTemplate},
		{"unsafe link", func(in *CreateTemplateInput) { in.Body = `{{link "javascript:alert(1)"}}` }, domain.ErrInvalidTemplate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			in := base
			tt.edit(&in)
			if _, err := h.svc.CreateTemplate(context.Background(), in); !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if len(h.templates.rows) != 2 {
				t.Fatal("an invalid template must not be stored")
			}
		})
	}
}

func TestCreateTemplate_InsertFailure(t *testing.T) {
	h := newHarness(t)
	h.templates.insertErr = errBoom
	_, err := h.svc.CreateTemplate(context.Background(), CreateTemplateInput{
		TenantID: tenantA, Name: "Receipt", Channel: domain.ChannelSMS, Locale: "en", Body: "Hello",
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateTemplate_BumpsVersionAndSnapshotsPriorContent(t *testing.T) {
	h := newHarness(t)
	before := h.templates.rows["tpl-email"]
	got, err := h.svc.UpdateTemplate(context.Background(), UpdateTemplateInput{
		TenantID: tenantA, ID: "tpl-email", Name: "Welcome v2", Locale: "vi", Subject: "Chao {{name}}", Body: "Xin chao",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := before
	want.Name, want.Locale, want.Subject, want.Body, want.Version = "Welcome v2", "vi", "Chao {{name}}", "Xin chao", 2
	if got != want || h.templates.rows["tpl-email"] != want {
		t.Fatalf("template = %+v\nwant %+v", got, want)
	}
	wantSnap := domain.TemplateVersion{
		ID: "id-1", TemplateID: "tpl-email", Version: 1, Subject: before.Subject, Body: before.Body, CreatedAt: now,
	}
	if len(h.templates.versions) != 1 || h.templates.versions[0] != wantSnap {
		t.Fatalf("snapshot = %+v\nwant %+v", h.templates.versions, wantSnap)
	}
}

func TestUpdateTemplate_ValidatesAgainstTheStoredChannel(t *testing.T) {
	h := newHarness(t)
	// tpl-sms is an sms template: a subject is not allowed, whatever the caller sends.
	_, err := h.svc.UpdateTemplate(context.Background(), UpdateTemplateInput{
		TenantID: tenantA, ID: "tpl-sms", Name: "Ping", Locale: "en", Subject: "Hi", Body: "Hello",
	})
	if !errors.Is(err, domain.ErrInvalidTemplate) {
		t.Fatalf("err = %v", err)
	}
	if h.templates.rows["tpl-sms"].Version != 1 || len(h.templates.versions) != 0 {
		t.Fatal("a rejected update must not change the template or its history")
	}
}

func TestUpdateTemplate_Errors(t *testing.T) {
	valid := UpdateTemplateInput{TenantID: tenantA, ID: "tpl-email", Name: "W", Locale: "en", Subject: "S", Body: "B"}
	tests := []struct {
		name  string
		setup func(*harness, *UpdateTemplateInput)
		err   error
	}{
		{"unknown id", func(_ *harness, in *UpdateTemplateInput) { in.ID = "nope" }, domain.ErrNotFound},
		{"other tenant", func(_ *harness, in *UpdateTemplateInput) { in.TenantID = tenantB }, domain.ErrNotFound},
		{"concurrent update", func(h *harness, _ *UpdateTemplateInput) { h.templates.updateErr = domain.ErrConflict }, domain.ErrConflict},
		{"store failure", func(h *harness, _ *UpdateTemplateInput) { h.templates.updateErr = errBoom }, errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			in := valid
			tt.setup(h, &in)
			if _, err := h.svc.UpdateTemplate(context.Background(), in); !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestGetAndListTemplatesAreTenantScoped(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.GetTemplate(context.Background(), tenantB, "tpl-email"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant's template: err = %v", err)
	}
	if got, err := h.svc.GetTemplate(context.Background(), tenantA, "tpl-email"); err != nil || got.Name != "Welcome" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if list, err := h.svc.ListTemplates(context.Background(), tenantA); err != nil || len(list) != 2 {
		t.Fatalf("list A = %d, %v", len(list), err)
	}
	if list, err := h.svc.ListTemplates(context.Background(), tenantB); err != nil || len(list) != 0 {
		t.Fatalf("list B = %d, %v", len(list), err)
	}
	h.templates.findErr = errBoom
	if _, err := h.svc.GetTemplate(context.Background(), tenantA, "tpl-email"); !errors.Is(err, errBoom) {
		t.Fatalf("get failure: err = %v", err)
	}
	if _, err := h.svc.ListTemplates(context.Background(), tenantA); !errors.Is(err, errBoom) {
		t.Fatalf("list failure: err = %v", err)
	}
}

func TestPreviewTemplate_RendersVariablesAndShortensLinksForTheTenant(t *testing.T) {
	h := newHarness(t)
	got, err := h.svc.PreviewTemplate(context.Background(), tenantA, "tpl-email", map[string]string{"name": "Duy"})
	if err != nil {
		t.Fatal(err)
	}
	want := Preview{Subject: "Hi Duy", Body: "Hello Duy, open https://l.test/abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preview = %+v\nwant %+v", got, want)
	}
	if !reflect.DeepEqual(h.shortener.calls, []string{tenantA + " https://example.com/start"}) {
		t.Fatalf("shortener calls = %v", h.shortener.calls)
	}
}

func TestPreviewTemplate_ReportsMissingVariables(t *testing.T) {
	h := newHarness(t)
	got, err := h.svc.PreviewTemplate(context.Background(), tenantA, "tpl-sms", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "Hi " || !reflect.DeepEqual(got.Missing, []string{"name"}) {
		t.Fatalf("preview = %+v", got)
	}
	if len(h.shortener.calls) != 0 {
		t.Fatal("a template without links must not call the shortener")
	}
}

func TestPreviewTemplate_Errors(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.PreviewTemplate(context.Background(), tenantB, "tpl-email", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant: err = %v", err)
	}
	h.shortener.err = errBoom
	if _, err := h.svc.PreviewTemplate(context.Background(), tenantA, "tpl-email", nil); !errors.Is(err, errBoom) {
		t.Fatalf("shortener failure: err = %v", err)
	}
}

func TestPreferences_DefaultToEnabledForEveryChannel(t *testing.T) {
	h := newHarness(t)
	h.settings.rows[tenantA+"|user-1"] = []domain.Setting{{Channel: domain.ChannelSMS, Enabled: false}}
	got, err := h.svc.Preferences(context.Background(), tenantA, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Setting{{Channel: domain.ChannelEmail, Enabled: true}, {Channel: domain.ChannelSMS, Enabled: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferences = %+v, want %+v", got, want)
	}
}

func TestSetPreference_ThenMarketingIsSuppressed(t *testing.T) {
	h := newHarness(t)
	err := h.svc.SetPreference(context.Background(), SetPreferenceInput{
		TenantID: tenantA, UserRef: "user-1", Channel: domain.ChannelEmail, Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	in := emailSend()
	in.Kind = domain.KindMarketing
	res, err := h.svc.Send(context.Background(), in)
	if err != nil || res.State != domain.StateSuppressed {
		t.Fatalf("send after opt-out = %+v, %v", res, err)
	}

	// Opting back in restores delivery.
	if err := h.svc.SetPreference(context.Background(), SetPreferenceInput{
		TenantID: tenantA, UserRef: "user-1", Channel: domain.ChannelEmail, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	res, err = h.svc.Send(context.Background(), in)
	if err != nil || res.State != domain.StateQueued {
		t.Fatalf("send after opt-in = %+v, %v", res, err)
	}
}

func TestPreferences_Errors(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.Preferences(ctx, tenantA, ""); !errors.Is(err, domain.ErrInvalidUserRef) {
		t.Fatalf("empty user_ref: err = %v", err)
	}
	if err := h.svc.SetPreference(ctx, SetPreferenceInput{TenantID: tenantA, Channel: domain.ChannelEmail}); !errors.Is(err, domain.ErrInvalidUserRef) {
		t.Fatalf("set without user_ref: err = %v", err)
	}
	if err := h.svc.SetPreference(ctx, SetPreferenceInput{TenantID: tenantA, UserRef: "u", Channel: "push"}); !errors.Is(err, domain.ErrInvalidChannel) {
		t.Fatalf("set bad channel: err = %v", err)
	}
	h.settings.listErr, h.settings.upsertErr = errBoom, errBoom
	if _, err := h.svc.Preferences(ctx, tenantA, "u"); !errors.Is(err, errBoom) {
		t.Fatalf("list failure: err = %v", err)
	}
	if err := h.svc.SetPreference(ctx, SetPreferenceInput{TenantID: tenantA, UserRef: "u", Channel: domain.ChannelEmail}); !errors.Is(err, errBoom) {
		t.Fatalf("upsert failure: err = %v", err)
	}
}

func TestListNotifications_IsTenantScopedAndCapped(t *testing.T) {
	h := newHarness(t)
	h.log.rows["n-1"] = domain.Notification{ID: "n-1", TenantID: tenantA}
	h.log.rows["n-2"] = domain.Notification{ID: "n-2", TenantID: tenantB}
	got, err := h.svc.ListNotifications(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "n-1" || h.log.listLimit != 50 {
		t.Fatalf("list = %+v (limit %d)", got, h.log.listLimit)
	}
	h.log.findErr = errBoom
	if _, err := h.svc.ListNotifications(context.Background(), tenantA); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestNotification_ReturnsEventsAndHidesOtherTenants(t *testing.T) {
	h := newHarness(t)
	h.log.rows["n-1"] = domain.Notification{ID: "n-1", TenantID: tenantA, State: domain.StateSent}
	h.log.events["n-1"] = []domain.Event{{Type: "clicked", TS: now, Meta: "abc"}}
	ctx := context.Background()

	got, err := h.svc.Notification(ctx, tenantA, "n-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Notification.State != domain.StateSent || len(got.Events) != 1 || got.Events[0].Type != "clicked" {
		t.Fatalf("detail = %+v", got)
	}
	if _, err := h.svc.Notification(ctx, tenantB, "n-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant: err = %v", err)
	}
	h.log.eventsErr = errBoom
	if _, err := h.svc.Notification(ctx, tenantA, "n-1"); !errors.Is(err, errBoom) {
		t.Fatalf("events failure: err = %v", err)
	}
	h.log.findErr = errBoom
	if _, err := h.svc.Notification(ctx, tenantA, "n-1"); !errors.Is(err, errBoom) {
		t.Fatalf("find failure: err = %v", err)
	}
}

// The domain keeps its own copy of the wire vocabulary so it stays stdlib-only; this
// pins the two copies together.
func TestDomainVocabularyMatchesTheContract(t *testing.T) {
	pairs := [][2]string{
		{domain.ChannelEmail, contracts.ChannelEmail}, {domain.ChannelSMS, contracts.ChannelSMS},
		{domain.KindTransactional, contracts.KindTransactional}, {domain.KindMarketing, contracts.KindMarketing},
		{domain.StateQueued, contracts.StateQueued}, {domain.StateSent, contracts.StateSent},
		{domain.StateFailed, contracts.StateFailed}, {domain.StateSuppressed, contracts.StateSuppressed},
	}
	for _, p := range pairs {
		if p[0] != p[1] {
			t.Errorf("domain %q != contract %q", p[0], p[1])
		}
	}
}
