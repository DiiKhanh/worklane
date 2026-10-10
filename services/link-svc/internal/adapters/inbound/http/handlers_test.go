package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// --- fakes ---

type fakeSvc struct {
	shortenRes app.ShortenResult
	links      []app.LinkSummary
	detail     app.LinkDetail
	longURL    string
	err        error

	lastShorten app.ShortenInput
	lastResolve app.ResolveInput
	lastTenant  string
	lastCode    string
	calls       int
}

func (f *fakeSvc) Shorten(_ context.Context, in app.ShortenInput) (app.ShortenResult, error) {
	f.calls++
	f.lastShorten = in
	return f.shortenRes, f.err
}

func (f *fakeSvc) Resolve(_ context.Context, in app.ResolveInput) (string, error) {
	f.calls++
	f.lastResolve = in
	return f.longURL, f.err
}

func (f *fakeSvc) ListLinks(_ context.Context, tenantID string) ([]app.LinkSummary, error) {
	f.calls++
	f.lastTenant = tenantID
	return f.links, f.err
}

func (f *fakeSvc) LinkDetail(_ context.Context, tenantID, code string) (app.LinkDetail, error) {
	f.calls++
	f.lastTenant, f.lastCode = tenantID, code
	return f.detail, f.err
}

// stubIntrospector resolves any opaque key to tenant t1, so the API-key auth branch passes
// in these handler tests (auth resolution itself is covered in middleware_test.go).
type stubIntrospector struct{}

func (stubIntrospector) Introspect(context.Context, string) (string, error) { return "t1", nil }

func newServer(svc linkhttp.LinkService) http.Handler {
	// These tests authenticate with an opaque API key, which never reaches the JWT branch,
	// so a nil verifier is sufficient; the introspector stub resolves the key to a tenant.
	return linkhttp.NewRouter(svc, nil, stubIntrospector{})
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

// --- create ---

func TestCreate_New_201(t *testing.T) {
	svc := &fakeSvc{shortenRes: app.ShortenResult{Code: "abc123", ShortURL: "https://l.test/abc123", Created: true}}
	w := do(t, newServer(svc), "POST", "/v1/links", testKey, `{"long_url":"https://example.com/a"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}
	got := decode[map[string]string](t, w)
	if got["code"] != "abc123" || got["short_url"] != "https://l.test/abc123" {
		t.Fatalf("unexpected body: %v", got)
	}
	if svc.lastShorten != (app.ShortenInput{TenantID: "t1", LongURL: "https://example.com/a"}) {
		t.Fatalf("unexpected use-case input: %+v", svc.lastShorten)
	}
}

func TestCreate_Dedup_200SameMapping(t *testing.T) {
	svc := &fakeSvc{shortenRes: app.ShortenResult{Code: "abc123", ShortURL: "https://l.test/abc123", Created: false}}
	w := do(t, newServer(svc), "POST", "/v1/links", testKey, `{"long_url":"https://example.com/a"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for an existing mapping, got %d", w.Code)
	}
	if got := decode[map[string]string](t, w); got["code"] != "abc123" {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestCreate_BadBody_400(t *testing.T) {
	for name, body := range map[string]string{
		"missing field": `{}`,
		"not json":      `long_url=https://example.com`,
		"wrong type":    `{"long_url":42}`,
		"oversized":     `{"long_url":"https://example.com/` + strings.Repeat("a", 32<<10) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &fakeSvc{}
			w := do(t, newServer(svc), "POST", "/v1/links", testKey, body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", w.Code)
			}
			if svc.calls != 0 {
				t.Fatal("use case must not run for a rejected body")
			}
		})
	}
}

func TestCreate_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid url", domain.ErrInvalidURL, http.StatusBadRequest},
		{"url too long", domain.ErrURLTooLong, http.StatusBadRequest},
		{"rate limited", domain.ErrRateLimited, http.StatusTooManyRequests},
		{"wrapped rate limited", errors.Join(errors.New("ctx"), domain.ErrRateLimited), http.StatusTooManyRequests},
		{"unexpected", errors.New("dial tcp 10.0.0.5:3306: refused"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, newServer(&fakeSvc{err: tc.err}), "POST", "/v1/links", testKey, `{"long_url":"x"}`)
			if w.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, w.Code)
			}
		})
	}
}

func TestInternalErrorDoesNotLeakDetail(t *testing.T) {
	svc := &fakeSvc{err: errors.New("dial tcp 10.0.0.5:3306: refused")}
	w := do(t, newServer(svc), "GET", "/v1/links", testKey, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
	if got := decode[map[string]string](t, w); got["error"] != "internal error" {
		t.Fatalf("internal detail leaked: %v", got)
	}
}

// --- auth ---

func TestV1RequiresCredentials(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"POST", "/v1/links", `{"long_url":"https://example.com"}`},
		{"GET", "/v1/links", ""},
		{"GET", "/v1/links/abc123", ""},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			svc := &fakeSvc{}
			w := do(t, newServer(svc), rt.method, rt.path, "", rt.body)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("want 401, got %d", w.Code)
			}
			if svc.calls != 0 {
				t.Fatal("use case must not run without credentials")
			}
		})
	}
}

// --- list / detail ---

