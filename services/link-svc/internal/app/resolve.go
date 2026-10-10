package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	contracts "github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// ResolveInput is one public redirect request. ClientIP is the raw address; it is
// hashed before it leaves this use case and is never stored or published.
type ResolveInput struct {
	Code     string
	Referer  string
	UA       string
	ClientIP string
}

// Resolve returns the long URL for a code (cache-aside: Redis first, DB on a miss) and
// records the click in the background. It returns domain.ErrNotFound for an unknown
// code.
func (s *Service) Resolve(ctx context.Context, in ResolveInput) (string, error) {
	if !domain.ValidCode(in.Code) {
		return "", domain.ErrNotFound
	}
	target, err := s.lookupTarget(ctx, in.Code)
	if err != nil {
		return "", err
	}
	s.trackClick(ctx, contracts.ClickedEvent{
		Code: in.Code, TenantID: target.TenantID, TS: s.d.Clock.Now().UTC(),
		Referer: domain.TruncateClickMeta(in.Referer), UA: domain.TruncateClickMeta(in.UA),
		IPHash: domain.HashIP(in.ClientIP),
	})
	return target.LongURL, nil
}

// lookupTarget is the cache-aside read. A cache error is treated as a miss: Redis
// being down must degrade the redirect to a DB read, not break it.
func (s *Service) lookupTarget(ctx context.Context, code string) (Target, error) {
	cached, ok, err := s.d.Cache.Get(ctx, code)
	if err != nil {
		log.Printf("link: cache get %s: %v", code, err)
	} else if ok {
		return cached, nil
	}

	link, err := s.d.Repo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Target{}, domain.ErrNotFound
		}
		return Target{}, fmt.Errorf("find link by code: %w", err)
	}
	target := Target{LongURL: link.LongURL, TenantID: link.TenantID}
	if err := s.d.Cache.Set(ctx, code, target, s.cfg.CacheTTL); err != nil {
		log.Printf("link: cache set %s: %v", code, err)
	}
	return target, nil
}

// trackClick publishes link.clicked without blocking the redirect: analytics is
// best-effort, the 302 is the product.
//
// The publish runs on its own goroutine with a context detached from the request
// (context.WithoutCancel), because the request context is cancelled as soon as the
// 302 is written - reusing it would abort most publishes. The semaphore caps how many
// publishes may be in flight; when a slow broker fills it, the click is dropped
// instead of letting goroutines accumulate without bound.
func (s *Service) trackClick(ctx context.Context, evt contracts.ClickedEvent) {
	select {
	case s.clicks <- struct{}{}:
	default:
		log.Printf("link: click for %s dropped: %d publishes already in flight", evt.Code, cap(s.clicks))
		return
	}
	go func() {
		defer func() { <-s.clicks }()
		pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.ClickPublishTimeout)
		defer cancel()
		if err := s.d.Pub.Publish(pubCtx, s.cfg.ClickedTopic, evt); err != nil {
			log.Printf("link: publish click for %s: %v", evt.Code, err)
		}
	}()
}
