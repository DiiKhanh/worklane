package domain

import "time"

// APIKey is a machine credential belonging to a tenant. The plaintext is never stored;
// only its hash (see pkg/security.HashKey) lives in the database.
type APIKey struct {
	ID        string
	TenantID  string
	Status    string
	CreatedAt time.Time
}
