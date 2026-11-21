package mysqlrepo

import (
	"context"
	"crypto/sha256"
	"errors"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

var _ app.Repo = (*Repo)(nil)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) ByDestination(ctx context.Context, tenant, destination string) (domain.Link, error) {
	hash := sha256.Sum256([]byte(destination))
	var row domain.Link
	err := r.db.WithContext(ctx).Table("links").Where("tenant_id = ? AND long_url_hash = ?", tenant, hash[:]).Take(&row).Error
	if err != nil {
		return domain.Link{}, translate(err)
	}
	if row.LongURL != destination {
		return domain.Link{}, domain.ErrUnavailable
	}
	return row, nil
}

func (r *Repo) Insert(ctx context.Context, row domain.Link) (domain.Link, error) {
	// Database defaults establish creation time, matching stored millisecond
	// precision and the clock used for permanent disable.
	err := r.db.WithContext(ctx).Exec("INSERT INTO links (id, code, tenant_id, long_url) VALUES (?, ?, ?, ?)", row.ID, row.Code, row.TenantID, row.LongURL).Error
	if err != nil {
		return domain.Link{}, translate(err)
	}
	return r.Get(ctx, row.TenantID, row.Code)
}

func (r *Repo) Get(ctx context.Context, tenant, code string) (domain.Link, error) {
	var row domain.Link
	err := r.db.WithContext(ctx).Table("links").Where("tenant_id = ? AND code = ?", tenant, code).Take(&row).Error
	return row, translate(err)
}

func (r *Repo) ByCode(ctx context.Context, code string) (domain.Link, error) {
	var row domain.Link
	err := r.db.WithContext(ctx).Table("links").Where("code = ?", code).Take(&row).Error
	return row, translate(err)
}

func (r *Repo) List(ctx context.Context, tenant string, before *domain.Position, limit int) ([]domain.Link, error) {
	query := r.db.WithContext(ctx).Table("links").Where("tenant_id = ?", tenant)
	if before != nil {
		query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", before.CreatedAt, before.CreatedAt, before.ID)
	}
	rows := make([]domain.Link, 0)
	err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error
	return rows, translate(err)
}

func (r *Repo) Disable(ctx context.Context, tenant, code string) (domain.Link, error) {
	// Autocommit finishes before returning. The conditional update preserves
	// the first timestamp across both repeated and concurrent requests.
	err := r.db.WithContext(ctx).Exec(`UPDATE links
		SET disabled_at = GREATEST(created_at, UTC_TIMESTAMP(3))
		WHERE tenant_id = ? AND code = ? AND disabled_at IS NULL`, tenant, code).Error
	if err != nil {
		return domain.Link{}, translate(err)
	}
	return r.Get(ctx, tenant, code)
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrDuplicate
	}
	return domain.ErrUnavailable
}
