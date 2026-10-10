package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	notifhttp "github.com/duykhanh/worklane/services/notification-api/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/notification-api/internal/app"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// --- fakes ---

// fakeSvc records the last input of each use case and returns canned results; err is
// returned by every method.
type fakeSvc struct {
	err error

	sendRes   app.SendResult
	notifs    []domain.Notification
	detail    app.NotificationDetail
	template  domain.Template
	templates []domain.Template
	preview   app.Preview
	settings  []domain.Setting

	lastSend    app.SendInput
	lastCreate  app.CreateTemplateInput
	lastUpdate  app.UpdateTemplateInput
	lastSetPref app.SetPreferenceInput
	lastVars    map[string]string
	lastTenant  string
	lastID      string
	lastUserRef string
	calls       int
}

func (f *fakeSvc) Send(_ context.Context, in app.SendInput) (app.SendResult, error) {
	f.calls++
	f.lastSend = in
	return f.sendRes, f.err
}

func (f *fakeSvc) ListNotifications(_ context.Context, tenantID string) ([]domain.Notification, error) {
	f.calls++
	f.lastTenant = tenantID
	return f.notifs, f.err
}

func (f *fakeSvc) Notification(_ context.Context, tenantID, id string) (app.NotificationDetail, error) {
	f.calls++
	f.lastTenant, f.lastID = tenantID, id
	return f.detail, f.err
}

func (f *fakeSvc) CreateTemplate(_ context.Context, in app.CreateTemplateInput) (domain.Template, error) {
	f.calls++
	f.lastCreate = in
	return f.template, f.err
}

func (f *fakeSvc) UpdateTemplate(_ context.Context, in app.UpdateTemplateInput) (domain.Template, error) {
	f.calls++
	f.lastUpdate = in
	return f.template, f.err
}

func (f *fakeSvc) GetTemplate(_ context.Context, tenantID, id string) (domain.Template, error) {
	f.calls++
	f.lastTenant, f.lastID = tenantID, id
	return f.template, f.err
}

func (f *fakeSvc) ListTemplates(_ context.Context, tenantID string) ([]domain.Template, error) {
	f.calls++
	f.lastTenant = tenantID
	return f.templates, f.err
}

func (f *fakeSvc) PreviewTemplate(_ context.Context, tenantID, id string, vars map[string]string) (app.Preview, error) {
	f.calls++
	f.lastTenant, f.lastID, f.lastVars = tenantID, id, vars
	return f.preview, f.err
}

func (f *fakeSvc) Preferences(_ context.Context, tenantID, userRef string) ([]domain.Setting, error) {
	f.calls++
	f.lastTenant, f.lastUserRef = tenantID, userRef
	return f.settings, f.err
}

func (f *fakeSvc) SetPreference(_ context.Context, in app.SetPreferenceInput) error {
	f.calls++
	f.lastSetPref = in
	return f.err
}

// stubIntrospector resolves any opaque key to tenant t1, so the API-key auth branch passes
// in these handler tests (auth resolution itself is covered in middleware_test.go).
type stubIntrospector struct{}

func (stubIntrospector) Introspect(context.Context, string) (string, error) { return "t1", nil }

func newServer(svc notifhttp.NotificationService) http.Handler {
	// These tests authenticate with an opaque API key, which never reaches the JWT branch,
	// so a nil verifier is sufficient; the introspector stub resolves the key to a tenant.
	return notifhttp.NewRouter(svc, nil, stubIntrospector{})
}

const testKey = "testkey"

func do(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return out
}

var (
	created = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	updated = time.Date(2026, 9, 2, 11, 30, 0, 0, time.UTC)
)

func sampleTemplate() domain.Template {
	return domain.Template{
		ID: "tpl-1", TenantID: "t1", Name: "Welcome", Channel: "email", Locale: "en",
		Subject: "Hi {{name}}", Body: "Hello {{name}}", Version: 3, Status: "active",
		// A non-UTC zone proves the adapter normalizes timestamps on the way out.
		CreatedAt: created.In(time.FixedZone("ICT", 7*3600)), UpdatedAt: updated,
	}
}

// --- health / auth ---

