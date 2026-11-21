package http_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/kafkabus"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type unavailableBroker struct {
	started chan struct{}
	once    sync.Once
}

func TestHTTPRedirectsSurviveUnavailableKafka(t *testing.T) {
	cache, _ := redirectCache(t)
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com/"}, nil
	}}
	publisher, err := kafkabus.New([]string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("Kafka outage prevented construction: %v", err)
	}
	emitter := app.NewEmitter(publisher, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	t.Cleanup(func() {
		_ = emitter.Close(ctx)
		_ = publisher.Close(ctx)
	})
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, cache, time.Now), emitter, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/G", nil))
	if response.Code != 302 {
		t.Fatalf("Kafka outage changed redirect status: %d", response.Code)
	}
	if err := emitter.Close(ctx); err != nil {
		t.Fatalf("Kafka outage prevented bounded drain: %v", err)
	}
	if err := publisher.Close(ctx); err != nil {
		t.Fatalf("Kafka outage prevented bounded producer shutdown: %v", err)
	}
	if stats := emitter.Stats(); stats.Failed != 1 || stats.Published != 0 {
		t.Fatalf("Kafka failure was not safely counted: %+v", stats)
	}
}

func (b *unavailableBroker) Publish(ctx context.Context, _ string, _ any) error {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return ctx.Err()
}

func TestHTTPRedirectsDoNotWaitForKafkaOrQueueCapacity(t *testing.T) {
	cache, _ := redirectCache(t)
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com/"}, nil
	}}
	broker := &unavailableBroker{started: make(chan struct{})}
	emitter := app.NewEmitter(broker, 1)
	t.Cleanup(func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = emitter.Close(ctx)
	})
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, cache, time.Now), emitter, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := func() int {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/G", nil))
		return response.Code
	}
	if request() != 302 {
		t.Fatal("initial redirect failed")
	}
	<-broker.started
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		for range 20 {
			if status := request(); status != 302 {
				t.Errorf("Kafka outage changed redirect status: %d", status)
			}
		}
	}()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("redirects waited for the stalled Kafka worker")
	}
	if stats := emitter.Stats(); stats.Overflow != 19 {
		t.Fatalf("full queue did not account for drops: %+v", stats)
	}
	metrics := httptest.NewRecorder()
	router.ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if metrics.Code != 200 || !strings.Contains(metrics.Body.String(), `link_click_dropped_total{reason="overflow"} 19`) {
		t.Fatal("overflow was not exposed in service metrics")
	}
}
