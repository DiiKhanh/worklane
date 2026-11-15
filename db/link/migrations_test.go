//go:build integration

package link_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestLinkSchema(t *testing.T) {
	db, dsn := newDatabase(t)
	dir, err := filepath.Abs("migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New("file://"+dir, "mysql://"+dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Up(); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	insert := func(id int64, code, tenant, destination string) error {
		_, err := db.Exec(`INSERT INTO links (id, code, tenant_id, long_url, created_at)
			VALUES (?, ?, ?, ?, '2026-11-15 12:00:00.123')`, id, code, tenant, destination)
		return err
	}

	t.Run("case identity and exact tenant dedup", func(t *testing.T) {
		const destination = "https://example.com/?a=1&b=2#part"
		if err := insert(10, "a", "tenant-a", destination); err != nil {
			t.Fatal(err)
		}
		if err := insert(36, "A", "tenant-b", destination); err != nil {
			t.Fatalf("case-sensitive code or cross-tenant destination rejected: %v", err)
		}
		assertMySQLError(t, insert(11, "b", "tenant-a", destination), 1062)
		assertMySQLError(t, insert(12, "a", "tenant-c", "https://other.example/"), 1062)
		for i, url := range []string{
			"https://example.com/?b=2&a=1#part", "https://example.com/?a=1&b=2#other",
			"https://example.com/", "https://example.com", "https://EXAMPLE.com/",
		} {
			if err := insert(int64(100+i), string(rune('c'+i)), "tenant-a", url); err != nil {
				t.Fatalf("distinct exact destination rejected: %v", err)
			}
		}
		var tenant string
		if err := db.QueryRow("SELECT tenant_id FROM links WHERE code = ?", "a").Scan(&tenant); err != nil || tenant != "tenant-a" {
			t.Fatalf("lowercase lookup: %q, %v", tenant, err)
		}
		if err := db.QueryRow("SELECT tenant_id FROM links WHERE code = ?", "A").Scan(&tenant); err != nil || tenant != "tenant-b" {
			t.Fatalf("uppercase lookup: %q, %v", tenant, err)
		}
		var hash string
		if err := db.QueryRow("SELECT HEX(long_url_hash) FROM links WHERE code = 'e'").Scan(&hash); err != nil || hash != "0F115DB062B7C0DD030B16878C99DEA5C354B49DC37B38EB8846179C7783E9D7" {
			t.Fatalf("SHA256 of exact URL: %q, %v", hash, err)
		}
	})

	t.Run("immutable mapping and permanent disable", func(t *testing.T) {
		for _, query := range []string{
			"UPDATE links SET long_url = 'https://changed.example/' WHERE code = 'a'",
			"UPDATE links SET code = 'z' WHERE code = 'a'",
			"UPDATE links SET id = 999 WHERE code = 'a'",
			"UPDATE links SET tenant_id = 'tenant-c' WHERE code = 'a'",
			"UPDATE links SET created_at = '2026-11-14' WHERE code = 'a'",
			"DELETE FROM links WHERE code = 'a'",
		} {
			_, err := db.Exec(query)
			assertMySQLError(t, err, 1644)
		}
		for range 2 {
			if _, err := db.Exec("UPDATE links SET disabled_at = '2026-11-15 13:00:00.123' WHERE code = 'a'"); err != nil {
				t.Fatalf("idempotent disable: %v", err)
			}
		}
		var state string
		if err := db.QueryRow("SELECT state FROM links WHERE code = 'a'").Scan(&state); err != nil || state != "disabled" {
			t.Fatalf("disabled state: %q, %v", state, err)
		}
		for _, value := range []string{"NULL", "'2026-11-15 14:00:00'"} {
			_, err := db.Exec("UPDATE links SET disabled_at = " + value + " WHERE code = 'a'")
			assertMySQLError(t, err, 1644)
		}
		assertMySQLError(t, insert(13, "j", "tenant-a", "https://example.com/?a=1&b=2#part"), 1062)
	})

	t.Run("byte limits and stable newest first order", func(t *testing.T) {
		for i, url := range []string{strings.Repeat("a", 4096), strings.Repeat("é", 2048)} {
			if err := insert(int64(200+i), string(rune('k'+i)), "tenant-page", url); err != nil {
				t.Fatalf("4096-byte destination rejected: %v", err)
			}
		}
		assertMySQLError(t, insert(202, "m", "tenant-page", strings.Repeat("é", 2049)), 3819)
		assertMySQLError(t, insert(203, "n", "tenant-page", ""), 3819)
		var newest string
		if err := db.QueryRow("SELECT code FROM links WHERE tenant_id = ? ORDER BY created_at DESC, id DESC LIMIT 1", "tenant-page").Scan(&newest); err != nil || newest != "l" {
			t.Fatalf("stable tie-break: %q, %v", newest, err)
		}
	})

	t.Run("canonical Base62 identity", func(t *testing.T) {
		for _, code := range []string{"", "a ", "a/b", "01", "a\n"} {
			assertMySQLError(t, insert(999, code, "tenant-code", "https://code.example/"), 3819)
		}
	})

	t.Run("concurrent dedup enforced by unique index", func(t *testing.T) {
		const workers = 8
		results := make(chan error, workers)
		var wg sync.WaitGroup
		for i := range workers {
			wg.Go(func() {
				results <- insert(int64(300+i), "C"+string(rune('a'+i)), "tenant-race", "https://concurrent.example/")
			})
		}
		wg.Wait()
		close(results)
		succeeded := 0
		for err := range results {
			if err == nil {
				succeeded++
			} else {
				assertMySQLError(t, err, 1062)
			}
		}
		if succeeded != 1 {
			t.Fatalf("got %d mappings for one destination", succeeded)
		}
	})

	t.Run("analytics ownership and detail cleanup preserve aggregates", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO link_clicks (tenant_id, code, ts, referrer_origin, device)
			VALUES ('tenant-a', 'A', '2026-11-15', '', 'unknown')`)
		assertMySQLError(t, err, 1452)
		_, err = db.Exec(`INSERT INTO link_daily_clicks (tenant_id, code, day, clicks)
			VALUES ('tenant-a', 'A', '2026-11-15', 1)`)
		assertMySQLError(t, err, 1452)
		_, err = db.Exec(`INSERT INTO link_clicks (tenant_id, code, ts, referrer_origin, device)
			VALUES ('tenant-a', 'a', '2026-10-01', 'https://example.com', 'desktop'),
			('tenant-a', 'a', '2026-11-15', '', 'unknown')`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO link_daily_clicks (tenant_id, code, day, clicks)
			VALUES ('tenant-a', 'a', '2026-10-01', 1), ('tenant-a', 'a', '2026-11-15', 1)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("DELETE FROM link_clicks WHERE ts < '2026-10-16'"); err != nil {
			t.Fatal(err)
		}
		var details, total int
		if err := db.QueryRow("SELECT COUNT(*) FROM link_clicks WHERE tenant_id = 'tenant-a' AND code = 'a'").Scan(&details); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT SUM(clicks) FROM link_daily_clicks WHERE tenant_id = 'tenant-a' AND code = 'a'").Scan(&total); err != nil || total != 2 || details != 1 {
			t.Fatalf("retention changed lifetime counts: details=%d total=%d err=%v", details, total, err)
		}
		_, err = db.Exec(`INSERT INTO link_clicks (tenant_id, code, ts, device) VALUES ('tenant-a', 'a', '2026-11-15', 'bot')`)
		assertMySQLError(t, err, 3819)
		_, err = db.Exec(`INSERT INTO link_daily_clicks (tenant_id, code, day, clicks) VALUES ('tenant-a', 'a', '2026-11-16', -1)`)
		assertMySQLError(t, err, 3819)
	})

	t.Run("rollback and reapply", func(t *testing.T) {
		if err := m.Down(); err != nil {
			t.Fatalf("rollback: %v", err)
		}
		if err := m.Up(); err != nil {
			t.Fatalf("reapply: %v", err)
		}
		if err := insert(1, "1", "tenant-new", "https://fresh.example/"); err != nil {
			t.Fatal(err)
		}
	})
}

func assertMySQLError(t *testing.T, err error, number uint16) {
	t.Helper()
	var mysqlErr *driver.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != number {
		t.Fatalf("want MySQL error %d, got %v", number, err)
	}
}

// A local socket may target a separately initialized, disposable MySQL server.
// Every run creates its own database; no existing database is migrated or erased.
func newDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	var dsn string
	if socket := os.Getenv("LINK_TEST_MYSQL_SOCKET"); socket != "" {
		config := driver.NewConfig()
		config.User, config.Net, config.Addr = "root", "unix", socket
		dsn = config.FormatDSN()
	} else {
		password := make([]byte, 16)
		if _, err := rand.Read(password); err != nil {
			t.Fatal(err)
		}
		ctr, err := tcmysql.Run(ctx, "mysql:8.0", tcmysql.WithDatabase("link_test"),
			tcmysql.WithUsername("root"), tcmysql.WithPassword(hex.EncodeToString(password)))
		if err != nil {
			t.Fatalf("start isolated MySQL: %v", err)
		}
		t.Cleanup(func() {
			if err := ctr.Terminate(ctx); err != nil {
				t.Errorf("terminate MySQL: %v", err)
			}
		})
		dsn, err = ctr.ConnectionString(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	config, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.DBName = ""
	admin, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "worklane_link_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Errorf("drop isolated database: %v", err)
		}
	})
	config.DBName, config.ParseTime, config.MultiStatements = name, true, true
	dsn = config.FormatDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dsn
}
