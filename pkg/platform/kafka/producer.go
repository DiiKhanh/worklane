package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
)

// Producer publishes enveloped JSON events. Its Publish method matches otp-api's
// app.Publisher port (structural typing - no import of the app package needed).
type Producer struct{ sp sarama.SyncProducer }

// NewProducer builds a synchronous producer. Sync (wait for the broker ack) keeps the
// MVP simple and gives back a real error if publishing fails, which the caller can
// surface. Idempotent/async batching is a later optimization.
func NewProducer(brokers []string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true // required by SyncProducer
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	sp, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka: new producer: %w", err)
	}
	return &Producer{sp: sp}, nil
}

// keyer lets an event choose its partition key without this package importing the
// event's type. Any event with a PartitionKey() string method opts in.
type keyer interface{ PartitionKey() string }

// Publish wraps event in an envelope and sends it to topic. If the event provides a
// PartitionKey, it becomes the message key so related events keep per-key order.
func (p *Producer) Publish(_ context.Context, topic string, event any) error {
	payload, err := Wrap(event)
	if err != nil {
		return err
	}
	msg := &sarama.ProducerMessage{Topic: topic, Value: sarama.ByteEncoder(payload)}
	if k, ok := event.(keyer); ok {
		msg.Key = sarama.StringEncoder(k.PartitionKey())
	}
	if _, _, err := p.sp.SendMessage(msg); err != nil {
		return fmt.Errorf("kafka: send to %s: %w", topic, err)
	}
	return nil
}

func (p *Producer) Close() error { return p.sp.Close() }
