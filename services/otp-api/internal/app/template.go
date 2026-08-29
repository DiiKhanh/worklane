package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/pkg/templating"
)

var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrVersionNotFound  = errors.New("template version not found")
	ErrTemplateExists   = errors.New("template already exists for channel+locale")
)

// Template is a logical message template: one per (channel, locale). ActiveVersionID
// points at the currently published version the dispatcher renders.
type Template struct {
	ID, Name, Channel, Locale, Status, ActiveVersionID string
	CreatedAt, UpdatedAt                               time.Time
}

// TemplateVersion is an immutable snapshot of one edit.
type TemplateVersion struct {
	ID, TemplateID                         string
	VersionNo                              int
	Subject, Body, Status, Note, CreatedBy string
	CreatedAt                              time.Time
}

// TemplateRepo is the durable store for templates (MySQL).
type TemplateRepo interface {
	List(ctx context.Context) ([]Template, error)
	Get(ctx context.Context, id string) (Template, []TemplateVersion, error)
	Create(ctx context.Context, t Template, first TemplateVersion) error
	AddVersion(ctx context.Context, v TemplateVersion) (versionNo int, err error)
	Publish(ctx context.Context, templateID, versionID string) (channel, locale string, err error)
}

// TemplateCache lets Publish invalidate the dispatcher's cache-aside key.
type TemplateCache interface {
	Invalidate(ctx context.Context, channel, locale string) error
}

// IDGen abstracts id creation so tests are deterministic.
type IDGen interface{ New() string }

// TemplateService is the template CRUD + preview use case.
type TemplateService struct {
	repo  TemplateRepo
	cache TemplateCache
	clock Clock
	ids   IDGen
}

func NewTemplateService(repo TemplateRepo, cache TemplateCache, clock Clock, ids IDGen) *TemplateService {
	return &TemplateService{repo: repo, cache: cache, clock: clock, ids: ids}
}

type CreateTemplateInput struct {
	Name, Channel, Locale, Subject, Body, Note, Author string
}

func (s *TemplateService) Create(ctx context.Context, in CreateTemplateInput) (Template, error) {
	if err := templating.Validate(in.Channel, in.Subject, in.Body); err != nil {
		return Template{}, err
	}
	now := s.clock.Now()
	t := Template{ID: s.ids.New(), Name: in.Name, Channel: in.Channel, Locale: in.Locale,
		Status: "active", CreatedAt: now, UpdatedAt: now}
	first := TemplateVersion{ID: s.ids.New(), TemplateID: t.ID, VersionNo: 1, Subject: in.Subject,
		Body: in.Body, Status: "draft", Note: in.Note, CreatedBy: in.Author, CreatedAt: now}
	if err := s.repo.Create(ctx, t, first); err != nil {
		return Template{}, err
	}
	return t, nil
}

type AddVersionInput struct {
	TemplateID, Subject, Body, Note, Author string
}

func (s *TemplateService) AddVersion(ctx context.Context, in AddVersionInput) (TemplateVersion, error) {
	t, _, err := s.repo.Get(ctx, in.TemplateID)
	if err != nil {
		return TemplateVersion{}, err
	}
	if err := templating.Validate(t.Channel, in.Subject, in.Body); err != nil {
		return TemplateVersion{}, err
	}
	v := TemplateVersion{ID: s.ids.New(), TemplateID: in.TemplateID, Subject: in.Subject,
		Body: in.Body, Status: "draft", Note: in.Note, CreatedBy: in.Author, CreatedAt: s.clock.Now()}
	no, err := s.repo.AddVersion(ctx, v)
	if err != nil {
		return TemplateVersion{}, err
	}
	v.VersionNo = no
	return v, nil
}

func (s *TemplateService) Publish(ctx context.Context, templateID, versionID string) error {
	channel, locale, err := s.repo.Publish(ctx, templateID, versionID)
	if err != nil {
		return err
	}
	// Best-effort cache invalidation; a stale cache self-heals on TTL, so a cache error
	// must not fail an otherwise-successful publish.
	_ = s.cache.Invalidate(ctx, channel, locale)
	return nil
}

func (s *TemplateService) List(ctx context.Context) ([]Template, error) { return s.repo.List(ctx) }

func (s *TemplateService) Get(ctx context.Context, id string) (Template, []TemplateVersion, error) {
	return s.repo.Get(ctx, id)
}

// Preview renders through the exact same engine the dispatcher uses, with sample values,
// so what an author sees is what a recipient gets.
func (s *TemplateService) Preview(subject, body string) (string, string) {
	return templating.Render(subject, body, templating.Vars{Code: "123456", Expiry: "5 minutes"})
}
