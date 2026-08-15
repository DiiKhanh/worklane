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

// Verify checks a submitted code. It enforces the attempt lock (anti brute-force),
// compares in constant time, and on success deletes the code (single-use) and marks the
// request verified.
//
// Returns: nil on success; ErrNotFound if no active code (expired/absent);
// ErrTooManyAttempts once the attempt cap is reached; ErrCodeMismatch on a wrong guess.
func (s *Service) Verify(ctx context.Context, in VerifyInput) error {
	key := codeKey(in.TenantID, in.Recipient)
	rec, err := s.d.Store.Get(ctx, key)
	if err != nil {
		return err // ErrNotFound when the code expired or never existed
	}
	if rec.Attempts >= s.cfg.MaxVerifyAttempts {
		return domain.ErrTooManyAttempts
	}
	if !domain.VerifyHash(rec.Hash, in.Code, rec.Salt) {
		// Count the failed attempt and persist it, so the cap survives across requests.
		// The wrong guess itself always returns a mismatch (the caller used a valid try);
		// the lock is enforced by the guard above on the NEXT call once the cap is hit -
		// after which even the correct code is rejected.
		rec.Attempts++
		_ = s.d.Store.Save(ctx, key, rec, s.cfg.TTL)
		return domain.ErrCodeMismatch
	}
	// Success: delete the code so it cannot be reused, then record the verified state.
	_ = s.d.Store.Delete(ctx, key)
	return s.d.Repo.UpdateState(ctx, rec.Salt, contracts.StateVerified)
}
