// Package http defines the Link management API wire contract. Routing and
// workflows are implemented in subsequent tickets.
package http

import (
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
)

const (
	DefaultPageSize     = 20
	MaxPageSize         = 100
	MaxCursorBytes      = 256
	MaxDestinationBytes = 4096
	AnalyticsDays       = 14
	RecentVisitLimit    = 20
)

type CreateRequest struct {
	// The byte limit and URL policy require boundary validation, not a character
	// count tag. The accepted string is preserved exactly for storage and dedup.
	LongURL string `json:"long_url" binding:"required"`
}

// CreateResponse is identical for a new mapping (201) and active dedup (200).
type CreateResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

type ListRequest struct {
	Cursor string `form:"cursor" binding:"max=256"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// Metadata is used for list items, detail and successful idempotent disable.
// It deliberately has no analytics dependency. Timestamps use RFC3339 in UTC;
// DisabledAt is null for active links and immutable after permanent disable.
type Metadata struct {
	Code       string     `json:"code"`
	ShortURL   string     `json:"short_url"`
	LongURL    string     `json:"long_url"`
	State      link.State `json:"state"`
	CreatedAt  time.Time  `json:"created_at"`
	DisabledAt *time.Time `json:"disabled_at"`
}

type ListResponse struct {
	Items []Metadata `json:"items"`
	// Null marks the final page. Cursors are opaque, scoped to the authenticated
	// tenant and order by created_at DESC, id DESC; IDs stay off the JSON wire.
	NextCursor *string `json:"next_cursor"`
}

// AnalyticsResponse is returned only after a successful analytics read. Errors
// use ErrorResponse, never a synthetic zero. Collections encode as [] when empty.
type AnalyticsResponse struct {
	TotalClicks int64         `json:"total_clicks"`
	Series      []DailyClicks `json:"series"`
	Recent      []Visit       `json:"recent"`
}

type DailyClicks struct {
	Date   string `json:"date"` // UTC YYYY-MM-DD; today and the preceding 13 days
	Clicks int64  `json:"clicks"`
}

type Visit struct {
	TS             time.Time   `json:"ts"`
	ReferrerOrigin string      `json:"referrer_origin"` // empty for unknown
	Device         link.Device `json:"device"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

// ErrorResponse preserves the repository's {"error": string} envelope. Retry
// policy follows the HTTP status; quota 429 also supplies Retry-After seconds.
type ErrorResponse struct {
	Error string `json:"error"`
}

const (
	ErrorInvalidRequest     = "link: invalid request"
	ErrorMissingCredentials = "missing credentials"
	ErrorInvalidToken       = "invalid token"
	ErrorInvalidAPIKey      = "invalid api key"
	ErrorAuthUnavailable    = "auth temporarily unavailable"
	ErrorNotFound           = "link: not found"
	ErrorLinkDisabled       = "LINK_DISABLED"
	ErrorRateLimited        = "link: rate limited"
	ErrorUnavailable        = "link: unavailable"
	ErrorInternal           = "internal error"
)
