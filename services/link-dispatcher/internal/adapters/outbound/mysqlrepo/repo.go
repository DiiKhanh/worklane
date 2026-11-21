package mysqlrepo

import (
	"context"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/domain"
)

var _ app.ClickStore = (*Repo)(nil)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// Expire bounds each deletion transaction using the timestamp retention index.
// Daily aggregates are independent and never participate in detail cleanup.
func (r *Repo) Expire(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, domain.ErrInvalid
	}
	result := r.db.WithContext(ctx).Exec("DELETE FROM link_clicks WHERE ts <= ? ORDER BY ts LIMIT ?", now.UTC().Add(-link.DetailRetention), limit)
	if result.Error != nil {
		return 0, domain.ErrUnavailable
	}
	return result.RowsAffected, nil
}

func (r *Repo) Record(ctx context.Context, event link.ClickedEvent, now time.Time) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO link_daily_clicks (tenant_id, code, day, clicks)
			VALUES (?, ?, ?, 1) ON DUPLICATE KEY UPDATE clicks = clicks + 1`, event.TenantID, event.Code, event.TS.UTC().Format(time.DateOnly)).Error; err != nil {
			return err
		}
		if !event.TS.After(now.UTC().Add(-link.DetailRetention)) {
			return nil
		}
		return tx.Exec(`INSERT INTO link_clicks (tenant_id, code, ts, referrer_origin, device)
			VALUES (?, ?, ?, ?, ?)`, event.TenantID, event.Code, event.TS.UTC(), event.ReferrerOrigin, event.Device).Error
	})
	if err == nil {
		return nil
	}
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1452 {
		return domain.ErrInvalid
	}
	return domain.ErrUnavailable
}
