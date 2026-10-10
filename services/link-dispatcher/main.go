// Command link-dispatcher is the click ingest service: it consumes link.clicked from
// Kafka and appends each click to link_clicks, keeping the database write off link-svc's
// redirect hot path.
//
// Composition root: reads config, builds adapters, injects them into the handler, and
// runs the consumer until interrupted.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/inbound/consumer"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func main() {
	// The DSN must keep the driver default loc=UTC: clicks are bucketed by UTC day.
	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/link?parseTime=true&multiStatements=true")
	brokers := config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"})
	clickedTopic := config.Env("KAFKA_TOPIC_LINK_CLICKED", "link.clicked")
	group := config.Env("KAFKA_GROUP", "link-dispatcher")

	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("link-dispatcher: mysql: %v", err)
	}

	handler := app.NewHandler(app.Deps{Repo: mysqlrepo.New(db), Clock: realClock{}})

	cons, err := kafka.NewConsumer(brokers, group, clickedTopic, consumer.New(handler).Handle)
	if err != nil {
		log.Fatalf("link-dispatcher: kafka consumer: %v", err)
	}
	defer func() { _ = cons.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		log.Printf("link-dispatcher: consuming %s (group %s)", clickedTopic, group)
		if err := cons.Start(ctx); err != nil {
			log.Fatalf("link-dispatcher: consume: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("link-dispatcher: shutting down")
	cancel()
}
