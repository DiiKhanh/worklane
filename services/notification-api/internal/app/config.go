package app

import "time"

// Config holds the tunable policy for the notification use cases.
type Config struct {
	EmailTopic string // Kafka topic for email sends, e.g. notification.email.requested
	SMSTopic   string // Kafka topic for sms sends, e.g. notification.sms.requested

	TenantLimitMax    int           // sends a tenant may make per window
	TenantLimitWindow time.Duration // fixed window for TenantLimitMax
	UserLimitMax      int           // sends one user_ref may receive per window
	UserLimitWindow   time.Duration // fixed window for UserLimitMax

	ListLimit int // newest notifications returned by ListNotifications
}

// Deps bundles the outbound ports the use cases depend on. Grouping them in a struct
// keeps New's signature stable as ports are added.
type Deps struct {
	Templates TemplateRepo
	Log       LogRepo
	Settings  SettingsRepo
	Counter   Counter
	Pub       Publisher
	Shortener Shortener
	IDs       IDGen
	Clock     Clock
}

// Service is notification-api's application layer: it orchestrates the domain and the
// ports to implement Send, template CRUD + preview, preferences and the log reads.
type Service struct {
	d   Deps
	cfg Config
}

// New wires the dependencies and config into a Service.
func New(d Deps, cfg Config) *Service { return &Service{d: d, cfg: cfg} }
