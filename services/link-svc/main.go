// Command link-svc exposes short-link management and public redirects.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/pkg/idgen"
	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/pkg/security"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/identity"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/kafkabus"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

type codeGenerator struct{ snowflake *idgen.Snowflake }

func (g codeGenerator) New() (int64, string, error) {
	id, err := g.snowflake.Next()
	if err != nil {
		return 0, "", err
	}
	return id, idgen.Encode(uint64(id)), nil
}

func required(name string) string {
	value := config.EnvOrFile(name, "")
	if value == "" {
		log.Fatalf("link-svc: %s is required", name)
	}
	return value
}

func main() {
	node, err := strconv.ParseInt(required("LINK_NODE_ID"), 10, 64)
	if err != nil {
		log.Fatal("link-svc: invalid LINK_NODE_ID")
	}
	ids, err := idgen.NewSnowflake(node, time.Now)
	if err != nil {
		log.Fatal("link-svc: invalid LINK_NODE_ID")
	}
	limit, err := strconv.Atoi(config.Env("LINK_CREATE_LIMIT", "10"))
	if err != nil {
		log.Fatal("link-svc: invalid LINK_CREATE_LIMIT")
	}
	window, err := time.ParseDuration(config.Env("LINK_CREATE_WINDOW", "1m"))
	if err != nil {
		log.Fatal("link-svc: invalid LINK_CREATE_WINDOW")
	}
	publicBase := required("LINK_PUBLIC_BASE")
	verifier, err := security.NewVerifier(required("AUTH_JWT_PUBLIC_KEY"))
	if err != nil {
		log.Fatal("link-svc: invalid AUTH_JWT_PUBLIC_KEY")
	}
	internalToken := required("INTERNAL_API_TOKEN")
	dbConfig, err := driver.ParseDSN(required("MYSQL_DSN"))
	if err != nil || dbConfig.DBName == "" {
		log.Fatal("link-svc: invalid MYSQL_DSN")
	}
	dbConfig.ParseTime, dbConfig.MultiStatements, dbConfig.Loc = true, true, time.UTC
	if dbConfig.Params == nil {
		dbConfig.Params = make(map[string]string)
	}
	dbConfig.Params["time_zone"] = "'+00:00'"
	dsn := dbConfig.FormatDSN()
	redisOptions, err := redis.ParseURL(config.Env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		log.Fatal("link-svc: invalid REDIS_URL")
	}
	// Cache operations have shorter deadlines than the HTTP request. The Redis
	// client must honor those deadlines when a server accepts but stops replying.
	redisOptions.ContextTimeoutEnabled = true
	rc := redis.NewClient(redisOptions)
	defer rc.Close()
	store, err := redisstore.New(rc, limit, window)
	if err != nil {
		log.Fatal("link-svc: invalid create quota configuration")
	}
	intro := identity.NewCached(identity.NewClient(config.Env("AUTH_SVC_URL", "http://localhost:8889"), internalToken), rc, config.EnvDuration("INTROSPECT_CACHE_TTL", time.Minute))
	// Validate HTTP configuration before any schema changes or listener startup.
	if _, err := linkhttp.NewRouter(nil, verifier, intro, publicBase, nil, nil, nil); err != nil {
		log.Fatal(err)
	}
	if err := mysql.Migrate(dsn, config.Env("MIGRATIONS_DIR", "db/link/migrations")); err != nil {
		log.Fatal("link-svc: migration failed")
	}
	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatal("link-svc: database unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("link-svc: database pool unavailable")
	}
	defer sqlDB.Close()
	publisher, err := kafkabus.New(strings.Split(config.Env("KAFKA_BROKERS", "localhost:9092"), ","))
	if err != nil {
		log.Fatal("link-svc: invalid Kafka configuration")
	}
	const clickQueueCapacity = 256
	clicks := app.NewEmitter(publisher, clickQueueCapacity)
	svc := app.New(mysqlrepo.New(db), store, store, codeGenerator{snowflake: ids})
	resolver := app.NewResolver(mysqlrepo.New(db), store, time.Now)
	router, err := linkhttp.NewRouter(svc, verifier, intro, publicBase, resolver, clicks, app.NewAnalytics(mysqlrepo.New(db), time.Now))
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: config.Env("HTTP_ADDR", ":8890"), Handler: router,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 32 * 1024}
	go func() {
		log.Printf("link-svc: listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("link-svc: server failed")
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Print("link-svc: shutdown failed")
	}
	if err := clicks.Close(ctx); err != nil {
		log.Print("link-svc: click drain deadline exceeded")
	}
	if err := publisher.Close(ctx); err != nil {
		log.Print("link-svc: Kafka shutdown deadline exceeded")
	}
	stats := clicks.Stats()
	log.Printf("link-svc: clicks published=%d overflow=%d failed=%d shutdown=%d", stats.Published, stats.Overflow, stats.Failed, stats.Shutdown)
}
