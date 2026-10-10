// Package mysqlrepo is notification-api's outbound persistence adapter: it implements
// the app.TemplateRepo, app.LogRepo and app.SettingsRepo ports on top of MySQL via
// GORM. As an adapter it may import app, domain, and pkg - but nothing here leaks back
// into those layers.
package mysqlrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/notification-api/internal/app"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// Templates implements app.TemplateRepo.
type Templates struct{ db *gorm.DB }

func NewTemplates(db *gorm.DB) *Templates { return &Templates{db: db} }

var _ app.TemplateRepo = (*Templates)(nil)

// --- GORM row models (private; mapped to domain types at the boundary) ---

type templateRow struct {
	ID        string
	TenantID  string
	Name      string
	Channel   string
	Locale    string
	Subject   string
	Body      string
	Version   int
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (templateRow) TableName() string { return "templates" }

func (r templateRow) toDomain() domain.Template {
	return domain.Template{
		ID: r.ID, TenantID: r.TenantID, Name: r.Name, Channel: r.Channel, Locale: r.Locale,
		Subject: r.Subject, Body: r.Body, Version: r.Version, Status: r.Status,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

type templateVersionRow struct {
	ID         string
	TemplateID string
	Version    int
	Subject    string
	Body       string
	CreatedAt  time.Time
}

func (templateVersionRow) TableName() string { return "template_versions" }

// --- app.TemplateRepo implementation ---

// Insert persists a new template. Timestamps are stored in UTC because DATETIME carries
// no zone: every reader assumes UTC.
func (r *Templates) Insert(ctx context.Context, t domain.Template) error {
	row := templateRow{
		ID: t.ID, TenantID: t.TenantID, Name: t.Name, Channel: t.Channel, Locale: t.Locale,
		Subject: t.Subject, Body: t.Body, Version: t.Version, Status: t.Status,
		CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("mysql: insert template: %w", err)
	}
	return nil
}

// Find loads one template. The tenant is part of the predicate, so another tenant's id
// is indistinguishable from a missing one.
func (r *Templates) Find(ctx context.Context, tenantID, id string) (domain.Template, error) {
	var row templateRow
	err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Template{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Template{}, fmt.Errorf("mysql: find template: %w", err)
	}
	return row.toDomain(), nil
}

// List returns the tenant's templates, most recently updated first.
func (r *Templates) List(ctx context.Context, tenantID string) ([]domain.Template, error) {
	var rows []templateRow
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("updated_at DESC, id DESC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: list templates: %w", err)
	}
	out := make([]domain.Template, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toDomain())
	}
	return out, nil
}

// Update replaces the template content and records the snapshot of what it replaced, in
// one transaction.
//
// The optimistic lock is the "version = prior.Version" predicate on the UPDATE itself:
// the check and the write are a single statement, so two concurrent updates that both
// read version N cannot both match - the loser affects zero rows. A read-then-write
// check would let both through. The loser then needs one more read to tell a stale
// version (ErrConflict) from a template that is not this tenant's (ErrNotFound).
// Relying on RowsAffected is safe here even though MySQL counts changed rather than
// matched rows by default: a matching row always changes, because version is bumped.
//
// Only the replaceable columns are written; channel, status and created_at never
// change through this path.
func (r *Templates) Update(ctx context.Context, next domain.Template, prior domain.TemplateVersion) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&templateRow{}).
			Where("id = ? AND tenant_id = ? AND version = ?", next.ID, next.TenantID, prior.Version).
			Updates(map[string]any{
				"name": next.Name, "locale": next.Locale, "subject": next.Subject, "body": next.Body,
				"version": next.Version, "updated_at": next.UpdatedAt.UTC(),
			})
		if res.Error != nil {
			return fmt.Errorf("mysql: update template: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			var n int64
			err := tx.Model(&templateRow{}).Where("id = ? AND tenant_id = ?", next.ID, next.TenantID).Count(&n).Error
			if err != nil {
				return fmt.Errorf("mysql: update template: %w", err)
			}
			if n == 0 {
				return domain.ErrNotFound
			}
			return domain.ErrConflict
		}
		snap := templateVersionRow{
			ID: prior.ID, TemplateID: prior.TemplateID, Version: prior.Version,
			Subject: prior.Subject, Body: prior.Body, CreatedAt: prior.CreatedAt.UTC(),
		}
		if err := tx.Create(&snap).Error; err != nil {
			return fmt.Errorf("mysql: snapshot template version: %w", err)
		}
		return nil
	})
}
