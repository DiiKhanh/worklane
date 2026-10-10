// Package app is link-dispatcher's application layer: the Handle use case that turns a
// link.clicked event into one link_clicks row. It depends on ports (Repo, Clock) and the
// shared contract - never on adapters or pkg/platform.
package app

import (
	"context"
	"time"
)

// Click is one redirect to persist. Referer, UA and IPHash may be empty; the repo
// stores an empty value as NULL.
type Click struct {
	Code     string
	TenantID string
	TS       time.Time // always UTC: link-svc groups clicks by UTC calendar day
	Referer  string
	UA       string
	IPHash   string
}

// Repo appends clicks to the click log.
type Repo interface {
	InsertClick(ctx context.Context, c Click) error
}

// Clock abstracts time so the fallback timestamp is testable.
type Clock interface{ Now() time.Time }
