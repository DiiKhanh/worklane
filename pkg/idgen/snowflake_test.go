package idgen_test

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/idgen"
)

func TestSnowflakeVectors(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g, err := idgen.NewSnowflake(1, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{4096, 4097, 4098} {
		got, err := g.Next()
		if err != nil || got != want {
			t.Fatalf("Next() = %d, %v, want %d", got, err, want)
		}
	}
	now = now.Add(time.Millisecond)
	if got, err := g.Next(); err != nil || got != 4198400 {
		t.Fatalf("next millisecond = %d, %v, want 4198400", got, err)
	}
}

func TestSnowflakeRejectsInvalidConfiguration(t *testing.T) {
	for _, node := range []int64{-1, 1024} {
		if _, err := idgen.NewSnowflake(node, time.Now); !errors.Is(err, idgen.ErrInvalidNodeID) {
			t.Fatalf("node %d: %v", node, err)
		}
	}
	if _, err := idgen.NewSnowflake(0, nil); err == nil {
		t.Fatal("nil clock accepted")
	}
}

func TestSnowflakeClockAndSequenceFailuresDoNotReuseIDs(t *testing.T) {
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := epoch.Add(-time.Millisecond)
	g, err := idgen.NewSnowflake(1023, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Next(); !errors.Is(err, idgen.ErrClockBeforeEpoch) {
		t.Fatalf("before epoch: %v", err)
	}
	now = epoch.Add(time.Millisecond)
	var last int64 = -1
	for range 4096 {
		id, err := g.Next()
		if err != nil || id <= last {
			t.Fatalf("non-increasing ID: %d after %d, %v", id, last, err)
		}
		last = id
	}
	for range 2 {
		if _, err := g.Next(); !errors.Is(err, idgen.ErrSequenceExhausted) {
			t.Fatalf("sequence wrapped: %v", err)
		}
	}
	now = epoch
	if _, err := g.Next(); !errors.Is(err, idgen.ErrClockRollback) {
		t.Fatalf("clock rollback: %v", err)
	}
	now = epoch.Add(2 * time.Millisecond)
	if id, err := g.Next(); err != nil || id <= last {
		t.Fatalf("failed to recover after clock advance: %d, %v", id, err)
	}
	now = epoch.Add(((1 << 41) - 1) * time.Millisecond)
	for range 4095 {
		if _, err := g.Next(); err != nil {
			t.Fatal(err)
		}
	}
	if id, err := g.Next(); err != nil || id != math.MaxInt64 {
		t.Fatalf("last possible ID = %d, %v", id, err)
	}
	now = now.Add(time.Millisecond)
	if _, err := g.Next(); !errors.Is(err, idgen.ErrTimestampExhausted) {
		t.Fatalf("timestamp overflow: %v", err)
	}
}

func TestSnowflakeConcurrentNodes(t *testing.T) {
	const workers, perWorker = 16, 2000
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	// Advance before either node can exhaust its millisecond sequence.
	var clockMu sync.Mutex
	calls := 0
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		calls++
		return now.Add(time.Duration(calls/2048) * time.Millisecond)
	}
	a, err := idgen.NewSnowflake(0, clock)
	if err != nil {
		t.Fatal(err)
	}
	b, err := idgen.NewSnowflake(1023, clock)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(chan int64, workers*perWorker)
	var wg sync.WaitGroup
	for i := range workers {
		g := a
		if i%2 == 1 {
			g = b
		}
		wg.Go(func() {
			var last int64 = -1
			for range perWorker {
				id, err := g.Next()
				if err != nil || id <= last {
					t.Errorf("non-increasing ID: %d after %d, %v", id, last, err)
					return
				}
				last = id
				ids <- id
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[int64]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID: %d", id)
		}
		seen[id] = true
	}
	if len(seen) != workers*perWorker {
		t.Fatalf("got %d IDs, want %d", len(seen), workers*perWorker)
	}
}
