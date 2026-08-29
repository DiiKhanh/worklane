// Package mysqltemplates is otp-api's outbound persistence adapter for template CRUD.
// It implements app.TemplateRepo over MySQL via GORM. Publish is transactional: it
// flips version statuses and repoints templates.active_version_id atomically.
package mysqltemplates

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// Repo implements app.TemplateRepo.
type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

var _ app.TemplateRepo = (*Repo)(nil)

// --- GORM row models (private; mapped to app types at the boundary) ---

type templateRow struct {
	ID              string
	Name            string
	Channel         string
	Locale          string
	Status          string
	ActiveVersionID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (templateRow) TableName() string { return "templates" }

type versionRow struct {
	ID         string
	TemplateID string
	VersionNo  int
	Subject    string
	Body       string
	Status     string
	Note       string
	CreatedBy  string
	CreatedAt  time.Time
}

func (versionRow) TableName() string { return "template_versions" }

// --- app.TemplateRepo implementation ---

func (r *Repo) List(ctx context.Context) ([]app.Template, error) {
	var rows []templateRow
	if err := r.db.WithContext(ctx).Order("channel, locale").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]app.Template, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAppTemplate(row))
	}
	return out, nil
}

func (r *Repo) Get(ctx context.Context, id string) (app.Template, []app.TemplateVersion, error) {
	var t templateRow
	if err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return app.Template{}, nil, app.ErrTemplateNotFound
		}
		return app.Template{}, nil, err
	}
	var vs []versionRow
	if err := r.db.WithContext(ctx).Where("template_id = ?", id).Order("version_no DESC").Find(&vs).Error; err != nil {
		return app.Template{}, nil, err
	}
	versions := make([]app.TemplateVersion, 0, len(vs))
	for _, v := range vs {
		versions = append(versions, toAppVersion(v))
	}
	return toAppTemplate(t), versions, nil
}

func (r *Repo) Create(ctx context.Context, t app.Template, first app.TemplateVersion) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&versionRow{
			ID: first.ID, TemplateID: t.ID, VersionNo: 1, Subject: first.Subject,
			Body: first.Body, Status: "draft", Note: first.Note, CreatedBy: first.CreatedBy,
			CreatedAt: first.CreatedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&templateRow{
			ID: t.ID, Name: t.Name, Channel: t.Channel, Locale: t.Locale,
			Status: "active", ActiveVersionID: nil, CreatedAt: t.CreatedAt, UpdatedAt: t.CreatedAt,
		}).Error
	})
}

func (r *Repo) AddVersion(ctx context.Context, v app.TemplateVersion) (int, error) {
	var next int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var max struct{ N int }
		if err := tx.Model(&versionRow{}).Select("COALESCE(MAX(version_no),0) AS n").
			Where("template_id = ?", v.TemplateID).Scan(&max).Error; err != nil {
			return err
		}
		next = max.N + 1
		return tx.Create(&versionRow{
			ID: v.ID, TemplateID: v.TemplateID, VersionNo: next, Subject: v.Subject,
			Body: v.Body, Status: "draft", Note: v.Note, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
		}).Error
	})
	return next, err
}

func (r *Repo) Publish(ctx context.Context, templateID, versionID string) (string, string, error) {
	var channel, locale string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t templateRow
		if err := tx.First(&t, "id = ?", templateID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return app.ErrTemplateNotFound
			}
			return err
		}
		var v versionRow
		if err := tx.First(&v, "id = ? AND template_id = ?", versionID, templateID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return app.ErrVersionNotFound
			}
			return err
		}
		// Supersede the currently published version, publish the chosen one, repoint active.
		if err := tx.Model(&versionRow{}).Where("template_id = ? AND status = ?", templateID, "published").
			Update("status", "superseded").Error; err != nil {
			return err
		}
		if err := tx.Model(&versionRow{}).Where("id = ?", versionID).Update("status", "published").Error; err != nil {
			return err
		}
		if err := tx.Model(&templateRow{}).Where("id = ?", templateID).
			Updates(map[string]any{"active_version_id": versionID, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		channel, locale = t.Channel, t.Locale
		return nil
	})
	return channel, locale, err
}

func toAppTemplate(r templateRow) app.Template {
	av := ""
	if r.ActiveVersionID != nil {
		av = *r.ActiveVersionID
	}
	return app.Template{
		ID: r.ID, Name: r.Name, Channel: r.Channel, Locale: r.Locale, Status: r.Status,
		ActiveVersionID: av, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toAppVersion(r versionRow) app.TemplateVersion {
	return app.TemplateVersion{
		ID: r.ID, TemplateID: r.TemplateID, VersionNo: r.VersionNo, Subject: r.Subject,
		Body: r.Body, Status: r.Status, Note: r.Note, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
	}
}
