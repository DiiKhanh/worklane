//go:build integration

package http_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/duykhanh/worklane/pkg/idgen"
	platformmysql "github.com/duykhanh/worklane/pkg/platform/mysql"
	platformredis "github.com/duykhanh/worklane/pkg/platform/redis"
	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/pkg/security/securitytest"
	linkhttp "github.com/duykhanh/worklane/services/link-svc/internal/adapters/inbound/http"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/identity"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/mysqlrepo"
	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/redisstore"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

type integrationIDs struct{ generator *idgen.Snowflake }

func (g integrationIDs) New() (int64, string, error) {
	id, err := g.generator.Next()
	return id, idgen.Encode(uint64(id)), err
}

type apiFixture struct {
	server       *httptest.Server
	issuer       *security.Issuer
	tenant       string
	other        string
	apiKey       string
	redis        *redis.Client
	db           *sql.DB
	repo         *mysqlrepo.Repo
	identityDown atomic.Bool
}

func newAPI(t *testing.T, limit int, window time.Duration) *apiFixture {
	t.Helper()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	unique := hex.EncodeToString(suffix)
	f := &apiFixture{tenant: "owner-" + unique, other: "other-" + unique, apiKey: "test-api-" + unique}
	dsn := isolatedDSN(t, "worklane_link_http_"+unique)
	migrations, err := filepath.Abs("../../../../../../db/link/migrations")
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
	f.redis = isolatedRedis(t)
	store, err := redisstore.New(f.redis, limit, window)
	if err != nil {
		t.Fatal(err)
	}
	priv, pub := securitytest.KeyPair(t)
	f.issuer, err = security.NewIssuer(priv, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := security.NewVerifier(pub)
	if err != nil {
		t.Fatal(err)
	}
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.identityDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/internal/introspect" || r.Header.Get("X-Internal-Token") != "test-internal-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"active": body.Token == f.apiKey, "tenant_id": f.tenant})
	}))
	t.Cleanup(identityServer.Close)
	intro := identity.NewCached(identity.NewClient(identityServer.URL, "test-internal-token"), f.redis, time.Minute)
	// Database timestamps stay fixed for pagination, while codes remain unique
	// across fixtures that share the disposable Redis instance.
	generator, err := idgen.NewSnowflake(13, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.repo = mysqlrepo.New(db)
	svc := app.New(f.repo, store, store, integrationIDs{generator: generator})
	resolver := app.NewResolver(f.repo, store, time.Now)
	router, err := linkhttp.NewRouter(svc, verifier, intro, "https://short.example/", resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}

func isolatedDSN(t *testing.T, name string) string {
	t.Helper()
	ctx := context.Background()
	var config *driver.Config
	if socket := os.Getenv("LINK_TEST_MYSQL_SOCKET"); socket != "" {
		config = driver.NewConfig()
		config.User, config.Net, config.Addr = "root", "unix", socket
	} else {
		password := make([]byte, 16)
		if _, err := rand.Read(password); err != nil {
			t.Fatal(err)
		}
		container, err := tcmysql.Run(ctx, "mysql:8.0", tcmysql.WithUsername("root"), tcmysql.WithPassword(hex.EncodeToString(password)))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = container.Terminate(ctx) })
		dsn, err := container.ConnectionString(ctx)
		if err != nil {
			t.Fatal(err)
		}
		config, err = driver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		config.DBName = ""
	}
	admin, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if _, err := admin.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error(err)
		}
	})
	config.DBName, config.ParseTime, config.MultiStatements, config.Loc = name, true, true, time.UTC
	if config.Params == nil {
		config.Params = make(map[string]string)
	}
	config.Params["time_zone"] = "'+00:00'"
	// Freeze database time so pagination exercises the ID tie-breaker on every row.
	config.Params["timestamp"] = "1794830400.123"
	return config.FormatDSN()
}

func isolatedRedis(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("LINK_TEST_REDIS_URL")
	if url == "" {
		container, err := tcredis.Run(context.Background(), "redis:7")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })
		url, err = container.ConnectionString(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	client, err := platformredis.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func (f *apiFixture) jwt(t *testing.T, tenant string) string {
	t.Helper()
	token, _, err := f.issuer.Issue("test-user", tenant, "test@example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (f *apiFixture) request(t *testing.T, method, path, token string, body any) *http.Response {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	return f.rawRequest(t, method, path, token, data)
}

func (f *apiFixture) rawRequest(t *testing.T, method, path, token string, data []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readResponse[T any](t *testing.T, response *http.Response, status int) T {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("want status %d, got %d: %s", status, response.StatusCode, body)
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
