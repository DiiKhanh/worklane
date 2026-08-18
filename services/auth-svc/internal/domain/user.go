package domain

// User is a dashboard operator that belongs to a tenant.
type User struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Status       string
}
