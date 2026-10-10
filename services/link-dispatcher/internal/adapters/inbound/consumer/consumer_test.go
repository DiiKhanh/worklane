package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/link"
	platformkafka "github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

type fakeHandler struct {
	got []contracts.ClickedEvent
	err error
}

func (f *fakeHandler) Handle(_ context.Context, evt contracts.ClickedEvent) error {
	f.got = append(f.got, evt)
	return f.err
}

func wrap(t *testing.T, evt contracts.ClickedEvent) []byte {
	t.Helper()
	raw, err := platformkafka.Wrap(evt)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	return raw
}

func TestHandleDecodesEnvelope(t *testing.T) {
	want := contracts.ClickedEvent{
		Code: "aB3xYz", TenantID: "t1", TS: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
		Referer: "https://news.example.com/", UA: "Mozilla/5.0", IPHash: "hash",
	}
	h := &fakeHandler{}
	if err := New(h).Handle(context.Background(), wrap(t, want)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(h.got) != 1 || h.got[0] != want {
		t.Fatalf("handled %+v, want [%+v]", h.got, want)
	}
}

func TestHandleDropsUndecodableMessage(t *testing.T) {
	for name, raw := range map[string][]byte{
		"not json":       []byte("not json"),
		"no data":        []byte(`{"msg_type":"link.ClickedEvent"}`),
		"wrong data":     []byte(`{"msg_type":"link.ClickedEvent","data":"text"}`),
		"bad timestamp":  []byte(`{"msg_type":"link.ClickedEvent","data":{"code":"abc","ts":"yesterday"}}`),
		"empty envelope": nil,
	} {
		t.Run(name, func(t *testing.T) {
			h := &fakeHandler{}
			if err := New(h).Handle(context.Background(), raw); err != nil {
				t.Fatalf("Handle = %v, want nil so the poison message is not redelivered", err)
			}
			if len(h.got) != 0 {
				t.Fatalf("undecodable message reached the app: %+v", h.got)
			}
		})
	}
}

func TestHandleDropsInvalidEvent(t *testing.T) {
	h := &fakeHandler{err: fmt.Errorf("%w: code \"\"", app.ErrInvalidEvent)}
	if err := New(h).Handle(context.Background(), wrap(t, contracts.ClickedEvent{})); err != nil {
		t.Fatalf("Handle = %v, want nil so the invalid event is not redelivered", err)
	}
}

func TestHandleReturnsInfrastructureError(t *testing.T) {
	boom := errors.New("boom")
	h := &fakeHandler{err: boom}
	err := New(h).Handle(context.Background(), wrap(t, contracts.ClickedEvent{Code: "abc", TenantID: "t1"}))
	if !errors.Is(err, boom) {
		t.Fatalf("Handle = %v, want the error so Kafka redelivers", err)
	}
}
