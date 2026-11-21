package kafkabus_test

import (
	"context"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/kafkabus"
)

func TestDelayedPublicationCannotPanicAfterProducerShutdown(t *testing.T) {
	publisher, err := kafkabus.New([]string{"127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := publisher.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// A worker can be preempted before Publish and resume after bounded shutdown
	// has closed the producer. Admission must reject it without a channel panic.
	if err := publisher.Publish(ctx, "link.clicked", link.ClickedEvent{Code: "G"}); err == nil {
		t.Fatal("publication was accepted after producer shutdown")
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := publisher.Publish(canceled, "link.clicked", link.ClickedEvent{Code: "G"}); err == nil {
		t.Fatal("canceled publication was accepted after producer shutdown")
	}
}
