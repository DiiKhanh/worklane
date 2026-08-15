// Package mysqlrepo is otp-dispatcher's persistence adapter. It implements the
// dispatcher's app.Repo (delivery logs + state updates) over the shared MySQL schema.
// It is a separate implementation from otp-api's repo: services do not share internal
// code, they only share the database schema and Kafka contracts.
package mysqlrepo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type deliveryLogRow struct {
	ID        int64
	RequestID string
	TenantID  string
	Provider  string
	Status    string
	LatencyMs int64
	Error     string
	CreatedAt time.Time
}

func (deliveryLogRow) TableName() string { return "delivery_logs" }

func (r *Repo) InsertDeliveryLog(ctx context.Context, l app.DeliveryLog) error {
	row := deliveryLogRow{
		RequestID: l.RequestID, TenantID: l.TenantID, Provider: l.Provider,
		Status: l.Status, LatencyMs: l.LatencyMillis, Error: l.Error,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("mysql: insert delivery log: %w", err)
	}
	return nil
}

func (r *Repo) UpdateState(ctx context.Context, id, to string) error {
	if err := r.db.WithContext(ctx).Table("otp_requests").
		Where("id = ?", id).Update("state", to).Error; err != nil {
		return fmt.Errorf("mysql: update state: %w", err)
	}
	return nil
}
