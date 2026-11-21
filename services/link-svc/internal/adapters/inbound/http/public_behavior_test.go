package http_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type recordedClicks struct{ events []link.ClickedEvent }

func (c *recordedClicks) TryEmit(event link.ClickedEvent) bool {
	c.events = append(c.events, event)
	return true
}

func redirectCache(t *testing.T) (*redisstore.Store, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	cache, err := redisstore.New(client, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return cache, client
}

func TestPublicStatusAndClickPrivacy(t *testing.T) {
	cache, _ := redirectCache(t)
	disabledAt := time.Now()
	db := redirectDB{read: func(_ context.Context, code string) (domain.Link, error) {
		row := domain.Link{Code: code, TenantID: "owner", LongURL: "https://example.com/?secret=destination"}
		switch code {
		case "G":
			return row, nil
		case "H":
			row.DisabledAt = &disabledAt
			return row, nil
		case "I":
			return domain.Link{}, domain.ErrNotFound
		default:
			return domain.Link{}, errors.New("private dependency details")
		}
	}}
	clicks := &recordedClicks{}
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, cache, time.Now), clicks, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		for _, scenario := range []struct {
			code string
			want int
		}{{"G", 302}, {"H", 410}, {"I", 404}, {"J", 503}, {"invalid!", 404}} {
			req := httptest.NewRequest(method, "/"+scenario.code, nil)
			req.Header.Set("Referer", "https://user:password@REF.example:8443/private?token=secret#fragment")
			req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS) Mobile/secret")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != scenario.want || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s: status %d, cache %q", method, scenario.code, response.Code, response.Header().Get("Cache-Control"))
			}
			if response.Body.Len() != 0 || (scenario.want != 302 && response.Header().Get("Location") != "") {
				t.Fatal("response leaked a body or error Location")
			}
		}
	}
	if len(clicks.events) != 1 {
		t.Fatalf("only active GET 302 emits: got %d clicks", len(clicks.events))
	}
	event := clicks.events[0]
	if event.Code != "G" || event.TenantID != "owner" || event.ReferrerOrigin != "https://ref.example:8443" || event.Device != link.DeviceMobile || event.TS.IsZero() || event.TS.Location() != time.UTC {
		t.Fatalf("incorrect sanitized click: %+v", event)
	}
}

func TestReferrersAndDevicesAreSanitizedBeforeAdmission(t *testing.T) {
	cache, _ := redirectCache(t)
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com/"}, nil
	}}
	clicks := &recordedClicks{}
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, cache, time.Now), clicks, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		referrer, agent, origin string
		device                  link.Device
	}{
		{"http://example.com/a?secret=x", "Mozilla/5.0 (Windows NT 10.0)", "http://example.com", link.DeviceDesktop},
		{"https://[2001:db8::1]:8443/x", "Mozilla/5.0 (iPad; CPU OS)", "https://[2001:db8::1]:8443", link.DeviceTablet},
		{"https://example.com/", "Mozilla/5.0 (Linux; Android 13)", "https://example.com", link.DeviceTablet},
		{"/relative?secret=x", "curl/8.0", "", link.DeviceUnknown},
		{"javascript:secret", "", "", link.DeviceUnknown},
		{"https://example.com:99999/", "bot", "", link.DeviceUnknown},
		{"https://bad host/", "bot", "", link.DeviceUnknown},
		{strings.Repeat("x", 5000), "bot", "", link.DeviceUnknown},
	} {
		req := httptest.NewRequest("GET", "/G", nil)
		req.Header.Set("Referer", scenario.referrer)
		req.Header.Set("User-Agent", scenario.agent)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 302 {
			t.Fatalf("unexpected status %d", response.Code)
		}
		event := clicks.events[len(clicks.events)-1]
		if event.ReferrerOrigin != scenario.origin || event.Device != scenario.device {
			t.Fatalf("want origin %q device %q, got %q %q", scenario.origin, scenario.device, event.ReferrerOrigin, event.Device)
		}
	}
}
