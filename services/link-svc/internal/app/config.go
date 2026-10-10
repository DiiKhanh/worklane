package app

import "time"

// Config holds the tunable policy for the link use cases.
type Config struct {
	PublicBase   string        // LINK_PUBLIC_BASE, e.g. https://link.example.com; short_url = base + "/" + code
	CacheTTL     time.Duration // lifetime of a cached code -> target entry
	ClickedTopic string        // Kafka topic for link.clicked (config-driven, not hard-coded)

	CreateLimitMax    int           // links a tenant may create per window
	CreateLimitWindow time.Duration // fixed window for CreateLimitMax

	// MaxInflightClicks bounds concurrent link.clicked publishes. When the broker is
	// slow and this many are already in flight, further clicks are dropped rather than
	// piling up goroutines behind the redirect path.
	MaxInflightClicks int
	// ClickPublishTimeout bounds a single background publish.
	ClickPublishTimeout time.Duration
}

// Deps bundles the outbound ports the use cases depend on. Grouping them in a struct
// keeps New's signature stable as ports are added.
type Deps struct {
	Repo    Repo
	Cache   Cache
	Counter Counter
	Pub     Publisher
	IDs     IDGen
	Clock   Clock
}

// Service is link-svc's application layer: it orchestrates the domain and the ports to
// implement Shorten, Resolve, ListLinks and LinkDetail.
type Service struct {
	d      Deps
	cfg    Config
	clicks chan struct{} // semaphore over in-flight click publishes
}

// New wires the dependencies and config into a Service.
func New(d Deps, cfg Config) *Service {
	return &Service{d: d, cfg: cfg, clicks: make(chan struct{}, max(cfg.MaxInflightClicks, 1))}
}
