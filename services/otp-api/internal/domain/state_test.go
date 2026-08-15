package domain

import "testing"

func TestState_Transitions(t *testing.T) {
	ok := [][2]State{
		{StateRequested, StateSent}, {StateRequested, StateFailed},
		{StateSent, StateVerified}, {StateSent, StateExpired},
	}
	for _, p := range ok {
		if !p[0].CanTransition(p[1]) {
			t.Fatalf("expected %s->%s allowed", p[0], p[1])
		}
	}
	bad := [][2]State{
		{StateVerified, StateSent}, {StateExpired, StateVerified}, {StateFailed, StateSent},
		{StateRequested, StateVerified}, {StateSent, StateSent},
	}
	for _, p := range bad {
		if p[0].CanTransition(p[1]) {
			t.Fatalf("expected %s->%s forbidden", p[0], p[1])
		}
	}
}

// State values must equal the shared contract vocabulary so DB/wire strings match.
func TestState_UsesContractVocabulary(t *testing.T) {
	if string(StateVerified) != "verified" || string(StateRequested) != "requested" {
		t.Fatalf("state values drifted from contract vocabulary")
	}
}
