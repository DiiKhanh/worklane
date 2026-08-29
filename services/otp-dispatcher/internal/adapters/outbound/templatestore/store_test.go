package templatestore

import "testing"

// key must match otp-api's redisstore.TemplateCache.Invalidate, or publishes would clear
// a different key than the dispatcher reads. This guards that contract cheaply; the full
// cache-aside + fallback path is exercised by the compose e2e smoke.
func TestCacheKeyFormat(t *testing.T) {
	if got := key("email", "en"); got != "tmpl:email:en" {
		t.Fatalf("cache key = %q, want tmpl:email:en", got)
	}
}
