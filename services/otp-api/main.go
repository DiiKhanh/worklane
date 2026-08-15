// Command otp-api is the synchronous HTTP service: it validates requests, enforces
// API-key auth and rate limits, issues/verifies OTP codes, and publishes otp.requested.
//
// This file is the COMPOSITION ROOT - the one place allowed to know every concrete type.
// It reads config, builds the infrastructure adapters, injects them into the use cases,
// and starts the HTTP server. Nothing else in the service wires dependencies.
package main

import (
	"context"
	"errors"
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
	otphttp "github.com/duykhanh/worklane/services/otp-api/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/otp-api/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/otp-api/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
	redisURL := config.Env("REDIS_URL", "redis://localhost:6379/0")
	brokers := config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"})
	requestedTopic := config.Env("KAFKA_TOPIC_REQUESTED", "otp.requested")
	httpAddr := config.Env("HTTP_ADDR", ":8888")
	migrationsDir := config.Env("MIGRATIONS_DIR", "db/otp/migrations")

	// Schema first, so the service comes up against an up-to-date database.
	if err := mysql.Migrate(dsn, migrationsDir); err != nil {
		log.Fatalf("otp-api: migrate: %v", err)
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("otp-api: mysql: %v", err)
	}
	rc, err := redisplatform.Open(redisURL)
	if err != nil {
		log.Fatalf("otp-api: redis: %v", err)
	}
	prod, err := kafka.NewProducer(brokers)
	if err != nil {
		log.Fatalf("otp-api: kafka: %v", err)
	}
	defer func() { _ = prod.Close() }()

	// Adapters implement the ports; store is both CodeStore and Counter.
	repo := mysqlrepo.New(db)
	store := redisstore.New(rc)

	svc := app.New(app.Deps{
		Store: store, Counter: store, Repo: repo, Pub: prod, Clock: realClock{},
	}, app.Config{
		CodeLength:        config.EnvInt("OTP_CODE_LENGTH", 6),
		TTL:               config.EnvDuration("OTP_TTL", 5*time.Minute),
		MaxVerifyAttempts: config.EnvInt("OTP_MAX_ATTEMPTS", 3),
		RequestedTopic:    requestedTopic,
		Limits: app.LimitConfig{
			PerRecipientMax:    config.EnvInt("OTP_LIMIT_RECIPIENT", 5),
			PerRecipientWindow: config.EnvDuration("OTP_WINDOW_RECIPIENT", time.Hour),
			PerTenantMax:       config.EnvInt("OTP_LIMIT_TENANT", 100),
			PerTenantWindow:    config.EnvDuration("OTP_WINDOW_TENANT", time.Hour),
			ResendCooldown:     config.EnvDuration("OTP_RESEND_COOLDOWN", time.Minute),
		},
	})

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           otphttp.NewRouter(svc, repo),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start the server, then block until an interrupt for a graceful shutdown.
	go func() {
		log.Printf("otp-api: listening on %s", httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("otp-api: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("otp-api: shutdown: %v", err)
	}
	log.Println("otp-api: stopped")
}
