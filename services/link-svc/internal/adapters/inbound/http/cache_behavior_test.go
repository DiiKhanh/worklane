package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

func publicRouter(t *testing.T, repo app.CodeReader, cache app.MappingCache, now func() time.Time) http.Handler {
	t.Helper()
	router, err := linkhttp.NewRouter(nil, nil, nil, "https://short.example", app.NewResolver(repo, cache, now), nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func publicStatus(router http.Handler, code string) int {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("HEAD", "/"+code, nil))
	return response.Code
}

func TestCachedStateCannotOutliveReadDeadlineDuringDatabaseOutage(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "disabled"}[disabled], func(t *testing.T) {
			cache, _ := redirectCache(t)
			start := time.Now()
			now := start
			down := false
			db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
				if down {
					return domain.Link{}, domain.ErrUnavailable
				}
				row := domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com"}
				if disabled {
					row.DisabledAt = &start
				}
				return row, nil
			}}
			router := publicRouter(t, db, cache, func() time.Time { return now })
			want := 302
			if disabled {
				want = 410
			}
			if got := publicStatus(router, "G"); got != want {
				t.Fatalf("initial status %d, want %d", got, want)
			}
			down = true
			now = start.Add(59 * time.Second)
			if got := publicStatus(router, "G"); got != want {
				t.Fatalf("unexpired cache during outage: %d", got)
			}
			now = start.Add(time.Minute)
			if got := publicStatus(router, "G"); got != 503 {
				t.Fatalf("expired cache must fail closed at 60 seconds: %d", got)
			}
		})
	}
}

func TestConcurrentStaleRefillAfterDisableDoesNotExtendDeadline(t *testing.T) {
	cache, _ := redirectCache(t)
	start := time.Now()
	var elapsed atomic.Int64
	now := func() time.Time { return start.Add(time.Duration(elapsed.Load())) }
	readStarted, finishRead := make(chan struct{}), make(chan struct{})
	var reads atomic.Int64
	var down atomic.Bool
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		if down.Load() {
			return domain.Link{}, domain.ErrUnavailable
		}
		row := domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com"}
		if reads.Add(1) == 1 {
			// The database captured active state before a concurrent disable.
			close(readStarted)
			<-finishRead
			return row, nil
		}
		disabled := start.Add(time.Second)
		row.DisabledAt = &disabled
		return row, nil
	}}
	router := publicRouter(t, db, cache, now)
	result := make(chan int, 1)
	go func() { result <- publicStatus(router, "G") }()
	<-readStarted
	elapsed.Store(int64(time.Second))
	// A second reader observes durable disable and populates the cache first.
	if got := publicStatus(router, "G"); got != 410 {
		t.Fatalf("disabled state: %d", got)
	}
	elapsed.Store(int64(59 * time.Second))
	close(finishRead)
	if got := <-result; got != 302 {
		t.Fatalf("active snapshot may serve only inside its lease: %d", got)
	}
	down.Store(true)
	elapsed.Store(int64(time.Minute))
	if got := publicStatus(router, "G"); got != 503 {
		t.Fatalf("late active refill extended validity: %d", got)
	}
	down.Store(false)
	if got := publicStatus(router, "G"); got != 410 {
		t.Fatalf("authoritative disable must recover: %d", got)
	}
}

func TestSlowAuthoritativeReadCannotReturnExpiredActiveSnapshot(t *testing.T) {
	cache, _ := redirectCache(t)
	now := time.Now()
	db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
		now = now.Add(time.Minute)
		return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com"}, nil
	}}
	if got := publicStatus(publicRouter(t, db, cache, func() time.Time { return now }), "G"); got != 503 {
		t.Fatalf("slow read must not serve an expired snapshot: %d", got)
	}
}

func TestRedisFailureAndCorruptionFallBackToAuthoritativeState(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		cache, client := redirectCache(t)
		if unavailable {
			_ = client.Close()
		} else if err := client.Set(context.Background(), "link:mapping:G", "broken JSON", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		db := redirectDB{read: func(context.Context, string) (domain.Link, error) {
			return domain.Link{Code: "G", TenantID: "owner", LongURL: "https://example.com"}, nil
		}}
		if got := publicStatus(publicRouter(t, db, cache, time.Now), "G"); got != 302 {
			t.Fatalf("Redis failure must use MySQL: %d", got)
		}
	}
}
