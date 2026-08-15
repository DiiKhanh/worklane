package kafka

import (
	"context"
	"errors"
	"fmt"

	"github.com/IBM/sarama"
)

// HandlerFunc processes one message's raw value. Returning an error means "not
// handled": the message is NOT marked, so it will be redelivered (at-least-once).
type HandlerFunc func(ctx context.Context, value []byte) error

// Consumer is a sarama consumer-group wrapper for a single topic.
type Consumer struct {
	cg     sarama.ConsumerGroup
	topic  string
	handle HandlerFunc
}

// NewConsumer joins consumer group `group` and reads `topic`. OffsetOldest means a
// brand-new group replays existing messages, which is what we want for the dispatcher.
func NewConsumer(brokers []string, group, topic string, handle HandlerFunc) (*Consumer, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cg, err := sarama.NewConsumerGroup(brokers, group, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka: new consumer group: %w", err)
	}
	return &Consumer{cg: cg, topic: topic, handle: handle}, nil
}

// Start runs the consume loop until ctx is cancelled. Consume must be called in a loop
// because it returns each time the group rebalances; the loop rejoins until we stop.
func (c *Consumer) Start(ctx context.Context) error {
	h := &groupHandler{handle: c.handle}
	for {
		if err := c.cg.Consume(ctx, []string{c.topic}, h); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			return fmt.Errorf("kafka: consume: %w", err)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func (c *Consumer) Close() error { return c.cg.Close() }

// groupHandler adapts our HandlerFunc to sarama's ConsumerGroupHandler.
type groupHandler struct{ handle HandlerFunc }

func (groupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (groupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h groupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			if err := h.handle(sess.Context(), msg.Value); err != nil {
				// Do not mark on failure: the offset is not advanced, so the message is
				// redelivered later. This is the at-least-once guarantee.
				return nil
			}
			sess.MarkMessage(msg, "")
		case <-sess.Context().Done():
			return nil
		}
	}
}
