//go:build e2e && linke2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestLinkE2ERouteBoundaries(t *testing.T) {
	isolatedLinkStack(t)
	for _, code := range []string{"authProbe", "v1Probe"} {
		result := callLink(t, "HEAD", apiBase+"/"+code, "", "", http.StatusNotFound)
		if result.header.Get("Cache-Control") != "no-store" {
			t.Fatalf("code %q bypassed Link Service", code)
		}
	}
	callLink(t, "GET", apiBase+"/metrics", "", "", http.StatusNotFound)
}

func TestLinkE2ETrustedIP(t *testing.T) {
	isolatedLinkStack(t)
	key := seedKey(t)
	link := createLink(t, key, fmt.Sprintf(`{"long_url":"https://example.com/ingress/%s"}`, newID()))
	callLink(t, "HEAD", link.ShortURL, "", "", http.StatusFound)

	// Each request comes from the same peer and offers a distinct forwarded IP.
	// HEAD exercises the public limiter without creating click traffic.
	const (
		requests = 200
		rate     = 100
		burst    = 50
	)
	// Refill the shared peer bucket before the next test uses the gateway.
	t.Cleanup(func() { time.Sleep(time.Second * burst / rate) })
	start := make(chan struct{})
	results := make(chan linkResponse, requests)
	for i := 0; i < requests; i++ {
		go func() {
			ip := fmt.Sprintf("192.0.2.%d", i+1)
			<-start
			result, err := requestLink(t.Context(), "HEAD", link.ShortURL, "", "", http.Header{
				"X-Forwarded-For": {ip},
				"X-Real-Ip":       {ip},
				"Forwarded":       {"for=" + ip},
			})
			if err != nil {
				t.Errorf("HEAD: %v", err)
			}
			results <- result
		}()
	}
	began := time.Now()
	close(start)
	admitted, limited := 0, 0
	for i := 0; i < requests; i++ {
		result := <-results
		switch result.status {
		case http.StatusFound:
			admitted++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Errorf("unexpected status %d", result.status)
		}
	}
	elapsed := time.Since(began)
	t.Logf("one peer, %d claimed IPs in %s: %d admitted, %d limited", requests, elapsed.Round(time.Millisecond), admitted, limited)
	if admitted == 0 || limited == 0 || admitted > burst+int(rate*elapsed.Seconds()) {
		t.Fatal("requests did not share the peer's 100/s, burst-50 allowance")
	}
}