func TestList_ReturnsTenantLinks(t *testing.T) {
	created := time.Date(2026, 8, 16, 9, 12, 0, 0, time.FixedZone("ICT", 7*3600))
	svc := &fakeSvc{links: []app.LinkSummary{{Code: "abc123", Target: "https://example.com/a", Clicks: 7, CreatedAt: created}}}
	w := do(t, newServer(svc), "GET", "/v1/links", testKey, "")

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if svc.lastTenant != "t1" {
		t.Fatalf("list must be scoped to the authenticated tenant, got %q", svc.lastTenant)
	}
	got := decode[[]map[string]any](t, w)
	if len(got) != 1 {
		t.Fatalf("want 1 link, got %v", got)
	}
	l := got[0]
	if l["code"] != "abc123" || l["target"] != "https://example.com/a" || l["clicks"] != float64(7) {
		t.Fatalf("unexpected link: %v", l)
	}
	if l["created"] != "2026-08-16T02:12:00Z" {
		t.Fatalf("created must be RFC 3339 UTC, got %v", l["created"])
	}
}

func TestList_EmptyIsArrayNotNull(t *testing.T) {
	w := do(t, newServer(&fakeSvc{}), "GET", "/v1/links", testKey, "")
	if body := strings.TrimSpace(w.Body.String()); body != "[]" {
		t.Fatalf("want [], got %s", body)
	}
}

func TestDetail_Shape(t *testing.T) {
	ts := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	series := make([]int64, app.SeriesDays)
	series[app.SeriesDays-1] = 3
	svc := &fakeSvc{detail: app.LinkDetail{
		Code: "abc123", Target: "https://example.com/a", Clicks: 3, CreatedAt: ts, Series: series,
		Recent: []app.RecentClick{{TS: ts, Referer: "https://ref.example", Device: "iOS"}},
	}}
	w := do(t, newServer(svc), "GET", "/v1/links/abc123", testKey, "")

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if svc.lastTenant != "t1" || svc.lastCode != "abc123" {
		t.Fatalf("unexpected use-case args: tenant=%q code=%q", svc.lastTenant, svc.lastCode)
	}
	got := decode[struct {
		Code   string  `json:"code"`
		Target string  `json:"target"`
		Clicks int64   `json:"clicks"`
		Series []int64 `json:"series"`
		Recent []struct {
			TS     string `json:"ts"`
			Ref    string `json:"ref"`
			Geo    string `json:"geo"`
			Device string `json:"device"`
		} `json:"recent"`
	}](t, w)
	if got.Code != "abc123" || got.Target != "https://example.com/a" || got.Clicks != 3 {
		t.Fatalf("unexpected detail: %+v", got)
	}
	if len(got.Series) != app.SeriesDays || got.Series[app.SeriesDays-1] != 3 {
		t.Fatalf("unexpected series: %v", got.Series)
	}
	if len(got.Recent) != 1 || got.Recent[0].Ref != "https://ref.example" || got.Recent[0].Device != "iOS" ||
		got.Recent[0].TS != "2026-08-16T09:00:00Z" {
		t.Fatalf("unexpected recent: %+v", got.Recent)
	}
}

func TestDetail_NoClicksSerializesEmptyArrays(t *testing.T) {
	w := do(t, newServer(&fakeSvc{detail: app.LinkDetail{Code: "abc123"}}), "GET", "/v1/links/abc123", testKey, "")
	body := w.Body.String()
	if !strings.Contains(body, `"series":[]`) || !strings.Contains(body, `"recent":[]`) {
		t.Fatalf("want empty arrays, got %s", body)
	}
}

func TestDetail_NotFound_404(t *testing.T) {
	w := do(t, newServer(&fakeSvc{err: domain.ErrNotFound}), "GET", "/v1/links/nope", testKey, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// --- public routes ---

func TestRedirect_302PublicAndPassesClickMeta(t *testing.T) {
	svc := &fakeSvc{longURL: "https://example.com/a?x=1&y=2"}
	req := httptest.NewRequest("GET", "/abc123", nil) // deliberately no Authorization header
	req.Header.Set("Referer", "https://ref.example/page")
	req.Header.Set("User-Agent", "curl/8")
	req.RemoteAddr = "203.0.113.7:4711"
	w := httptest.NewRecorder()
	newServer(svc).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/a?x=1&y=2" {
		t.Fatalf("unexpected Location %q", loc)
	}
	want := app.ResolveInput{Code: "abc123", Referer: "https://ref.example/page", UA: "curl/8", ClientIP: "203.0.113.7"}
	if svc.lastResolve != want {
		t.Fatalf("unexpected use-case input: %+v", svc.lastResolve)
	}
}

func TestRedirect_UnknownCode_404(t *testing.T) {
	w := do(t, newServer(&fakeSvc{err: domain.ErrNotFound}), "GET", "/nope", "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	if w.Header().Get("Location") != "" {
		t.Fatal("a 404 must not carry a Location header")
	}
}

func TestStaticRoutesAreNotTreatedAsCodes(t *testing.T) {
	svc := &fakeSvc{longURL: "https://example.com"}
	h := newServer(svc)

	w := do(t, h, "GET", "/healthz", "", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("healthz: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/metrics", "", ""); w.Code != http.StatusOK {
		t.Fatalf("metrics: want 200, got %d", w.Code)
	}
	if svc.calls != 0 {
		t.Fatal("static routes must not reach Resolve")
	}
}
