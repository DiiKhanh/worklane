package main

import (
	"errors"
	"time"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/services/notification-api/internal/app"
)

// settings is everything notification-api reads from the environment, validated once at
// startup so a bad value stops the process instead of surfacing later as a subtle
// runtime bug.
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

	App app.Config
}

// loadSettings reads the environment and rejects values the service cannot run with.
func loadSettings() (settings, error) {
	s := settings{
		MySQLDSN:      config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/notification?parseTime=true&multiStatements=true"),
		RedisURL:      config.Env("REDIS_URL", "redis://localhost:6379/0"),
		Brokers:       config.EnvList("KAFKA_BROKERS", []string{"localhost:9092"}),
		HTTPAddr:      config.Env("HTTP_ADDR", ":8891"),
		MigrationsDir: config.Env("MIGRATIONS_DIR", "db/notification/migrations"),

		AuthSvcURL:    config.Env("AUTH_SVC_URL", "http://localhost:8889"),
		InternalToken: config.EnvOrFile("INTERNAL_API_TOKEN", ""),
		JWTPublicKey:  config.EnvOrFile("AUTH_JWT_PUBLIC_KEY", ""),
		IntrospectTTL: config.EnvDuration("INTROSPECT_CACHE_TTL", time.Minute),

		App: app.Config{
			EmailTopic:        config.Env("KAFKA_TOPIC_NOTIFICATION_EMAIL", "notification.email.requested"),
			SMSTopic:          config.Env("KAFKA_TOPIC_NOTIFICATION_SMS", "notification.sms.requested"),
			TenantLimitMax:    config.EnvInt("NOTIFICATION_LIMIT_TENANT", 1000),
			TenantLimitWindow: config.EnvDuration("NOTIFICATION_WINDOW_TENANT", time.Hour),
			UserLimitMax:      config.EnvInt("NOTIFICATION_LIMIT_USER", 10),
			UserLimitWindow:   config.EnvDuration("NOTIFICATION_WINDOW_USER", time.Hour),
			ListLimit:         config.EnvInt("NOTIFICATION_LIST_LIMIT", 50),
		},
	}
	if err := s.validate(); err != nil {
		return settings{}, err
	}
	return s, nil
}

func (s settings) validate() error {
	if s.JWTPublicKey == "" {
		return errors.New("AUTH_JWT_PUBLIC_KEY is required")
	}
	if s.InternalToken == "" {
		return errors.New("INTERNAL_API_TOKEN is required")
	}
	if s.App.EmailTopic == "" {
		return errors.New("KAFKA_TOPIC_NOTIFICATION_EMAIL must not be empty")
	}
	if s.App.SMSTopic == "" {
		return errors.New("KAFKA_TOPIC_NOTIFICATION_SMS must not be empty")
	}
	// A non-positive window would expire the counter on its first hit so the limit
	// never triggers; a non-positive max would reject every send.
	if s.App.TenantLimitMax <= 0 {
		return errors.New("NOTIFICATION_LIMIT_TENANT must be positive")
	}
	if s.App.TenantLimitWindow <= 0 {
		return errors.New("NOTIFICATION_WINDOW_TENANT must be positive")
	}
	if s.App.UserLimitMax <= 0 {
		return errors.New("NOTIFICATION_LIMIT_USER must be positive")
	}
	if s.App.UserLimitWindow <= 0 {
		return errors.New("NOTIFICATION_WINDOW_USER must be positive")
	}
	if s.App.ListLimit <= 0 {
		return errors.New("NOTIFICATION_LIST_LIMIT must be positive")
	}
	return nil
}
