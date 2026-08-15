package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	otphttp "github.com/duykhanh/worklane/services/otp-api/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// --- fakes ---

type fakeSvc struct {
	sendErr   error
	verifyErr error
	sendRes   app.SendResult
}

func (f *fakeSvc) Send(context.Context, app.SendInput) (app.SendResult, error) {
	return f.sendRes, f.sendErr
}
func (f *fakeSvc) Verify(context.Context, app.VerifyInput) error { return f.verifyErr }

type fakeRepo struct{ validHashedKey string }

func (f *fakeRepo) InsertRequest(context.Context, app.Request) error { return nil }
func (f *fakeRepo) UpdateState(context.Context, string, string) error { return nil }
func (f *fakeRepo) FindAPIKey(_ context.Context, hashedKey string) (app.APIKey, error) {
	if hashedKey == f.validHashedKey {
		return app.APIKey{ID: "k1", TenantID: "t1", Status: "active"}, nil
	}
	return app.APIKey{}, domain.ErrNotFound
}
func (f *fakeRepo) ListAPIKeys(context.Context, string) ([]app.APIKey, error) {
	return []app.APIKey{{ID: "k1", TenantID: "t1", Status: "active"}}, nil
}
func (f *fakeRepo) ListRequests(context.Context, string, int) ([]app.Request, error) {
	return []app.Request{{ID: "r1", TenantID: "t1", State: "verified"}}, nil
}
func (f *fakeRepo) ListDeliveryLogs(context.Context, string, int) ([]app.DeliveryLog, error) {
	return nil, nil
}

func newServer(svc otphttp.OTPService, repo app.Repo) http.Handler {
	return otphttp.NewRouter(svc, repo)
}

const testKey = "testkey"

func validRepo() *fakeRepo {
	return &fakeRepo{validHashedKey: domain.HashCode(testKey, "")}
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

func TestListAPIKeys_Returns200(t *testing.T) {
	h := newServer(&fakeSvc{}, validRepo())
	rr := do(t, h, "GET", "/v1/api-keys", testKey, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "k1") {
		t.Fatalf("expected api key in body: %s", rr.Body.String())
	}
}
