package mysqlrepo

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

var _ app.AnalyticsReader = (*Repo)(nil)

func (r *Repo) ReadAnalytics(ctx context.Context, tenant, code string, now time.Time) (domain.Analytics, error) {
	result := domain.Analytics{Recent: make([]domain.Visit, 0)}
	// One repeatable-read snapshot prevents a click committed between queries
	// from appearing in recent visits without its corresponding total.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner struct{ Code string }
		if err := tx.Table("links").Select("code").Where("tenant_id = ? AND code = ?", tenant, code).Take(&owner).Error; err != nil {
			return err
		}
		if err := tx.Table("link_daily_clicks").Select("COALESCE(SUM(clicks), 0)").Where("tenant_id = ? AND code = ?", tenant, code).Scan(&result.TotalClicks).Error; err != nil {
			return err
		}
		start := now.UTC().AddDate(0, 0, 1-domain.AnalyticsDays).Format(time.DateOnly)
		end := now.UTC().Format(time.DateOnly)
		if err := tx.Table("link_daily_clicks").Select("DATE_FORMAT(day, '%Y-%m-%d') AS date, clicks").Where("tenant_id = ? AND code = ? AND day BETWEEN ? AND ?", tenant, code, start, end).Order("day").Scan(&result.Series).Error; err != nil {
			return err
		}
		return tx.Table("link_clicks").Select("ts, referrer_origin, device").Where("tenant_id = ? AND code = ? AND ts > ? AND ts <= ?", tenant, code, now.UTC().Add(-link.DetailRetention), now.UTC()).Order("ts DESC, id DESC").Limit(domain.RecentVisitLimit).Scan(&result.Recent).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domain.Analytics{}, translate(err)
	}
	return result, nil
}
