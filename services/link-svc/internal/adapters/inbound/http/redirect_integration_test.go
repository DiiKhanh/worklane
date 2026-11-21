//go:build integration

package http_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
)

func publicResponse(t *testing.T, f *apiFixture, method, code string, want int, destination string) {
	t.Helper()
	client := *f.server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequest(method, f.server.URL+"/"+code, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want || response.Header.Get("Location") != destination || response.Header.Get("Cache-Control") != "no-store" || len(body) != 0 {
		t.Fatalf("%s: want %d Location %q; got %d Location %q, cache %q, body bytes %d", method, want, destination, response.StatusCode, response.Header.Get("Location"), response.Header.Get("Cache-Control"), len(body))
	}
}

func TestRealRedirectCacheAndDatabaseOutage(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	const destination = "https://EXAMPLE.com/a/../b?b=%2f&a=1#section"
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", f.jwt(t, f.tenant), linkhttp.CreateRequest{LongURL: destination}), 201)
	for _, method := range []string{"GET", "HEAD"} {
		publicResponse(t, f, method, created.Code, 302, destination)
		publicResponse(t, f, method, "G", 404, "")
	}
	cache, err := redisstore.New(f.redis, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := cache.Load(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("authoritative redirect did not populate Redis: %v", err)
	}
	if cached.Link.TenantID != f.tenant || cached.Link.LongURL != destination || cached.Link.DisabledAt != nil || !cached.ValidUntil.After(time.Now()) || cached.ValidUntil.After(time.Now().Add(time.Minute)) {
		t.Fatal("cached state or validity deadline does not match authoritative mapping")
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	publicResponse(t, f, "GET", created.Code, 302, destination)
	publicResponse(t, f, "HEAD", created.Code, 302, destination)
	publicResponse(t, f, "GET", "H", 503, "")
	cached.ValidUntil = time.Now().Add(-time.Second)
	if err := cache.Store(context.Background(), cached); err != nil {
		t.Fatal(err)
	}
	publicResponse(t, f, "GET", created.Code, 503, "")
	publicResponse(t, f, "HEAD", created.Code, 503, "")
}

func TestRealRedirectDisableAndRedisOutage(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	const destination = "https://example.com/disabled"
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination}), 201)
	if err := f.redis.Close(); err != nil {
		t.Fatal(err)
	}
	publicResponse(t, f, "GET", created.Code, 302, destination)
	readResponse[linkhttp.Metadata](t, f.request(t, "POST", "/v1/links/"+created.Code+"/disable", token, nil), 200)
	for _, method := range []string{"GET", "HEAD"} {
		publicResponse(t, f, method, created.Code, 410, "")
		publicResponse(t, f, method, "G", 404, "")
	}
}
