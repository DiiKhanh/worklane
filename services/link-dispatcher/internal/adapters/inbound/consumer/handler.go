package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/domain"
)

const (
	maxEventBytes      = 2048
	persistenceTimeout = 2 * time.Second
	failureBackoff     = time.Second
)

type Handler struct {
	app      *app.Handler
	rejected atomic.Uint64
	failed   atomic.Uint64
}

func New(app *app.Handler) *Handler { return &Handler{app: app} }

func (h *Handler) Counts() (rejected, failed uint64) { return h.rejected.Load(), h.failed.Load() }

func (h *Handler) Handle(ctx context.Context, value []byte) error {
	var envelope kafka.Envelope
	var event link.ClickedEvent
	if len(value) > maxEventBytes || !utf8.Valid(value) || decode(value, &envelope) != nil || envelope.MsgType != "link.ClickedEvent" || decode(envelope.Data, &event) != nil {
		h.rejected.Add(1)
		return nil
	}
	writeCtx, cancel := context.WithTimeout(ctx, persistenceTimeout)
	err := h.app.Handle(writeCtx, event)
	cancel()
	if errors.Is(err, domain.ErrInvalid) {
		h.rejected.Add(1)
		return nil
	}
	if err == nil {
		return nil
	}
	h.failed.Add(1)
	// End the claim without marking this message. Sarama cancels the session,
	// then rejoins at committed progress; no later offset can skip this failure.
	timer := time.NewTimer(failureBackoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	return err
}

func decode(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}
