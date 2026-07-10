// internal/fsm/state.go

package fsm

// FsmState represents one of the 7 states in the Relay handoff FSM.
type FsmState string

const (
	// Normal operating states
	StateRunning     FsmState = "RUNNING"
	StatePausing     FsmState = "PAUSING"
	StateSnapshotted FsmState = "SNAPSHOTTED"
	StateEnvBuilt    FsmState = "ENVELOPE_BUILT"
	StateDispatched  FsmState = "DISPATCHED"
	StateResuming    FsmState = "RESUMING"

	// Terminal error state
	StateError FsmState = "ERROR"
)

// ValidTransitions defines allowed state transitions.
// Transitions not in this map are invalid and will be rejected.
var ValidTransitions = map[FsmState][]FsmState{
	StateRunning:     {StatePausing, StateError},
	StatePausing:     {StateSnapshotted, StateError},
	StateSnapshotted: {StateEnvBuilt, StateError},
	StateEnvBuilt:    {StateDispatched, StateError},
	StateDispatched:  {StateResuming, StateError},
	StateResuming:    {StateRunning, StateError},
	StateError:       {}, // terminal
}

// CanTransition returns whether from→to is a valid transition.
func CanTransition(from, to FsmState) bool {
	targets, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}
