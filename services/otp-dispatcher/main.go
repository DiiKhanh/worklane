// Command otp-dispatcher is the asynchronous delivery service: it consumes otp.requested
// from Kafka, sends the email via Resend, records the delivery, and publishes
// otp.sent / otp.failed (and otp.dlq on failure).
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
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/inbound/consumer"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/resendmail"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/smtpmail"
	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
	brokers := config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"})
	requestedTopic := config.Env("KAFKA_TOPIC_REQUESTED", "otp.requested")
	group := config.Env("KAFKA_GROUP", "otp-dispatcher")
	resendKey := config.Env("RESEND_API_KEY", "")
	resendFrom := config.Env("RESEND_FROM", "OTP <otp@worklane.dev>")
	resendBase := config.Env("RESEND_BASE_URL", "https://api.resend.com")

	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("otp-dispatcher: mysql: %v", err)
	}
	prod, err := kafka.NewProducer(brokers)
	if err != nil {
		log.Fatalf("otp-dispatcher: kafka producer: %v", err)
	}
	defer func() { _ = prod.Close() }()

	// Provider is chosen by config: SMTP (MailHog) for local/e2e, Resend for production.
	// Both satisfy app.EmailProvider, so nothing downstream changes.
	var mail app.EmailProvider
	switch config.Env("EMAIL_PROVIDER", "resend") {
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

	handler := app.NewHandler(app.Deps{
		Mail: mail, Repo: repo, Pub: prod, Clock: realClock{},
	}, app.Config{
		SentTopic:   config.Env("KAFKA_TOPIC_SENT", "otp.sent"),
		FailedTopic: config.Env("KAFKA_TOPIC_FAILED", "otp.failed"),
		DLQTopic:    config.Env("KAFKA_TOPIC_DLQ", "otp.dlq"),
		Template: app.Template{
			Subject: config.Env("OTP_EMAIL_SUBJECT", "Your verification code"),
			BodyFmt: config.Env("OTP_EMAIL_BODY", "Your verification code is %s. It expires in 5 minutes."),
		},
	})

	cons, err := kafka.NewConsumer(brokers, group, requestedTopic, consumer.New(handler).Handle)
	if err != nil {
		log.Fatalf("otp-dispatcher: kafka consumer: %v", err)
	}
	defer func() { _ = cons.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
