//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// noRedirect returns the 302 itself instead of following it to the target.
var noRedirect = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// linkJSON calls link-svc's tenant API through Traefik and decodes the JSON body into out
// (skipped when out is nil). It returns the HTTP status.
func linkJSON(t *testing.T, method, path, key, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(method, apiBase+path, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, b, err)
		}
	}
	return resp.StatusCode
}

// follow requests a short code on the public link host and returns the status and
// Location header without following the redirect.
func follow(t *testing.T, code string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, apiBase+"/"+code, nil)
	if err != nil {
		t.Fatalf("GET /%s: %v", code, err)
	}
	req.Host = linkHost
	req.Header.Set("Referer", "https://e2e.example/")
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("GET /%s: %v", code, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get("Location")
}

type created struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

type detail struct {
	Code   string  `json:"code"`
	Target string  `json:"target"`
	Clicks int64   `json:"clicks"`
	Series []int64 `json:"series"`
	Recent []struct {
		Ref string `json:"ref"`
	} `json:"recent"`
}

// waitForClicks polls the detail endpoint until the click pipeline (link-svc -> Kafka ->
// link-dispatcher -> MySQL) has persisted at least want clicks.
func waitForClicks(t *testing.T, key, code string, want int64) detail {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var d detail
	for time.Now().Before(deadline) {
		d = detail{}
		if status := linkJSON(t, http.MethodGet, "/v1/links/"+code, key, "", &d); status != http.StatusOK {
			t.Fatalf("detail: want 200, got %d", status)
		}
		if d.Clicks >= want {
			return d
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("want %d clicks for %s within timeout, got %d", want, code, d.Clicks)
	return d
}

func TestE2E_LinkShortenRedirectTrack(t *testing.T) {
	key := seedKey(t)
	target := fmt.Sprintf("https://example.com/e2e/%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"long_url":%q}`, target)

	var first created
	if status := linkJSON(t, http.MethodPost, "/v1/links", key, body, &first); status != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", status)
	}
	if first.Code == "" || first.ShortURL == "" {
		t.Fatalf("create: want code and short_url, got %+v", first)
	}

	// Re-posting the same URL returns the existing mapping (dedup), not a new code.
	var again created
	if status := linkJSON(t, http.MethodPost, "/v1/links", key, body, &again); status != http.StatusOK {
		t.Fatalf("dedup: want 200, got %d", status)
	}
	if again.Code != first.Code {
		t.Fatalf("dedup: want code %s, got %s", first.Code, again.Code)
	}

	// Two clicks: the first is served from the cache warmed on create, both must be tracked.
	const clicks = 2
	for i := 0; i < clicks; i++ {
		status, location := follow(t, first.Code)
		if status != http.StatusFound || location != target {
			t.Fatalf("redirect %d: want 302 to %s, got %d to %q", i, target, status, location)
		}
	}

	d := waitForClicks(t, key, first.Code, clicks)
	if d.Target != target {
		t.Fatalf("detail: want target %s, got %s", target, d.Target)
	}
	// Summed rather than read from the last bucket, so a run across UTC midnight still passes.
	var total int64
	for _, n := range d.Series {
		total += n
	}
	if len(d.Series) != 14 || total != clicks {
		t.Fatalf("detail: want a 14-day series totalling %d, got %v", clicks, d.Series)
	}
	if len(d.Recent) != clicks || d.Recent[0].Ref != "https://e2e.example/" {
		t.Fatalf("detail: want %d recent clicks with the referer, got %+v", clicks, d.Recent)
	}

	var list []struct {
		Code   string `json:"code"`
		Clicks int64  `json:"clicks"`
	}
	if status := linkJSON(t, http.MethodGet, "/v1/links", key, "", &list); status != http.StatusOK {
		t.Fatalf("list: want 200, got %d", status)
	}
	if len(list) != 1 || list[0].Code != first.Code || list[0].Clicks != clicks {
		t.Fatalf("list: want the one link with %d clicks, got %+v", clicks, list)
	}
}

func TestE2E_LinkUnknownCodeNotFound(t *testing.T) {
	if status, _ := follow(t, "zzzzzzzzzz"); status != http.StatusNotFound {
		t.Fatalf("unknown code: want 404, got %d", status)
	}
}

func TestE2E_LinkCreateRequiresCredential(t *testing.T) {
	if status := linkJSON(t, http.MethodPost, "/v1/links", "", `{"long_url":"https://example.com"}`, nil); status != http.StatusUnauthorized {
		t.Fatalf("no key: want 401, got %d", status)
	}
}

func TestE2E_LinkDetailIsTenantScoped(t *testing.T) {
	owner, other := seedKey(t), seedKey(t)
	var c created
	body := fmt.Sprintf(`{"long_url":"https://example.com/scoped/%d"}`, time.Now().UnixNano())
	if status := linkJSON(t, http.MethodPost, "/v1/links", owner, body, &c); status != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", status)
	}
	if status := linkJSON(t, http.MethodGet, "/v1/links/"+c.Code, other, "", nil); status != http.StatusNotFound {
		t.Fatalf("other tenant: want 404, got %d", status)
	}
}
