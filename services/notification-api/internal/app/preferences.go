package app

import (
	"context"
	"fmt"

	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// SetPreferenceInput turns one channel on or off for one of the tenant's users.
type SetPreferenceInput struct {
	TenantID string
	UserRef  string
	Channel  string
	Enabled  bool
}

// Preferences returns the user's setting for every supported channel, in a stable
// order. A channel with no stored row is enabled (opt-out model).
func (s *Service) Preferences(ctx context.Context, tenantID, userRef string) ([]domain.Setting, error) {
	if err := requireUserRef(userRef); err != nil {
		return nil, err
	}
	stored, err := s.d.Settings.List(ctx, tenantID, userRef)
	if err != nil {
		return nil, fmt.Errorf("load preferences: %w", err)
	}
	channels := domain.Channels()
	out := make([]domain.Setting, 0, len(channels))
	for _, ch := range channels {
		st := domain.Setting{Channel: ch, Enabled: true}
		for _, row := range stored {
			if row.Channel == ch {
				st.Enabled = row.Enabled
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// SetPreference stores the user's opt-in state for one channel.
func (s *Service) SetPreference(ctx context.Context, in SetPreferenceInput) error {
	if err := requireUserRef(in.UserRef); err != nil {
		return err
	}
	if !domain.ValidChannel(in.Channel) {
		return domain.ErrInvalidChannel
	}
	st := domain.Setting{Channel: in.Channel, Enabled: in.Enabled}
	if err := s.d.Settings.Upsert(ctx, in.TenantID, in.UserRef, st); err != nil {
		return fmt.Errorf("save preference: %w", err)
	}
	return nil
}

func requireUserRef(userRef string) error {
	if userRef == "" {
		return domain.ErrInvalidUserRef
	}
	return domain.ValidateUserRef(userRef)
}
