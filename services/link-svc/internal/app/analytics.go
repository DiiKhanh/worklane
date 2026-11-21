package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type AnalyticsReader interface {
	ReadAnalytics(context.Context, string, string, time.Time) (domain.Analytics, error)
}

type AnalyticsService struct {
	repo AnalyticsReader
	now  func() time.Time
}

func NewAnalytics(repo AnalyticsReader, now func() time.Time) *AnalyticsService {
	return &AnalyticsService{repo: repo, now: now}
}

func (s *AnalyticsService) Analytics(ctx context.Context, tenant, code string) (domain.Analytics, error) {
	now := s.now().UTC()
	result, err := s.repo.ReadAnalytics(ctx, tenant, code, now)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Analytics{}, domain.ErrNotFound
		}
		return domain.Analytics{}, domain.ErrUnavailable
	}
	counts := make(map[string]int64, len(result.Series))
	for _, day := range result.Series {
		counts[day.Date] = day.Clicks
	}
	result.Series = make([]domain.DailyClicks, 0, domain.AnalyticsDays)
	for offset := 1 - domain.AnalyticsDays; offset <= 0; offset++ {
		date := now.AddDate(0, 0, offset).Format(time.DateOnly)
		result.Series = append(result.Series, domain.DailyClicks{Date: date, Clicks: counts[date]})
	}
	return result, nil
}
