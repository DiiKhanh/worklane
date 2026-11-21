package app

import (
	"context"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/domain"
)

type ClickStore interface {
	Record(context.Context, domain.Click, time.Time) error
}

type Handler struct {
	store ClickStore
	now   func() time.Time
}

func New(store ClickStore, now func() time.Time) *Handler { return &Handler{store: store, now: now} }

func (h *Handler) Handle(ctx context.Context, event link.ClickedEvent) error {
	now := h.now().UTC()
	click := domain.Click{Code: event.Code, TenantID: event.TenantID, TS: event.TS.UTC(), ReferrerOrigin: event.ReferrerOrigin, Device: domain.Device(event.Device)}
	if err := domain.Validate(click, now); err != nil {
		return err
	}
	click.TS = click.TS.Truncate(time.Millisecond)
	return h.store.Record(ctx, click, now)
}
