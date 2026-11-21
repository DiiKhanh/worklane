//go:build integration

package main_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	driver "github.com/go-sql-driver/mysql"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/platform/kafka"
	platformmysql "github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/inbound/consumer"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

type ingestionFixture struct {
	db           *sql.DB
	repo         *mysqlrepo.Repo
	admin        sarama.ClusterAdmin
	producer     sarama.SyncProducer
	brokers      []string
	topic, group string
	now          time.Time
}

func newIngestion(t *testing.T) *ingestionFixture {
	t.Helper()
	ctx := context.Background()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	unique := hex.EncodeToString(suffix)
	f := &ingestionFixture{topic: "link.clicked.test-" + unique, group: "link-test-" + unique, now: time.Date(2026, 11, 21, 0, 0, 1, 0, time.UTC)}
	cfg := driver.NewConfig()
	cfg.User, cfg.Net, cfg.Addr = "root", "unix", os.Getenv("LINK_TEST_MYSQL_SOCKET")
	if cfg.Addr == "" {
		container, err := tcmysql.Run(ctx, "mysql:8.0", tcmysql.WithUsername("root"), tcmysql.WithPassword(unique))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = container.Terminate(ctx) })
		dsn, err := container.ConnectionString(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err = driver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		cfg.DBName = ""
	}
	adminDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })
	name := "worklane_link_ingest_" + unique
	if _, err := adminDB.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := adminDB.Exec("DROP DATABASE `" + name + "`")
		if err != nil {
			t.Error(err)
		}
	})
	cfg.DBName, cfg.ParseTime, cfg.MultiStatements, cfg.Loc = name, true, true, time.UTC
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	cfg.Params["time_zone"] = "'+00:00'"
	dsn := cfg.FormatDSN()
	migrations, err := filepath.Abs("../../db/link/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if err := platformmysql.Migrate(dsn, migrations); err != nil {
		t.Fatal(err)
	}
	db, err := platformmysql.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	f.db, err = db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	f.repo = mysqlrepo.New(db)
	if _, err := f.db.Exec("INSERT INTO links (id, code, tenant_id, long_url) VALUES (1, 'a', 'owner', 'https://example.com/')"); err != nil {
		t.Fatal(err)
	}
	if address := os.Getenv("LINK_TEST_KAFKA_BROKERS"); address != "" {
		f.brokers = strings.Split(address, ",")
	} else {
		container, err := tcredpanda.Run(ctx, "redpandadata/redpanda:v23.3.3")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = container.Terminate(ctx) })
		address, err := container.KafkaSeedBroker(ctx)
		if err != nil {
			t.Fatal(err)
		}
		f.brokers = []string{address}
	}
	f.admin, err = sarama.NewClusterAdmin(f.brokers, sarama.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.admin.Close() })
	if err := consumer.EnsureTopic(f.brokers, f.topic); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.admin.DeleteTopic(f.topic) })
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	f.producer, err = sarama.NewSyncProducer(f.brokers, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.producer.Close() })
	return f
}

func (f *ingestionFixture) start(t *testing.T) (*consumer.Handler, func()) {
	t.Helper()
	h := consumer.New(app.New(f.repo, func() time.Time { return f.now }))
	c, err := kafka.NewConsumer(f.brokers, f.group, f.topic, h.Handle)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	stop := func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}
	return h, stop
}

func (f *ingestionFixture) publish(t *testing.T, event link.ClickedEvent) int64 {
	t.Helper()
	value, err := kafka.Wrap(event)
	if err != nil {
		t.Fatal(err)
	}
	_, offset, err := f.producer.SendMessage(&sarama.ProducerMessage{Topic: f.topic, Key: sarama.StringEncoder(event.Code), Value: sarama.ByteEncoder(value)})
	if err != nil {
		t.Fatal(err)
	}
	return offset
}

func (f *ingestionFixture) progress(t *testing.T) int64 {
	t.Helper()
	response, err := f.admin.ListConsumerGroupOffsets(f.group, map[string][]int32{f.topic: {0}})
	if err != nil {
		t.Fatal(err)
	}
	block := response.GetBlock(f.topic, 0)
	if block == nil {
		return -1
	}
	return block.Offset
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("pipeline did not reach expected state within 10 seconds")
}

func (f *ingestionFixture) counts(t *testing.T) (int64, int64) {
	t.Helper()
	var total, details int64
	if err := f.db.QueryRow("SELECT COALESCE(SUM(clicks), 0) FROM link_daily_clicks").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM link_clicks").Scan(&details); err != nil {
		t.Fatal(err)
	}
	return total, details
}

func TestKafkaClickPersistsBeforeProgressAndRedeliveryCountsAgain(t *testing.T) {
	f := newIngestion(t)
	_, stop := f.start(t)
	t.Cleanup(stop)
	event := link.ClickedEvent{TenantID: "owner", Code: "a", TS: f.now.Add(-time.Second), ReferrerOrigin: "https://example.com", Device: link.DeviceMobile}
	first := f.publish(t, event)
	eventually(t, func() bool { return f.progress(t) == first+1 })
	if total, details := f.counts(t); total != 1 || details != 1 {
		t.Fatalf("got total=%d details=%d", total, details)
	}
	// Previously emitted clicks remain ingestible after permanent disable.
	if _, err := f.db.Exec("UPDATE links SET disabled_at = UTC_TIMESTAMP(3) WHERE code = 'a'"); err != nil {
		t.Fatal(err)
	}
	second := f.publish(t, event)
	eventually(t, func() bool { return f.progress(t) == second+1 })
	if total, details := f.counts(t); total != 2 || details != 2 {
		t.Fatalf("redelivery got total=%d details=%d", total, details)
	}
	var day, origin, device string
	if err := f.db.QueryRow("SELECT DATE_FORMAT(day, '%Y-%m-%d') FROM link_daily_clicks").Scan(&day); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT referrer_origin, device FROM link_clicks LIMIT 1").Scan(&origin, &device); err != nil {
		t.Fatal(err)
	}
	if day != "2026-11-21" || origin != "https://example.com" || device != "mobile" {
		t.Fatalf("unexpected stored metadata: %s %s %s", day, origin, device)
	}
}
