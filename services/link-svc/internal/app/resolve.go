package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

const (
	MaxMappingAge     = time.Minute
	cacheTimeout      = 100 * time.Millisecond
	resolutionTimeout = 2 * time.Second
)

type CodeReader interface {
	ByCode(context.Context, string) (domain.Link, error)
}

type CachedMapping struct {
	Link       domain.Link
	ValidUntil time.Time
}

type MappingCache interface {
	Load(context.Context, string) (CachedMapping, error)
	Store(context.Context, CachedMapping) error
}

type Resolver struct {
	repo  CodeReader
	cache MappingCache
	now   func() time.Time
}

func NewResolver(repo CodeReader, cache MappingCache, now func() time.Time) *Resolver {
	return &Resolver{repo: repo, cache: cache, now: now}
}

func (r *Resolver) Resolve(ctx context.Context, code string) (domain.Link, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
	cached, err := r.cache.Load(cacheCtx, code)
	cancel()
	if err == nil && cached.Link.Code == code && cached.Link.TenantID != "" && cached.Link.LongURL != "" && r.now().Before(cached.ValidUntil) {
		return cached.Link, nil
	}
	// A delayed read/refill cannot acquire a new lease after disable commits.
	deadline := r.now().Add(MaxMappingAge)
	readCtx, cancel := context.WithTimeout(ctx, resolutionTimeout)
	mapping, err := r.repo.ByCode(readCtx, code)
	cancel()
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Link{}, domain.ErrNotFound
	}
	if err != nil || !r.now().Before(deadline) {
		return domain.Link{}, domain.ErrUnavailable
	}
	cacheCtx, cancel = context.WithTimeout(ctx, cacheTimeout)
	_ = r.cache.Store(cacheCtx, CachedMapping{Link: mapping, ValidUntil: deadline})
	cancel()
	if !r.now().Before(deadline) {
		return domain.Link{}, domain.ErrUnavailable
	}
	return mapping, nil
}
