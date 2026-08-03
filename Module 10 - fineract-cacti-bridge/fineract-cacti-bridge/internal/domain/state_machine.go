package domain

import "github.com/fineract/cacti-bridge/pkg/flog"

// allowedTransitions defines the legal edges of the settlement state machine.
// The saga rolls forward (lock -> release -> burn); before RELEASED it may
// compensate (unlock); after RELEASED it may only roll forward.
var allowedTransitions = map[TransferStatus]map[TransferStatus]bool{
	StatusInitiated: {
		StatusLocked: true,
		StatusFailed: true,
	},
	StatusLocked: {
		StatusReleased:     true,
		StatusCompensating: true,
		StatusFailed:       true,
	},
	StatusReleased: {
		StatusBurned: true,
		StatusFailed: true,
	},
	StatusCompensating: {
		StatusCompensated: true,
		StatusFailed:      true,
	},
	StatusBurned:      {},
	StatusCompensated: {},
	StatusFailed:      {},
}

// CanTransition reports whether from -> to is a legal state transition.
func CanTransition(from, to TransferStatus) bool {
	return allowedTransitions[from][to]
}

// Transition validates and applies a state change to a transfer, returning a
// typed error if the edge is illegal. Callers persist after a successful move.
func (t *Transfer) Transition(to TransferStatus) error {
	if t.Status == to {
		return nil // idempotent no-op
	}
	if !CanTransition(t.Status, to) {
		return NewConflictError(
			"illegal settlement transition "+string(t.Status)+" -> "+string(to), nil)
	}
	t.Status = to
	return nil
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_state_machine.log at runtime.
var _ = func() bool { flog.For("10_state_machine").Info("source file initialized"); return true }()
