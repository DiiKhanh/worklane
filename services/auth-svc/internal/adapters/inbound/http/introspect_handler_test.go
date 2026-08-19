package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

type introspectSvc struct {
	tenantID string
	active   bool
	err      error
}

func (s introspectSvc) Login(context.Context, string, string) (app.LoginResult, error) {
	return app.LoginResult{}, nil
}
func (s introspectSvc) Introspect(_ context.Context, _ string) (string, bool, error) {
	return s.tenantID, s.active, s.err
}
func (s introspectSvc) ListAPIKeys(context.Context, string) ([]domain.APIKey, error) {
	return nil, nil
}
func (s introspectSvc) CreateAPIKey(context.Context, string) (string, string, error) {
	return "", "", nil
}
func (s introspectSvc) RevokeAPIKey(context.Context, string, string) error { return nil }

func TestIntrospect_ActiveWithToken(t *testing.T) {
	const secret = "test-internal-secret"
	svc := introspectSvc{tenantID: "t42", active: true}
	r := NewRouter(svc, nil, secret)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/internal/introspect",
		strings.NewReader(`{"token":"wl_live_abc123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", secret)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got introspectResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.Active || got.TenantID != "t42" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestIntrospect_MissingToken401(t *testing.T) {
	r := NewRouter(introspectSvc{}, nil, "real-secret")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/internal/introspect",
		strings.NewReader(`{"token":"wl_live_abc123"}`))
	req.Header.Set("Content-Type", "application/json")
	// No X-Internal-Token header.
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
