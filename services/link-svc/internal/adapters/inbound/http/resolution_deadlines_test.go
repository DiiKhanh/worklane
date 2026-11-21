package http_test

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type stalledCache struct{}

func (stalledCache) Load(ctx context.Context, _ string) (app.CachedMapping, error) {
	<-ctx.Done()
	return app.CachedMapping{}, ctx.Err()
}
func (stalledCache) Store(ctx context.Context, _ app.CachedMapping) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestStalledRedisCannotConsumeRedirectBudget(t *testing.T) {
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com/"}, nil
	}}
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, stalledCache{}, time.Now), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response := httptest.NewRecorder()
	started := time.Now()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/G", nil).WithContext(ctx))
	if response.Code != 302 || time.Since(started) >= 750*time.Millisecond {
		t.Fatalf("Redis consumed redirect budget: status %d, elapsed %s", response.Code, time.Since(started))
	}
}

func TestConcurrentDisableAndLateRefillCannotExtendDeadlineDuringDBOutage(t *testing.T) {
	cache, _ := redirectCache(t)
	start := time.Now()
	var seconds atomic.Int64
	now := func() time.Time { return start.Add(time.Duration(seconds.Load()) * time.Second) }
	readStarted, finishRead := make(chan struct{}), make(chan struct{})
	var reads atomic.Int64
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		if reads.Add(1) == 1 {
			close(readStarted)
			<-finishRead
			return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com/"}, nil
		}
		return domain.Link{}, domain.ErrUnavailable
	}}
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(db, cache, now), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/G", nil))
		first <- response.Code
	}()
	<-readStarted
	// A disable commits and evicts while an older authoritative read is paused.
	seconds.Store(10)
	if err := cache.Evict(context.Background(), "G"); err != nil {
		t.Fatal(err)
	}
	seconds.Store(59)
	close(finishRead)
	if status := <-first; status != 302 {
		t.Fatalf("unexpired original read: %d", status)
	}
	for _, instant := range []int64{59, 60, 70, 119} {
		seconds.Store(instant)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/G", nil))
		want := 503
		if instant == 59 {
			want = 302
		}
		if response.Code != want {
			t.Fatalf("at +%ds: want %d, got %d; late refill extended validity", instant, want, response.Code)
		}
	}
}
