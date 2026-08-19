// Package mysqlrepo is auth-svc's outbound persistence adapter: it implements the
// app.Repo port on top of MySQL via GORM.
package mysqlrepo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/auth-svc/internal/app"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

// Repo implements app.Repo on MySQL/GORM.
type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type userRow struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Status       string
	CreatedAt    time.Time
}

func (userRow) TableName() string { return "users" }

// FindUserByEmail returns domain.ErrUserNotFound when no row matches.
func (r *Repo) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("mysql: find user: %w", err)
	}
	return domain.User{
		ID: row.ID, TenantID: row.TenantID, Email: row.Email,
		PasswordHash: row.PasswordHash, Status: row.Status,
	}, nil
}

var _ app.Repo = (*Repo)(nil)

type apiKeyRow struct {
	ID        string
	TenantID  string
	HashedKey string
	Status    string
	CreatedAt time.Time
}

func (apiKeyRow) TableName() string { return "api_keys" }

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// FindAPIKeyByHash returns domain.ErrAPIKeyNotFound when no row matches.
func (r *Repo) FindAPIKeyByHash(ctx context.Context, hashedKey string) (domain.APIKey, error) {
	var row apiKeyRow
	err := r.db.WithContext(ctx).Where("hashed_key = ?", hashedKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("mysql: find api key: %w", err)
	}
	return domain.APIKey{
		ID: row.ID, TenantID: row.TenantID,
		Status: row.Status, CreatedAt: row.CreatedAt,
	}, nil
}

// InsertAPIKey stores a new api key row and returns its generated id.
func (r *Repo) InsertAPIKey(ctx context.Context, tenantID, hashedKey string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	row := apiKeyRow{ID: id, TenantID: tenantID, HashedKey: hashedKey, Status: "active"}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", fmt.Errorf("mysql: insert api key: %w", err)
	}
	return id, nil
}

// ListAPIKeys returns all api keys for the tenant, newest first.
func (r *Repo) ListAPIKeys(ctx context.Context, tenantID string) ([]domain.APIKey, error) {
	var rows []apiKeyRow
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mysql: list api keys: %w", err)
	}
	out := make([]domain.APIKey, len(rows))
	for i, row := range rows {
		out[i] = domain.APIKey{
			ID: row.ID, TenantID: row.TenantID,
			Status: row.Status, CreatedAt: row.CreatedAt,
		}
	}
	return out, nil
}

// RevokeAPIKey sets status='revoked' for the given key, scoped by tenant.
// Returns domain.ErrAPIKeyNotFound if no matching row was updated.
func (r *Repo) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	res := r.db.WithContext(ctx).
		Model(&apiKeyRow{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("status", "revoked")
	if res.Error != nil {
		return fmt.Errorf("mysql: revoke api key: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrAPIKeyNotFound
	}
	return nil
}
