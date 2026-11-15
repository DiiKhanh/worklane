package http_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
)

func TestManagementWireContracts(t *testing.T) {
	created := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	disabled := created.Add(time.Hour)
	metadata := linkhttp.Metadata{
		Code: "aZ1", ShortURL: "https://short.example/aZ1", LongURL: "https://example.com/?b=2&a=1#section",
		State: link.StateActive, CreatedAt: created,
	}
	activeJSON := `{"code":"aZ1","short_url":"https://short.example/aZ1","long_url":"https://example.com/?b=2\u0026a=1#section","state":"active","created_at":"2026-11-15T12:00:00Z","disabled_at":null}`
	nextCursor := "opaque"
	for _, tc := range []struct {
		name string
		body any
		want string
	}{
		{"create request", linkhttp.CreateRequest{LongURL: "https://example.com/"}, `{"long_url":"https://example.com/"}`},
		{"create and dedup response", linkhttp.CreateResponse{Code: "aZ1", ShortURL: metadata.ShortURL}, `{"code":"aZ1","short_url":"https://short.example/aZ1"}`},
		{"detail", metadata, activeJSON},
		{"list", linkhttp.ListResponse{Items: []linkhttp.Metadata{metadata}, NextCursor: &nextCursor}, `{"items":[` + activeJSON + `],"next_cursor":"opaque"}`},
		{"empty list", linkhttp.ListResponse{Items: []linkhttp.Metadata{}}, `{"items":[],"next_cursor":null}`},
		{"disabled conflict", linkhttp.ErrorResponse{Error: linkhttp.ErrorLinkDisabled}, `{"error":"LINK_DISABLED"}`},
		{"unavailable", linkhttp.ErrorResponse{Error: linkhttp.ErrorUnavailable}, `{"error":"link: unavailable"}`},
		{"health", linkhttp.HealthResponse{Status: "ok"}, `{"status":"ok"}`},
	} {
		t.Run(tc.name, func(t *testing.T) { assertJSON(t, tc.body, tc.want) })
	}
	metadata.State = link.StateDisabled
	metadata.DisabledAt = &disabled
	assertJSON(t, metadata, `{"code":"aZ1","short_url":"https://short.example/aZ1","long_url":"https://example.com/?b=2\u0026a=1#section","state":"disabled","created_at":"2026-11-15T12:00:00Z","disabled_at":"2026-11-15T13:00:00Z"}`)
}

func TestAnalyticsWireContract(t *testing.T) {
	assertJSON(t, linkhttp.AnalyticsResponse{
		TotalClicks: 12,
		Series:      []linkhttp.DailyClicks{{Date: "2026-11-15", Clicks: 2}},
		Recent:      []linkhttp.Visit{{TS: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), Device: link.DeviceUnknown}},
	}, `{"total_clicks":12,"series":[{"date":"2026-11-15","clicks":2}],"recent":[{"ts":"2026-11-15T12:00:00Z","referrer_origin":"","device":"unknown"}]}`)
	assertJSON(t, linkhttp.AnalyticsResponse{Series: []linkhttp.DailyClicks{}, Recent: []linkhttp.Visit{}}, `{"total_clicks":0,"series":[],"recent":[]}`)
}

func assertJSON(t *testing.T, body any, want string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected any
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("wire contract = %s, want %s", raw, want)
	}
}
