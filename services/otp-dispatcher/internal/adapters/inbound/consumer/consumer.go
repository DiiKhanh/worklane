// Package consumer is otp-dispatcher's inbound (driving) adapter. It decodes the Kafka
// envelope into the shared event type and calls the application handler. Decoding lives
// here (a transport concern), so the app layer never imports the kafka platform package.
package consumer

import (
	"context"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	platformkafka "github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

// EventHandler adapts raw Kafka bytes to the app.Handler use case.
type EventHandler struct{ h *app.Handler }

func New(h *app.Handler) *EventHandler { return &EventHandler{h: h} }

// Handle decodes an otp.requested envelope and dispatches it. A malformed message
// returns an error (redelivered by the consumer group); business outcomes are handled
// inside app.Handler.
func (e *EventHandler) Handle(ctx context.Context, raw []byte) error {
	var evt contracts.RequestedEvent
	if _, err := platformkafka.Unwrap(raw, &evt); err != nil {
		return err
	}
	return e.h.Handle(ctx, evt)
}
