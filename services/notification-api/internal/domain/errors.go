package domain

import "errors"

// Sentinel errors returned by the notification domain and application layers. Adapters
// map these to transport-specific codes (e.g. HTTP status) at the edge.
var (
	ErrNotFound              = errors.New("notification: not found")
	ErrAlreadyExists         = errors.New("notification: already exists")
	ErrConflict              = errors.New("notification: template was modified concurrently")
	ErrRateLimited           = errors.New("notification: rate limited")
	ErrInvalidChannel        = errors.New("notification: channel must be email or sms")
	ErrInvalidKind           = errors.New("notification: kind must be transactional or marketing")
	ErrInvalidRecipient      = errors.New("notification: recipient is not valid for the channel")
	ErrInvalidUserRef        = errors.New("notification: user_ref is missing or too long")
	ErrInvalidIdempotencyKey = errors.New("notification: idempotency key too long")
	ErrInvalidTemplate       = errors.New("notification: invalid template")
	ErrTemplateRequired      = errors.New("notification: template_id is required")
	ErrTemplateArchived      = errors.New("notification: template is archived")
	ErrChannelMismatch       = errors.New("notification: template channel does not match the notification channel")
)
