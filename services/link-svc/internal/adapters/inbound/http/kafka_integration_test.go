//go:build integration

package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/kafkabus"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

func TestHTTPClickReachesIsolatedKafkaWithSanitizedPayload(t *testing.T) {
	brokers := isolatedKafka(t)
	config := sarama.NewConfig()
	admin, err := sarama.NewClusterAdmin(brokers, config)
	if err != nil {
		t.Fatal(err)
	}
	err = admin.CreateTopic("link.clicked", &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1}, false)
	_ = admin.Close()
	var topicError *sarama.TopicError
	if err != nil && (!errors.As(err, &topicError) || topicError.Err != sarama.ErrTopicAlreadyExists) {
		t.Fatal(err)
	}
	consumer, err := sarama.NewConsumer(brokers, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = consumer.Close() })
	partition, err := consumer.ConsumePartition("link.clicked", 0, sarama.OffsetNewest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = partition.Close() })
	publisher, err := kafkabus.New(brokers)
	if err != nil {
		t.Fatal(err)
	}
	emitter := app.NewEmitter(publisher, 4)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := emitter.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := publisher.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	f := newAPI(t, 100, time.Minute)
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", f.jwt(t, f.tenant), linkhttp.CreateRequest{LongURL: "https://example.com/?destination=private"}), 201)
	cache, err := redisstore.New(f.redis, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(f.repo, cache, time.Now), emitter, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cases := []struct{ referrer, wantOrigin string }{
		{"https://user:secret@REF.example:8443/private?token=sensitive#fragment", "https://ref.example:8443"},
		{"https://例.com/private?token=sensitive", ""},
		{"https://" + strings.Repeat("x", 505) + ".example/private?token=sensitive", ""},
	}
	for _, tc := range cases {
		for _, method := range []string{"HEAD", "GET"} {
			req, err := http.NewRequest(method, server.URL+"/"+created.Code, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Referer", tc.referrer)
			req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone) Mobile/raw-metadata")
			req.Header.Set("X-Forwarded-For", "192.0.2.12")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != 302 {
				t.Fatalf("%s returned %d", method, response.StatusCode)
			}
		}
		select {
		case message := <-partition.Messages():
			var event link.ClickedEvent
			kind, err := kafka.Unwrap(message.Value, &event)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "link.ClickedEvent" || string(message.Key) != created.Code || event.Code != created.Code || event.TenantID != f.tenant || event.ReferrerOrigin != tc.wantOrigin || event.Device != link.DeviceMobile || event.TS.IsZero() {
				t.Fatalf("incorrect Kafka event: %+v", event)
			}
			var envelope kafka.Envelope
			if err := json.Unmarshal(message.Value, &envelope); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(envelope.Data, &fields); err != nil || len(fields) != 5 {
				t.Fatal("unexpected event fields reached Kafka")
			}
			for _, forbidden := range []string{"secret", "private", "sensitive", "fragment", "raw-metadata", "192.0.2.12", "user_agent", "ip_hash"} {
				if strings.Contains(string(message.Value), forbidden) {
					t.Fatalf("raw request metadata reached Kafka: %s", forbidden)
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("sanitized GET click did not reach Kafka")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := emitter.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if stats := emitter.Stats(); stats.Published != uint64(len(cases)) || stats.Failed != 0 {
		t.Fatalf("HEAD emitted a click or GET publication failed: %+v", stats)
	}
}

func isolatedKafka(t *testing.T) []string {
	t.Helper()
	if address := os.Getenv("LINK_TEST_KAFKA_BROKERS"); address != "" {
		return strings.Split(address, ",")
	}
	container, err := tcredpanda.Run(context.Background(), "redpandadata/redpanda:v23.3.3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	broker, err := container.KafkaSeedBroker(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return []string{broker}
}
