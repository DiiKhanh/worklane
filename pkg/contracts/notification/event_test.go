package notification

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestedEventWireShape(t *testing.T) {
	e := RequestedEvent{
		NotificationID: "n-1", TenantID: "t-1", Channel: ChannelEmail, Recipient: "a@b.co",
		TemplateID: "tpl-1", Variables: map[string]string{"name": "An"}, Kind: KindMarketing, UserRef: "u-1",
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"notification_id":"n-1","tenant_id":"t-1","channel":"email","recipient":"a@b.co",` +
		`"template_id":"tpl-1","variables":{"name":"An"},"kind":"marketing","user_ref":"u-1"}`
	if string(b) != want {
		t.Fatalf("wire shape:\n got %s\nwant %s", b, want)
	}
	var back RequestedEvent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.NotificationID != e.NotificationID || back.Variables["name"] != "An" {
		t.Fatalf("round trip lost data: %+v", back)
	}
}

func TestPartitionKeyIsNotificationID(t *testing.T) {
	if k := (RequestedEvent{NotificationID: "n-1"}).PartitionKey(); k != "n-1" {
		t.Fatalf("requested: got %q", k)
	}
	if k := (SentEvent{NotificationID: "n-1"}).PartitionKey(); k != "n-1" {
		t.Fatalf("sent: got %q", k)
	}
	if k := (FailedEvent{NotificationID: "n-1"}).PartitionKey(); k != "n-1" {
		t.Fatalf("failed: got %q", k)
	}
}

// Downstream topics must never carry the raw recipient or the template variables.
func TestDownstreamEventsOmitRecipientAndVariables(t *testing.T) {
	for name, e := range map[string]any{
		"sent":   SentEvent{NotificationID: "n-1", TenantID: "t-1", Channel: ChannelSMS, Provider: "twilio"},
		"failed": FailedEvent{NotificationID: "n-1", TenantID: "t-1", Channel: ChannelSMS, Provider: "twilio", Error: "boom"},
	} {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"recipient", "variables"} {
			if strings.Contains(string(b), field) {
				t.Fatalf("%s event leaks %q: %s", name, field, b)
			}
		}
	}
}

func TestValidChannelAndKind(t *testing.T) {
	for _, c := range []string{ChannelEmail, ChannelSMS} {
		if !ValidChannel(c) {
			t.Fatalf("channel %q should be valid", c)
		}
	}
	for _, c := range []string{"", "push", "EMAIL"} {
		if ValidChannel(c) {
			t.Fatalf("channel %q should be invalid", c)
		}
	}
	for _, k := range []string{KindTransactional, KindMarketing} {
		if !ValidKind(k) {
			t.Fatalf("kind %q should be valid", k)
		}
	}
	for _, k := range []string{"", "promo"} {
		if ValidKind(k) {
			t.Fatalf("kind %q should be invalid", k)
		}
	}
}
