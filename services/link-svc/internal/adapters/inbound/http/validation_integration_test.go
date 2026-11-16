//go:build integration

package http_test

import (
	"strings"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestManagementDestinationPolicy(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	for _, destination := range []string{
		"", "/relative", "//example.com/path", "ftp://example.com/", "https:example.com",
		"https://", "http:///path", "https://user:pass@example.com/", "https://@example.com/",
		"https://example.com/\r\nInjected:yes", "https://example.com/\x00", "https://example.com/\u0085",
		"http://localhost/", "http://LOCALHOST.:8080/", "http://test.localhost/", "http://ｌｏｃａｌｈｏｓｔ/",
		"https://short.example/a", "http://SHORT.example.:99/a", "https://ｓｈｏｒｔ.example/",
		"http://0.0.0.0/", "http://127.0.0.1/", "http://10.1.2.3/", "http://172.16.0.1/", "http://192.168.0.1/",
		"http://169.254.169.254/", "http://100.64.0.1/", "http://192.0.2.1/", "http://198.18.0.1/",
		"http://198.51.100.1/", "http://203.0.113.1/", "http://224.0.0.1/", "http://240.0.0.1/",
		"http://2130706433/", "http://127.1/", "http://0177.0.0.1/", "http://0x7f000001/",
		"http://[::]/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "http://[fc00::1]/",
		"http://[fe80::1%25eth0]/", "http://[2001:db8::1]/", "http://[3fff::1]/", "http://[4000::1]/",
		"http://[5f00::1]/", "http://[ff02::1]/", "http://[fec0::1]/",
		"https://example.com:99999/", "https://example.com:/", "https://example.com\\@localhost/",
		"https://example.com/" + strings.Repeat("a", 4096), "https://example.com/" + strings.Repeat("é", 2040),
	} {
		t.Run(destination, func(t *testing.T) {
			out := readResponse[linkhttp.ErrorResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination}), 400)
			if out.Error != "link: invalid request" {
				t.Fatalf("unredacted validation error: %+v", out)
			}
		})
	}
	for _, destination := range []string{
		"http://example.invalid/", "HTTPS://EXAMPLE.com/?b=2&a=1#part", "https://example.com/é",
		"http://8.8.8.8/", "https://[2606:4700:4700::1111]/", "http://[::ffff:8.8.8.8]/",
		"http://192.0.0.9/", "https://example.com:8443/", "https://example.com/" + strings.Repeat("a", 4074) + "é",
	} {
		created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: destination}), 201)
		metadata := readResponse[linkhttp.Metadata](t, f.request(t, "GET", "/v1/links/"+created.Code, token, nil), 200)
		if metadata.LongURL != destination {
			t.Fatal("accepted destination was normalized")
		}
	}
}

func TestManagementRejectsMalformedBodies(t *testing.T) {
	f := newAPI(t, 10, time.Minute)
	token := f.jwt(t, f.tenant)
	for _, body := range []string{
		``, `null`, `[]`, `{}`, `{"long_url":12}`, `{"long_url":"https://example.com/","tenant_id":"foreign"}`,
		`{"long_url":"https://example.com/"} {}`, strings.Repeat(" ", 32768) + `{}`,
		"{\"long_url\":\"https://example.com/\xff\"}",
	} {
		readResponse[linkhttp.ErrorResponse](t, f.rawRequest(t, "POST", "/v1/links", token, []byte(body)), 400)
	}
}
