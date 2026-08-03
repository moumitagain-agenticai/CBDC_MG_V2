package domain

import "github.com/fineract/cacti-bridge/pkg/flog"

// TransferStatus is the state of a cross-chain settlement in the
// lock-release-burn saga.
type TransferStatus string

const (
	// StatusInitiated: the settlement record exists; no ledger action yet.
	StatusInitiated TransferStatus = "INITIATED"
	// StatusLocked: value is locked/escrowed on the source ledger.
	StatusLocked TransferStatus = "LOCKED"
	// StatusReleased: value is released/credited on the destination ledger.
	// This is the point of no return — the transfer can only roll forward.
	StatusReleased TransferStatus = "RELEASED"
	// StatusBurned: the locked source value is burned; the saga is complete.
	StatusBurned TransferStatus = "BURNED"
	// StatusCompensating: release failed; the source lock is being unwound.
	StatusCompensating TransferStatus = "COMPENSATING"
	// StatusCompensated: the source lock was unwound; the saga is rolled back.
	StatusCompensated TransferStatus = "COMPENSATED"
	// StatusFailed: a terminal, unrecoverable failure.
	StatusFailed TransferStatus = "FAILED"
)

// IsTerminal reports whether the status is an end state.
func (s TransferStatus) IsTerminal() bool {
	switch s {
	case StatusBurned, StatusCompensated, StatusFailed:
		return true
	default:
		return false
	}
}

// InFlight reports whether a transfer needs the coordinator to make progress
// (used by startup recovery).
func (s TransferStatus) InFlight() bool {
	switch s {
	case StatusInitiated, StatusLocked, StatusReleased, StatusCompensating:
		return true
	default:
		return false
	}
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_status.log at runtime.
var _ = func() bool { flog.For("10_status").Info("source file initialized"); return true }()
