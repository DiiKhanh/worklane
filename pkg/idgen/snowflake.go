package idgen

import (
	"fmt"
	"sync"
	"time"
)

// Bit layout of an id, high to low: [ 41 bits ms-since-epoch | 10 bits node | 12 bits sequence ].
// The sign bit stays 0, so ids are positive and fit a MySQL BIGINT.
const (
	nodeBits     = 10
	sequenceBits = 12

	// MaxNodeID is the largest node id that fits the layout (1023).
	MaxNodeID = int64(1)<<nodeBits - 1

	maxSequence = int64(1)<<sequenceBits - 1
	nodeShift   = sequenceBits
	timeShift   = nodeBits + sequenceBits
)

// epoch is the custom zero point (2026-01-01 UTC). Counting from here instead of 1970
// keeps ids (and their base62 codes) short and gives the 41-bit timestamp ~69 years.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Snowflake mints unique, strictly increasing 64-bit ids without coordination. Two
// generators never collide as long as their node ids differ, so every replica must be
// configured with its own node id.
type Snowflake struct {
	now    func() time.Time
	nodeID int64

	mu       sync.Mutex
	lastMs   int64
	sequence int64
}

// NewSnowflake returns a generator for the given node id (0..MaxNodeID).
func NewSnowflake(nodeID int64) (*Snowflake, error) {
	return NewSnowflakeWithClock(nodeID, time.Now)
}

// NewSnowflakeWithClock is NewSnowflake with an injectable clock for deterministic tests.
func NewSnowflakeWithClock(nodeID int64, now func() time.Time) (*Snowflake, error) {
	if nodeID < 0 || nodeID > MaxNodeID {
		return nil, fmt.Errorf("idgen: node id %d out of range [0, %d]", nodeID, MaxNodeID)
	}
	return &Snowflake{now: now, nodeID: nodeID}, nil
}

// Next returns the next id. It is safe for concurrent use and never blocks on the clock:
// the timestamp field is a logical clock that follows wall time but never moves back.
// If wall time stalls or regresses (NTP step), or more than 4096 ids are requested in
// one millisecond, the logical clock runs ahead by a millisecond rather than sleeping
// or reusing a (timestamp, sequence) pair. That guarantee is per process: a restart
// during a backwards clock step can still repeat ids, which is why the id column is
// also a primary key.
func (s *Snowflake) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	ms := s.now().Sub(epoch).Milliseconds()
	if ms > s.lastMs {
		s.lastMs = ms
		s.sequence = 0
	} else if s.sequence < maxSequence {
		s.sequence++
	} else {
		s.lastMs++
		s.sequence = 0
	}
	return s.lastMs<<timeShift | s.nodeID<<nodeShift | s.sequence
}
