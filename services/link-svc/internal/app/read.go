package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

const (
	// SeriesDays is the length of the per-link daily click series.
	SeriesDays = 14

	listLimit   = 100
	recentLimit = 20
	day         = 24 * time.Hour
)

// LinkSummary is one row of the tenant's link list.
type LinkSummary struct {
	Code      string
	Target    string
	Clicks    int64
	CreatedAt time.Time
}

// RecentClick is one recent click in the detail view. Device is classified from the
// stored user agent at read time; Geo is empty until a GeoIP source exists.
type RecentClick struct {
	TS      time.Time
	Referer string
	Device  string
	Geo     string
}

// LinkDetail is the dashboard detail read model for one link.
type LinkDetail struct {
	Code      string
	Target    string
	Clicks    int64
	CreatedAt time.Time
	Series    []int64 // SeriesDays UTC-day buckets, oldest first, today last
	Recent    []RecentClick
}

// ListLinks returns the tenant's most recent links with their lifetime click counts.
func (s *Service) ListLinks(ctx context.Context, tenantID string) ([]LinkSummary, error) {
	stats, err := s.d.Repo.ListByTenant(ctx, tenantID, listLimit)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	out := make([]LinkSummary, 0, len(stats))
	for _, st := range stats {
		out = append(out, LinkSummary{
			Code: st.Link.Code, Target: st.Link.LongURL, Clicks: st.Clicks, CreatedAt: st.Link.CreatedAt,
		})
	}
	return out, nil
}

// LinkDetail returns one link with its click total, 14-day series and recent clicks.
// A code owned by another tenant is reported as domain.ErrNotFound, so the endpoint
// does not reveal which codes exist outside the caller's tenant.
func (s *Service) LinkDetail(ctx context.Context, tenantID, code string) (LinkDetail, error) {
	if !domain.ValidCode(code) {
		return LinkDetail{}, domain.ErrNotFound
	}
	link, err := s.d.Repo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return LinkDetail{}, domain.ErrNotFound
		}
		return LinkDetail{}, fmt.Errorf("find link by code: %w", err)
	}
	if link.TenantID != tenantID {
		return LinkDetail{}, domain.ErrNotFound
	}

	total, err := s.d.Repo.CountClicks(ctx, code)
	if err != nil {
		return LinkDetail{}, fmt.Errorf("count clicks: %w", err)
	}
	today := s.d.Clock.Now().UTC().Truncate(day)
	start := today.Add(-(SeriesDays - 1) * day)
	daily, err := s.d.Repo.DailyClicks(ctx, code, start)
	if err != nil {
		return LinkDetail{}, fmt.Errorf("daily clicks: %w", err)
	}
	clicks, err := s.d.Repo.RecentClicks(ctx, code, recentLimit)
	if err != nil {
		return LinkDetail{}, fmt.Errorf("recent clicks: %w", err)
	}

	recent := make([]RecentClick, 0, len(clicks))
	for _, c := range clicks {
		recent = append(recent, RecentClick{TS: c.TS, Referer: c.Referer, Device: domain.DeviceFromUA(c.UA)})
	}
	return LinkDetail{
		Code: link.Code, Target: link.LongURL, Clicks: total, CreatedAt: link.CreatedAt,
		Series: buildSeries(daily, start), Recent: recent,
	}, nil
}

// buildSeries spreads the sparse per-day counts onto a dense SeriesDays window starting
// at start (midnight UTC). The DB only returns days that had clicks, so the gaps must
// be filled with zeros here or the chart would silently compress time.
func buildSeries(daily []DayCount, start time.Time) []int64 {
	series := make([]int64, SeriesDays)
	for _, d := range daily {
		dayStart := d.Day.UTC().Truncate(day)
		if dayStart.Before(start) {
			continue
		}
		i := int(dayStart.Sub(start) / day)
		if i < SeriesDays {
			series[i] += d.Clicks
		}
	}
	return series
}
