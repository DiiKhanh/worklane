package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/duykhanh/worklane/pkg/templating"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// CreateTemplateInput is the authored content of a new template.
type CreateTemplateInput struct {
	TenantID string
	Name     string
	Channel  string
	Locale   string
	Subject  string
	Body     string
}

// UpdateTemplateInput replaces a template's editable fields. The channel is fixed at
// creation: sends are validated against it, so it cannot change under them.
type UpdateTemplateInput struct {
	TenantID string
	ID       string
	Name     string
	Locale   string
	Subject  string
	Body     string
}

// Preview is a template rendered with sample variables. Missing lists the variables
// the template references that were not supplied (they rendered empty).
type Preview struct {
	Subject string
	Body    string
	Missing []string
}

// CreateTemplate validates and stores a new template at version 1.
func (s *Service) CreateTemplate(ctx context.Context, in CreateTemplateInput) (domain.Template, error) {
	if err := validateTemplate(in.Name, in.Channel, in.Locale, in.Subject, in.Body); err != nil {
		return domain.Template{}, err
	}
	now := s.d.Clock.Now().UTC()
	t := domain.Template{
		ID: s.d.IDs.New(), TenantID: in.TenantID, Name: in.Name, Channel: in.Channel,
		Locale: in.Locale, Subject: in.Subject, Body: in.Body,
		Version: 1, Status: domain.TemplateActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.d.Templates.Insert(ctx, t); err != nil {
		return domain.Template{}, fmt.Errorf("insert template: %w", err)
	}
	return t, nil
}

// UpdateTemplate replaces the template's content, bumps its version and snapshots the
// content it replaced into the version history.
func (s *Service) UpdateTemplate(ctx context.Context, in UpdateTemplateInput) (domain.Template, error) {
	cur, err := s.GetTemplate(ctx, in.TenantID, in.ID)
	if err != nil {
		return domain.Template{}, err
	}
	if err := validateTemplate(in.Name, cur.Channel, in.Locale, in.Subject, in.Body); err != nil {
		return domain.Template{}, err
	}
	now := s.d.Clock.Now().UTC()
	prior := domain.TemplateVersion{
		ID: s.d.IDs.New(), TemplateID: cur.ID, Version: cur.Version,
		Subject: cur.Subject, Body: cur.Body, CreatedAt: now,
	}
	next := cur
	next.Name, next.Locale, next.Subject, next.Body = in.Name, in.Locale, in.Subject, in.Body
	next.Version = cur.Version + 1
	next.UpdatedAt = now
	if err := s.d.Templates.Update(ctx, next, prior); err != nil {
		if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
			return domain.Template{}, err
		}
		return domain.Template{}, fmt.Errorf("update template: %w", err)
	}
	return next, nil
}

// GetTemplate returns one of the tenant's templates.
func (s *Service) GetTemplate(ctx context.Context, tenantID, id string) (domain.Template, error) {
	t, err := s.d.Templates.Find(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Template{}, err
		}
		return domain.Template{}, fmt.Errorf("find template: %w", err)
	}
	return t, nil
}

// ListTemplates returns the tenant's templates.
func (s *Service) ListTemplates(ctx context.Context, tenantID string) ([]domain.Template, error) {
	ts, err := s.d.Templates.List(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	return ts, nil
}

// PreviewTemplate renders a stored template with the given variables through the same
// render function and Shortener port the dispatcher uses at delivery, so the preview
// cannot differ from the delivered message.
func (s *Service) PreviewTemplate(ctx context.Context, tenantID, id string, vars map[string]string) (Preview, error) {
	t, err := s.GetTemplate(ctx, tenantID, id)
	if err != nil {
		return Preview{}, err
	}
	shorten := func(ctx context.Context, longURL string) (string, error) {
		return s.d.Shortener.Shorten(ctx, tenantID, longURL)
	}
	msg, err := templating.RenderMessage(ctx, t.Subject, t.Body, vars, shorten)
	if err != nil {
		return Preview{}, fmt.Errorf("render preview: %w", err)
	}
	return Preview{Subject: msg.Subject, Body: msg.Body, Missing: msg.Missing}, nil
}

// validateTemplate applies the structural rules and the token syntax rules. Both
// surface as domain.ErrInvalidTemplate (or ErrInvalidChannel) so the edge maps one
// error to "bad request" while the message still names the offending token.
func validateTemplate(name, channel, locale, subject, body string) error {
	if err := domain.ValidateTemplateFields(name, channel, locale, subject, body); err != nil {
		return err
	}
	if err := templating.ValidateMessage(channel, subject, body); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidTemplate, err)
	}
	return nil
}
