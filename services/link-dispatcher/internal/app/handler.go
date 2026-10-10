package app

import (
	"context"
	"errors"
	"fmt"

	contracts "github.com/duykhanh/worklane/pkg/contracts/link"
)

const (
	// Column widths of link_clicks (db/link/migrations/0001_init.up.sql).
	maxCodeLen     = 16
	maxTenantIDLen = 36
	maxMetaLen     = 255
	ipHashLen      = 64 // hex sha256
)

// ErrInvalidEvent marks an event that can never be persisted, however often it is
// redelivered. The inbound adapter drops it instead of asking Kafka for a retry.
var ErrInvalidEvent = errors.New("invalid link.clicked event")

// Deps bundles the ports the handler depends on.
type Deps struct {
	Repo  Repo
	Clock Clock
}

// Handler is the click ingest use case: validate, normalize, persist.
type Handler struct{ d Deps }

func NewHandler(d Deps) *Handler { return &Handler{d: d} }

// Handle persists one link.clicked event. An event without a usable code or tenant
// returns ErrInvalidEvent (terminal). A repo error is returned as-is so Kafka redelivers
// the message; a redelivery after a successful insert double-counts the click, which is
// accepted for analytics (at-least-once, no idempotency key).
func (h *Handler) Handle(ctx context.Context, evt contracts.ClickedEvent) error {
	if !validCode(evt.Code) {
		return fmt.Errorf("%w: code %q", ErrInvalidEvent, evt.Code)
	}
	if evt.TenantID == "" || len(evt.TenantID) > maxTenantIDLen {
		return fmt.Errorf("%w: tenant id %q", ErrInvalidEvent, evt.TenantID)
	}

	ts := evt.TS
	if ts.IsZero() {
		ts = h.d.Clock.Now()
	}
	click := Click{
		Code: evt.Code, TenantID: evt.TenantID, TS: ts.UTC(),
		Referer: truncate(evt.Referer), UA: truncate(evt.UA), IPHash: evt.IPHash,
	}
	if len(click.IPHash) != ipHashLen {
		// Not a sha256 hex digest: keep the click, drop the unusable field.
		click.IPHash = ""
	}
	if err := h.d.Repo.InsertClick(ctx, click); err != nil {
		return fmt.Errorf("insert click: %w", err)
	}
	return nil
}

// validCode reports whether s has the shape of a link code: 1..16 base62 characters.
func validCode(s string) bool {
	if s == "" || len(s) > maxCodeLen {
		return false
	}
	for _, c := range s {
		isBase62 := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !isBase62 {
			return false
		}
	}
	return true
}

// truncate cuts s to the referer / ua column width, counted in characters like MySQL's
// VARCHAR(255), without splitting a multi-byte rune. link-svc already truncates; this
// keeps one oversized event from failing its insert forever under strict SQL mode.
func truncate(s string) string {
	if len(s) <= maxMetaLen {
		return s
	}
	r := []rune(s)
	if len(r) <= maxMetaLen {
		return s
	}
	return string(r[:maxMetaLen])
}
