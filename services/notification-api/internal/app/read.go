package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// NotificationDetail is one notification with its engagement events.
type NotificationDetail struct {
	Notification domain.Notification
	Events       []domain.Event
}

// ListNotifications returns the tenant's newest notifications, capped at ListLimit.
func (s *Service) ListNotifications(ctx context.Context, tenantID string) ([]domain.Notification, error) {
	ns, err := s.d.Log.List(ctx, tenantID, s.cfg.ListLimit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return ns, nil
}

// Notification returns one of the tenant's notifications with its events. Another
// tenant's id is indistinguishable from a missing one (domain.ErrNotFound).
func (s *Service) Notification(ctx context.Context, tenantID, id string) (NotificationDetail, error) {
	n, err := s.d.Log.Find(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return NotificationDetail{}, err
		}
		return NotificationDetail{}, fmt.Errorf("find notification: %w", err)
	}
	events, err := s.d.Log.Events(ctx, n.ID)
	if err != nil {
		return NotificationDetail{}, fmt.Errorf("list notification events: %w", err)
	}
	return NotificationDetail{Notification: n, Events: events}, nil
}
