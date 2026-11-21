//go:build integration

package http_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestAnalyticsZeroOwnershipAndUnavailable(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: "https://example.com/analytics"}), 201)
	path := "/v1/links/" + created.Code + "/analytics"
	readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", path, "", nil), 401)
	zero := readResponse[linkhttp.AnalyticsResponse](t, f.request(t, "GET", path, token, nil), 200)
	if zero.TotalClicks != 0 || len(zero.Series) != 14 || zero.Recent == nil || len(zero.Recent) != 0 {
		t.Fatalf("incorrect empty analytics: %+v", zero)
	}
	for _, day := range zero.Series {
		if day.Clicks != 0 {
			t.Fatal("empty day is nonzero")
		}
	}
	for _, code := range []string{created.Code, "missing", "invalid!"} {
		absent := readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", "/v1/links/"+code+"/analytics", f.jwt(t, f.other), nil), 404)
		if absent.Error != linkhttp.ErrorNotFound {
			t.Fatal(absent)
		}
	}
	// Break analytics independently of metadata, preserving the mapping tables.
	if _, err := f.db.Exec("DROP TABLE link_clicks"); err != nil {
		t.Fatal(err)
	}
	unavailable := readResponse[linkhttp.ErrorResponse](t, f.request(t, "GET", path, token, nil), 503)
	if unavailable.Error != linkhttp.ErrorUnavailable {
		t.Fatal(unavailable)
	}
	readResponse[linkhttp.Metadata](t, f.request(t, "GET", "/v1/links/"+created.Code, token, nil), 200)
}

func TestAnalyticsLifetimeUTCSeriesNewestTwentyAndDisabledHistory(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	f.analyticsNow = time.Date(2026, 11, 21, 0, 0, 1, 0, time.UTC).In(time.FixedZone("-0700", -7*60*60))
	token := f.jwt(t, f.tenant)
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: "https://example.com/history"}), 201)
	for _, day := range []struct {
		date   string
		clicks int
	}{{"2026-09-01", 3}, {"2026-11-07", 7}, {"2026-11-08", 2}, {"2026-11-20", 4}, {"2026-11-21", 5}} {
		if _, err := f.db.Exec("INSERT INTO link_daily_clicks (tenant_id, code, day, clicks) VALUES (?, ?, ?, ?)", f.tenant, created.Code, day.date, day.clicks); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 25; i++ {
		// The final two visits share a timestamp, exercising the insertion-ID tie.
		ts := f.analyticsNow.UTC().Add(-time.Second).Add(time.Duration(min(i, 23)) * time.Millisecond)
		if _, err := f.db.Exec("INSERT INTO link_clicks (tenant_id, code, ts, referrer_origin, device) VALUES (?, ?, ?, ?, 'desktop')", f.tenant, created.Code, ts, fmt.Sprintf("https://visit-%d.example", i)); err != nil {
			t.Fatal(err)
		}
	}
	// These rows remain physically present until cleanup, but are expired for reads.
	for _, ts := range []time.Time{f.analyticsNow.UTC().Add(-30 * 24 * time.Hour), f.analyticsNow.UTC().Add(-31 * 24 * time.Hour)} {
		if _, err := f.db.Exec("INSERT INTO link_clicks (tenant_id, code, ts, device) VALUES (?, ?, ?, 'unknown')", f.tenant, created.Code, ts); err != nil {
			t.Fatal(err)
		}
	}
	path := "/v1/links/" + created.Code + "/analytics"
	response := f.request(t, "GET", path, f.apiKey, nil)
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("analytics can be cached across tenants")
	}
	got := readResponse[linkhttp.AnalyticsResponse](t, response, 200)
	wantSeries := []linkhttp.DailyClicks{{Date: "2026-11-08", Clicks: 2}, {Date: "2026-11-09"}, {Date: "2026-11-10"}, {Date: "2026-11-11"}, {Date: "2026-11-12"}, {Date: "2026-11-13"}, {Date: "2026-11-14"}, {Date: "2026-11-15"}, {Date: "2026-11-16"}, {Date: "2026-11-17"}, {Date: "2026-11-18"}, {Date: "2026-11-19"}, {Date: "2026-11-20", Clicks: 4}, {Date: "2026-11-21", Clicks: 5}}
	if got.TotalClicks != 21 || !reflect.DeepEqual(got.Series, wantSeries) || len(got.Recent) != 20 {
		t.Fatalf("incorrect analytics: %+v", got)
	}
	if got.Recent[0].ReferrerOrigin != "https://visit-24.example" || got.Recent[1].ReferrerOrigin != "https://visit-23.example" || got.Recent[19].ReferrerOrigin != "https://visit-5.example" || got.Recent[0].TS.Format(time.RFC3339Nano) != "2026-11-21T00:00:00.023Z" {
		t.Fatalf("incorrect recent ordering: %+v", got.Recent)
	}
	readResponse[linkhttp.Metadata](t, f.request(t, "POST", "/v1/links/"+created.Code+"/disable", token, nil), 200)
	disabled := readResponse[linkhttp.AnalyticsResponse](t, f.request(t, "GET", path, token, nil), 200)
	if !reflect.DeepEqual(got, disabled) {
		t.Fatal("disable changed analytics")
	}
	// After details expire, lifetime aggregates remain available on an old link.
	f.analyticsNow = f.analyticsNow.AddDate(0, 0, 31)
	old := readResponse[linkhttp.AnalyticsResponse](t, f.request(t, "GET", path, token, nil), 200)
	if old.TotalClicks != 21 || len(old.Recent) != 0 {
		t.Fatalf("old link lost totals: %+v", old)
	}
	for _, day := range old.Series {
		if day.Clicks != 0 {
			t.Fatal("old series is nonzero")
		}
	}
}
