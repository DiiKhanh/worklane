package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/templating"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

type fakeTemplateAPI struct{ published bool }

func (fakeTemplateAPI) List(context.Context) ([]app.Template, error) { return nil, nil }
func (fakeTemplateAPI) Get(context.Context, string) (app.Template, []app.TemplateVersion, error) {
	return app.Template{}, nil, nil
}
func (fakeTemplateAPI) Create(context.Context, app.CreateTemplateInput) (app.Template, error) {
	return app.Template{ID: "t1"}, nil
}
func (fakeTemplateAPI) AddVersion(context.Context, app.AddVersionInput) (app.TemplateVersion, error) {
	return app.TemplateVersion{ID: "v2", VersionNo: 2}, nil
}
func (f *fakeTemplateAPI) Publish(context.Context, string, string) error {
	f.published = true
	return nil
}
func (fakeTemplateAPI) Preview(subject, body string) (string, string) {
	return templating.Render(subject, body, templating.Vars{Code: "123456", Expiry: "5 minutes"})
}

func TestCreateTemplateRequiresJWT(t *testing.T) {
	h := &TemplateHandlers{svc: &fakeTemplateAPI{}}
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest("POST", "/v1/templates",
		strings.NewReader(`{"name":"X","channel":"email","locale":"en","subject":"S","body":"{{code}}"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	// No actor_email in context => an API-key caller, which must be forbidden.
	h.Create(c)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("api-key caller must be forbidden from template CRUD, got %d", rr.Code)
	}
}

func TestPreviewRendersSampleValues(t *testing.T) {
	h := &TemplateHandlers{svc: &fakeTemplateAPI{}}
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Set(authorCtxKey, "a@b.co")
	c.Request = httptest.NewRequest("POST", "/v1/templates/preview",
		strings.NewReader(`{"channel":"email","subject":"Code {{code}}","body":"expires {{expiry}}"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Preview(c)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "123456") || !strings.Contains(rr.Body.String(), "5 minutes") {
		t.Fatalf("preview not rendered: %s", rr.Body.String())
	}
}

func TestPreviewRejectsUnknownVariable(t *testing.T) {
	h := &TemplateHandlers{svc: &fakeTemplateAPI{}}
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Set(authorCtxKey, "a@b.co")
	c.Request = httptest.NewRequest("POST", "/v1/templates/preview",
		strings.NewReader(`{"channel":"email","subject":"S","body":"hi {{name}}"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Preview(c)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown variable must be rejected, got %d", rr.Code)
	}
}
