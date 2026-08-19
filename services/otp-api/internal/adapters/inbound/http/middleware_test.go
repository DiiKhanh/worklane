package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/pkg/security/securitytest"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// stubIntrospector stands in for the identity client: the opaque-key branch calls it
// instead of reaching auth-svc.
type stubIntrospector struct {
	tenant string
	err    error
}

func (s stubIntrospector) Introspect(context.Context, string) (string, error) {
	return s.tenant, s.err
}

func testRouter(v *security.Verifier, intro app.Introspector) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/probe", authenticate(v, intro), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(tenantCtxKey))
	})
	return r
}

func TestAuthenticate_ValidJWT(t *testing.T) {
	priv, pub := securitytest.KeyPair(t)
	iss, _ := security.NewIssuer(priv, time.Hour)
	ver, _ := security.NewVerifier(pub)
	tok, _, _ := iss.Issue("u1", "tenant-jwt", "a@b.co", time.Now())

	r := testRouter(ver, stubIntrospector{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "tenant-jwt" {
		t.Fatalf("jwt path: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticate_ValidAPIKey(t *testing.T) {
	r := testRouter(nil, stubIntrospector{tenant: "tenant-key"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer opaque-key-no-dots")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "tenant-key" {
		t.Fatalf("api-key path: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticate_InvalidAPIKey_401(t *testing.T) {
	r := testRouter(nil, stubIntrospector{err: app.ErrInvalidAPIKey})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer bad-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown/revoked key, got %d", w.Code)
	}
}

func TestAuthenticate_IntrospectDown_503(t *testing.T) {
	r := testRouter(nil, stubIntrospector{err: errors.New("connection refused")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when identity service errors, got %d", w.Code)
	}
}

func TestAuthenticate_MissingCredentials_401(t *testing.T) {
	r := testRouter(nil, stubIntrospector{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without Authorization, got %d", w.Code)
	}
}
