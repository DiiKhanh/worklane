package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// SendInput is the request to issue an OTP. IdempotencyKey is optional; when present a
// repeated call returns the original request instead of issuing a new code.
type SendInput struct {
	TenantID       string
	Recipient      string
	IdempotencyKey string
}

// SendResult carries the new request id and the plaintext code. Code is returned only
// so the caller can hand it to the dispatcher via the Kafka event; it is never logged.
type SendResult struct {
	RequestID string
	Code      string
}

func codeKey(tenantID, recipient string) string {
	return fmt.Sprintf("otp:code:%s:%s", tenantID, recipient)
}

// Send issues an OTP: idempotency check, rate limit, generate code, store hash+TTL,
// insert the audit row, and publish otp.requested for the dispatcher to deliver.
func (s *Service) Send(ctx context.Context, in SendInput) (SendResult, error) {
	// Idempotency: a repeated key returns the prior request without re-publishing, so a
	// client retrying a timed-out request does not send a second email.
	if in.IdempotencyKey != "" {
		idemKey := fmt.Sprintf("otp:idem:%s:%s", in.TenantID, in.IdempotencyKey)
		exists, err := s.d.Counter.Exists(ctx, idemKey)
		if err != nil {
			return SendResult{}, err
		}
		if exists {
			rec, err := s.d.Store.Get(ctx, codeKey(in.TenantID, in.Recipient))
			if err != nil {
				return SendResult{}, err
			}
			// The stored salt equals the request id (set below), so we can return the
			// original id without a second lookup table.
			return SendResult{RequestID: rec.Salt}, nil
		}
		if err := s.d.Counter.Set(ctx, idemKey, s.cfg.TTL); err != nil {
			return SendResult{}, err
		}
	}

	if err := s.rl.CheckAndCount(ctx, in.TenantID, in.Recipient); err != nil {
		return SendResult{}, err
	}

	code, err := domain.GenerateCode(s.cfg.CodeLength)
	if err != nil {
		return SendResult{}, err
	}
	requestID := newID()
	salt := requestID // reuse the request id as the per-request salt (unique per request)
	rec := CodeRecord{Hash: domain.HashCode(code, salt), Salt: salt}
	if err := s.d.Store.Save(ctx, codeKey(in.TenantID, in.Recipient), rec, s.cfg.TTL); err != nil {
		return SendResult{}, err
	}

	if err := s.d.Repo.InsertRequest(ctx, Request{
		ID: requestID, TenantID: in.TenantID, Recipient: in.Recipient,
		Channel: string(domain.ChannelEmail), State: contracts.StateRequested, CreatedAt: s.d.Clock.Now(),
	}); err != nil {
		return SendResult{}, err
	}

	evt := contracts.RequestedEvent{
		RequestID: requestID, TenantID: in.TenantID, Recipient: in.Recipient,
		Channel: string(domain.ChannelEmail), Code: code,
	}
	if err := s.d.Pub.Publish(ctx, s.cfg.RequestedTopic, evt); err != nil {
		return SendResult{}, err
	}
	return SendResult{RequestID: requestID, Code: code}, nil
}

// newID returns a random 128-bit hex id used as both request id and hash salt.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
