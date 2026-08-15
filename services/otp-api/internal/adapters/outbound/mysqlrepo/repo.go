// Package mysqlrepo is otp-api's outbound persistence adapter: it implements the
// app.Repo port on top of MySQL via GORM. As an adapter it may import app, domain, and
// pkg - but nothing here leaks back into those layers.
package mysqlrepo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
	"github.com/duykhanh/worklane/services/otp-api/internal/domain"
)

// Repo implements app.Repo.
type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// --- GORM row models (private; mapped to app types at the boundary) ---

type apiKeyRow struct {
	ID        string
	TenantID  string
	HashedKey string
	Status    string
	CreatedAt time.Time
}

func (apiKeyRow) TableName() string { return "api_keys" }

type otpRequestRow struct {
	ID              string
	TenantID        string
	RecipientMasked string
	Channel         string
	State           string
	CreatedAt       time.Time
}

func (otpRequestRow) TableName() string { return "otp_requests" }

type deliveryLogRow struct {
	ID        int64
	RequestID string
	TenantID  string
	Provider  string
	Status    string
	LatencyMs int64
	Error     string
	CreatedAt time.Time
}

func (deliveryLogRow) TableName() string { return "delivery_logs" }

// --- app.Repo implementation ---

// InsertRequest persists an audit row. The recipient is masked here so plaintext PII
// never reaches the durable store, regardless of what the caller passed.
func (r *Repo) InsertRequest(ctx context.Context, req app.Request) error {
	row := otpRequestRow{
		ID:              req.ID,
		TenantID:        req.TenantID,
		RecipientMasked: domain.MaskRecipient(req.Recipient),
		Channel:         req.Channel,
		State:           req.State,
		CreatedAt:       req.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("mysql: insert request: %w", err)
	}
	return nil
}

func (r *Repo) UpdateState(ctx context.Context, id, to string) error {
	res := r.db.WithContext(ctx).Model(&otpRequestRow{}).Where("id = ?", id).Update("state", to)
	if res.Error != nil {
		return fmt.Errorf("mysql: update state: %w", res.Error)
	}
	return nil
}

func (r *Repo) FindAPIKey(ctx context.Context, hashedKey string) (app.APIKey, error) {
	var row apiKeyRow
	if err := r.db.WithContext(ctx).Where("hashed_key = ?", hashedKey).First(&row).Error; err != nil {
		return app.APIKey{}, fmt.Errorf("mysql: find api key: %w", err)
	}
	return app.APIKey{ID: row.ID, TenantID: row.TenantID, Status: row.Status}, nil
}

func (r *Repo) ListAPIKeys(ctx context.Context, tenantID string) ([]app.APIKey, error) {
	var rows []apiKeyRow
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mysql: list api keys: %w", err)
	}
	out := make([]app.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.APIKey{ID: row.ID, TenantID: row.TenantID, Status: row.Status})
	}
	return out, nil
}

func (r *Repo) ListRequests(ctx context.Context, tenantID string, limit int) ([]app.Request, error) {
	var rows []otpRequestRow
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mysql: list requests: %w", err)
	}
	out := make([]app.Request, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.Request{
			ID: row.ID, TenantID: row.TenantID, Recipient: row.RecipientMasked,
			Channel: row.Channel, State: row.State, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func (r *Repo) ListDeliveryLogs(ctx context.Context, tenantID string, limit int) ([]app.DeliveryLog, error) {
	var rows []deliveryLogRow
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mysql: list delivery logs: %w", err)
	}
	out := make([]app.DeliveryLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.DeliveryLog{
			RequestID: row.RequestID, Provider: row.Provider, Status: row.Status,
			LatencyMillis: row.LatencyMs, Error: row.Error,
		})
	}
	return out, nil
}
