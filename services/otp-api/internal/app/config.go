package app

import "time"

// Config holds the tunable OTP policy for the Send/Verify use cases.
type Config struct {
	CodeLength        int
	TTL               time.Duration
	MaxVerifyAttempts int
	RequestedTopic    string // Kafka topic for otp.requested (config-driven, not hard-coded)
	Limits            LimitConfig
}

// Deps bundles the outbound ports the use cases depend on. Grouping them in a struct
// keeps New's signature stable as ports are added.
type Deps struct {
	Store   CodeStore
	Counter Counter
	Repo    Repo
	Pub     Publisher
	Clock   Clock
}

// Service is otp-api's application layer: it orchestrates the domain and the ports to
// implement Send and Verify.
type Service struct {
	d   Deps
	cfg Config
	rl  *RateLimiter
}

// New wires the dependencies and config into a Service and builds the rate limiter.
func New(d Deps, cfg Config) *Service {
	return &Service{d: d, cfg: cfg, rl: NewRateLimiter(d.Counter, cfg.Limits)}
}
