package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeLongURL(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", MaxLongURLLen)
	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{"https", "https://example.com/a?b=c#d", "https://example.com/a?b=c#d", nil},
		{"http", "http://example.com", "http://example.com", nil},
		{"trims whitespace", "  https://example.com/x \n", "https://example.com/x", nil},
		{"uppercase scheme", "HTTPS://example.com", "HTTPS://example.com", nil},
		{"host with port", "http://localhost:8080/p", "http://localhost:8080/p", nil},
		{"empty", "", "", ErrInvalidURL},
		{"blank", "   ", "", ErrInvalidURL},
		{"relative", "/just/a/path", "", ErrInvalidURL},
		{"no scheme", "example.com/x", "", ErrInvalidURL},
		{"javascript scheme", "javascript:alert(1)", "", ErrInvalidURL},
		{"data scheme", "data:text/html,<b>x</b>", "", ErrInvalidURL},
		{"ftp scheme", "ftp://example.com/f", "", ErrInvalidURL},
		{"missing host", "https:///path", "", ErrInvalidURL},
		{"port without host", "https://:443/path", "", ErrInvalidURL},
		{"control char", "https://example.com/\x00", "", ErrInvalidURL},
		{"too long", long, "", ErrURLTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLongURL(tt.in)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHashURL_KnownVectorAndWidth(t *testing.T) {
	// sha256("abc"), the FIPS 180-2 test vector.
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashURL("abc"); got != want {
		t.Fatalf("HashURL(abc) = %s, want %s", got, want)
	}
	if HashURL("https://a.example") == HashURL("https://b.example") {
		t.Fatal("different urls must hash differently")
	}
}

func TestHashIP(t *testing.T) {
	if got := HashIP(""); got != "" {
		t.Fatalf("empty ip must stay empty, got %q", got)
	}
	got := HashIP("203.0.113.7")
	if len(got) != 64 || strings.Contains(got, "203.0.113.7") {
		t.Fatalf("ip hash must be a 64-char digest without the raw ip, got %q", got)
	}
}

func TestValidCode(t *testing.T) {
	valid := []string{"a", "7Xq2Ab", "0", strings.Repeat("Z", MaxCodeLen)}
	for _, c := range valid {
		if !ValidCode(c) {
			t.Errorf("ValidCode(%q) = false, want true", c)
		}
	}
	invalid := []string{"", "favicon.ico", "a-b", "a/b", "a b", "mã", strings.Repeat("a", MaxCodeLen+1)}
	for _, c := range invalid {
		if ValidCode(c) {
			t.Errorf("ValidCode(%q) = true, want false", c)
		}
	}
}

func TestTruncateClickMeta(t *testing.T) {
	short := "Mozilla/5.0"
	if got := TruncateClickMeta(short); got != short {
		t.Fatalf("short value changed: %q", got)
	}
	if got := TruncateClickMeta(strings.Repeat("a", 300)); len(got) != MaxClickMetaLen {
		t.Fatalf("ascii len = %d, want %d", len(got), MaxClickMetaLen)
	}
	// 200 two-byte runes is 400 bytes but only 200 characters: it fits VARCHAR(255).
	fits := strings.Repeat("é", 200)
	if got := TruncateClickMeta(fits); got != fits {
		t.Fatal("value within the character limit must not be truncated")
	}
	got := TruncateClickMeta(strings.Repeat("é", 300))
	if n := len([]rune(got)); n != MaxClickMetaLen {
		t.Fatalf("rune len = %d, want %d", n, MaxClickMetaLen)
	}
	if strings.ContainsRune(got, '�') {
		t.Fatal("truncation split a multi-byte rune")
	}
}

func TestDeviceFromUA(t *testing.T) {
	tests := []struct{ ua, want string }{
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15", DeviceIOS},
		{"Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15", DeviceIOS},
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36", DeviceAndroid},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", DeviceWindows},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15", DeviceMacOS},
		{"Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/128.0", DeviceLinux},
		{"curl/8.6.0", DeviceOther},
		{"", DeviceOther},
	}
	for _, tt := range tests {
		if got := DeviceFromUA(tt.ua); got != tt.want {
			t.Errorf("DeviceFromUA(%q) = %s, want %s", tt.ua, got, tt.want)
		}
	}
}
