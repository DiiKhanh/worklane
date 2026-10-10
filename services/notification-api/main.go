// Command notification-api is the notification platform's synchronous HTTP service: it
// manages templates, accepts sends for authenticated tenants (preferences, rate limits,
// idempotency) and publishes them to the per-channel notification.*.requested topics
// for the dispatcher to deliver.
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

	"github.com/google/uuid"

	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	redisplatform "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/pkg/templating"
	notifhttp "github.com/duykhanh/worklane/services/notification-api/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/notification-api/internal/adapters/outbound/identity"
	"github.com/duykhanh/worklane/services/notification-api/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/notification-api/internal/adapters/outbound/rediscounter"
	"github.com/duykhanh/worklane/services/notification-api/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// uuidGen implements app.IDGen with random (v4) UUIDs, 36 chars like the id columns.
type uuidGen struct{}

func (uuidGen) New() string { return uuid.NewString() }

// noShortener implements app.Shortener until the link-svc adapter exists (how this
// service authenticates to link-svc on a tenant's behalf is still undecided). Previewing
// a template that contains {{link}} fails with templating.ErrNoShortener instead of
// showing a URL that delivery would not produce; templates without links are unaffected.
type noShortener struct{}

func (noShortener) Shorten(context.Context, string, string) (string, error) {
	return "", templating.ErrNoShortener
}

func main() {
	cfg, err := loadSettings()
	if err != nil {
		log.Fatalf("notification-api: config: %v", err)
	}

	// Schema first, so the service comes up against an up-to-date database.
	// notification-api owns the notification schema; the dispatcher only writes to it.
	if err := mysql.Migrate(cfg.MySQLDSN, cfg.MigrationsDir); err != nil {
		log.Fatalf("notification-api: migrate: %v", err)
	}
	db, err := mysql.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("notification-api: mysql: %v", err)
	}
	rc, err := redisplatform.Open(cfg.RedisURL)
	if err != nil {
		log.Fatalf("notification-api: redis: %v", err)
	}
	prod, err := kafka.NewProducer(cfg.Brokers)
	if err != nil {
		log.Fatalf("notification-api: kafka: %v", err)
	}
	defer func() { _ = prod.Close() }()

	verifier, err := security.NewVerifier(cfg.JWTPublicKey)
	if err != nil {
		log.Fatalf("notification-api: verifier: %v", err)
	}
	// Machine API keys are resolved by asking auth-svc (introspection), cached in Redis so
	// the hot path is amortized O(1). Human JWTs never touch this - they verify locally.
	introspector := identity.NewCached(identity.NewClient(cfg.AuthSvcURL, cfg.InternalToken), rc, cfg.IntrospectTTL)

	// Adapters implement the ports; the three repos share one connection pool.
	svc := app.New(app.Deps{
		Templates: mysqlrepo.NewTemplates(db),
		Log:       mysqlrepo.NewLog(db),
		Settings:  mysqlrepo.NewSettings(db),
		Counter:   rediscounter.New(rc),
		Pub:       prod,
		Shortener: noShortener{},
		IDs:       uuidGen{},
		Clock:     realClock{},
	}, cfg.App)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           notifhttp.NewRouter(svc, verifier, introspector),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start the server, then block until an interrupt for a graceful shutdown.
	go func() {
		log.Printf("notification-api: listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("notification-api: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("notification-api: shutdown: %v", err)
	}
	log.Println("notification-api: stopped")
}
