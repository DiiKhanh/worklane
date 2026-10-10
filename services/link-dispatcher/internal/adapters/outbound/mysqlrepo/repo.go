// Package mysqlrepo is link-dispatcher's persistence adapter. It implements the
// dispatcher's app.Repo (appending to link_clicks) over the link MySQL schema. It is a
// separate implementation from link-svc's repo: services do not share internal code,
// they only share the database schema and Kafka contracts.
package mysqlrepo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

var _ app.Repo = (*Repo)(nil)

// clickRow maps link_clicks. referer, ua and ip_hash are nullable columns.
type clickRow struct {
	ID       int64
	Code     string
	TenantID string
	TS       time.Time      `gorm:"column:ts"`
	Referer  sql.NullString `gorm:"column:referer"`
	UA       sql.NullString `gorm:"column:ua"`
	IPHash   sql.NullString `gorm:"column:ip_hash"`
}

func (clickRow) TableName() string { return "link_clicks" }

// InsertClick appends one click. ts is written in UTC because link-svc buckets clicks
// with DATE(ts) on a zone-less DATETIME; an empty optional field is stored as NULL.
func (r *Repo) InsertClick(ctx context.Context, c app.Click) error {
	row := clickRow{
		Code: c.Code, TenantID: c.TenantID, TS: c.TS.UTC(),
		Referer: nullable(c.Referer), UA: nullable(c.UA), IPHash: nullable(c.IPHash),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("mysql: insert click: %w", err)
	}
	return nil
}

func nullable(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
