// Command link-dispatcher persists asynchronous click analytics from Kafka.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/inbound/consumer"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

const (
	cleanupInterval  = time.Minute
	cleanupBatchSize = 1000
	cleanupTimeout   = 2 * time.Second
	shutdownTimeout  = 10 * time.Second
)

func main() {
	dsn := config.EnvOrFile("MYSQL_DSN", "")
	dbConfig, err := driver.ParseDSN(dsn)
	if dsn == "" || err != nil || dbConfig.DBName == "" {
		log.Fatal("link-dispatcher: MYSQL_DSN is required and must name the link database")
	}
	dbConfig.ParseTime, dbConfig.Loc = true, time.UTC
	if dbConfig.Params == nil {
		dbConfig.Params = make(map[string]string)
	}
	dbConfig.Params["time_zone"] = "'+00:00'"
	db, err := mysql.Open(dbConfig.FormatDSN())
	if err != nil {
		log.Fatal("link-dispatcher: database unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("link-dispatcher: database pool unavailable")
	}
	defer sqlDB.Close()
	brokers := config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"})
	topic := config.Env("KAFKA_TOPIC_CLICKED", "link.clicked")
	if err := consumer.EnsureTopic(brokers, topic); err != nil {
		log.Fatal("link-dispatcher: Kafka topic retention configuration failed")
	}
	repo := mysqlrepo.New(db)
	handler := consumer.New(app.New(repo, time.Now))
	c, err := kafka.NewConsumer(brokers, config.Env("KAFKA_GROUP", "link-dispatcher"), topic, handler.Handle)
	if err != nil {
		log.Fatal("link-dispatcher: Kafka unavailable")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	consumed := make(chan error, 1)
	cleaned := make(chan struct{})
	go func() { consumed <- c.Start(ctx) }()
	go func() { defer close(cleaned); cleanup(ctx, repo) }()
	unexpectedStop := false
	select {
	case <-ctx.Done():
	case err := <-consumed:
		unexpectedStop = true
		// Preserve the completion signal for the shared shutdown path.
		consumed <- err
	}
	cancel()
	deadline, finish := context.WithTimeout(context.Background(), shutdownTimeout)
	defer finish()
	closed := make(chan struct{})
	go func() { <-consumed; _ = c.Close(); <-cleaned; close(closed) }()
	select {
	case <-closed:
	case <-deadline.Done():
		log.Print("link-dispatcher: shutdown deadline exceeded")
	}
	rejected, failed := handler.Counts()
	log.Printf("link-dispatcher: rejected=%d persistence_failures=%d", rejected, failed)
	if unexpectedStop {
		log.Fatal("link-dispatcher: consumer stopped")
	}
}

func cleanup(ctx context.Context, repo *mysqlrepo.Repo) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		batchCtx, cancel := context.WithTimeout(ctx, cleanupTimeout)
		deleted, err := repo.Expire(batchCtx, time.Now(), cleanupBatchSize)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Print("link-dispatcher: detail cleanup failed")
		}
		if err == nil && deleted == cleanupBatchSize {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
