// Package mysqlrepo is link-svc's outbound persistence adapter: it implements the
// app.Repo port on top of MySQL via GORM. As an adapter it may import app, domain, and
// pkg - but nothing here leaks back into those layers.
package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// dayLayout is how the database renders a calendar day (DATE cast to text).
const dayLayout = "2006-01-02"

// Repo implements app.Repo.
type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

var _ app.Repo = (*Repo)(nil)

// --- GORM row models (private; mapped to app/domain types at the boundary) ---

type linkRow struct {
	ID          int64
	Code        string
	TenantID    string
	LongURL     string `gorm:"column:long_url"`
	LongURLHash string `gorm:"column:long_url_hash"`
	CreatedAt   time.Time
}

func (linkRow) TableName() string { return "links" }

func (r linkRow) toDomain() domain.Link {
	return domain.Link{
		ID: r.ID, Code: r.Code, TenantID: r.TenantID,
		LongURL: r.LongURL, LongURLHash: r.LongURLHash, CreatedAt: r.CreatedAt.UTC(),
	}
}

// linkStatRow is a links row plus the lifetime click count selected alongside it. Link
// is an exported, explicitly embedded field: GORM skips an anonymous field whose type
// is unexported, which would silently leave every link column zero.
type linkStatRow struct {
	Link   linkRow `gorm:"embedded"`
	Clicks int64
}

// clickRow reads link_clicks. referer and ua are nullable columns.
type clickRow struct {
	TS      time.Time `gorm:"column:ts"`
	Referer sql.NullString
	UA      sql.NullString `gorm:"column:ua"`
}

type dayCountRow struct {
	Day    string
	Clicks int64
}

// --- app.Repo implementation ---

// Insert persists a new link. created_at is stored in UTC because DATETIME carries no
// zone: every reader and the per-day aggregation assume UTC.
//
// A failed insert is reported as domain.ErrAlreadyExists when the tenant already has a
// row for the same long_url_hash, i.e. a concurrent Shorten won the uq_links_tenant_url
// index. The check is a follow-up read rather than parsing the driver's duplicate-key
// error: it stays driver-agnostic and cannot confuse that index with uq_links_code,
// whose violation (a code collision) must surface as a plain error.
func (r *Repo) Insert(ctx context.Context, l domain.Link) error {
	row := linkRow{
		ID: l.ID, Code: l.Code, TenantID: l.TenantID,
		LongURL: l.LongURL, LongURLHash: l.LongURLHash, CreatedAt: l.CreatedAt.UTC(),
	}
	err := r.db.WithContext(ctx).Create(&row).Error
	if err == nil {
		return nil
	}
	if _, findErr := r.FindByURLHash(ctx, l.TenantID, l.LongURLHash); findErr == nil {
		return domain.ErrAlreadyExists
	}
	return fmt.Errorf("mysql: insert link: %w", err)
}

func (r *Repo) FindByCode(ctx context.Context, code string) (domain.Link, error) {
	return r.findOne(ctx, "find link by code", "code = ?", code)
}

func (r *Repo) FindByURLHash(ctx context.Context, tenantID, longURLHash string) (domain.Link, error) {
	return r.findOne(ctx, "find link by url hash", "tenant_id = ? AND long_url_hash = ?", tenantID, longURLHash)
}

// findOne loads a single link, translating a missing row into domain.ErrNotFound so the
// application layer never has to know about gorm.ErrRecordNotFound.
func (r *Repo) findOne(ctx context.Context, op, where string, args ...any) (domain.Link, error) {
	var row linkRow
	err := r.db.WithContext(ctx).Where(where, args...).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Link{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Link{}, fmt.Errorf("mysql: %s: %w", op, err)
	}
	return row.toDomain(), nil
}

// ListByTenant returns the tenant's newest links with their lifetime click counts. The
// count is a correlated subquery over idx_clicks_code_ts, evaluated only for the rows
// that survive the LIMIT - cheaper than joining and grouping the whole click log.
func (r *Repo) ListByTenant(ctx context.Context, tenantID string, limit int) ([]app.LinkStat, error) {
	var rows []linkStatRow
	err := r.db.WithContext(ctx).Table("links").
		Select("links.*, (SELECT COUNT(*) FROM link_clicks WHERE link_clicks.code = links.code) AS clicks").
		Where("links.tenant_id = ?", tenantID).
		Order("links.created_at DESC, links.id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: list links: %w", err)
	}
	out := make([]app.LinkStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.LinkStat{Link: row.Link.toDomain(), Clicks: row.Clicks})
	}
	return out, nil
}

func (r *Repo) CountClicks(ctx context.Context, code string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table("link_clicks").Where("code = ?", code).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("mysql: count clicks: %w", err)
	}
	return n, nil
}

// DailyClicks counts clicks per UTC calendar day for ts >= since, oldest day first. Only
// days that had clicks are returned. The day is cast to text so it scans the same way
// whatever the driver does with DATE values.
func (r *Repo) DailyClicks(ctx context.Context, code string, since time.Time) ([]app.DayCount, error) {
	var rows []dayCountRow
	err := r.db.WithContext(ctx).Table("link_clicks").
		Select("CAST(DATE(ts) AS CHAR) AS day, COUNT(*) AS clicks").
		Where("code = ? AND ts >= ?", code, since.UTC()).
		Group("DATE(ts)").Order("DATE(ts) ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: daily clicks: %w", err)
	}
	out := make([]app.DayCount, 0, len(rows))
	for _, row := range rows {
		d, err := time.ParseInLocation(dayLayout, row.Day, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("mysql: daily clicks: parse day %q: %w", row.Day, err)
		}
		out = append(out, app.DayCount{Day: d, Clicks: row.Clicks})
	}
	return out, nil
}

func (r *Repo) RecentClicks(ctx context.Context, code string, limit int) ([]app.Click, error) {
	var rows []clickRow
	err := r.db.WithContext(ctx).Table("link_clicks").Select("ts, referer, ua").
		Where("code = ?", code).Order("ts DESC, id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: recent clicks: %w", err)
	}
	out := make([]app.Click, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.Click{TS: row.TS.UTC(), Referer: row.Referer.String, UA: row.UA.String})
	}
	return out, nil
}
