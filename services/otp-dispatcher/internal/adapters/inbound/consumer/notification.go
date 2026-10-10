package consumer

import (
	"context"
	"errors"
	"log"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	platformkafka "github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

// NotificationDeliverer is the use case this adapter drives; *app.NotificationHandler
// satisfies it.
type NotificationDeliverer interface {
	Handle(ctx context.Context, evt contracts.RequestedEvent) error
}

var _ NotificationDeliverer = (*app.NotificationHandler)(nil)

// NotificationEventHandler adapts raw Kafka bytes to the notification delivery use case.
type NotificationEventHandler struct{ h NotificationDeliverer }

func NewNotification(h NotificationDeliverer) *NotificationEventHandler {
	return &NotificationEventHandler{h: h}
}

// Handle decodes a notification.<channel>.requested envelope and dispatches it.
//
// A message that can never succeed (undecodable bytes, or an event the app rejects as
// invalid) is logged and dropped by returning nil: the platform consumer stops at the
// first unmarked message, so returning an error for a poison message would stall its
// partition for good. Any other error is logged and returned so the message is
// redelivered. The event itself is never logged - it carries the raw recipient.
func (e *NotificationEventHandler) Handle(ctx context.Context, raw []byte) error {
	var evt contracts.RequestedEvent
	if _, err := platformkafka.Unwrap(raw, &evt); err != nil {
		log.Printf("otp-dispatcher: dropping undecodable notification message: %v", err)
		return nil
	}
	err := e.h.Handle(ctx, evt)
	if errors.Is(err, app.ErrInvalidNotification) {
		log.Printf("otp-dispatcher: dropping notification message: %v", err)
		return nil
	}
	if err != nil {
		log.Printf("otp-dispatcher: notification %s not handled, will be redelivered: %v", evt.NotificationID, err)
	}
	return err
}
