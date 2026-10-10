// Package domain is the pure core of the notification bounded context: the Template and
// Notification entities and the rules for what may be authored and sent. It imports
// only the standard library.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// Channel, kind and state values. They equal the wire vocabulary in
// pkg/contracts/notification; the domain keeps its own copy so it stays stdlib-only.
const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"

	KindTransactional = "transactional"
	KindMarketing     = "marketing"

	StateQueued     = "queued"
	StateSent       = "sent"
	StateFailed     = "failed"
	StateSuppressed = "suppressed"
)

const (
	// MaxUserRefLen matches the notification_settings.user_ref column (VARCHAR(255)).
	MaxUserRefLen = 255
	// MaxIdempotencyKeyLen bounds the client-supplied key. The key is hashed into the
	// notification id, so the bound only keeps request sizes sane.
	MaxIdempotencyKeyLen = 255
)

var e164Re = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// Channels lists the supported delivery channels in a stable order.
func Channels() []string { return []string{ChannelEmail, ChannelSMS} }

// ValidChannel reports whether c is a delivery channel the platform supports.
func ValidChannel(c string) bool { return c == ChannelEmail || c == ChannelSMS }

// ValidKind reports whether k is a known notification kind.
func ValidKind(k string) bool { return k == KindTransactional || k == KindMarketing }

// ValidateRecipient checks that recipient is well-formed for its channel: an email
// address for email, an E.164 phone number for sms.
func ValidateRecipient(channel, recipient string) error {
	switch channel {
	case ChannelEmail:
		if addr, err := mail.ParseAddress(recipient); err != nil || addr.Address != recipient {
			return ErrInvalidRecipient
		}
	case ChannelSMS:
		if !e164Re.MatchString(recipient) {
			return ErrInvalidRecipient
		}
	default:
		return ErrInvalidChannel
	}
	return nil
}

// ValidateUserRef checks an optional tenant-side user identifier fits its column.
func ValidateUserRef(userRef string) error {
	if len(userRef) > MaxUserRefLen {
		return ErrInvalidUserRef
	}
	return nil
}

// Mask hides PII in a recipient for notification_log rows and logs. Email keeps the
// first character and the domain (d***@gmail.com); a phone keeps the country prefix and
// the last two digits (+84***67). Anything malformed becomes a fixed "***".
func Mask(channel, recipient string) string {
	if channel == ChannelSMS {
		if len(recipient) < 5 || recipient[0] != '+' {
			return "***"
		}
		return recipient[:3] + "***" + recipient[len(recipient)-2:]
	}
	at := strings.IndexByte(recipient, '@')
	if at <= 0 {
		return "***"
	}
	return recipient[:1] + "***" + recipient[at:]
}

// IDFromIdempotencyKey derives the notification id for a client-supplied idempotency
// key. The id is a UUID-shaped digest of (tenant, key): deterministic, so a retried
// send maps to the same notification_log row, and tenant-scoped, so one tenant cannot
// collide with (or probe for) another tenant's keys.
func IDFromIdempotencyKey(tenantID, key string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + key))
	h := hex.EncodeToString(sum[:16])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// Notification is one notification_log row. The raw recipient is never part of it.
type Notification struct {
	ID              string
	TenantID        string
	Channel         string
	RecipientMasked string
	TemplateID      string
	Kind            string
	State           string
	Provider        string
	ProviderMsgID   string
	LatencyMS       int64
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Event is one notification_events row (delivered | opened | clicked).
type Event struct {
	Type string
	TS   time.Time
	Meta string
}

// Setting is a user's opt-in state for one channel. No stored row means enabled.
type Setting struct {
	Channel string
	Enabled bool
}
