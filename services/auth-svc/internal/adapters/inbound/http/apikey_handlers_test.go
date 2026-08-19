package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/pkg/security/securitytest"
	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

type fullSvc struct {
	listKeys []domain.APIKey
	plainKey string
	keyID    string
	err      error
}

func (s fullSvc) Login(context.Context, string, string) (app.LoginResult, error) {
	return app.LoginResult{}, nil
}
func (s fullSvc) Introspect(context.Context, string) (string, bool, error) { return "", false, nil }
func (s fullSvc) ListAPIKeys(_ context.Context, _ string) ([]domain.APIKey, error) {
	return s.listKeys, s.err
}
func (s fullSvc) CreateAPIKey(_ context.Context, _ string) (string, string, error) {
	return s.plainKey, s.keyID, s.err
}
func (s fullSvc) RevokeAPIKey(context.Context, string, string) error { return s.err }

func issueTestJWT(t *testing.T) (token string, verifier *security.Verifier) {
	t.Helper()
	privPEM, pubPEM := securitytest.KeyPair(t)
	issuer, err := security.NewIssuer(privPEM, time.Hour)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	verifier, err = security.NewVerifier(pubPEM)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	tok, _, err := issuer.Issue("u1", "t1", "a@b.co", time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return tok, verifier
}

func TestCreateAPIKey_Returns201WithPlaintext(t *testing.T) {
	tok, v := issueTestJWT(t)
	svc := fullSvc{plainKey: "wl_live_abc123", keyID: "k1"}
	r := NewRouter(svc, v, "")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	var got createKeyResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Key != "wl_live_abc123" || got.ID != "k1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestListAPIKeys_Returns200(t *testing.T) {
	tok, v := issueTestJWT(t)
	now := time.Now().Truncate(time.Second)
	svc := fullSvc{listKeys: []domain.APIKey{
		{ID: "k1", TenantID: "t1", Status: "active", CreatedAt: now},
	}}
	r := NewRouter(svc, v, "")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got []apiKeyDTO
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "k1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRevokeAPIKey_Returns204(t *testing.T) {
	tok, v := issueTestJWT(t)
	r := NewRouter(fullSvc{}, v, "")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/auth/api-keys/k1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestAPIKeys_RequireJWT(t *testing.T) {
	r := NewRouter(fullSvc{}, nil, "")

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/auth/api-keys"},
		{"POST", "/auth/api-keys"},
		{"DELETE", "/auth/api-keys/k1"},
	}
	for _, rt := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(rt.method, rt.path, nil)
		// No Authorization header.
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", rt.method, rt.path, w.Code)
		}
	}
}
