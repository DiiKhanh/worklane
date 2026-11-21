//go:build integration

package main_test

import (
	"context"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-dispatcher/internal/app"
)

func TestDetailExpirationPreservesTotalsAndLateEventsCannotRecreateDetails(t *testing.T) {
	f := newIngestion(t)
	ctx := context.Background()
	old := link.ClickedEvent{TenantID: "owner", Code: "a", TS: f.now.Add(-31 * 24 * time.Hour), Device: link.DeviceUnknown}
	previous := app.New(f.repo, func() time.Time { return f.now.Add(-2 * 24 * time.Hour) })
	if err := previous.Handle(ctx, old); err != nil {
		t.Fatal(err)
	}
	if total, details := f.counts(t); total != 1 || details != 1 {
		t.Fatalf("initial total=%d details=%d", total, details)
	}
	deleted, err := f.repo.Expire(ctx, f.now, 1)
	if err != nil || deleted != 1 {
		t.Fatalf("expiration deleted=%d error=%v", deleted, err)
	}
	h := app.New(f.repo, func() time.Time { return f.now })
	if err := h.Handle(ctx, old); err != nil {
		t.Fatal(err)
	}
	if total, details := f.counts(t); total != 2 || details != 0 {
		t.Fatalf("late event recreated detail or reduced totals: %d %d", total, details)
	}
	boundary := old
	boundary.TS = f.now.Add(-link.DetailRetention)
	if err := h.Handle(ctx, boundary); err != nil {
		t.Fatal(err)
	}
	boundary.TS = boundary.TS.Add(time.Millisecond)
	if err := h.Handle(ctx, boundary); err != nil {
		t.Fatal(err)
	}
	deleted, err = f.repo.Expire(ctx, f.now, 1)
	if err != nil || deleted != 0 {
		t.Fatalf("fresh detail expired: %d %v", deleted, err)
	}
	if total, details := f.counts(t); total != 4 || details != 1 {
		t.Fatalf("retention boundary total=%d details=%d", total, details)
	}
}
