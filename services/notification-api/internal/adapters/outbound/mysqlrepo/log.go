package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/notification-api/internal/app"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// Log implements app.LogRepo.
type Log struct{ db *gorm.DB }

func NewLog(db *gorm.DB) *Log { return &Log{db: db} }

var _ app.LogRepo = (*Log)(nil)

// logRow maps notification_log. The nullable columns are sql.Null* so an unset domain
// field is stored as NULL rather than as an empty string or a zero latency.
type logRow struct {
	ID              string
	TenantID        string
	Channel         string
	RecipientMasked string
	TemplateID      sql.NullString
	Kind            string
	State           string
	Provider        sql.NullString
	ProviderMsgID   sql.NullString
	LatencyMS       sql.NullInt64 `gorm:"column:latency_ms"`
	Error           sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (logRow) TableName() string { return "notification_log" }

func (r logRow) toDomain() domain.Notification {
	return domain.Notification{
		ID: r.ID, TenantID: r.TenantID, Channel: r.Channel, RecipientMasked: r.RecipientMasked,
		TemplateID: r.TemplateID.String, Kind: r.Kind, State: r.State,
		Provider: r.Provider.String, ProviderMsgID: r.ProviderMsgID.String,
		LatencyMS: r.LatencyMS.Int64, Error: r.Error.String,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// eventRow reads notification_events. meta is a nullable column.
type eventRow struct {
	Type string
	TS   time.Time `gorm:"column:ts"`
	Meta sql.NullString
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// Insert persists a new notification_log row.
//
// A failed insert is reported as domain.ErrAlreadyExists when a row with the same id is
// already there, i.e. an earlier or concurrent send with the same idempotency key won
// the primary key. The check is a follow-up read rather than parsing the driver's
// duplicate-key error, which keeps it driver-agnostic. The read is deliberately not
// tenant-scoped: the primary key is global, so any row with this id is the conflict.
func (r *Log) Insert(ctx context.Context, n domain.Notification) error {
	row := logRow{
		ID: n.ID, TenantID: n.TenantID, Channel: n.Channel, RecipientMasked: n.RecipientMasked,
		TemplateID: nullString(n.TemplateID), Kind: n.Kind, State: n.State,
		Provider: nullString(n.Provider), ProviderMsgID: nullString(n.ProviderMsgID),
		LatencyMS: sql.NullInt64{Int64: n.LatencyMS, Valid: n.LatencyMS != 0},
		Error:     nullString(n.Error),
		CreatedAt: n.CreatedAt.UTC(), UpdatedAt: n.UpdatedAt.UTC(),
	}
	err := r.db.WithContext(ctx).Create(&row).Error
	if err == nil {
		return nil
	}
	var existing int64
	if cerr := r.db.WithContext(ctx).Model(&logRow{}).Where("id = ?", n.ID).Count(&existing).Error; cerr == nil && existing > 0 {
		return domain.ErrAlreadyExists
	}
	return fmt.Errorf("mysql: insert notification: %w", err)
}

// Find loads one notification. The tenant is part of the predicate, so another tenant's
// id is indistinguishable from a missing one.
func (r *Log) Find(ctx context.Context, tenantID, id string) (domain.Notification, error) {
	var row logRow
	err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Notification{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Notification{}, fmt.Errorf("mysql: find notification: %w", err)
	}
	return row.toDomain(), nil
}

// List returns the tenant's newest notifications, served by idx_notif_tenant
// (tenant_id, created_at). The id tiebreak keeps the order stable within one second.
func (r *Log) List(ctx context.Context, tenantID string, limit int) ([]domain.Notification, error) {
	var rows []logRow
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: list notifications: %w", err)
	}
	out := make([]domain.Notification, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toDomain())
	}
	return out, nil
}

// Events returns a notification's engagement events, oldest first.
func (r *Log) Events(ctx context.Context, notificationID string) ([]domain.Event, error) {
	var rows []eventRow
	err := r.db.WithContext(ctx).Table("notification_events").Select("type, ts, meta").
		Where("notification_id = ?", notificationID).Order("ts ASC, id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: list notification events: %w", err)
	}
	out := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Event{Type: row.Type, TS: row.TS.UTC(), Meta: row.Meta.String})
	}
	return out, nil
}

// MarkFailed moves a queued notification to failed with a reason. The state predicate
// makes it a no-op on a row that already left queued: if the broker accepted the
// message despite reporting an error and the dispatcher has recorded the delivery, a
// sent row must not be rewritten to failed. Matching no row is therefore not an error.
func (r *Log) MarkFailed(ctx context.Context, id, reason string, at time.Time) error {
	err := r.db.WithContext(ctx).Model(&logRow{}).
		Where("id = ? AND state = ?", id, domain.StateQueued).
		Updates(map[string]any{"state": domain.StateFailed, "error": reason, "updated_at": at.UTC()}).Error
	if err != nil {
		return fmt.Errorf("mysql: mark notification failed: %w", err)
	}
	return nil
}
