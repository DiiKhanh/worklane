// Command auth-svc is the identity service: it authenticates dashboard users
// (email+password) and issues short-lived EdDSA JWTs.
//
// This file is the COMPOSITION ROOT - the one place allowed to know every concrete type.
// It reads config, builds the infrastructure adapters, injects them into the use case,
// and starts the HTTP server.
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
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	redisplatform "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/pkg/security"
	authhttp "github.com/duykhanh/worklane/services/auth-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/auth-svc/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/auth-svc/internal/adapters/outbound/ratelimit"
	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
	redisURL := config.Env("REDIS_URL", "redis://localhost:6379/0")
	priv := config.EnvOrFile("AUTH_JWT_PRIVATE_KEY", "")
	pub := config.EnvOrFile("AUTH_JWT_PUBLIC_KEY", "")
	ttl := config.EnvDuration("AUTH_TOKEN_TTL", time.Hour)
	httpAddr := config.Env("HTTP_ADDR", ":8889")

	if priv == "" || pub == "" {
		log.Fatal("auth-svc: AUTH_JWT_PRIVATE_KEY and AUTH_JWT_PUBLIC_KEY are required")
	}
	issuer, err := security.NewIssuer(priv, ttl)
	if err != nil {
		log.Fatalf("auth-svc: issuer: %v", err)
	}
	verifier, err := security.NewVerifier(pub)
	if err != nil {
		log.Fatalf("auth-svc: verifier: %v", err)
	}

	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("auth-svc: mysql: %v", err)
	}
	rc, err := redisplatform.Open(redisURL)
	if err != nil {
		log.Fatalf("auth-svc: redis: %v", err)
	}

	limiter := ratelimit.New(rc, config.EnvInt("LOGIN_RATE_MAX", 10), config.EnvDuration("LOGIN_RATE_WINDOW", 15*time.Minute))
	svc := app.New(app.Deps{Repo: mysqlrepo.New(db), Issuer: issuer, Limiter: limiter, Clock: realClock{}})

	srv := &http.Server{Addr: httpAddr, Handler: authhttp.NewRouter(svc, verifier), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("auth-svc: listening on %s", httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("auth-svc: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("auth-svc: shutdown: %v", err)
	}
	log.Println("auth-svc: stopped")
}
