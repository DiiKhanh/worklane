// Package link holds the cross-service contract for the link bounded context: the
// Kafka event payload shared by link-svc (producer) and link-dispatcher (consumer).
// Like the otp contract it is a dependency-free shared kernel.
package link

import "time"

// ClickedEvent is published by link-svc on link.clicked for every redirect and
// persisted by link-dispatcher. It carries a hash of the client IP, never the raw
// address: click analytics does not need PII.
type ClickedEvent struct {
	Code     string    `json:"code"`
	TenantID string    `json:"tenant_id"`
	TS       time.Time `json:"ts"`
	Referer  string    `json:"referer"`
	UA       string    `json:"ua"`
	IPHash   string    `json:"ip_hash"`
}

// PartitionKey keeps one link's clicks on the same Kafka partition, so they are
// consumed in the order they happened.
func (e ClickedEvent) PartitionKey() string { return e.Code }
