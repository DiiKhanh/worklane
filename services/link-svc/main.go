// Command link-svc is the URL shortener's synchronous HTTP service: it creates short
// links for authenticated tenants, serves the public 302 redirect from a Redis
// cache-aside lookup, and publishes link.clicked for link-dispatcher to persist.
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

	"github.com/duykhanh/worklane/pkg/idgen"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	redisplatform "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/pkg/security"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/identity"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/rediscache"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	cfg, err := loadSettings()
	if err != nil {
		log.Fatalf("link-svc: config: %v", err)
	}

	// Schema first, so the service comes up against an up-to-date database. link-svc
	// owns the link schema; link-dispatcher only writes to it.
	if err := mysql.Migrate(cfg.MySQLDSN, cfg.MigrationsDir); err != nil {
		log.Fatalf("link-svc: migrate: %v", err)
	}
	db, err := mysql.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("link-svc: mysql: %v", err)
	}
	rc, err := redisplatform.Open(cfg.RedisURL)
	if err != nil {
		log.Fatalf("link-svc: redis: %v", err)
	}
	prod, err := kafka.NewProducer(cfg.Brokers)
	if err != nil {
		log.Fatalf("link-svc: kafka: %v", err)
	}
	defer func() { _ = prod.Close() }()

	verifier, err := security.NewVerifier(cfg.JWTPublicKey)
	if err != nil {
		log.Fatalf("link-svc: verifier: %v", err)
	}
	// Machine API keys are resolved by asking auth-svc (introspection), cached in Redis so
	// the hot path is amortized O(1). Human JWTs never touch this - they verify locally.
	introspector := identity.NewCached(identity.NewClient(cfg.AuthSvcURL, cfg.InternalToken), rc, cfg.IntrospectTTL)

	// Each replica needs its own node id, or two pods can mint the same short code.
	ids, err := idgen.NewSnowflake(cfg.NodeID)
	if err != nil {
		log.Fatalf("link-svc: idgen: %v", err)
	}

	// Adapters implement the ports; store is both Cache and Counter.
	store := rediscache.New(rc)
	svc := app.New(app.Deps{
		Repo: mysqlrepo.New(db), Cache: store, Counter: store, Pub: prod, IDs: ids, Clock: realClock{},
	}, cfg.App)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           linkhttp.NewRouter(svc, verifier, introspector),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start the server, then block until an interrupt for a graceful shutdown.
	go func() {
		log.Printf("link-svc: listening on %s (node %d)", cfg.HTTPAddr, cfg.NodeID)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("link-svc: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("link-svc: shutdown: %v", err)
	}
	log.Println("link-svc: stopped")
}
