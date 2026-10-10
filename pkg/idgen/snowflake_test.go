package idgen

import (
	"sync"
	"testing"
	"time"
)

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestNewSnowflakeRejectsOutOfRangeNode(t *testing.T) {
	for _, id := range []int64{-1, MaxNodeID + 1} {
		if _, err := NewSnowflake(id); err == nil {
			t.Errorf("node id %d: want error", id)
		}
	}
	for _, id := range []int64{0, MaxNodeID} {
		if _, err := NewSnowflake(id); err != nil {
			t.Errorf("node id %d: unexpected error %v", id, err)
		}
	}
}

func TestNextEncodesLayout(t *testing.T) {
	sf, _ := NewSnowflakeWithClock(5, fixedNow(epoch.Add(1234*time.Millisecond)))
	first, second := sf.Next(), sf.Next()

	if got := first >> timeShift; got != 1234 {
		t.Errorf("timestamp = %d, want 1234", got)
	}
	if got := first >> nodeShift & MaxNodeID; got != 5 {
		t.Errorf("node = %d, want 5", got)
	}
	if got := first & maxSequence; got != 0 {
		t.Errorf("first sequence = %d, want 0", got)
	}
	if got := second & maxSequence; got != 1 {
		t.Errorf("second sequence = %d, want 1", got)
	}
}

func TestNextStrictlyIncreasesWhenSequenceOverflows(t *testing.T) {
	// A frozen clock forces every id into the same millisecond, so 3 * 4096 ids must
	// spill into later logical milliseconds instead of repeating.
	sf, _ := NewSnowflakeWithClock(1, fixedNow(epoch.Add(time.Second)))
	prev := sf.Next()
	for i := 0; i < 3*int(maxSequence+1); i++ {
		id := sf.Next()
		if id <= prev {
			t.Fatalf("id %d not greater than previous %d", id, prev)
		}
		prev = id
	}
}

func TestNextStaysMonotonicWhenClockGoesBack(t *testing.T) {
	now := epoch.Add(time.Hour)
	sf, _ := NewSnowflakeWithClock(1, func() time.Time { return now })
	before := sf.Next()
	now = now.Add(-time.Minute)
	if after := sf.Next(); after <= before {
		t.Fatalf("id went backwards after clock regression: %d then %d", before, after)
	}
}

func TestNextIsPositiveBeforeEpoch(t *testing.T) {
	sf, _ := NewSnowflakeWithClock(MaxNodeID, fixedNow(epoch.Add(-time.Hour)))
	if id := sf.Next(); id < 0 {
		t.Fatalf("id = %d, want non-negative", id)
	}
}

func TestNextUniqueUnderConcurrency(t *testing.T) {
	const goroutines, perGoroutine = 16, 2000
	sf, _ := NewSnowflake(7)

	out := make(chan int64, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				out <- sf.Next()
			}
		}()
	}
	wg.Wait()
	close(out)

	seen := make(map[int64]struct{}, goroutines*perGoroutine)
	for id := range out {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = struct{}{}
	}
}

func TestDistinctNodesNeverCollide(t *testing.T) {
	// Same clock, same call count: only the node id tells the two pods' ids apart.
	now := fixedNow(epoch.Add(time.Minute))
	a, _ := NewSnowflakeWithClock(1, now)
	b, _ := NewSnowflakeWithClock(2, now)
	seen := make(map[int64]struct{})
	for i := 0; i < 5000; i++ {
		for _, id := range []int64{a.Next(), b.Next()} {
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate id %d across nodes", id)
			}
			seen[id] = struct{}{}
		}
	}
}
