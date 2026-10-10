package http

import (
	"time"

	"github.com/duykhanh/worklane/services/notification-api/internal/app"
	"github.com/duykhanh/worklane/services/notification-api/internal/domain"
)

// Request/response bodies for the notification HTTP API. Field rules (required name,
// known channel, column widths, ...) are enforced by the use cases, so every rejection
// carries the same domain message whichever transport it came through; `binding` tags
// are only used where a missing field could not be told apart from its zero value.

type templateRequest struct {
	Name    string `json:"name"`
	Channel string `json:"channel"` // create only; a template's channel never changes
	Locale  string `json:"locale"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type templateDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Locale    string    `json:"locale"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Version   int       `json:"version"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type previewRequest struct {
	Variables map[string]string `json:"variables"`
}

type previewResponse struct {
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	Missing []string `json:"missing"` // referenced variables that were not supplied
}

type sendRequest struct {
	Channel        string            `json:"channel"`
	Recipient      string            `json:"recipient"`
	TemplateID     string            `json:"template_id"`
	Variables      map[string]string `json:"variables"`
	Kind           string            `json:"kind"`
	UserRef        string            `json:"user_ref"`
	IdempotencyKey string            `json:"idempotency_key"`
}

type sendResponse struct {
	NotificationID string `json:"notification_id"`
	State          string `json:"state"` // queued, suppressed, or the earlier send's state
}

// preferenceRequest uses a *bool: with a plain bool a missing "enabled" would decode as
// false and silently opt the user out.
type preferenceRequest struct {
	UserRef string `json:"user_ref"`
	Channel string `json:"channel"`
	Enabled *bool  `json:"enabled" binding:"required"`
}

type preferenceDTO struct {
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
}

type preferencesResponse struct {
	UserRef     string          `json:"user_ref"`
	Preferences []preferenceDTO `json:"preferences"`
}

// notificationDTO never carries the raw recipient: the log only stores the masked form.
type notificationDTO struct {
	ID            string    `json:"id"`
	Channel       string    `json:"channel"`
	Recipient     string    `json:"recipient"`
	TemplateID    string    `json:"template_id"`
	Kind          string    `json:"kind"`
	State         string    `json:"state"`
	Provider      string    `json:"provider"`
	ProviderMsgID string    `json:"provider_msg_id"`
	LatencyMS     int64     `json:"latency_ms"`
	Error         string    `json:"error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type eventDTO struct {
	Type string    `json:"type"`
	TS   time.Time `json:"ts"`
	Meta string    `json:"meta"`
}

type notificationDetailDTO struct {
	notificationDTO
	Events []eventDTO `json:"events"`
}

func templateFromDomain(t domain.Template) templateDTO {
	return templateDTO{
		ID: t.ID, Name: t.Name, Channel: t.Channel, Locale: t.Locale, Subject: t.Subject,
		Body: t.Body, Version: t.Version, Status: t.Status,
		CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
}

// The list mappers build non-nil slices so an empty result serializes as [] not null.

func templatesFromDomain(ts []domain.Template) []templateDTO {
	out := make([]templateDTO, 0, len(ts))
	for _, t := range ts {
		out = append(out, templateFromDomain(t))
	}
	return out
}

func previewFromApp(p app.Preview) previewResponse {
	return previewResponse{Subject: p.Subject, Body: p.Body, Missing: append([]string{}, p.Missing...)}
}

func preferencesFromDomain(userRef string, settings []domain.Setting) preferencesResponse {
	out := make([]preferenceDTO, 0, len(settings))
	for _, s := range settings {
		out = append(out, preferenceDTO{Channel: s.Channel, Enabled: s.Enabled})
	}
	return preferencesResponse{UserRef: userRef, Preferences: out}
}

func notificationFromDomain(n domain.Notification) notificationDTO {
	return notificationDTO{
		ID: n.ID, Channel: n.Channel, Recipient: n.RecipientMasked, TemplateID: n.TemplateID,
		Kind: n.Kind, State: n.State, Provider: n.Provider, ProviderMsgID: n.ProviderMsgID,
		LatencyMS: n.LatencyMS, Error: n.Error,
		CreatedAt: n.CreatedAt.UTC(), UpdatedAt: n.UpdatedAt.UTC(),
	}
}

func notificationsFromDomain(ns []domain.Notification) []notificationDTO {
	out := make([]notificationDTO, 0, len(ns))
	for _, n := range ns {
		out = append(out, notificationFromDomain(n))
	}
	return out
}

func detailFromApp(d app.NotificationDetail) notificationDetailDTO {
	events := make([]eventDTO, 0, len(d.Events))
	for _, e := range d.Events {
		events = append(events, eventDTO{Type: e.Type, TS: e.TS.UTC(), Meta: e.Meta})
	}
	return notificationDetailDTO{notificationDTO: notificationFromDomain(d.Notification), Events: events}
}
