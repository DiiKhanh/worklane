package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/notification"
	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/inbound/consumer"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/notifrepo"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

// notificationSettings is the configuration of the generic notification handlers that
// run beside the OTP handler. An empty DSN means they are disabled.
type notificationSettings struct {
	DSN         string
	Topics      map[string]string // channel -> requested topic
	Group       string            // consumer group prefix; each channel gets "<group>-<channel>"
	SentTopic   string
	FailedTopic string
	DLQTopic    string
	MaxAttempts int
	BaseBackoff time.Duration
}

func (s notificationSettings) enabled() bool { return s.DSN != "" }

// loadNotificationSettings reads the notification env config and fails on a value that
// would make delivery misbehave. Nothing is validated while the feature is disabled.
func loadNotificationSettings() (notificationSettings, error) {
	s := notificationSettings{
		DSN: config.Env("NOTIFICATION_MYSQL_DSN", ""),
		Topics: map[string]string{
			contracts.ChannelEmail: config.Env("KAFKA_TOPIC_NOTIFICATION_EMAIL", "notification.email.requested"),
			contracts.ChannelSMS:   config.Env("KAFKA_TOPIC_NOTIFICATION_SMS", "notification.sms.requested"),
		},
		Group:       config.Env("KAFKA_GROUP_NOTIFICATION", "notification-dispatcher"),
		SentTopic:   config.Env("KAFKA_TOPIC_NOTIFICATION_SENT", "notification.sent"),
		FailedTopic: config.Env("KAFKA_TOPIC_NOTIFICATION_FAILED", "notification.failed"),
		DLQTopic:    config.Env("KAFKA_TOPIC_NOTIFICATION_DLQ", "notification.dlq"),
		MaxAttempts: config.EnvInt("NOTIFICATION_MAX_ATTEMPTS", 3),
		BaseBackoff: config.EnvDuration("NOTIFICATION_RETRY_BACKOFF", 500*time.Millisecond),
	}
	if !s.enabled() {
		return s, nil
	}
	var errs []error
	for name, v := range map[string]string{
		"KAFKA_TOPIC_NOTIFICATION_EMAIL": s.Topics[contracts.ChannelEmail], "KAFKA_TOPIC_NOTIFICATION_SMS": s.Topics[contracts.ChannelSMS],
		"KAFKA_GROUP_NOTIFICATION": s.Group, "KAFKA_TOPIC_NOTIFICATION_SENT": s.SentTopic,
		"KAFKA_TOPIC_NOTIFICATION_FAILED": s.FailedTopic, "KAFKA_TOPIC_NOTIFICATION_DLQ": s.DLQTopic,
	} {
		if v == "" {
			errs = append(errs, fmt.Errorf("%s must not be empty", name))
		}
	}
	if s.MaxAttempts < 1 {
		errs = append(errs, errors.New("NOTIFICATION_MAX_ATTEMPTS must be at least 1"))
	}
	if s.BaseBackoff <= 0 {
		errs = append(errs, errors.New("NOTIFICATION_RETRY_BACKOFF must be positive"))
	}
	return s, errors.Join(errs...)
}

// sleepCtx waits for d, returning early with the context's error if it is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// startNotificationConsumers wires the notification handler onto the shared Sender
// registry and starts one consumer per channel topic. Each channel has its own consumer
// group, so a channel can later be scaled (or stalled) independently of the other.
// The returned func closes the consumers.
func startNotificationConsumers(ctx context.Context, s notificationSettings, brokers []string, senders map[string]app.Sender, pub app.Publisher) (func(), error) {
	db, err := mysql.Open(s.DSN)
	if err != nil {
		return nil, fmt.Errorf("notification mysql: %w", err)
	}
	repo := notifrepo.New(db)
	// Shorten is left nil until the link-svc client exists: a template with a {{link}}
	// then fails delivery instead of going out with an unshortened or missing URL.
	handler := consumer.NewNotification(app.NewNotificationHandler(app.NotificationDeps{
		Senders: senders, Templates: repo, Log: repo, Pub: pub, Clock: realClock{}, Sleep: sleepCtx,
	}, app.NotificationConfig{
		SentTopic: s.SentTopic, FailedTopic: s.FailedTopic, DLQTopic: s.DLQTopic,
		MaxAttempts: s.MaxAttempts, BaseBackoff: s.BaseBackoff,
	}))

	var consumers []*kafka.Consumer
	closeAll := func() {
		for _, c := range consumers {
			_ = c.Close()
		}
	}
	for channel, topic := range s.Topics {
		group := s.Group + "-" + channel
		cons, err := kafka.NewConsumer(brokers, group, topic, handler.Handle)
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("notification consumer %s: %w", topic, err)
		}
		consumers = append(consumers, cons)
		go func() {
			log.Printf("otp-dispatcher: consuming %s (group %s)", topic, group)
			if err := cons.Start(ctx); err != nil {
				log.Fatalf("otp-dispatcher: consume %s: %v", topic, err)
			}
		}()
	}
	return closeAll, nil
}
