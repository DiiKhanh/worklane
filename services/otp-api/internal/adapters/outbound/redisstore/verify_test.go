//go:build integration

package redisstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

func TestVerify_ConcurrentAttemptsRespectCap(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()
	const key = "otp:code:t1:a@b.co"
	const requestID = "req-1"
	rec := app.CodeRecord{Salt: requestID, Hash: domain.HashCode("111111", requestID)}
	if err := store.Save(ctx, key, rec, time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	const total, maxAttempts = 12, 3
	svc := app.New(app.Deps{Store: store}, app.Config{MaxVerifyAttempts: maxAttempts})
	in := app.VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: "000000"}
	start := make(chan struct{})
	results := make(chan error, total)
	for range total {
		go func() {
			<-start
			results <- svc.Verify(ctx, in)
		}()
	}
	close(start)

	compared := 0
	for range total {
		switch err := <-results; err {
		case domain.ErrCodeMismatch:
			compared++
		case domain.ErrTooManyAttempts:
		default:
			t.Errorf("verify: %v", err)
		}
	}
	if compared != maxAttempts {
		t.Fatalf("compared %d wrong codes, want %d comparisons", compared, maxAttempts)
	}
	record, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if record.Attempts != maxAttempts {
		t.Fatalf("stored attempts = %d, want %d", record.Attempts, maxAttempts)
	}
}
