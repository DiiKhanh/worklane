package domain

import contracts "github.com/duykhanh/worklane/pkg/contracts/otp"

// State is the lifecycle of an OTP request. Its underlying string values come from
// the shared contract vocabulary so the same tokens are used on the wire, in MySQL,
// and across services.
type State string

const (
	StateRequested State = contracts.StateRequested
	StateSent      State = contracts.StateSent
	StateFailed    State = contracts.StateFailed
	StateVerified  State = contracts.StateVerified
	StateExpired   State = contracts.StateExpired
)

// transitions encodes the allowed state machine. Terminal states (failed, verified,
// expired) have no outgoing edges. A missing key/edge means the transition is denied.
var transitions = map[State]map[State]bool{
	StateRequested: {StateSent: true, StateFailed: true},
	StateSent:      {StateVerified: true, StateExpired: true},
	StateFailed:    {},
	StateVerified:  {},
	StateExpired:   {},
}

// CanTransition reports whether moving from s to the given state is allowed.
func (s State) CanTransition(to State) bool { return transitions[s][to] }
