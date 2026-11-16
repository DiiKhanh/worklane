package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type RateLimited struct{ RetryAfter time.Duration }

func (*RateLimited) Error() string { return "link: rate limited" }

type Service struct {
	repo  Repo
	quota Quota
	cache Cache
	ids   IDGenerator
}

func New(repo Repo, quota Quota, cache Cache, ids IDGenerator) *Service {
	return &Service{repo: repo, quota: quota, cache: cache, ids: ids}
}

func (s *Service) Create(ctx context.Context, tenant, destination string) (domain.Link, bool, error) {
	retry, err := s.quota.Take(ctx, tenant)
	if err != nil {
		return domain.Link{}, false, domain.ErrUnavailable
	}
	if retry > 0 {
		return domain.Link{}, false, &RateLimited{RetryAfter: retry}
	}
	existing, err := s.repo.ByDestination(ctx, tenant, destination)
	if err == nil {
		return active(existing)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Link{}, false, domain.ErrUnavailable
	}
	id, code, err := s.ids.New()
	if err != nil {
		return domain.Link{}, false, domain.ErrUnavailable
	}
	created, err := s.repo.Insert(ctx, domain.Link{ID: id, Code: code, TenantID: tenant, LongURL: destination})
	if errors.Is(err, domain.ErrDuplicate) {
		// Only a matching original destination can resolve an insertion race.
		// ID/code collisions or a hash collision must never return another URL.
		existing, err = s.repo.ByDestination(ctx, tenant, destination)
		if err == nil {
			return active(existing)
		}
	}
	if err != nil {
		return domain.Link{}, false, domain.ErrUnavailable
	}
	return created, true, nil
}

func active(mapping domain.Link) (domain.Link, bool, error) {
	if mapping.DisabledAt != nil {
		return domain.Link{}, false, domain.ErrDisabled
	}
	return mapping, false, nil
}

func (s *Service) Get(ctx context.Context, tenant, code string) (domain.Link, error) {
	return s.repo.Get(ctx, tenant, code)
}

func (s *Service) List(ctx context.Context, tenant string, before *domain.Position, limit int) ([]domain.Link, error) {
	return s.repo.List(ctx, tenant, before, limit)
}

func (s *Service) Disable(ctx context.Context, tenant, code string) (domain.Link, error) {
	mapping, err := s.repo.Disable(ctx, tenant, code)
	if err != nil {
		return domain.Link{}, err
	}
	// Persistence is authoritative. Eviction failure cannot undo a committed
	// disable; redirect cache age is bounded independently by the resolver.
	_ = s.cache.Evict(ctx, mapping.Code)
	return mapping, nil
}
