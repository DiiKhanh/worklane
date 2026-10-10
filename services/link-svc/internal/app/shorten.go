package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/duykhanh/worklane/pkg/idgen"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

// ShortenInput is a tenant's request to shorten one URL.
type ShortenInput struct {
	TenantID string
	LongURL  string
}

// ShortenResult is the code -> short URL mapping. Created is false when the tenant had
// already shortened the same URL and the existing mapping was returned.
type ShortenResult struct {
	Code     string
	ShortURL string
	Created  bool
}

// Shorten validates the URL, enforces the tenant's create quota, and returns the
// tenant's existing code for that URL or mints a new one (snowflake id -> base62).
func (s *Service) Shorten(ctx context.Context, in ShortenInput) (ShortenResult, error) {
	longURL, err := domain.NormalizeLongURL(in.LongURL)
	if err != nil {
		return ShortenResult{}, err
	}
	if err := s.checkCreateLimit(ctx, in.TenantID); err != nil {
		return ShortenResult{}, err
	}

	hash := domain.HashURL(longURL)
	existing, err := s.d.Repo.FindByURLHash(ctx, in.TenantID, hash)
	if err == nil {
		return s.shortenResult(existing.Code, false), nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return ShortenResult{}, fmt.Errorf("find link by url hash: %w", err)
	}

	id := s.d.IDs.Next()
	link := domain.Link{
		ID: id, Code: idgen.Encode(uint64(id)), TenantID: in.TenantID,
		LongURL: longURL, LongURLHash: hash, CreatedAt: s.d.Clock.Now().UTC(),
	}
	if err := s.d.Repo.Insert(ctx, link); err != nil {
		if !errors.Is(err, domain.ErrAlreadyExists) {
			return ShortenResult{}, fmt.Errorf("insert link: %w", err)
		}
		// Check-then-insert is not atomic: a concurrent request for the same URL won the
		// unique index between our lookup and insert. Its row is the mapping to return.
		winner, err := s.d.Repo.FindByURLHash(ctx, in.TenantID, hash)
		if err != nil {
			return ShortenResult{}, fmt.Errorf("find link after duplicate insert: %w", err)
		}
		return s.shortenResult(winner.Code, false), nil
	}

	// Warm the cache so the first click is already a hit. Best-effort: the link is
	// durable, and a failed warm-up only costs one DB read on the first redirect.
	target := Target{LongURL: link.LongURL, TenantID: link.TenantID}
	if err := s.d.Cache.Set(ctx, link.Code, target, s.cfg.CacheTTL); err != nil {
		log.Printf("link: warm cache for %s: %v", link.Code, err)
	}
	return s.shortenResult(link.Code, true), nil
}

func (s *Service) shortenResult(code string, created bool) ShortenResult {
	return ShortenResult{Code: code, ShortURL: s.shortURL(code), Created: created}
}

func (s *Service) shortURL(code string) string {
	return strings.TrimRight(s.cfg.PublicBase, "/") + "/" + code
}

// checkCreateLimit counts this create against the tenant's fixed window. The counter is
// incremented with the window as TTL, so the first create opens the window and the
// whole count rolls off together.
func (s *Service) checkCreateLimit(ctx context.Context, tenantID string) error {
	n, err := s.d.Counter.Incr(ctx, "link:rl:tenant:"+tenantID, s.cfg.CreateLimitWindow)
	if err != nil {
		return fmt.Errorf("count link create: %w", err)
	}
	if int(n) > s.cfg.CreateLimitMax {
		return domain.ErrRateLimited
	}
	return nil
}
