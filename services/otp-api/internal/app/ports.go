// Package app is otp-api's application layer: the Send/Verify use cases and the
// outbound ports they depend on. It imports the pure domain and the shared contract,
// never an adapter, driver, or pkg/platform (enforced by the arch test).
package app

import (
	"context"
	"time"
)

// CodeRecord is what the CodeStore keeps in Redis for an active OTP: the salted hash,
// the salt (which equals the request id), and the running verify-attempt count. The
// plaintext code is never stored.
type CodeRecord struct {
	Hash     string
	Salt     string
	Attempts int
}

// DeliveryLog is one provider send attempt (written by otp-dispatcher, read by the
// dashboard). Defined here because the read endpoints live in otp-api.
type DeliveryLog struct {
	RequestID     string
	Provider      string
	Status        string
	LatencyMillis int64
	Error         string
}

// APIKey is the resolved tenant credential looked up from a hashed key.
type APIKey struct {
	ID       string
	TenantID string
	Status   string
}

// Request is the app-layer view of an otp_requests row, decoupled from the domain
// entity so the persistence shape can evolve independently of business rules.
type Request struct {
	ID        string
	TenantID  string
	Recipient string
	Channel   string
	State     string
	CreatedAt time.Time
}

// Clock abstracts the current time so use cases are deterministic under test.
type Clock interface{ Now() time.Time }

// CodeStore is the hot OTP state (Redis): hash + salt + attempts, TTL-bound.
type CodeStore interface {
	Save(ctx context.Context, key string, rec CodeRecord, ttl time.Duration) error
	Get(ctx context.Context, key string) (CodeRecord, error)
	Delete(ctx context.Context, key string) error
}

// Counter backs rate limiting, cooldown markers, and idempotency keys (Redis).
type Counter interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Exists(ctx context.Context, key string) (bool, error)
	Set(ctx context.Context, key string, ttl time.Duration) error
}

// Repo is the durable audit store (MySQL). Reads are tenant-scoped for the dashboard.
type Repo interface {
	InsertRequest(ctx context.Context, r Request) error
	UpdateState(ctx context.Context, id, to string) error
	FindAPIKey(ctx context.Context, hashedKey string) (APIKey, error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error)
	ListRequests(ctx context.Context, tenantID string, limit int) ([]Request, error)
	ListDeliveryLogs(ctx context.Context, tenantID string, limit int) ([]DeliveryLog, error)
}

// Publisher publishes domain events to Kafka. The topic is passed in (config-driven),
// never hard-coded at the call site.
type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
}
