// Package mysqlrepo is auth-svc's outbound persistence adapter: it implements the
// app.Repo port on top of MySQL via GORM.
package mysqlrepo

import (
	"context"
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
