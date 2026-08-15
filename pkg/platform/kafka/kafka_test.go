//go:build integration

package kafka_test

import (
	"context"
	"testing"
	"time"

	"github.com/IBM/sarama"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
)

func TestProduceThenConsume(t *testing.T) {
	ctx := context.Background()
	ctr, err := tcredpanda.Run(ctx, "redpandadata/redpanda:v23.3.3")
	if err != nil {
		t.Fatalf("start redpanda: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	broker, err := ctr.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatalf("seed broker: %v", err)
	}
	brokers := []string{broker}
	const topic = "otp.requested"

	// Create the topic explicitly so the test does not depend on auto-create.
	admin, err := sarama.NewClusterAdmin(brokers, sarama.NewConfig())
	if err != nil {
		t.Fatalf("cluster admin: %v", err)
	}
	if err := admin.CreateTopic(topic, &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1}, false); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	_ = admin.Close()

	prod, err := kafka.NewProducer(brokers)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	t.Cleanup(func() { _ = prod.Close() })

	want := contracts.RequestedEvent{RequestID: "r1", TenantID: "t1", Recipient: "a@b.co", Channel: "email", Code: "123456"}
	if err := prod.Publish(ctx, topic, want); err != nil {
		t.Fatalf("publish: %v", err)
	}

	received := make(chan []byte, 1)
	cons, err := kafka.NewConsumer(brokers, "test-group", topic, func(_ context.Context, v []byte) error {
		received <- v
		return nil
	})
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	consumeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = cons.Start(consumeCtx) }()

	select {
	case raw := <-received:
		var got contracts.RequestedEvent
		msgType, err := kafka.Unwrap(raw, &got)
		if err != nil {
			t.Fatalf("unwrap: %v", err)
		}
		if got != want {
			t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
		}
		if msgType != "otp.RequestedEvent" {
			t.Fatalf("unexpected msg_type discriminator: %q", msgType)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the consumer to receive the event")
	}
	cancel()
	_ = cons.Close()
}
