package idgen

import (
	"errors"
	"sync"
	"time"
)

const (
	nodeBits     = 10
	sequenceBits = 12
	maxNodeID    = (1 << nodeBits) - 1
	maxSequence  = (1 << sequenceBits) - 1
	maxTimestamp = (1 << 41) - 1
	// Epoch is fixed across every instance and must never change after issuance.
	epochMillis int64 = 1767225600000 // 2026-01-01T00:00:00Z
)

var (
	ErrInvalidNodeID      = errors.New("idgen: node ID must be between 0 and 1023")
	ErrClockBeforeEpoch   = errors.New("idgen: clock before epoch")
	ErrClockRollback      = errors.New("idgen: clock moved backwards")
	ErrSequenceExhausted  = errors.New("idgen: millisecond sequence exhausted")
	ErrTimestampExhausted = errors.New("idgen: timestamp exhausted")
)

// Snowflake issues signed 63-bit IDs: 41 timestamp, 10 node and 12 sequence bits.
// It must be constructed with NewSnowflake, shared by callers and never copied.
type Snowflake struct {
	mu         sync.Mutex
	nodeID     int64
	now        func() time.Time
	lastMillis int64
	sequence   int64
}

// NewSnowflake requires an explicitly assigned node ID and a clock (time.Now in
// production). Each live instance must own a distinct node ID. On restart, the
// same node must not be reused until the clock exceeds its last issued timestamp.
// Node allocation and restart fencing belong to the deployment, not this process.
func NewSnowflake(nodeID int64, now func() time.Time) (*Snowflake, error) {
	if nodeID < 0 || nodeID > maxNodeID {
		return nil, ErrInvalidNodeID
	}
	if now == nil {
		return nil, errors.New("idgen: clock required")
	}
	return &Snowflake{nodeID: nodeID, now: now, lastMillis: -1}, nil
}

// Next fails without advancing state when the clock or sequence is unsafe.
// Sequence exhaustion can be retried after the clock advances; it never spins,
// sleeps, invents a future timestamp or wraps the sequence to issue a duplicate.
func (s *Snowflake) Next() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	millis := s.now().UnixMilli() - epochMillis
	switch {
	case millis < 0:
		return 0, ErrClockBeforeEpoch
	case millis > maxTimestamp:
		return 0, ErrTimestampExhausted
	case millis < s.lastMillis:
		return 0, ErrClockRollback
	case millis == s.lastMillis && s.sequence == maxSequence:
		return 0, ErrSequenceExhausted
	}
	if millis == s.lastMillis {
		s.sequence++
	} else {
		s.sequence = 0
	}
	s.lastMillis = millis
	return millis<<(nodeBits+sequenceBits) | s.nodeID<<sequenceBits | s.sequence, nil
}
