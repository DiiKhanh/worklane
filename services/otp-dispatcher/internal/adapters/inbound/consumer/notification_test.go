package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	platformkafka "github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

type fakeDeliverer struct {
	got []contracts.RequestedEvent
	err error
}

func (f *fakeDeliverer) Handle(_ context.Context, evt contracts.RequestedEvent) error {
	f.got = append(f.got, evt)
	return f.err
}

func wrap(t *testing.T, evt contracts.RequestedEvent) []byte {
	t.Helper()
	raw, err := platformkafka.Wrap(evt)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNotificationHandle_DecodesAndDispatches(t *testing.T) {
	f := &fakeDeliverer{}
	evt := contracts.RequestedEvent{NotificationID: "n-1", TenantID: "t-1", Channel: "sms", Variables: map[string]string{"a": "b"}}
	if err := NewNotification(f).Handle(context.Background(), wrap(t, evt)); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 || f.got[0].NotificationID != "n-1" || f.got[0].Variables["a"] != "b" {
		t.Fatalf("got %+v", f.got)
	}
}

func TestNotificationHandle_DropsPoisonMessages(t *testing.T) {
	f := &fakeDeliverer{err: fmt.Errorf("%w: no log row", app.ErrInvalidNotification)}
	h := NewNotification(f)
	if err := h.Handle(context.Background(), []byte("not json")); err != nil {
		t.Fatalf("undecodable: %v", err)
	}
	if len(f.got) != 0 {
		t.Fatal("undecodable message must not reach the handler")
	}
	if err := h.Handle(context.Background(), wrap(t, contracts.RequestedEvent{NotificationID: "n-1"})); err != nil {
		t.Fatalf("invalid event: %v", err)
	}
}

func TestNotificationHandle_ReturnsInfrastructureErrors(t *testing.T) {
	boom := errors.New("db down")
	f := &fakeDeliverer{err: boom}
	err := NewNotification(f).Handle(context.Background(), wrap(t, contracts.RequestedEvent{NotificationID: "n-1"}))
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
