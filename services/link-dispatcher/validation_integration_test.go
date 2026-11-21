//go:build integration

package main_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/inbound/consumer"
)

func TestInvalidEventsAreSkippedWithoutRetainingRawMetadata(t *testing.T) {
	f := newIngestion(t)
	h, stop := f.start(t)
	t.Cleanup(stop)
	base := link.ClickedEvent{TenantID: "owner", Code: "a", TS: f.now.Add(-time.Second), Device: link.DeviceUnknown}
	var invalid [][]byte
	for _, change := range []func(*link.ClickedEvent){
		func(e *link.ClickedEvent) { e.Code = "invalid!" },
		func(e *link.ClickedEvent) { e.Code = "00" },
		func(e *link.ClickedEvent) { e.Code = "ZZZZZZZZZZZ" },
		func(e *link.ClickedEvent) { e.Code = "A" }, // Case-sensitive absent code.
		func(e *link.ClickedEvent) { e.TenantID = "foreign" },
		func(e *link.ClickedEvent) { e.TenantID = "" },
		func(e *link.ClickedEvent) { e.TenantID = strings.Repeat("x", 37) },
		func(e *link.ClickedEvent) { e.TS = time.Time{} },
		func(e *link.ClickedEvent) { e.TS = f.now.Add(time.Hour) },
		func(e *link.ClickedEvent) { e.Device = "raw-user-agent" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://user:secret@example.com" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://example.com/private" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://example.com?secret=raw" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://example.com#private" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://example.com:99999" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://example.com:" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://例.com" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "https://" + strings.Repeat("x", 513) },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "javascript:raw" },
		func(e *link.ClickedEvent) { e.ReferrerOrigin = "/relative" },
	} {
		event := base
		change(&event)
		value, err := kafka.Wrap(event)
		if err != nil {
			t.Fatal(err)
		}
		invalid = append(invalid, value)
	}
	invalid = append(invalid, []byte("{"), []byte("null"), []byte(strings.Repeat("x", 3000)))
	for _, field := range []string{"user_agent", "ip", "ip_hash", "referrer"} {
		value, err := json.Marshal(map[string]any{"msg_type": "link.ClickedEvent", "data": map[string]any{"code": "a", "tenant_id": "owner", "ts": base.TS, "device": "unknown", "referrer_origin": "", field: "sensitive-raw-metadata"}})
		if err != nil {
			t.Fatal(err)
		}
		invalid = append(invalid, value)
	}
	invalid = append(invalid, []byte(`{"msg_type":"other.Event","data":{}}`), []byte(`{"msg_type":"link.ClickedEvent","data":{},"ip":"private"}`))
	for _, value := range invalid {
		if _, _, err := f.producer.SendMessage(&sarama.ProducerMessage{Topic: f.topic, Key: sarama.StringEncoder("a"), Value: sarama.ByteEncoder(value)}); err != nil {
			t.Fatal(err)
		}
	}
	last := f.publish(t, base)
	eventually(t, func() bool { return f.progress(t) == last+1 })
	if total, details := f.counts(t); total != 1 || details != 1 {
		t.Fatalf("invalid events retained: %d %d", total, details)
	}
	if rejected, failed := h.Counts(); rejected != uint64(len(invalid)) || failed != 0 {
		t.Fatalf("rejected=%d failures=%d", rejected, failed)
	}
}

func TestEventTimeUsesUTCAndTopicRetentionIsBounded(t *testing.T) {
	f := newIngestion(t)
	if err := consumer.EnsureTopic(f.brokers, f.topic); err != nil {
		t.Fatal(err)
	}
	entries, err := f.admin.DescribeConfig(sarama.ConfigResource{Type: sarama.TopicResource, Name: f.topic})
	if err != nil {
		t.Fatal(err)
	}
	settings := make(map[string]string)
	for _, entry := range entries {
		settings[entry.Name] = entry.Value
	}
	for key, want := range map[string]string{"cleanup.policy": "delete", "retention.ms": "604800000", "segment.ms": "3600000", "file.delete.delay.ms": "60000", "message.timestamp.type": "LogAppendTime"} {
		if settings[key] != want {
			t.Fatalf("%s=%s, want %s", key, settings[key], want)
		}
	}
	_, stop := f.start(t)
	t.Cleanup(stop)
	event := link.ClickedEvent{TenantID: "owner", Code: "a", TS: time.Date(2026, 11, 21, 5, 29, 59, 123000000, time.FixedZone("+0530", 19800)), ReferrerOrigin: "http://[2001:db8::1]:8443", Device: link.DeviceTablet}
	last := f.publish(t, event)
	eventually(t, func() bool { return f.progress(t) == last+1 })
	var day string
	var ts time.Time
	if err := f.db.QueryRow("SELECT DATE_FORMAT(day, '%Y-%m-%d') FROM link_daily_clicks").Scan(&day); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT ts FROM link_clicks").Scan(&ts); err != nil {
		t.Fatal(err)
	}
	if day != "2026-11-20" || ts.Format(time.RFC3339Nano) != "2026-11-20T23:59:59.123Z" {
		t.Fatalf("UTC boundary: day=%s ts=%s", day, ts)
	}
}
