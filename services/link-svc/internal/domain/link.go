// Package domain is the pure core of the link bounded context: the Link entity and
// the rules for what may be shortened. It imports only the standard library.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxLongURLLen bounds the target URL. 2048 is the de-facto browser/proxy limit; a
	// longer target would not survive the redirect anyway.
	MaxLongURLLen = 2048
	// MaxCodeLen matches the links.code column (VARCHAR(16)).
	MaxCodeLen = 16
	// MaxClickMetaLen matches the link_clicks.referer / ua columns (VARCHAR(255)).
	MaxClickMetaLen = 255
)

// Link is one short code -> long URL mapping owned by a tenant.
type Link struct {
	ID          int64 // snowflake; Code is its base62 encoding
	Code        string
	TenantID    string
	LongURL     string
	LongURLHash string // HashURL(LongURL), the dedup key within a tenant
	CreatedAt   time.Time
}

// NormalizeLongURL trims the input and checks it is an absolute http(s) URL with a
// host. Restricting the scheme matters for a redirector: a short link that resolves to
// javascript: or data: would turn the service into an XSS/phishing vector.
func NormalizeLongURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrInvalidURL
	}
	if len(s) > MaxLongURLLen {
		return "", ErrURLTooLong
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", ErrInvalidURL
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" {
		return "", ErrInvalidURL
	}
	return s, nil
}

// HashURL is the hex sha256 of the long URL. long_url is TEXT and cannot carry a
// unique index, so dedup indexes this fixed-width digest instead.
func HashURL(longURL string) string { return sha256Hex(longURL) }

// HashIP is the hex sha256 of the client IP, so click analytics never stores the raw
// address. An empty IP stays empty rather than hashing to a constant.
func HashIP(ip string) string {
	if ip == "" {
		return ""
	}
	return sha256Hex(ip)
}

// ValidCode reports whether s could be a code minted by this service: 1..MaxCodeLen
// base62 characters. Checking the shape first keeps junk paths (/favicon.ico,
// scanner probes) from costing a cache and DB lookup each.
func ValidCode(s string) bool {
	if s == "" || len(s) > MaxCodeLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// TruncateClickMeta cuts a referer / user-agent to the column width on a rune
// boundary, so an oversized header never fails the click insert.
func TruncateClickMeta(s string) string {
	if len(s) <= MaxClickMetaLen {
		return s // bytes <= limit implies runes <= limit
	}
	r := []rune(s)
	if len(r) <= MaxClickMetaLen {
		return s
	}
	return string(r[:MaxClickMetaLen])
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
