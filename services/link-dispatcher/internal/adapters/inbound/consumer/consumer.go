// Package consumer is link-dispatcher's inbound (driving) adapter. It decodes the Kafka
// envelope into the shared event type and calls the application handler. Decoding lives
// here (a transport concern), so the app layer never imports the kafka platform package.
package consumer

import (
	"context"
	"errors"
	"log"

	contracts "github.com/duykhanh/worklane/pkg/contracts/link"
	platformkafka "github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

// ClickHandler is the use case this adapter drives; *app.Handler satisfies it.
type ClickHandler interface {
	Handle(ctx context.Context, evt contracts.ClickedEvent) error
}

var _ ClickHandler = (*app.Handler)(nil)

// EventHandler adapts raw Kafka bytes to the click ingest use case.
type EventHandler struct{ h ClickHandler }

func New(h ClickHandler) *EventHandler { return &EventHandler{h: h} }

// Handle decodes a link.clicked envelope and persists it.
//
// A message that can never succeed (undecodable bytes, or an event the app rejects as
// invalid) is logged and dropped by returning nil. The platform consumer stops at the
// first unmarked message, so returning an error for a poison message would stall its
// partition for good; losing one click is the cheaper failure for best-effort analytics.
// Any other error (the database being down) is returned so the message is redelivered.
func (e *EventHandler) Handle(ctx context.Context, raw []byte) error {
	var evt contracts.ClickedEvent
	if _, err := platformkafka.Unwrap(raw, &evt); err != nil {
		log.Printf("link-dispatcher: dropping undecodable message: %v", err)
		return nil
	}
	err := e.h.Handle(ctx, evt)
	if errors.Is(err, app.ErrInvalidEvent) {
		log.Printf("link-dispatcher: dropping message: %v", err)
		return nil
	}
	return err
}
