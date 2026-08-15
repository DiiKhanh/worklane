//go:build e2e

// Package e2e_test exercises the full send -> real email -> verify flow against a
// running docker-compose stack (see README.md). It seeds its own API key via MySQL,
// calls the API through Traefik, and reads the delivered code from MailHog.
package e2e_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/pkg/security"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	apiBase = env("E2E_API_BASE", "http://localhost")
	mailhog = env("E2E_MAILHOG", "http://localhost:8025")
	dsn     = env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// seedKey mints a tenant + API key directly in MySQL and returns the plaintext key.
func seedKey(t *testing.T) string {
	t.Helper()
	db, err := mysql.Open(dsn)
	if err != nil {
		t.Fatalf("mysql open (is the stack up?): %v", err)
	}
	key, err := security.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tid := newID()
	if err := db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", tid, "e2e").Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO api_keys (id, tenant_id, hashed_key, status) VALUES (?, ?, ?, 'active')",
		newID(), tid, security.HashKey(key),
	).Error; err != nil {
		t.Fatalf("seed key: %v", err)
	}
	return key
}

func postJSON(t *testing.T, path, key, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, apiBase+path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

var codeRe = regexp.MustCompile(`\b(\d{6})\b`)

// waitForCode polls MailHog's search API for a message to recipient and extracts the code.
func waitForCode(t *testing.T, recipient string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("%s/api/v2/search?kind=to&query=%s", mailhog, recipient))
		if err == nil {
			var out struct {
				Items []struct {
					Content struct{ Body string } `json:"Content"`
				} `json:"items"`
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			_ = json.Unmarshal(b, &out)
			if len(out.Items) > 0 {
				if m := codeRe.FindStringSubmatch(out.Items[0].Content.Body); m != nil {
					return m[1]
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("no OTP email arrived for %s within timeout", recipient)
	return ""
}

func TestE2E_SendReceiveVerify(t *testing.T) {
	key := seedKey(t)
	recipient := fmt.Sprintf("e2e-%d@example.com", time.Now().UnixNano())

	if code := postJSON(t, "/v1/otp/send", key, fmt.Sprintf(`{"recipient":%q}`, recipient)); code != http.StatusAccepted {
		t.Fatalf("send: want 202, got %d", code)
	}

	otp := waitForCode(t, recipient)

	if code := postJSON(t, "/v1/otp/verify", key, fmt.Sprintf(`{"recipient":%q,"code":"000000"}`, recipient)); code != http.StatusUnauthorized {
		t.Fatalf("wrong code: want 401, got %d", code)
	}
	if code := postJSON(t, "/v1/otp/verify", key, fmt.Sprintf(`{"recipient":%q,"code":%q}`, recipient, otp)); code != http.StatusOK {
		t.Fatalf("correct code: want 200, got %d", code)
	}
}

func TestE2E_NoKeyRejected(t *testing.T) {
	if code := postJSON(t, "/v1/otp/send", "", `{"recipient":"x@y.co"}`); code != http.StatusUnauthorized {
		t.Fatalf("no key: want 401, got %d", code)
	}
}
