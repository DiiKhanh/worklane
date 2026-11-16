//go:build integration

package http_test

import (
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestManagementAtomicQuotaIncludesDedup(t *testing.T) {
	f := newAPI(t, 10, time.Minute)
	token := f.jwt(t, f.tenant)
	const workers = 24
	start := make(chan struct{})
	results := make(chan *http.Response, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			results <- f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: "https://quota.example/"})
		})
	}
	close(start)
	wg.Wait()
	close(results)
	created, dedup, limited := 0, 0, 0
	for response := range results {
		switch response.StatusCode {
		case 201:
			created++
			readResponse[linkhttp.CreateResponse](t, response, 201)
		case 200:
			dedup++
			readResponse[linkhttp.CreateResponse](t, response, 200)
		case 429:
			limited++
			retry, err := strconv.Atoi(response.Header.Get("Retry-After"))
			if err != nil || retry < 1 || retry > 60 {
				t.Fatalf("invalid Retry-After: %q", response.Header.Get("Retry-After"))
			}
			out := readResponse[linkhttp.ErrorResponse](t, response, 429)
			if out.Error != "link: rate limited" {
				t.Fatalf("unexpected quota error: %+v", out)
			}
		default:
			t.Fatalf("unexpected status: %d", response.StatusCode)
		}
	}
	if created != 1 || dedup != 9 || limited != 14 {
		t.Fatalf("non-atomic quota: created=%d dedup=%d limited=%d", created, dedup, limited)
	}
	readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", f.jwt(t, f.other), linkhttp.CreateRequest{LongURL: "https://quota.example/"}), 201)
	readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links", token, nil), 200)
}

func TestManagementQuotaWindowExpires(t *testing.T) {
	f := newAPI(t, 1, 200*time.Millisecond)
	token := f.jwt(t, f.tenant)
	body := linkhttp.CreateRequest{LongURL: "https://window.example/"}
	readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, body), 201)
	readResponse[linkhttp.ErrorResponse](t, f.request(t, "POST", "/v1/links", token, body), 429)
	time.Sleep(250 * time.Millisecond)
	readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, body), 200)
}

func TestManagementRedisFailureKeepsMetadataAndDisableAvailable(t *testing.T) {
	f := newAPI(t, 10, time.Minute)
	token := f.jwt(t, f.tenant)
	body := linkhttp.CreateRequest{LongURL: "https://failure.example/"}
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, body), 201)
	if err := f.redis.Close(); err != nil {
		t.Fatal(err)
	}
	out := readResponse[linkhttp.ErrorResponse](t, f.request(t, "POST", "/v1/links", token, body), 503)
	if out.Error != "link: unavailable" {
		t.Fatalf("quota failure leaked: %+v", out)
	}
	path := "/v1/links/" + created.Code
	readResponse[linkhttp.Metadata](t, f.request(t, "GET", path, token, nil), 200)
	disabled := readResponse[linkhttp.Metadata](t, f.request(t, "POST", path+"/disable", token, nil), 200)
	read := readResponse[linkhttp.Metadata](t, f.request(t, "GET", path, token, nil), 200)
	if read.State != "disabled" || read.DisabledAt == nil || !read.DisabledAt.Equal(*disabled.DisabledAt) {
		t.Fatal("eviction failure prevented durable disable")
	}
}

func TestManagementDatabaseFailureDoesNotFabricateAbsence(t *testing.T) {
	f := newAPI(t, 10, time.Minute)
	token := f.jwt(t, f.tenant)
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/v1/links", nil}, {"GET", "/v1/links/a", nil}, {"POST", "/v1/links/a/disable", nil},
		{"POST", "/v1/links", linkhttp.CreateRequest{LongURL: "https://failure.example/"}},
	} {
		out := readResponse[linkhttp.ErrorResponse](t, f.request(t, endpoint.method, endpoint.path, token, endpoint.body), 503)
		if out.Error != "link: unavailable" {
			t.Fatalf("database error leaked: %+v", out)
		}
	}
	readResponse[linkhttp.HealthResponse](t, f.request(t, "GET", "/healthz", "", nil), 200)
}
