//go:build integration

package http_test

import (
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestManagementAuthentication(t *testing.T) {
	f := newAPI(t, 10, time.Minute)
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/v1/links"}, {"GET", "/v1/links"}, {"GET", "/v1/links/a"}, {"POST", "/v1/links/a/disable"},
	} {
		out := readResponse[linkhttp.ErrorResponse](t, f.request(t, endpoint.method, endpoint.path, "", nil), 401)
		if out.Error != "missing credentials" {
			t.Fatalf("unexpected auth response: %+v", out)
		}
	}
	for _, token := range []string{"invalid.jwt.token", f.jwt(t, "")} {
		out := readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links", token, nil), 401)
		if out.Error != "invalid token" {
			t.Fatalf("invalid JWT accepted: %+v", out)
		}
	}
	invalid := readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links", "unknown-key", nil), 401)
	if invalid.Error != "invalid api key" {
		t.Fatalf("invalid key accepted: %+v", invalid)
	}
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", f.apiKey, linkhttp.CreateRequest{LongURL: "https://machine.example/"}), 201)
	owned := readResponse[linkhttp.Metadata](t, f.request(t, "GET", "/v1/links/"+created.Code, f.jwt(t, f.tenant), nil), 200)
	if owned.Code != created.Code {
		t.Fatal("API key and JWT resolved to different tenants")
	}
	f.identityDown.Store(true)
	// Existing positive introspection caching follows the OTP authentication path.
	readResponse[linkhttp.Metadata](t, f.request(t, "GET", "/v1/links/"+created.Code, f.apiKey, nil), 200)
	unavailable := readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links", "uncached-key", nil), 503)
	if unavailable.Error != "auth temporarily unavailable" {
		t.Fatalf("identity outage misreported: %+v", unavailable)
	}
	health := readResponse[linkhttp.HealthResponse](t, f.request(t, "GET", "/healthz", "", nil), 200)
	if health.Status != "ok" {
		t.Fatalf("unexpected health: %+v", health)
	}
}
