package security

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey_FormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		key, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if len(key) < 32 {
			t.Fatalf("key too short: %q (%d)", key, len(key))
		}
		// base64url, no padding: must not contain +, /, or =.
		if strings.ContainsAny(key, "+/=") {
			t.Fatalf("key is not url-safe: %q", key)
		}
		if seen[key] {
			t.Fatalf("duplicate key generated: %q", key)
		}
		seen[key] = true
	}
}

func TestHashKey_DeterministicAndDistinct(t *testing.T) {
	if HashKey("abc") != HashKey("abc") {
		t.Fatal("hash must be deterministic")
	}
	if HashKey("abc") == HashKey("abd") {
		t.Fatal("different keys must hash differently")
	}
	if len(HashKey("abc")) != 64 { // hex sha-256
		t.Fatalf("unexpected hash length: %d", len(HashKey("abc")))
	}
}
