// Command otp-dispatcher is the asynchronous delivery service: it consumes otp.requested
// from Kafka, sends the email via Resend, records the delivery, and publishes
// otp.sent / otp.failed (and otp.dlq on failure). When NOTIFICATION_MYSQL_DSN is set it
// also consumes the per-channel notification.*.requested topics (see notification.go).
//
// Composition root: reads config, builds adapters, injects them into the handler, and
// runs the consumer until interrupted.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	redisplatform "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/inbound/consumer"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/resendmail"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/smtpmail"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/templatestore"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/twiliosms"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
	redisURL := config.Env("REDIS_URL", "redis://localhost:6379/0")
	brokers := config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"})
	requestedTopic := config.Env("KAFKA_TOPIC_REQUESTED", "otp.requested")
	group := config.Env("KAFKA_GROUP", "otp-dispatcher")
	resendKey := config.Env("RESEND_API_KEY", "")
	resendFrom := config.Env("RESEND_FROM", "OTP <otp@worklane.dev>")
	resendBase := config.Env("RESEND_BASE_URL", "https://api.resend.com")
	twilioSID := config.Env("TWILIO_ACCOUNT_SID", "")
	twilioToken := config.Env("TWILIO_AUTH_TOKEN", "")
	twilioFrom := config.Env("TWILIO_FROM", "")
	twilioBase := config.Env("TWILIO_BASE_URL", "https://api.twilio.com")

	notif, err := loadNotificationSettings()
	if err != nil {
		log.Fatalf("otp-dispatcher: notification config: %v", err)
	}

	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("otp-dispatcher: mysql: %v", err)
	}
	rc, err := redisplatform.Open(redisURL)
	if err != nil {
		log.Fatalf("otp-dispatcher: redis: %v", err)
	}
	prod, err := kafka.NewProducer(brokers)
	if err != nil {
		log.Fatalf("otp-dispatcher: kafka producer: %v", err)
	}
	defer func() { _ = prod.Close() }()

	// Provider is chosen by config: SMTP (MailHog) for local/e2e, Resend for production.
	// Both satisfy app.EmailProvider, so nothing downstream changes.
	var mail app.EmailProvider
	providerLabel := config.Env("EMAIL_PROVIDER", "resend")
	switch providerLabel {
	case "smtp":
		mail = smtpmail.New(
			config.Env("SMTP_HOST", "mailhog"), config.EnvInt("SMTP_PORT", 1025),
			resendFrom, config.Env("SMTP_USER", ""), config.Env("SMTP_PASS", ""),
		)
		log.Println("otp-dispatcher: using SMTP email provider")
	default:
		mail = resendmail.New(resendKey, resendFrom, resendBase, &http.Client{Timeout: 10 * time.Second})
		log.Println("otp-dispatcher: using Resend email provider")
	}
	repo := mysqlrepo.New(db)

	sms := twiliosms.New(twilioSID, twilioToken, twilioFrom, twilioBase, &http.Client{Timeout: 10 * time.Second})

	// Templates come from the DB (single source of truth) via Redis cache-aside; the env
	// values below are the fallback used only when no active DB template exists. Bodies use
	// {{code}}/{{expiry}} (rendered by pkg/templating), not printf %s.
	templates := templatestore.New(db, rc, config.EnvDuration("TEMPLATE_CACHE_TTL", 10*time.Minute))
	fallback := map[string]app.Template{
		"email": {
			Subject: config.Env("OTP_EMAIL_SUBJECT", "Your verification code"),
			Body:    config.Env("OTP_EMAIL_BODY", "Your verification code is {{code}}. It expires in {{expiry}}."),
		},
		"sms": {
			Body: config.Env("OTP_SMS_BODY", "Your verification code is {{code}}. It expires in {{expiry}}."),
		},
	}

	// One Sender registry serves both the OTP handler and the notification handler.
	senders := map[string]app.Sender{
		"email": app.NewEmailSender(mail, providerLabel),
		"sms":   app.NewSMSSender(sms, "twilio"),
	}
	handler := app.NewHandler(app.Deps{
		Senders:   senders,
		Templates: templates,
		Repo:      repo,
		Pub:       prod,
		Clock:     realClock{},
	}, app.Config{
		SentTopic:   config.Env("KAFKA_TOPIC_SENT", "otp.sent"),
		FailedTopic: config.Env("KAFKA_TOPIC_FAILED", "otp.failed"),
		DLQTopic:    config.Env("KAFKA_TOPIC_DLQ", "otp.dlq"),
		ExpiryText:  config.Env("OTP_EXPIRY_TEXT", "5 minutes"),
		Fallback:    fallback,
	})

	cons, err := kafka.NewConsumer(brokers, group, requestedTopic, consumer.New(handler).Handle)
	if err != nil {
		log.Fatalf("otp-dispatcher: kafka consumer: %v", err)
	}
	defer func() { _ = cons.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Generic notifications are opt-in: without NOTIFICATION_MYSQL_DSN the dispatcher
	// stays a pure OTP worker, so a missing notification DB can never take OTP down.
	if notif.enabled() {
		closeNotif, err := startNotificationConsumers(ctx, notif, brokers, senders, prod)
		if err != nil {
			log.Fatalf("otp-dispatcher: %v", err)
		}
		defer closeNotif()
	} else {
		log.Println("otp-dispatcher: notification handlers disabled (NOTIFICATION_MYSQL_DSN not set)")
	}

	go func() {
		log.Printf("otp-dispatcher: consuming %s (group %s)", requestedTopic, group)
		if err := cons.Start(ctx); err != nil {
			log.Fatalf("otp-dispatcher: consume: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("otp-dispatcher: shutting down")
	cancel()
}
