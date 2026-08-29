package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	otphttp "github.com/duykhanh/worklane/services/otp-api/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// --- fakes ---

type fakeSvc struct {
	sendErr   error
	verifyErr error
	sendRes   app.SendResult
	lastInput app.SendInput
}

func (f *fakeSvc) Send(_ context.Context, in app.SendInput) (app.SendResult, error) {
	f.lastInput = in
	return f.sendRes, f.sendErr
}
func (f *fakeSvc) Verify(context.Context, app.VerifyInput) error { return f.verifyErr }

type fakeRepo struct {
	stats           app.Stats
	lastStatsTenant string
}

func (f *fakeRepo) InsertRequest(context.Context, app.Request) error  { return nil }
func (f *fakeRepo) UpdateState(context.Context, string, string) error { return nil }
func (f *fakeRepo) ListRequests(context.Context, string, int) ([]app.Request, error) {
	return []app.Request{{ID: "r1", TenantID: "t1", State: "verified"}}, nil
}
func (f *fakeRepo) ListDeliveryLogs(context.Context, string, int) ([]app.DeliveryLog, error) {
	return nil, nil
}
func (f *fakeRepo) Stats(_ context.Context, tenantID string, _ time.Time) (app.Stats, error) {
	f.lastStatsTenant = tenantID
	return f.stats, nil
}

// stubIntrospector resolves any opaque key to tenant t1, so the API-key auth branch passes
// in these handler tests (auth resolution itself is covered in middleware_test.go).
type stubIntrospector struct{}

func (stubIntrospector) Introspect(context.Context, string) (string, error) { return "t1", nil }

func newServer(svc otphttp.OTPService, repo app.Repo) http.Handler {
	// These tests authenticate with an opaque API key, which never reaches the JWT branch,
	// so a nil verifier is sufficient; the introspector stub resolves the key to a tenant.
	return otphttp.NewRouter(svc, repo, nil, nil, stubIntrospector{})
}

const testKey = "testkey"

func validRepo() *fakeRepo {
	return &fakeRepo{}
}

func do(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestSend_ValidKey_Returns202(t *testing.T) {
	svc := &fakeSvc{sendRes: app.SendResult{RequestID: "r1"}}
	h := newServer(svc, validRepo())
	rr := do(t, h, "POST", "/v1/otp/send", testKey, `{"recipient":"a@b.co"}`)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (%s)", rr.Code, rr.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out["request_id"] != "r1" {
		t.Fatalf("want request_id r1, got %v", out)
	}
}

func TestSend_SMSChannel_202(t *testing.T) {
	svc := &fakeSvc{sendRes: app.SendResult{RequestID: "r1"}}
	h := newServer(svc, validRepo())
	rr := do(t, h, "POST", "/v1/otp/send", testKey, `{"recipient":"+84901234567","channel":"sms"}`)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (%s)", rr.Code, rr.Body.String())
	}
	if svc.lastInput.Channel != "sms" {
		t.Fatalf("service got channel %q, want sms", svc.lastInput.Channel)
	}
}

func TestSend_InvalidRecipient_400(t *testing.T) {
	svc := &fakeSvc{sendErr: domain.ErrInvalidRecipient}
	h := newServer(svc, validRepo())
	rr := do(t, h, "POST", "/v1/otp/send", testKey, `{"recipient":"+84901234567","channel":"email"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestSend_NoKey_Returns401(t *testing.T) {
	h := newServer(&fakeSvc{}, validRepo())
	rr := do(t, h, "POST", "/v1/otp/send", "", `{"recipient":"a@b.co"}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without key, got %d", rr.Code)
	}
}

func TestSend_RateLimited_Returns429(t *testing.T) {
	svc := &fakeSvc{sendErr: domain.ErrRateLimited}
	h := newServer(svc, validRepo())
	rr := do(t, h, "POST", "/v1/otp/send", testKey, `{"recipient":"a@b.co"}`)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", rr.Code)
	}
}

func TestVerify_WrongCode_Returns401(t *testing.T) {
	svc := &fakeSvc{verifyErr: domain.ErrCodeMismatch}
	h := newServer(svc, validRepo())
	rr := do(t, h, "POST", "/v1/otp/verify", testKey, `{"recipient":"a@b.co","code":"000000"}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 on code mismatch, got %d", rr.Code)
	}
}

func TestHealthz_NoAuth_200(t *testing.T) {
	h := newServer(&fakeSvc{}, validRepo())
	// no Authorization header on purpose
	rr := do(t, h, "GET", "/healthz", "", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 for /healthz without auth, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Fatalf("want status ok in body, got %s", rr.Body.String())
	}
}

func TestStats_ValidKey_ReturnsTenantStats(t *testing.T) {
	bucket := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	repo := &fakeRepo{stats: app.Stats{
		SentToday:        2,
		VerifyRate:       0.5,
		Failed:           1,
		P50LatencyMillis: 123,
		Series: []app.StatsPoint{
			{T: bucket, Requested: 1, Sent: 2, Verified: 1, Failed: 0},
		},
		Funnel: app.StatsFunnel{Requested: 4, Sent: 3, Verified: 1},
	}}
	h := newServer(&fakeSvc{}, repo)
	rr := do(t, h, "GET", "/v1/stats", testKey, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if repo.lastStatsTenant != "t1" {
		t.Fatalf("stats tenant = %q, want t1", repo.lastStatsTenant)
	}
	var out struct {
		SentToday        int64   `json:"sent_today"`
		VerifyRate       float64 `json:"verify_rate"`
		Failed           int64   `json:"failed"`
		P50LatencyMillis int64   `json:"p50_latency_ms"`
		Series           []struct {
			T         string `json:"t"`
			Requested int64  `json:"requested"`
			Sent      int64  `json:"sent"`
			Verified  int64  `json:"verified"`
			Failed    int64  `json:"failed"`
		} `json:"series"`
		Funnel struct {
			Requested int64 `json:"requested"`
			Sent      int64 `json:"sent"`
			Verified  int64 `json:"verified"`
		} `json:"funnel"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode stats response: %v", err)
	}
	if out.SentToday != 2 || out.VerifyRate != 0.5 || out.Failed != 1 || out.P50LatencyMillis != 123 {
		t.Fatalf("unexpected stats response: %+v", out)
	}
	if len(out.Series) != 1 || out.Series[0].T != "2026-08-28T08:00:00Z" {
		t.Fatalf("unexpected series: %+v", out.Series)
	}
	if out.Funnel.Requested != 4 || out.Funnel.Sent != 3 || out.Funnel.Verified != 1 {
		t.Fatalf("unexpected funnel: %+v", out.Funnel)
	}
}

func TestStats_NoKey_Returns401(t *testing.T) {
	h := newServer(&fakeSvc{}, validRepo())
	rr := do(t, h, "GET", "/v1/stats", "", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without key, got %d", rr.Code)
	}
}