func TestHealth_PublicAndOK(t *testing.T) {
	w := do(t, newServer(&fakeSvc{}), "GET", "/healthz", "", "")
	if w.Code != http.StatusOK || decode[map[string]string](t, w)["status"] != "ok" {
		t.Fatalf("healthz: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestV1_RequiresCredentials(t *testing.T) {
	routes := []struct{ method, path string }{
		{"GET", "/v1/templates"}, {"POST", "/v1/templates"}, {"GET", "/v1/templates/x"},
		{"PUT", "/v1/templates/x"}, {"POST", "/v1/templates/x/preview"},
		{"POST", "/v1/notifications"}, {"GET", "/v1/notifications"}, {"GET", "/v1/notifications/x"},
		{"GET", "/v1/preferences"}, {"PUT", "/v1/preferences"},
	}
	for _, r := range routes {
		svc := &fakeSvc{}
		w := do(t, newServer(svc), r.method, r.path, "", `{}`)
		if w.Code != http.StatusUnauthorized || svc.calls != 0 {
			t.Errorf("%s %s: want 401 and no use-case call, got %d (calls=%d)", r.method, r.path, w.Code, svc.calls)
		}
	}
}

// --- templates ---

func TestCreateTemplate_201(t *testing.T) {
	svc := &fakeSvc{template: sampleTemplate()}
	w := do(t, newServer(svc), "POST", "/v1/templates", testKey,
		`{"name":"Welcome","channel":"email","locale":"en","subject":"Hi {{name}}","body":"Hello {{name}}"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}
	want := app.CreateTemplateInput{
		TenantID: "t1", Name: "Welcome", Channel: "email", Locale: "en",
		Subject: "Hi {{name}}", Body: "Hello {{name}}",
	}
	if svc.lastCreate != want {
		t.Fatalf("unexpected use-case input: %+v", svc.lastCreate)
	}
	got := decode[map[string]any](t, w)
	wantBody := map[string]any{
		"id": "tpl-1", "name": "Welcome", "channel": "email", "locale": "en",
		"subject": "Hi {{name}}", "body": "Hello {{name}}", "version": float64(3), "status": "active",
		"created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-02T11:30:00Z",
	}
	if !reflect.DeepEqual(got, wantBody) {
		t.Fatalf("unexpected body:\n got %v\nwant %v", got, wantBody)
	}
}

func TestCreateTemplate_BadBody_400(t *testing.T) {
	for name, body := range map[string]string{
		"empty":        ``,
		"malformed":    `{"name":`,
		"wrong type":   `{"name":42}`,
		"over the cap": `{"body":"` + strings.Repeat("a", 300<<10) + `"}`,
	} {
		svc := &fakeSvc{}
		w := do(t, newServer(svc), "POST", "/v1/templates", testKey, body)
		if w.Code != http.StatusBadRequest || svc.calls != 0 {
			t.Errorf("%s: want 400 and no use-case call, got %d (calls=%d)", name, w.Code, svc.calls)
		}
	}
}

func TestListTemplates(t *testing.T) {
	svc := &fakeSvc{templates: []domain.Template{sampleTemplate()}}
	w := do(t, newServer(svc), "GET", "/v1/templates", testKey, "")
	if w.Code != http.StatusOK || svc.lastTenant != "t1" {
		t.Fatalf("code=%d tenant=%q", w.Code, svc.lastTenant)
	}
	got := decode[[]map[string]any](t, w)
	if len(got) != 1 || got[0]["id"] != "tpl-1" {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestListTemplates_EmptyIsArray(t *testing.T) {
	w := do(t, newServer(&fakeSvc{}), "GET", "/v1/templates", testKey, "")
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("want [], got %s", w.Body.String())
	}
}

func TestGetTemplate(t *testing.T) {
	svc := &fakeSvc{template: sampleTemplate()}
	w := do(t, newServer(svc), "GET", "/v1/templates/tpl-1", testKey, "")
	if w.Code != http.StatusOK || svc.lastTenant != "t1" || svc.lastID != "tpl-1" {
		t.Fatalf("code=%d tenant=%q id=%q", w.Code, svc.lastTenant, svc.lastID)
	}
	if decode[map[string]any](t, w)["name"] != "Welcome" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUpdateTemplate_IgnoresChannelInBody(t *testing.T) {
	svc := &fakeSvc{template: sampleTemplate()}
	w := do(t, newServer(svc), "PUT", "/v1/templates/tpl-1", testKey,
		`{"name":"Welcome 2","channel":"sms","locale":"vi","subject":"S","body":"B"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	want := app.UpdateTemplateInput{TenantID: "t1", ID: "tpl-1", Name: "Welcome 2", Locale: "vi", Subject: "S", Body: "B"}
	if svc.lastUpdate != want {
		t.Fatalf("unexpected use-case input: %+v", svc.lastUpdate)
	}
	if decode[map[string]any](t, w)["version"] != float64(3) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUpdateTemplate_BadBody_400(t *testing.T) {
	svc := &fakeSvc{}
	w := do(t, newServer(svc), "PUT", "/v1/templates/tpl-1", testKey, `not json`)
	if w.Code != http.StatusBadRequest || svc.calls != 0 {
		t.Fatalf("want 400 and no use-case call, got %d (calls=%d)", w.Code, svc.calls)
	}
}

func TestPreviewTemplate(t *testing.T) {
	svc := &fakeSvc{preview: app.Preview{Subject: "Hi An", Body: "Hello An", Missing: []string{"plan"}}}
	w := do(t, newServer(svc), "POST", "/v1/templates/tpl-1/preview", testKey, `{"variables":{"name":"An"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if svc.lastTenant != "t1" || svc.lastID != "tpl-1" || !reflect.DeepEqual(svc.lastVars, map[string]string{"name": "An"}) {
		t.Fatalf("unexpected use-case input: %q %q %v", svc.lastTenant, svc.lastID, svc.lastVars)
	}
	got := decode[map[string]any](t, w)
	want := map[string]any{"subject": "Hi An", "body": "Hello An", "missing": []any{"plan"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestPreviewTemplate_BodyOptional_MissingIsArray(t *testing.T) {
	for _, body := range []string{``, `{}`} {
		svc := &fakeSvc{preview: app.Preview{Body: "Hello"}}
		w := do(t, newServer(svc), "POST", "/v1/templates/tpl-1/preview", testKey, body)
		if w.Code != http.StatusOK || svc.calls != 1 {
			t.Fatalf("body %q: want 200, got %d: %s", body, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"missing":[]`) {
			t.Fatalf("body %q: want missing as [], got %s", body, w.Body.String())
		}
	}
}

func TestPreviewTemplate_NonStringVariable_400(t *testing.T) {
	svc := &fakeSvc{}
	w := do(t, newServer(svc), "POST", "/v1/templates/tpl-1/preview", testKey, `{"variables":{"n":1}}`)
	if w.Code != http.StatusBadRequest || svc.calls != 0 {
		t.Fatalf("want 400 and no use-case call, got %d (calls=%d)", w.Code, svc.calls)
	}
}

// --- send ---

const sendBody = `{"channel":"email","recipient":"an@example.com","template_id":"tpl-1",` +
	`"variables":{"name":"An"},"kind":"marketing","user_ref":"u-9","idempotency_key":"k-1"}`

func TestSend_Accepted_202(t *testing.T) {
	svc := &fakeSvc{sendRes: app.SendResult{NotificationID: "n-1", State: "queued"}}
	w := do(t, newServer(svc), "POST", "/v1/notifications", testKey, sendBody)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	want := app.SendInput{
		TenantID: "t1", Channel: "email", Recipient: "an@example.com", TemplateID: "tpl-1",
		Variables: map[string]string{"name": "An"}, Kind: "marketing", UserRef: "u-9", IdempotencyKey: "k-1",
	}
	if !reflect.DeepEqual(svc.lastSend, want) {
		t.Fatalf("unexpected use-case input: %+v", svc.lastSend)
	}
	got := decode[map[string]string](t, w)
	if !reflect.DeepEqual(got, map[string]string{"notification_id": "n-1", "state": "queued"}) {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestSend_Suppressed_202(t *testing.T) {
	svc := &fakeSvc{sendRes: app.SendResult{NotificationID: "n-1", State: "suppressed"}}
	w := do(t, newServer(svc), "POST", "/v1/notifications", testKey, sendBody)
	if w.Code != http.StatusAccepted || decode[map[string]string](t, w)["state"] != "suppressed" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSend_Duplicate_200(t *testing.T) {
	svc := &fakeSvc{sendRes: app.SendResult{NotificationID: "n-1", State: "sent", Duplicate: true}}
	w := do(t, newServer(svc), "POST", "/v1/notifications", testKey, sendBody)
	got := decode[map[string]string](t, w)
	if w.Code != http.StatusOK || got["notification_id"] != "n-1" || got["state"] != "sent" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSend_UnknownTemplate_404NamesTheTemplate(t *testing.T) {
	svc := &fakeSvc{err: domain.ErrNotFound}
	w := do(t, newServer(svc), "POST", "/v1/notifications", testKey, sendBody)
	if w.Code != http.StatusNotFound || decode[map[string]string](t, w)["error"] != "template not found" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSend_BadBody_400(t *testing.T) {
	for name, body := range map[string]string{
		"empty":               ``,
		"malformed":           `{`,
		"non-string variable": `{"variables":{"count":3}}`,
	} {
		svc := &fakeSvc{}
		w := do(t, newServer(svc), "POST", "/v1/notifications", testKey, body)
		if w.Code != http.StatusBadRequest || svc.calls != 0 {
			t.Errorf("%s: want 400 and no use-case call, got %d (calls=%d)", name, w.Code, svc.calls)
		}
	}
}

// --- reads ---

func sampleNotification() domain.Notification {
	return domain.Notification{
		ID: "n-1", TenantID: "t1", Channel: "email", RecipientMasked: "a***@example.com",
		TemplateID: "tpl-1", Kind: "transactional", State: "sent", Provider: "resend",
		ProviderMsgID: "msg-7", LatencyMS: 420, CreatedAt: created, UpdatedAt: updated,
	}
}

func TestListNotifications(t *testing.T) {
	svc := &fakeSvc{notifs: []domain.Notification{sampleNotification()}}
	w := do(t, newServer(svc), "GET", "/v1/notifications", testKey, "")
	if w.Code != http.StatusOK || svc.lastTenant != "t1" {
		t.Fatalf("code=%d tenant=%q", w.Code, svc.lastTenant)
	}
	got := decode[[]map[string]any](t, w)
	want := []map[string]any{{
		"id": "n-1", "channel": "email", "recipient": "a***@example.com", "template_id": "tpl-1",
		"kind": "transactional", "state": "sent", "provider": "resend", "provider_msg_id": "msg-7",
		"latency_ms": float64(420), "error": "",
		"created_at": "2026-09-01T10:00:00Z", "updated_at": "2026-09-02T11:30:00Z",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected body:\n got %v\nwant %v", got, want)
	}
}

func TestListNotifications_EmptyIsArray(t *testing.T) {
	w := do(t, newServer(&fakeSvc{}), "GET", "/v1/notifications", testKey, "")
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("want [], got %s", w.Body.String())
	}
}

func TestGetNotification_WithEvents(t *testing.T) {
	svc := &fakeSvc{detail: app.NotificationDetail{
		Notification: sampleNotification(),
		Events:       []domain.Event{{Type: "delivered", TS: updated, Meta: "ok"}},
	}}
	w := do(t, newServer(svc), "GET", "/v1/notifications/n-1", testKey, "")
	if w.Code != http.StatusOK || svc.lastTenant != "t1" || svc.lastID != "n-1" {
		t.Fatalf("code=%d tenant=%q id=%q", w.Code, svc.lastTenant, svc.lastID)
	}
	got := decode[map[string]any](t, w)
	wantEvents := []any{map[string]any{"type": "delivered", "ts": "2026-09-02T11:30:00Z", "meta": "ok"}}
	// The notification fields are flattened next to events, not nested.
	if got["id"] != "n-1" || got["recipient"] != "a***@example.com" || !reflect.DeepEqual(got["events"], wantEvents) {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestGetNotification_NoEventsIsArray(t *testing.T) {
	svc := &fakeSvc{detail: app.NotificationDetail{Notification: sampleNotification()}}
	w := do(t, newServer(svc), "GET", "/v1/notifications/n-1", testKey, "")
	if !strings.Contains(w.Body.String(), `"events":[]`) {
		t.Fatalf("want events as [], got %s", w.Body.String())
	}
}

// --- preferences ---

func TestGetPreferences(t *testing.T) {
	svc := &fakeSvc{settings: []domain.Setting{{Channel: "email", Enabled: true}, {Channel: "sms", Enabled: false}}}
	w := do(t, newServer(svc), "GET", "/v1/preferences?user_ref=u-9", testKey, "")
	if w.Code != http.StatusOK || svc.lastTenant != "t1" || svc.lastUserRef != "u-9" {
		t.Fatalf("code=%d tenant=%q user=%q", w.Code, svc.lastTenant, svc.lastUserRef)
	}
	got := decode[map[string]any](t, w)
	want := map[string]any{"user_ref": "u-9", "preferences": []any{
		map[string]any{"channel": "email", "enabled": true},
		map[string]any{"channel": "sms", "enabled": false},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestSetPreference_OptOut(t *testing.T) {
	svc := &fakeSvc{}
	w := do(t, newServer(svc), "PUT", "/v1/preferences", testKey, `{"user_ref":"u-9","channel":"sms","enabled":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if svc.lastSetPref != (app.SetPreferenceInput{TenantID: "t1", UserRef: "u-9", Channel: "sms", Enabled: false}) {
		t.Fatalf("unexpected use-case input: %+v", svc.lastSetPref)
	}
	want := map[string]any{"user_ref": "u-9", "channel": "sms", "enabled": false}
	if got := decode[map[string]any](t, w); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestSetPreference_OptIn(t *testing.T) {
	svc := &fakeSvc{}
	do(t, newServer(svc), "PUT", "/v1/preferences", testKey, `{"user_ref":"u-9","channel":"sms","enabled":true}`)
	if !svc.lastSetPref.Enabled {
		t.Fatalf("enabled=true was not passed through: %+v", svc.lastSetPref)
	}
}

// A missing "enabled" must be rejected, not read as false (which would opt the user out).
func TestSetPreference_MissingEnabled_400(t *testing.T) {
	svc := &fakeSvc{}
	w := do(t, newServer(svc), "PUT", "/v1/preferences", testKey, `{"user_ref":"u-9","channel":"sms"}`)
	if w.Code != http.StatusBadRequest || svc.calls != 0 {
		t.Fatalf("want 400 and no use-case call, got %d (calls=%d)", w.Code, svc.calls)
	}
}

// --- error mapping ---

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrConflict, http.StatusConflict},
		{domain.ErrRateLimited, http.StatusTooManyRequests},
		{domain.ErrInvalidChannel, http.StatusBadRequest},
		{domain.ErrInvalidKind, http.StatusBadRequest},
		{domain.ErrInvalidRecipient, http.StatusBadRequest},
		{domain.ErrInvalidUserRef, http.StatusBadRequest},
		{domain.ErrInvalidIdempotencyKey, http.StatusBadRequest},
		{fmt.Errorf("%w: name is required", domain.ErrInvalidTemplate), http.StatusBadRequest},
		{domain.ErrTemplateRequired, http.StatusBadRequest},
		{domain.ErrTemplateArchived, http.StatusBadRequest},
		{domain.ErrChannelMismatch, http.StatusBadRequest},
		{domain.ErrAlreadyExists, http.StatusInternalServerError},
		{errors.New("dial tcp 10.0.0.5:3306: connection refused"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		w := do(t, newServer(&fakeSvc{err: tc.err}), "PUT", "/v1/templates/tpl-1", testKey, `{}`)
		if w.Code != tc.want {
			t.Errorf("%v: want %d, got %d", tc.err, tc.want, w.Code)
		}
		msg := decode[map[string]string](t, w)["error"]
		if tc.want == http.StatusInternalServerError {
			if msg != "internal error" {
				t.Errorf("%v: internal detail leaked: %q", tc.err, msg)
			}
		} else if msg != tc.err.Error() {
			t.Errorf("%v: want the domain message, got %q", tc.err, msg)
		}
	}
}

// Every endpoint routes a use-case failure through the error mapping.
func TestEveryEndpoint_MapsUseCaseErrors(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"GET", "/v1/templates", ""},
		{"POST", "/v1/templates", `{}`},
		{"GET", "/v1/templates/x", ""},
		{"PUT", "/v1/templates/x", `{}`},
		{"POST", "/v1/templates/x/preview", `{}`},
		{"POST", "/v1/notifications", `{}`},
		{"GET", "/v1/notifications", ""},
		{"GET", "/v1/notifications/x", ""},
		{"GET", "/v1/preferences?user_ref=u", ""},
		{"PUT", "/v1/preferences", `{"enabled":true}`},
	}
	for _, r := range routes {
		w := do(t, newServer(&fakeSvc{err: domain.ErrRateLimited}), r.method, r.path, testKey, r.body)
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("%s %s: want 429, got %d: %s", r.method, r.path, w.Code, w.Body.String())
		}
	}
}
