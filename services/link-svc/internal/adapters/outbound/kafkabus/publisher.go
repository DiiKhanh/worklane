package kafkabus

import (
	"context"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"github.com/duykhanh/worklane/pkg/platform/kafka"
)

// Publisher uses the repository envelope with context-bounded acknowledgement.
// Its single caller is the emission worker, never an HTTP handler.
type Publisher struct {
	producer sarama.AsyncProducer
	done     chan struct{}
	close    sync.Once
}

func New(brokers []string) (*Publisher, error) {
	config := sarama.NewConfig()
	// Metadata is fetched on first publication so a Kafka outage cannot prevent
	// startup of otherwise healthy redirects. No retry can grow the backlog.
	config.Metadata.Full = false
	config.Metadata.Timeout = time.Second
	config.Metadata.Retry.Max = 0
	config.Net.DialTimeout = time.Second
	config.Net.ReadTimeout = time.Second
	config.Net.WriteTimeout = time.Second
	config.Net.MaxOpenRequests = 1
	config.Producer.Timeout = time.Second
	config.Producer.Retry.Max = 0
	config.Producer.Flush.MaxMessages = 1
	config.Producer.Retry.MaxBufferLength = 4096
	config.Producer.Retry.MaxBufferBytes = 32 * 1024 * 1024
	config.ChannelBufferSize = 1
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	producer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}
	p := &Publisher{producer: producer, done: make(chan struct{})}
	go p.results()
	return p, nil
}

func (p *Publisher) Publish(ctx context.Context, topic string, event any) error {
	data, err := kafka.Wrap(event)
	if err != nil {
		return err
	}
	result := make(chan error, 1)
	message := &sarama.ProducerMessage{Topic: topic, Value: sarama.ByteEncoder(data), Metadata: result}
	if key, ok := event.(interface{ PartitionKey() string }); ok {
		message.Key = sarama.StringEncoder(key.PartitionKey())
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.producer.Input() <- message:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-result:
		return err
	}
}

func (p *Publisher) results() {
	defer close(p.done)
	successes, failures := p.producer.Successes(), p.producer.Errors()
	for successes != nil || failures != nil {
		select {
		case message, ok := <-successes:
			if !ok {
				successes = nil
				continue
			}
			message.Metadata.(chan error) <- nil
		case failure, ok := <-failures:
			if !ok {
				failures = nil
				continue
			}
			failure.Msg.Metadata.(chan error) <- failure.Err
		}
	}
}

// Close follows Emitter.Close; no further Publish calls may start afterwards.
func (p *Publisher) Close(ctx context.Context) error {
	p.close.Do(p.producer.AsyncClose)
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
