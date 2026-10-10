package domain

import (
	"fmt"
	"strings"
	"time"
)

// Template status values.
const (
	TemplateActive   = "active"
	TemplateArchived = "archived"
)

const (
	// MaxTemplateNameLen and MaxSubjectLen match the templates columns (VARCHAR(255)).
	MaxTemplateNameLen = 255
	MaxSubjectLen      = 255
	// MaxLocaleLen matches templates.locale (VARCHAR(16)).
	MaxLocaleLen = 16
	// MaxBodyLen matches templates.body (MySQL TEXT holds 65535 bytes).
	MaxBodyLen = 65535
)

// Template is a tenant-scoped message template. Version is the current version number;
// every update bumps it and snapshots the prior content as a TemplateVersion.
type Template struct {
	ID        string
	TenantID  string
	Name      string
	Channel   string
	Locale    string
	Subject   string // email only; empty for sms
	Body      string // may contain {{var}} and {{link "url"}}
	Version   int
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TemplateVersion is an immutable snapshot of a template's content at one version.
type TemplateVersion struct {
	ID         string
	TemplateID string
	Version    int
	Subject    string
	Body       string
	CreatedAt  time.Time
}

// ValidateTemplateFields checks the structural rules of a template: a known channel,
// non-empty, column-sized name, locale and body, and no subject on sms. The token syntax of subject and
// body is validated separately by the shared render package.
func ValidateTemplateFields(name, channel, locale, subject, body string) error {
	switch {
	case !ValidChannel(channel):
		return ErrInvalidChannel
	case strings.TrimSpace(name) == "":
		return fmt.Errorf("%w: name is required", ErrInvalidTemplate)
	case len(name) > MaxTemplateNameLen:
		return fmt.Errorf("%w: name too long", ErrInvalidTemplate)
	case strings.TrimSpace(locale) == "":
		return fmt.Errorf("%w: locale is required", ErrInvalidTemplate)
	case len(locale) > MaxLocaleLen:
		return fmt.Errorf("%w: locale too long", ErrInvalidTemplate)
	case channel == ChannelSMS && subject != "":
		return fmt.Errorf("%w: sms templates have no subject", ErrInvalidTemplate)
	case len(subject) > MaxSubjectLen:
		return fmt.Errorf("%w: subject too long", ErrInvalidTemplate)
	case strings.TrimSpace(body) == "":
		return fmt.Errorf("%w: body is required", ErrInvalidTemplate)
	case len(body) > MaxBodyLen:
		return fmt.Errorf("%w: body too long", ErrInvalidTemplate)
	}
	return nil
}
