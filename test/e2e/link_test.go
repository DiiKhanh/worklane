//go:build e2e && linke2e

package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

var linkClient = &http.Client{
	Timeout: 5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type linkResponse struct {
	status int
	header http.Header
	body   []byte
}

func requestLink(ctx context.Context, method, address, key, body string, headers http.Header) (linkResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, strings.NewReader(body))
	if err != nil {
		return linkResponse{}, err
	}
	for name, values := range headers {
		req.Header[name] = values
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := linkClient.Do(req)
	if err != nil {
		return linkResponse{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return linkResponse{status: resp.StatusCode, header: resp.Header, body: data}, err
}

func callLink(t *testing.T, method, address, key, body string, status int) linkResponse {
	t.Helper()
	result, err := requestLink(t.Context(), method, address, key, body, nil)
	if err != nil {
		t.Fatalf("%s %s: %v", method, address, err)
	}
	if result.status != status {
		t.Fatalf("%s %s: got %d, want %d (%s)", method, address, result.status, status, result.body)
	}
	return result
}

func decodeLink[T any](t *testing.T, result linkResponse) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(result.body, &value); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return value
}

func isolatedLinkStack(t *testing.T) {
	t.Helper()
	u, err := url.Parse(os.Getenv("E2E_API_BASE"))
	if err != nil || u.Scheme != "http" || !loopback(u.Hostname()) {
		t.Fatal("E2E_API_BASE must explicitly select a local Compose gateway")
	}
	localDSN(t, "MYSQL_DSN", "identity")
}

func loopback(host string) bool {
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

func localDSN(t *testing.T, name, database string) string {
	t.Helper()
	raw := os.Getenv(name)
	cfg, err := mysql.ParseDSN(raw)
	if err != nil || raw == "" || cfg.Net != "tcp" || cfg.DBName != database {
		t.Fatalf("%s must specify the disposable %s test database", name, database)
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || !loopback(host) {
		t.Fatalf("%s must select a local test database", name)
	}
	return raw
}

type createdLink struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

func createLink(t *testing.T, key, body string) createdLink {
	t.Helper()
	link := decodeLink[createdLink](t, callLink(t, "POST", apiBase+"/v1/links", key, body, http.StatusCreated))
	if link.Code == "" || link.ShortURL != apiBase+"/"+link.Code {
		t.Fatalf("unexpected short URL %q for code %q", link.ShortURL, link.Code)
	}
	return link
}

type linkAnalytics struct {
	Total  int `json:"total_clicks"`
	Series []struct {
		Clicks int `json:"clicks"`
	} `json:"series"`
	Recent []struct {
		Origin string `json:"referrer_origin"`
		Device string `json:"device"`
	} `json:"recent"`
}

func TestLinkE2ELifecycle(t *testing.T) {
	isolatedLinkStack(t)
	linkDSN := localDSN(t, "LINK_E2E_MYSQL_DSN", "links")
	key, foreign := seedKey(t), seedKey(t)
	destination := "https://example.com/e2e/" + newID() + "?b=2&a=1#section"
	body := fmt.Sprintf(`{"long_url":%q}`, destination)
	callLink(t, "GET", apiBase+"/healthz", "", "", http.StatusOK)
	callLink(t, "GET", apiBase+"/v1/links", "", "", http.StatusUnauthorized)
	link := createLink(t, key, body)
	dedup := decodeLink[createdLink](t, callLink(t, "POST", apiBase+"/v1/links", key, body, http.StatusOK))
	if dedup != link {
		t.Fatalf("dedup returned a different mapping: %+v", dedup)
	}
	detail := apiBase + "/v1/links/" + link.Code
	analytics := detail + "/analytics"
	callLink(t, "GET", detail, foreign, "", http.StatusNotFound)
	callLink(t, "GET", analytics, foreign, "", http.StatusNotFound)
	callLink(t, "POST", detail+"/disable", foreign, "", http.StatusNotFound)

	head := callLink(t, "HEAD", link.ShortURL, "", "", http.StatusFound)
	if head.header.Get("Location") != destination || head.header.Get("Cache-Control") != "no-store" || len(head.body) != 0 {
		t.Fatalf("HEAD did not return the destination, no-store and an empty body: %+v", head)
	}
	zero := decodeLink[linkAnalytics](t, callLink(t, "GET", analytics, key, "", http.StatusOK))
	if zero.Total != 0 || len(zero.Recent) != 0 {
		t.Fatalf("expected zero clicks before GET, got %+v", zero)
	}
	clickAt := time.Now()
	ctx, cancel := context.WithDeadline(t.Context(), clickAt.Add(10*time.Second))
	defer cancel()
	get, err := requestLink(ctx, "GET", link.ShortURL, "", "", http.Header{
		"Referer":    {"https://referrer.example/path?private=value"},
		"User-Agent": {"Mozilla/5.0 iPhone Mobile"},
	})
	if err != nil || get.status != http.StatusFound || get.header.Get("Location") != destination || get.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET redirect: %+v, %v", get, err)
	}
	var observed linkAnalytics
	for {
		result, err := requestLink(ctx, "GET", analytics, key, "", nil)
		if err != nil || result.status != http.StatusOK {
			t.Fatalf("analytics poll: got %d, %v", result.status, err)
		}
		observed = decodeLink[linkAnalytics](t, result)
		if observed.Total > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	count := 0
	for _, day := range observed.Series {
		count += day.Clicks
	}
	if observed.Total != 1 || len(observed.Series) != 14 || count != 1 || len(observed.Recent) != 1 ||
		observed.Recent[0].Origin != "https://referrer.example" || observed.Recent[0].Device != "mobile" {
		t.Fatalf("unexpected analytics: %+v", observed)
	}
	t.Logf("GET visible after %s", time.Since(clickAt).Round(time.Millisecond))

	t.Run("metadata during analytics failure", func(t *testing.T) {
		db, err := sql.Open("mysql", linkDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.ExecContext(t.Context(), "RENAME TABLE link_clicks TO link_clicks_e2e"); err != nil {
			t.Fatalf("isolate analytics failure: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := db.ExecContext(ctx, "RENAME TABLE link_clicks_e2e TO link_clicks"); err != nil {
				t.Fatalf("restore analytics table: %v", err)
			}
		}()
		callLink(t, "GET", analytics, key, "", http.StatusServiceUnavailable)
		metadata := decodeLink[createdLink](t, callLink(t, "GET", detail, key, "", http.StatusOK))
		if metadata != link {
			t.Fatalf("copyable metadata changed: %+v", metadata)
		}
	})

	callLink(t, "POST", detail+"/disable", key, "", http.StatusOK)
	disabledAt := time.Now()
	ctx, cancelDisable := context.WithTimeout(t.Context(), time.Minute)
	defer cancelDisable()
	for {
		result, err := requestLink(ctx, "HEAD", link.ShortURL, "", "", nil)
		if err != nil {
			t.Fatalf("disable did not converge: %v", err)
		}
		if result.status == http.StatusGone {
			break
		}
		if result.status != http.StatusFound {
			t.Fatalf("unexpected status during disable: %d", result.status)
		}
		time.Sleep(100 * time.Millisecond)
	}
	gone, err := requestLink(ctx, "GET", link.ShortURL, "", "", nil)
	if err != nil || gone.status != http.StatusGone || gone.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("disabled GET: got %d, %v", gone.status, err)
	}
	t.Logf("GET returned 410 after %s", time.Since(disabledAt).Round(time.Millisecond))
	conflict := decodeLink[struct{ Error string }](t, callLink(t, "POST", apiBase+"/v1/links", key, body, http.StatusConflict))
	if conflict.Error != "LINK_DISABLED" {
		t.Fatalf("unexpected disabled conflict: %q", conflict.Error)
	}
	after := decodeLink[linkAnalytics](t, callLink(t, "GET", analytics, key, "", http.StatusOK))
	if after.Total != 1 {
		t.Fatalf("expected one historical click after disable, got %d", after.Total)
	}
}

func TestLinkE2EQuotaCORS(t *testing.T) {
	isolatedLinkStack(t)
	key := seedKey(t)
	body := fmt.Sprintf(`{"long_url":"https://example.com/quota/%s"}`, newID())
	createLink(t, key, body)
	address := apiBase + "/v1/links"
	for i := 0; i < 9; i++ {
		callLink(t, "POST", address, key, body, http.StatusOK)
	}
	const origin = "http://127.0.0.1:19781"
	result, err := requestLink(t.Context(), "POST", address, key, body, http.Header{"Origin": {origin}})
	if err != nil || result.status != http.StatusTooManyRequests {
		t.Fatalf("quota: got %d, %v", result.status, err)
	}
	retry, err := strconv.Atoi(result.header.Get("Retry-After"))
	allowOrigin := result.header.Get("Access-Control-Allow-Origin")
	if err != nil || retry < 1 || retry > 60 || (allowOrigin != "*" && allowOrigin != origin) ||
		!strings.EqualFold(result.header.Get("Access-Control-Expose-Headers"), "Retry-After") {
		t.Fatalf("quota retry guidance is unavailable to the browser: %v", result.header)
	}
}
