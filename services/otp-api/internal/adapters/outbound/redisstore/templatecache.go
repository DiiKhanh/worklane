package redisstore

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// TemplateCache implements app.TemplateCache: it deletes the dispatcher's cache-aside
// key so a published change is re-read on the next send. The key shape (tmpl:{channel}:
// {locale}) must match services/otp-dispatcher's templatestore.
type TemplateCache struct{ c *goredis.Client }

func NewTemplateCache(c *goredis.Client) *TemplateCache { return &TemplateCache{c: c} }

var _ app.TemplateCache = (*TemplateCache)(nil)

func (t *TemplateCache) Invalidate(ctx context.Context, channel, locale string) error {
	return t.c.Del(ctx, fmt.Sprintf("tmpl:%s:%s", channel, locale)).Err()
}
