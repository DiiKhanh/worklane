//go:build integration

package main_test

import (
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
)

func TestTransactionFailureDoesNotAdvanceOrSkipAndRestartRedelivers(t *testing.T) {
	f := newIngestion(t)
	if _, err := f.db.Exec(`CREATE TRIGGER fail_detail BEFORE INSERT ON link_clicks FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected detail failure'`); err != nil {
		t.Fatal(err)
	}
	h, stop := f.start(t)
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			stop()
		}
	})
	event := link.ClickedEvent{TenantID: "owner", Code: "a", TS: f.now.Add(-time.Second), Device: link.DeviceUnknown}
	first := f.publish(t, event)
	second := f.publish(t, event)
	eventually(t, func() bool { _, failed := h.Counts(); return failed > 0 })
	if total, details := f.counts(t); total != 0 || details != 0 {
		t.Fatalf("transaction partially committed: %d %d", total, details)
	}
	if progress := f.progress(t); progress > first {
		t.Fatalf("failed record was skipped: %d", progress)
	}
	stop()
	stopped = true
	if _, err := f.db.Exec("DROP TRIGGER fail_detail"); err != nil {
		t.Fatal(err)
	}
	_, stopAgain := f.start(t)
	t.Cleanup(stopAgain)
	eventually(t, func() bool { return f.progress(t) == second+1 })
	if total, details := f.counts(t); total != 2 || details != 2 {
		t.Fatalf("restart lost events: total=%d details=%d", total, details)
	}
}
