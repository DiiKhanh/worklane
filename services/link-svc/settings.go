package main

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

// maxNodeID is the largest Snowflake node id (10 bits, see pkg/idgen).
const maxNodeID = 1023

// settings is everything link-svc reads from the environment, validated once at startup
// so a bad value stops the process instead of surfacing later as a subtle runtime bug.
type settings struct {
	MySQLDSN      string
	RedisURL      string
	Brokers       []string
	HTTPAddr      string
	MigrationsDir string

	AuthSvcURL    string
	InternalToken string
	JWTPublicKey  string
	IntrospectTTL time.Duration

	NodeID int64
	App    app.Config
}

// loadSettings reads the environment and rejects values the service cannot run with.
func loadSettings() (settings, error) {
	nodeID, err := nodeIDFromEnv()
	if err != nil {
		return settings{}, err
	}
	s := settings{
		// The DSN must keep the driver default loc=UTC: clicks are bucketed by UTC day.
		MySQLDSN:      config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/link?parseTime=true&multiStatements=true"),
		RedisURL:      config.Env("REDIS_URL", "redis://localhost:6379/0"),
		Brokers:       config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"}),
		HTTPAddr:      config.Env("HTTP_ADDR", ":8890"),
		MigrationsDir: config.Env("MIGRATIONS_DIR", "db/link/migrations"),

		AuthSvcURL:    config.Env("AUTH_SVC_URL", "http://localhost:8889"),
		InternalToken: config.EnvOrFile("INTERNAL_API_TOKEN", ""),
		JWTPublicKey:  config.EnvOrFile("AUTH_JWT_PUBLIC_KEY", ""),
		IntrospectTTL: config.EnvDuration("INTROSPECT_CACHE_TTL", time.Minute),

		NodeID: nodeID,
		App: app.Config{
			PublicBase:          config.Env("LINK_PUBLIC_BASE", "http://localhost:8890"),
			CacheTTL:            config.EnvDuration("LINK_CACHE_TTL", time.Hour),
			ClickedTopic:        config.Env("KAFKA_TOPIC_LINK_CLICKED", "link.clicked"),
			CreateLimitMax:      config.EnvInt("LINK_LIMIT_TENANT", 100),
			CreateLimitWindow:   config.EnvDuration("LINK_WINDOW_TENANT", time.Hour),
			MaxInflightClicks:   config.EnvInt("LINK_MAX_INFLIGHT_CLICKS", 256),
			ClickPublishTimeout: config.EnvDuration("LINK_CLICK_PUBLISH_TIMEOUT", 2*time.Second),
		},
	}
	if err := s.validate(); err != nil {
		return settings{}, err
	}
	return s, nil
}

// nodeIDFromEnv parses LINK_NODE_ID strictly. Unlike the other knobs it must not fall
// back to a default on a typo: two replicas silently sharing node 0 can mint the same
// Snowflake id, and therefore the same short code.
func nodeIDFromEnv() (int64, error) {
	raw := config.Env("LINK_NODE_ID", "0")
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("LINK_NODE_ID %q is not an integer", raw)
	}
	if n < 0 || n > maxNodeID {
		return 0, fmt.Errorf("LINK_NODE_ID %d is out of range 0..%d", n, maxNodeID)
	}
	return n, nil
}

func (s settings) validate() error {
	if s.JWTPublicKey == "" {
		return errors.New("AUTH_JWT_PUBLIC_KEY is required")
	}
	if s.InternalToken == "" {
		return errors.New("INTERNAL_API_TOKEN is required")
	}
	base, err := url.Parse(s.App.PublicBase)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return fmt.Errorf("LINK_PUBLIC_BASE %q must be an absolute http(s) URL", s.App.PublicBase)
	}
	if s.App.ClickedTopic == "" {
		return errors.New("KAFKA_TOPIC_LINK_CLICKED must not be empty")
	}
	// A non-positive TTL would make Redis reject the cache write or, for the limiter,
	// expire the counter on its first hit so the limit never triggers.
	if s.App.CacheTTL <= 0 {
		return errors.New("LINK_CACHE_TTL must be positive")
	}
	if s.App.CreateLimitWindow <= 0 {
		return errors.New("LINK_WINDOW_TENANT must be positive")
	}
	if s.App.CreateLimitMax <= 0 {
		return errors.New("LINK_LIMIT_TENANT must be positive")
	}
	if s.App.MaxInflightClicks <= 0 {
		return errors.New("LINK_MAX_INFLIGHT_CLICKS must be positive")
	}
	if s.App.ClickPublishTimeout <= 0 {
		return errors.New("LINK_CLICK_PUBLISH_TIMEOUT must be positive")
	}
	return nil
}
