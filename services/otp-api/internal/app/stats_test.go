package app_test

import (
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/otp"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

func TestComputeStats_Rolling24HoursUTC(t *testing.T) {
	now := time.Date(2026, 8, 28, 8, 43, 0, 0, time.UTC)
	old := now.Add(-25 * time.Hour)

	requests := []app.Request{
		{ID: "pending", State: contracts.StateRequested, CreatedAt: now.Add(-23 * time.Hour)},
		{ID: "sent", State: contracts.StateSent, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "verified", State: contracts.StateVerified, CreatedAt: now.Add(-90 * time.Minute)},
		{ID: "failed", State: contracts.StateFailed, CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "old-expired", State: contracts.StateExpired, CreatedAt: old},
	}
	logs := []app.DeliveryLog{
		{RequestID: "sent", Status: contracts.StateSent, LatencyMillis: 300, CreatedAt: now.Add(-2 * time.Hour)},
		{RequestID: "verified", Status: contracts.StateSent, LatencyMillis: 100, CreatedAt: now.Add(-90 * time.Minute)},
		{RequestID: "failed", Status: contracts.StateFailed, LatencyMillis: 999, CreatedAt: now.Add(-30 * time.Minute)},
		{RequestID: "old-expired", Status: contracts.StateSent, LatencyMillis: 1, CreatedAt: old},
	}

	got := app.ComputeStats(requests, logs, now)
	if got.SentToday != 2 {
		t.Fatalf("sent today = %d, want 2", got.SentToday)
	}
	if got.VerifyRate != 0.5 {
		t.Fatalf("verify rate = %v, want 0.5", got.VerifyRate)
	}
	if got.Failed != 1 {
		t.Fatalf("failed = %d, want 1", got.Failed)
	}
	if got.P50LatencyMillis != 300 {
		t.Fatalf("p50 latency = %d, want 300", got.P50LatencyMillis)
	}
	if got.Funnel.Requested != 4 || got.Funnel.Sent != 2 || got.Funnel.Verified != 1 {
		t.Fatalf("funnel = %+v, want requested=4 sent=2 verified=1", got.Funnel)
	}

	current := findPoint(t, got.Series, now.Truncate(time.Hour))
	if current.Failed != 1 {
		t.Fatalf("current bucket failed = %d, want 1", current.Failed)
	}
	sentBucket := findPoint(t, got.Series, now.Add(-2*time.Hour).Truncate(time.Hour))
	if sentBucket.Sent != 1 {
		t.Fatalf("sent bucket sent = %d, want 1", sentBucket.Sent)
	}
}

func findPoint(t *testing.T, points []app.StatsPoint, ts time.Time) app.StatsPoint {
	t.Helper()
	for _, p := range points {
		if p.T.Equal(ts) {
			return p
		}
	}
	t.Fatalf("series missing bucket %s", ts.Format(time.RFC3339))
	return app.StatsPoint{}
}
