package app

import (
	"context"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/domain"
)

type ClickStore interface {
	Record(context.Context, link.ClickedEvent, time.Time) error
}

type Handler struct {
	store ClickStore
	now   func() time.Time
}

func New(store ClickStore, now func() time.Time) *Handler { return &Handler{store: store, now: now} }

func (h *Handler) Handle(ctx context.Context, event link.ClickedEvent) error {
	now := h.now().UTC()
	if err := domain.Validate(event, now); err != nil {
		return err
	}
	event.TS = event.TS.UTC().Truncate(time.Millisecond)
	return h.store.Record(ctx, event, now)
}
