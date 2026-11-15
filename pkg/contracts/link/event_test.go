package link_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
)

func TestClickedEventWireContract(t *testing.T) {
	event := link.ClickedEvent{
		Code: "aZ1", TenantID: "tenant-1", TS: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
		ReferrerOrigin: "https://example.com", Device: link.DeviceMobile,
	}
	want := `{"code":"aZ1","tenant_id":"tenant-1","ts":"2026-11-15T12:00:00Z","referrer_origin":"https://example.com","device":"mobile"}`
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected map[string]any
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("wire contract = %s, want %s", raw, want)
	}
	var got link.ClickedEvent
	if err := json.Unmarshal([]byte(want), &got); err != nil || !reflect.DeepEqual(got, event) {
		t.Fatalf("decode = %+v, %v", got, err)
	}
	if event.PartitionKey() != "aZ1" {
		t.Fatal("events for a link must share a partition")
	}
}
