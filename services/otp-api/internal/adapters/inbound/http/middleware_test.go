package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/pkg/security/securitytest"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// stubRepo satisfies the full app.Repo interface; only FindAPIKey is exercised here.
type stubRepo struct {
	ak  app.APIKey
	err error
}

func (s stubRepo) FindAPIKey(context.Context, string) (app.APIKey, error) { return s.ak, s.err }
func (stubRepo) InsertRequest(context.Context, app.Request) error         { return nil }
func (stubRepo) UpdateState(context.Context, string, string) error        { return nil }
func (stubRepo) ListAPIKeys(context.Context, string) ([]app.APIKey, error) { return nil, nil }
func (stubRepo) ListRequests(context.Context, string, int) ([]app.Request, error) {
	return nil, nil
}
func (stubRepo) ListDeliveryLogs(context.Context, string, int) ([]app.DeliveryLog, error) {
	return nil, nil
}

func testRouter(v *security.Verifier, repo app.Repo) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/probe", authenticate(v, repo), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(tenantCtxKey))
	})
	return r
}

func TestAuthenticate_ValidJWT(t *testing.T) {
	priv, pub := securitytest.KeyPair(t)
	iss, _ := security.NewIssuer(priv, time.Hour)
	ver, _ := security.NewVerifier(pub)
	tok, _, _ := iss.Issue("u1", "tenant-jwt", "a@b.co", time.Now())

	r := testRouter(ver, stubRepo{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "tenant-jwt" {
		t.Fatalf("jwt path: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticate_ValidAPIKey(t *testing.T) {
	_, pub := securitytest.KeyPair(t)
	ver, _ := security.NewVerifier(pub)
	repo := stubRepo{ak: app.APIKey{TenantID: "tenant-key", Status: "active"}}

	r := testRouter(ver, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer opaque-key-no-dots")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "tenant-key" {
		t.Fatalf("api-key path: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticate_Garbage401(t *testing.T) {
	_, pub := securitytest.KeyPair(t)
	ver, _ := security.NewVerifier(pub)
	r := testRouter(ver, stubRepo{err: context.DeadlineExceeded})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer nonsense")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}
