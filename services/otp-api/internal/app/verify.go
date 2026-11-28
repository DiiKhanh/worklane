package app

import (
	"context"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// VerifyInput is a verification attempt against the active code for (tenant, recipient).
type VerifyInput struct {
	TenantID  string
	Recipient string
	Code      string
}

// Verify reserves an attempt before comparing a submitted code in constant time.
// Every comparison, including a match on the last allowed attempt, uses one slot.
// On a match, Verify consumes the code and marks the request verified.
//
// A missing or expired code returns ErrNotFound. A request beyond the attempt cap
// returns ErrTooManyAttempts. A compared code that does not match returns ErrCodeMismatch.
func (s *Service) Verify(ctx context.Context, in VerifyInput) error {
	key := codeKey(in.TenantID, in.Recipient)
	rec, err := s.d.Store.ReserveAttempt(ctx, key, s.cfg.MaxVerifyAttempts)
	if err != nil {
		return err
	}
	if !domain.VerifyHash(rec.Hash, in.Code, rec.Salt) {
		return domain.ErrCodeMismatch
	}
	if err := s.d.Store.Consume(ctx, key, rec.Salt); err != nil {
		return err
	}
	return s.d.Repo.UpdateState(ctx, rec.Salt, contracts.StateVerified)
}
