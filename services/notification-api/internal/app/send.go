package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// SendInput is a tenant's request to send one templated notification.
type SendInput struct {
	TenantID       string
	Channel        string
	Recipient      string
	TemplateID     string
	Variables      map[string]string
	Kind           string
	UserRef        string // optional: the tenant's own user id, keys opt-out and the per-user limit
	IdempotencyKey string // optional: a retry with the same key returns the first notification
}

// SendResult identifies the notification. State is queued for an accepted send and
// suppressed for an opted-out one; Duplicate is true when the idempotency key matched
// an earlier send, in which case State is that notification's current state.
type SendResult struct {
	NotificationID string
	State          string
	Duplicate      bool
}

// Send validates the request, then applies the pipeline in order: opt-out (marketing
// only), rate limits, idempotent insert into notification_log, publish to the
// channel's topic. Delivery itself happens in the dispatcher.
func (s *Service) Send(ctx context.Context, in SendInput) (SendResult, error) {
	if err := s.validateSend(ctx, in); err != nil {
		return SendResult{}, err
	}

	id := s.d.IDs.New()
	if in.IdempotencyKey != "" {
		id = domain.IDFromIdempotencyKey(in.TenantID, in.IdempotencyKey)
	}
	now := s.d.Clock.Now().UTC()
	entry := domain.Notification{
		ID: id, TenantID: in.TenantID, Channel: in.Channel,
		RecipientMasked: domain.Mask(in.Channel, in.Recipient),
		TemplateID:      in.TemplateID, Kind: in.Kind, State: domain.StateQueued,
		CreatedAt: now, UpdatedAt: now,
	}

	optedOut, err := s.optedOut(ctx, in)
	if err != nil {
		return SendResult{}, err
	}
	if optedOut {
		entry.State = domain.StateSuppressed
		return s.record(ctx, entry)
	}

	if err := s.checkSendLimits(ctx, in); err != nil {
		return SendResult{}, err
	}

	res, err := s.record(ctx, entry)
	if err != nil || res.Duplicate {
		return res, err
	}

	vars := in.Variables
	if vars == nil {
		vars = map[string]string{}
	}
	evt := contracts.RequestedEvent{
		NotificationID: id, TenantID: in.TenantID, Channel: in.Channel, Recipient: in.Recipient,
		TemplateID: in.TemplateID, Variables: vars, Kind: in.Kind, UserRef: in.UserRef,
	}
	if err := s.d.Pub.Publish(ctx, s.topicFor(in.Channel), evt); err != nil {
		// The row exists but nothing will ever deliver it. Leaving it queued would make
		// a retry with the same idempotency key look accepted, so record the failure.
		// A cancelled request is a likely cause of the publish error, hence WithoutCancel.
		if merr := s.d.Log.MarkFailed(context.WithoutCancel(ctx), id, "publish failed", s.d.Clock.Now().UTC()); merr != nil {
			log.Printf("notification: mark %s failed after publish error: %v", id, merr)
		}
		return SendResult{}, fmt.Errorf("publish notification: %w", err)
	}
	return res, nil
}

// validateSend checks the request shape and that the template exists for this tenant,
// is active and targets the requested channel.
func (s *Service) validateSend(ctx context.Context, in SendInput) error {
	if !domain.ValidChannel(in.Channel) {
		return domain.ErrInvalidChannel
	}
	if !domain.ValidKind(in.Kind) {
		return domain.ErrInvalidKind
	}
	if err := domain.ValidateRecipient(in.Channel, in.Recipient); err != nil {
		return err
	}
	if err := domain.ValidateUserRef(in.UserRef); err != nil {
		return err
	}
	if len(in.IdempotencyKey) > domain.MaxIdempotencyKeyLen {
		return domain.ErrInvalidIdempotencyKey
	}
	if in.TemplateID == "" {
		return domain.ErrTemplateRequired
	}
	tpl, err := s.d.Templates.Find(ctx, in.TenantID, in.TemplateID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return fmt.Errorf("find template: %w", err)
	}
	if tpl.Status != domain.TemplateActive {
		return domain.ErrTemplateArchived
	}
	if tpl.Channel != in.Channel {
		return domain.ErrChannelMismatch
	}
	return nil
}

// optedOut reports whether the send must be suppressed. Only marketing respects the
// opt-out: suppressing a transactional notification (a receipt, a login code) is a bug.
// Without a user_ref there is no preference to look up, so the send goes through.
func (s *Service) optedOut(ctx context.Context, in SendInput) (bool, error) {
	if in.Kind != domain.KindMarketing || in.UserRef == "" {
		return false, nil
	}
	settings, err := s.d.Settings.List(ctx, in.TenantID, in.UserRef)
	if err != nil {
		return false, fmt.Errorf("load preferences: %w", err)
	}
	for _, st := range settings {
		if st.Channel == in.Channel {
			return !st.Enabled, nil
		}
	}
	return false, nil
}

// record inserts the notification_log row. A taken id means the idempotency key was
// already used: the earlier notification is returned and nothing new is sent.
func (s *Service) record(ctx context.Context, n domain.Notification) (SendResult, error) {
	err := s.d.Log.Insert(ctx, n)
	if err == nil {
		return SendResult{NotificationID: n.ID, State: n.State}, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return SendResult{}, fmt.Errorf("insert notification: %w", err)
	}
	existing, err := s.d.Log.Find(ctx, n.TenantID, n.ID)
	if err != nil {
		return SendResult{}, fmt.Errorf("find notification after duplicate insert: %w", err)
	}
	return SendResult{NotificationID: existing.ID, State: existing.State, Duplicate: true}, nil
}

// checkSendLimits counts this send against the tenant's fixed window and, when the
// request names a user, that user's window (the cap that prevents notification fatigue).
func (s *Service) checkSendLimits(ctx context.Context, in SendInput) error {
	if err := s.checkLimit(ctx, "notif:rl:tenant:"+in.TenantID, s.cfg.TenantLimitMax, s.cfg.TenantLimitWindow); err != nil {
		return err
	}
	if in.UserRef == "" {
		return nil
	}
	// user_ref is free-form, so it goes last: a ':' inside it cannot shift the key into
	// another tenant's namespace.
	key := "notif:rl:user:" + in.TenantID + ":" + in.UserRef
	return s.checkLimit(ctx, key, s.cfg.UserLimitMax, s.cfg.UserLimitWindow)
}

func (s *Service) checkLimit(ctx context.Context, key string, limit int, window time.Duration) error {
	n, err := s.d.Counter.Incr(ctx, key, window)
	if err != nil {
		return fmt.Errorf("count notification send: %w", err)
	}
	if int(n) > limit {
		return domain.ErrRateLimited
	}
	return nil
}

func (s *Service) topicFor(channel string) string {
	if channel == domain.ChannelSMS {
		return s.cfg.SMSTopic
	}
	return s.cfg.EmailTopic
}
