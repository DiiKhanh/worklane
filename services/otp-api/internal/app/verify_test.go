package app

import (
	"context"
	"testing"

	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

func TestVerify_ConcurrentAttemptsRespectCap(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newSvc()
	res, err := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	in := VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: wrongCode(res.Code)}

	const total = 12
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
	if compared != svc.cfg.MaxVerifyAttempts {
		t.Fatalf("compared %d wrong codes, want %d comparisons", compared, svc.cfg.MaxVerifyAttempts)
	}
}

func TestVerify_LastAttemptCanSucceedOnce(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newSvc()
	res, err := svc.Send(ctx, SendInput{TenantID: "t1", Recipient: "a@b.co"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	in := VerifyInput{TenantID: "t1", Recipient: "a@b.co", Code: wrongCode(res.Code)}
	for range svc.cfg.MaxVerifyAttempts - 1 {
		if err := svc.Verify(ctx, in); err != domain.ErrCodeMismatch {
			t.Fatalf("wrong code: got %v, want ErrCodeMismatch", err)
		}
	}
	in.Code = res.Code
	if err := svc.Verify(ctx, in); err != nil {
		t.Fatalf("correct code on last attempt: %v", err)
	}
	if got := repo.states[res.RequestID]; got != string(domain.StateVerified) {
		t.Fatalf("request state = %q, want verified", got)
	}
	if err := svc.Verify(ctx, in); err != domain.ErrNotFound {
		t.Fatalf("reuse: got %v, want ErrNotFound", err)
	}
}
