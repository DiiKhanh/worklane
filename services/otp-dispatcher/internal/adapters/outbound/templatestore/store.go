// Package templatestore implements app.TemplateSource with Redis cache-aside over the
// templates / template_versions tables. The cache key is tmpl:{channel}:{locale}; otp-api
// deletes it on publish. A short TTL is a safety net so a missed invalidation self-heals.
package templatestore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

// Store resolves active templates. It reads Redis first (cache-aside), then MySQL.
type Store struct {
	db  *gorm.DB
	rc  *goredis.Client
	ttl time.Duration
}

func New(db *gorm.DB, rc *goredis.Client, ttl time.Duration) *Store {
	return &Store{db: db, rc: rc, ttl: ttl}
}

var _ app.TemplateSource = (*Store)(nil)

func key(channel, locale string) string { return fmt.Sprintf("tmpl:%s:%s", channel, locale) }

// Active returns the active template for (channel, locale). found=false means no active
// row exists; the caller then uses its env fallback. Any error is returned so the caller
// can also fall back (it treats err and !found identically).
func (s *Store) Active(ctx context.Context, channel, locale string) (app.Template, bool, error) {
	k := key(channel, locale)
	if cached, err := s.rc.Get(ctx, k).Result(); err == nil {
		var t app.Template
		if json.Unmarshal([]byte(cached), &t) == nil {
			return t, true, nil
		}
	}

	var row struct {
		Subject string
		Body    string
	}
	err := s.db.WithContext(ctx).
		Table("templates AS t").
		Select("v.subject, v.body").
		Joins("JOIN template_versions v ON v.id = t.active_version_id").
		Where("t.channel = ? AND t.locale = ? AND t.status = ?", channel, locale, "active").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return app.Template{}, false, err
	}
	if row.Subject == "" && row.Body == "" {
		return app.Template{}, false, nil // no active row -> caller falls back
	}

	t := app.Template{Subject: row.Subject, Body: row.Body}
	if b, err := json.Marshal(t); err == nil {
		_ = s.rc.Set(ctx, k, b, s.ttl).Err()
	}
	return t, true, nil
}
