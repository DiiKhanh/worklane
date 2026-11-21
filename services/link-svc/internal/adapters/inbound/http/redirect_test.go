package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type redirectDB struct {
	read func(context.Context, string) (domain.Link, error)
}

type clickBroker struct{ events chan link.ClickedEvent }

func (b clickBroker) Publish(_ context.Context, _ string, event any) error {
	b.events <- event.(link.ClickedEvent)
	return nil
}

func TestRedirectEmitsOnlySanitizedActiveGetClicks(t *testing.T) {
	cache, _ := redirectCache(t)
	db := redirectDB{read: func(_ context.Context, code string) (domain.Link, error) {
		if code != "G" {
			return domain.Link{}, domain.ErrNotFound
		}
		return domain.Link{Code: code, TenantID: "owner", LongURL: "https://example.com"}, nil
	}}
	broker := clickBroker{events: make(chan link.ClickedEvent, 4)}
	emitter := app.NewEmitter(broker, 4)
	t.Cleanup(func() { _ = emitter.Close(context.Background()) })
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, cache, time.Now), emitter)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{{"HEAD", "/G"}, {"GET", "/missing"}, {"GET", "/G"}} {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		request.Header.Set("Referer", "https://user:password@EXAMPLE.org:8443/private?secret=yes#fragment")
		request.Header.Set("User-Agent", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) Mobile/15E148")
		request.Header.Set("X-Forwarded-For", "192.0.2.1")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(broker.events) != 1 {
		t.Fatalf("want exactly one active GET event, got %d", len(broker.events))
	}
	event := <-broker.events
	if event.Code != "G" || event.TenantID != "owner" || event.ReferrerOrigin != "https://example.org:8443" || event.Device != link.DeviceTablet || event.TS.IsZero() || event.TS.Location() != time.UTC {
		t.Fatalf("incorrect sanitized click: %+v", event)
	}
}

func (db redirectDB) ByCode(ctx context.Context, code string) (domain.Link, error) {
	return db.read(ctx, code)
}

func TestRedirectPreservesExactLocationAndHeadHasNoBody(t *testing.T) {
	cache, _ := redirectCache(t)
	const destination = "https://example.com/a%2fb?b=2&a=1#section"
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		return domain.Link{Code: "G", TenantID: "tenant", LongURL: destination}, nil
	}}
	resolver := app.NewResolver(db, cache, time.Now)
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/G", nil))
		if response.Code != 302 || response.Header().Get("Location") != destination {
			t.Fatalf("%s: status=%d Location=%q", method, response.Code, response.Header().Get("Location"))
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Body.Len() != 0 {
			t.Fatalf("%s: cache policy or empty body violated", method)
		}
	}
}
