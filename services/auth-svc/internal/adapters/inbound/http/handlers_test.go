package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

type fakeSvc struct {
	res app.LoginResult
	err error
}

func (f fakeSvc) Login(_ context.Context, _, _ string) (app.LoginResult, error) {
	return f.res, f.err
}
func (f fakeSvc) Introspect(context.Context, string) (string, bool, error)     { return "", false, nil }
func (f fakeSvc) ListAPIKeys(context.Context, string) ([]domain.APIKey, error) { return nil, nil }
func (f fakeSvc) CreateAPIKey(context.Context, string) (string, string, error) { return "", "", nil }
func (f fakeSvc) RevokeAPIKey(context.Context, string, string) error           { return nil }

func TestLogin_OK(t *testing.T) {
	svc := fakeSvc{res: app.LoginResult{
		Token: "tok", ExpiresAt: time.Unix(3600, 0),
		User: domain.User{ID: "u1", Email: "a@b.co", TenantID: "t1"},
	}}
	// verifier can be nil here: /auth/login does not use it.
	r := NewRouter(svc, nil, "")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"a@b.co","password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got loginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Token != "tok" || got.User.TenantID != "t1" {
		t.Fatalf("body wrong: %+v", got)
	}
}

func TestLogin_BadCredentials401(t *testing.T) {
	r := NewRouter(fakeSvc{err: domain.ErrInvalidCredentials}, nil, "")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"a@b.co","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHealthz_NoAuth_200(t *testing.T) {
	// verifier + internal token are unused by /healthz; nil/empty are fine.
	r := NewRouter(fakeSvc{}, nil, "")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for /healthz, got %d", w.Code)
	}
}
