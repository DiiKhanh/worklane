// Package notifrepo is the dispatcher's persistence adapter for the notification DB. It
// implements app.NotificationTemplates and app.NotificationLog over the schema owned by
// notification-api (db/notification/migrations): services share the schema, not code.
package notifrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

// Column widths of notification_log; an oversized value would be rejected by MySQL
// strict mode on every redelivery, each of which sends the message again.
const (
	maxProviderRunes      = 32
	maxProviderMsgIDRunes = 128
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type templateRow struct {
	Subject string
	Body    string
}

// Find returns the tenant's template. The status is deliberately not checked:
// notification-api accepted the send while the template was active, and archiving it
// afterwards must not fail a notification that is already queued.
func (r *Repo) Find(ctx context.Context, tenantID, id string) (app.Template, bool, error) {
	var row templateRow
	err := r.db.WithContext(ctx).Table("templates").Select("subject", "body").
		Where("id = ? AND tenant_id = ?", id, tenantID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.Template{}, false, nil
	}
	if err != nil {
		return app.Template{}, false, fmt.Errorf("mysql: find notification template: %w", err)
	}
	return app.Template{Subject: row.Subject, Body: row.Body}, true, nil
}

// State returns the current state of the tenant's notification_log row.
func (r *Repo) State(ctx context.Context, tenantID, id string) (string, bool, error) {
	var states []string
	err := r.db.WithContext(ctx).Table("notification_log").
		Where("id = ? AND tenant_id = ?", id, tenantID).Limit(1).Pluck("state", &states).Error
	if err != nil {
		return "", false, fmt.Errorf("mysql: notification state: %w", err)
	}
	if len(states) == 0 {
		return "", false, nil
	}
	return states[0], true, nil
}

// MarkSent records a delivery. It also overwrites a failed row: notification-api marks
// the row failed when its publish reports an error, and if that write lands while this
// delivery is in flight the truth is still that the message went out.
func (r *Repo) MarkSent(ctx context.Context, id string, o app.NotificationOutcome) error {
	err := r.db.WithContext(ctx).Table("notification_log").
		Where("id = ? AND state IN ?", id, []string{contracts.StateQueued, contracts.StateFailed}).
		Updates(map[string]any{
			"state":           contracts.StateSent,
			"provider":        truncate(o.Provider, maxProviderRunes),
			"provider_msg_id": nullString(truncate(o.ProviderMsgID, maxProviderMsgIDRunes)),
			"latency_ms":      o.LatencyMillis,
			"error":           nil,
			"updated_at":      o.At.UTC(),
		}).Error
	if err != nil {
		return fmt.Errorf("mysql: mark notification sent: %w", err)
	}
	return nil
}

// MarkFailed records a terminal failure on a row that is still queued; any other state
// is left untouched.
func (r *Repo) MarkFailed(ctx context.Context, id string, o app.NotificationOutcome) error {
	err := r.db.WithContext(ctx).Table("notification_log").
		Where("id = ? AND state = ?", id, contracts.StateQueued).
		Updates(map[string]any{
			"state":      contracts.StateFailed,
			"provider":   truncate(o.Provider, maxProviderRunes),
			"latency_ms": o.LatencyMillis,
			"error":      o.Error,
			"updated_at": o.At.UTC(),
		}).Error
	if err != nil {
		return fmt.Errorf("mysql: mark notification failed: %w", err)
	}
	return nil
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
