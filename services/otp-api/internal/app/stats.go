package app

import (
	"sort"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
)

const statsWindow = 24 * time.Hour

// Stats is the aggregate read model used by the dashboard Overview screen.
type Stats struct {
	SentToday        int64
	VerifyRate       float64
	Failed           int64
	P50LatencyMillis int64
	Series           []StatsPoint
	Funnel           StatsFunnel
}

// StatsPoint is one UTC hour bucket in the rolling stats window.
type StatsPoint struct {
	T         time.Time
	Requested int64
	Sent      int64
	Verified  int64
	Failed    int64
}

// StatsFunnel summarizes the request lifecycle for the same rolling window.
type StatsFunnel struct {
	Requested int64
	Sent      int64
	Verified  int64
}

// ComputeStats folds recent request rows and delivery logs into the dashboard's
// aggregate view. The totals use the exact rolling 24h window. The series uses UTC
// hour buckets that cover that window, so the first and last buckets may be partial.
func ComputeStats(requests []Request, logs []DeliveryLog, now time.Time) Stats {
	now = now.UTC()
	windowStart := now.Add(-statsWindow)
	stats := Stats{Series: newStatsSeries(windowStart, now)}

	buckets := make(map[time.Time]*StatsPoint, len(stats.Series))
	for i := range stats.Series {
		buckets[stats.Series[i].T] = &stats.Series[i]
	}

	for _, req := range requests {
		createdAt := req.CreatedAt.UTC()
		if outsideWindow(createdAt, windowStart, now) {
			continue
		}
		stats.Funnel.Requested++
		switch req.State {
		case contracts.StateVerified:
			stats.Funnel.Verified++
			stats.Funnel.Sent++
		case contracts.StateSent, contracts.StateExpired:
			stats.Funnel.Sent++
		case contracts.StateFailed:
			stats.Failed++
		}

		point := buckets[createdAt.Truncate(time.Hour)]
		if point == nil {
			continue
		}
		switch req.State {
		case contracts.StateVerified:
			point.Verified++
		case contracts.StateSent, contracts.StateExpired:
			point.Sent++
		case contracts.StateFailed:
			point.Failed++
		default:
			point.Requested++
		}
	}

	if stats.Funnel.Sent > 0 {
		stats.VerifyRate = float64(stats.Funnel.Verified) / float64(stats.Funnel.Sent)
	}

	latencies := make([]int64, 0, len(logs))
	for _, log := range logs {
		createdAt := log.CreatedAt.UTC()
		if outsideWindow(createdAt, windowStart, now) || log.Status != contracts.StateSent {
			continue
		}
		stats.SentToday++
		latencies = append(latencies, log.LatencyMillis)
	}
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		stats.P50LatencyMillis = latencies[len(latencies)/2]
	}

	return stats
}

func newStatsSeries(start, end time.Time) []StatsPoint {
	start = start.UTC().Truncate(time.Hour)
	end = end.UTC().Truncate(time.Hour)
	points := make([]StatsPoint, 0, int(end.Sub(start)/time.Hour)+1)
	for t := start; !t.After(end); t = t.Add(time.Hour) {
		points = append(points, StatsPoint{T: t})
	}
	return points
}

func outsideWindow(t, start, end time.Time) bool {
	return t.Before(start) || t.After(end)
}
