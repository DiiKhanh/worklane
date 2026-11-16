//go:build integration

package http_test

import (
	"encoding/base64"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestManagementPagination(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	codes := make([]string, 0, 23)
	for i := range 23 {
		out := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: "https://page.example/" + strconv.Itoa(i)}), 201)
		codes = append(codes, out.Code)
	}
	slices.Reverse(codes)
	first := readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links", token, nil), 200)
	if len(first.Items) != 20 || first.NextCursor == nil {
		t.Fatalf("wrong default page: %+v", first)
	}
	observed := make([]string, 0, 23)
	for _, item := range first.Items {
		observed = append(observed, item.Code)
	}
	// A later insertion must not shift the second page or duplicate an item.
	readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: "https://page.example/new"}), 201)
	second := readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links?cursor="+url.QueryEscape(*first.NextCursor), token, nil), 200)
	if len(second.Items) != 3 || second.NextCursor != nil {
		t.Fatalf("wrong final page: %+v", second)
	}
	for _, item := range second.Items {
		observed = append(observed, item.Code)
	}
	if !slices.Equal(observed, codes) {
		t.Fatalf("unstable pagination: got %v, want %v", observed, codes)
	}
	all := readResponse[linkhttp.ListResponse](t, f.request(t, "GET", "/v1/links?limit=100", token, nil), 200)
	if len(all.Items) != 24 || all.NextCursor != nil {
		t.Fatalf("wrong explicit page: %+v", all)
	}
	foreign := readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links?cursor="+url.QueryEscape(*first.NextCursor), f.jwt(t, f.other), nil), 400)
	if foreign.Error != "link: invalid request" {
		t.Fatal("foreign cursor was accepted")
	}
	for _, query := range []string{
		"limit=0", "limit=-1", "limit=101", "limit=", "limit=text", "limit=1&limit=2", "cursor=", "cursor=%zz",
		"cursor=not-a-cursor", "cursor=" + strings.Repeat("a", 257), "tenant_id=" + f.other,
		"cursor=" + base64.RawURLEncoding.EncodeToString([]byte(`{"t":"`+f.tenant+`","c":"2026-11-16T12:00:00Z"}`)),
	} {
		t.Run(query, func(t *testing.T) {
			readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links?"+query, token, nil), 400)
		})
	}
}
