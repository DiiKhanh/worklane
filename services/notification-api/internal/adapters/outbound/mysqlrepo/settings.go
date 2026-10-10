package mysqlrepo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/duykhanh/worklane/services/notification-api/internal/app"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// Settings implements app.SettingsRepo.
type Settings struct{ db *gorm.DB }

func NewSettings(db *gorm.DB) *Settings { return &Settings{db: db} }

var _ app.SettingsRepo = (*Settings)(nil)

type settingRow struct {
	TenantID string `gorm:"primaryKey"`
	UserRef  string `gorm:"primaryKey"`
	Channel  string `gorm:"primaryKey"`
	Enabled  bool
}

func (settingRow) TableName() string { return "notification_settings" }

// List returns the stored rows for one user, in a stable channel order. A channel with
// no row is enabled; filling that default in is the application layer's job.
func (r *Settings) List(ctx context.Context, tenantID, userRef string) ([]domain.Setting, error) {
	var rows []settingRow
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND user_ref = ?", tenantID, userRef).
		Order("channel ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: list notification settings: %w", err)
	}
	out := make([]domain.Setting, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Setting{Channel: row.Channel, Enabled: row.Enabled})
	}
	return out, nil
}

// Upsert writes the opt-in state for one (tenant, user, channel). It is a single
// INSERT ... ON DUPLICATE KEY UPDATE on the composite primary key, so two concurrent
// writes cannot both see "no row" and collide the way a read-then-insert would.
//
// Enabled is passed through a map because GORM's struct Create skips zero values that
// have a column default: enabled=false would be dropped and stored as the default TRUE,
// silently turning an opt-out into an opt-in.
func (r *Settings) Upsert(ctx context.Context, tenantID, userRef string, s domain.Setting) error {
	err := r.db.WithContext(ctx).Model(&settingRow{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "user_ref"}, {Name: "channel"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled"}),
	}).Create(map[string]any{
		"tenant_id": tenantID, "user_ref": userRef, "channel": s.Channel, "enabled": s.Enabled,
	}).Error
	if err != nil {
		return fmt.Errorf("mysql: upsert notification setting: %w", err)
	}
	return nil
}
