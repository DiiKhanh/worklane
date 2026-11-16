//go:build integration

package http_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestManagementLifecycle(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	const destination = "https://EXAMPLE.com/đường?b=2&a=1#section"
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination}), 201)
	if created.Code == "" || created.ShortURL != "https://short.example/"+created.Code {
		t.Fatalf("invalid short link: %+v", created)
	}
	dedup := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination}), 200)
	if dedup != created {
		t.Fatalf("dedup changed identity: %+v", dedup)
	}
	path := "/v1/links/" + created.Code
	readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links/"+strings.ToUpper(created.Code), token, nil), 404)
	metadata := readResponse[linkhttp.Metadata](t, f.request(t, "GET", path, token, nil), 200)
	if metadata.LongURL != destination || metadata.State != "active" || metadata.DisabledAt != nil || metadata.CreatedAt.IsZero() || metadata.ShortURL != created.ShortURL {
		t.Fatalf("invalid metadata: %+v", metadata)
	}
	listed := readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links", token, nil), 200)
	if len(listed.Items) != 1 || listed.Items[0].Code != created.Code || listed.NextCursor != nil {
		t.Fatalf("invalid list: %+v", listed)
	}
	disabled := readResponse[linkhttp.Metadata](t, f.request(t, "POST", path+"/disable", token, nil), 200)
	if disabled.State != "disabled" || disabled.DisabledAt == nil || disabled.DisabledAt.Before(disabled.CreatedAt) || disabled.LongURL != destination {
		t.Fatalf("invalid disabled metadata: %+v", disabled)
	}
	repeated := readResponse[linkhttp.Metadata](t, f.request(t, "POST", path+"/disable", token, nil), 200)
	if !repeated.DisabledAt.Equal(*disabled.DisabledAt) {
		t.Fatal("repeated disable changed timestamp")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			concurrent := readResponse[linkhttp.Metadata](t, f.request(t, "POST", path+"/disable", token, nil), 200)
			if concurrent.DisabledAt == nil || !concurrent.DisabledAt.Equal(*disabled.DisabledAt) {
				t.Error("concurrent disable changed timestamp")
			}
		})
	}
	wg.Wait()
	err := readResponse[linkhttp.ErrorResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination}), 409)
	if err.Error != "LINK_DISABLED" {
		t.Fatalf("unexpected conflict: %+v", err)
	}
	retained := readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links", token, nil), 200)
	if len(retained.Items) != 1 || retained.Items[0].State != "disabled" {
		t.Fatalf("disable removed history: %+v", retained)
	}
}

func TestManagementTenantIsolationAndConcurrentDedup(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	const destination = "https://concurrent.example/?x=1"
	const workers = 16
	results := make(chan *http.Response, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			results <- f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination})
		})
	}
	close(start)
	wg.Wait()
	close(results)
	var code string
	createdCount := 0
	for response := range results {
		if response.StatusCode == 201 {
			createdCount++
		} else if response.StatusCode != 200 {
			t.Fatalf("concurrent create returned %d", response.StatusCode)
		}
		out := readResponse[linkhttp.CreateResponse](t, response, response.StatusCode)
		if code != "" && code != out.Code {
			t.Fatal("concurrent destination received multiple codes")
		}
		code = out.Code
	}
	if createdCount != 1 {
		t.Fatalf("want exactly one creation, got %d", createdCount)
	}
	otherToken := f.jwt(t, f.other)
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/v1/links/" + code}, {"POST", "/v1/links/" + code + "/disable"},
	} {
		out := readResponse[linkhttp.ErrorResponse](t, f.request(t, endpoint.method, endpoint.path, otherToken, nil), 404)
		if out.Error != "link: not found" {
			t.Fatalf("foreign mapping leaked: %+v", out)
		}
	}
	empty := readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links", otherToken, nil), 200)
	if empty.Items == nil || len(empty.Items) != 0 || empty.NextCursor != nil {
		t.Fatalf("foreign list leaked: %+v", empty)
	}
	other := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", otherToken, linkhttp.CreateRequest{LongURL: destination}), 201)
	if other.Code == code {
		t.Fatal("different tenants share a mapping")
	}
	for _, distinct := range []string{"https://concurrent.example/?x=2", "https://concurrent.example/?x=1#part", "https://CONCURRENT.example/?x=1", "https://concurrent.example?x=1"} {
		out := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: distinct}), 201)
		if out.Code == code {
			t.Fatal("distinct destination strings deduplicated")
		}
	}
}
