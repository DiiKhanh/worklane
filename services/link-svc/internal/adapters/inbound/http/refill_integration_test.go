//go:build integration

package http_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type pausedSnapshot struct {
	repo            app.CodeReader
	started, resume chan struct{}
	paused          atomic.Bool
}

func (p *pausedSnapshot) ByCode(ctx context.Context, code string) (domain.Link, error) {
	row, err := p.repo.ByCode(ctx, code)
	if !p.paused.Swap(true) {
		close(p.started)
		<-p.resume
	}
	return row, err
}

func TestRealDisableStaleRefillAndDatabaseOutage(t *testing.T) {
	f := newAPI(t, 100, time.Minute)
	token := f.jwt(t, f.tenant)
	created := readResponse[linkhttp.CreateResponse](t, f.request(t, "POST", "/v1/links", token, linkhttp.CreateRequest{LongURL: "https://example.com/refill"}), 201)
	cache, err := redisstore.New(f.redis, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var elapsed atomic.Int64
	now := func() time.Time { return start.Add(time.Duration(elapsed.Load()) * time.Second) }
	reader := &pausedSnapshot{repo: f.repo, started: make(chan struct{}), resume: make(chan struct{})}
	router := publicRouter(t, reader, cache, now)
	response := make(chan int, 1)
	go func() { response <- publicStatus(router, created.Code) }()
	<-reader.started
	elapsed.Store(10)
	readResponse[linkhttp.Metadata](t, f.request(t, "POST", "/v1/links/"+created.Code+"/disable", token, nil), 200)
	// A different request caches the committed disabled state before the older
	// active snapshot refills Redis. Both reads use the real MySQL repository.
	publicResponse(t, f, "HEAD", created.Code, 410, "")
	elapsed.Store(59)
	close(reader.resume)
	if status := <-response; status != 302 {
		t.Fatalf("original snapshot should still be inside its lease: %d", status)
	}
	cached, err := cache.Load(context.Background(), created.Code)
	if err != nil || cached.Link.DisabledAt != nil || cached.ValidUntil.After(start.Add(time.Minute)) {
		t.Fatalf("stale refill did not retain its read deadline: %v", err)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, instant := range []int64{60, 70, 119} {
		elapsed.Store(instant)
		if status := publicStatus(router, created.Code); status != 503 {
			t.Fatalf("at +%ds, stale active state survived the deadline during DB outage: %d", instant, status)
		}
	}
}
