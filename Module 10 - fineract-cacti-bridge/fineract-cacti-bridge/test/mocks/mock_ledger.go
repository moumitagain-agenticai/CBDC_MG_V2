package mocks

import (
	"context"
	"sync"

	"github.com/fineract/cacti-bridge/internal/ports"
)

// MockLedger is a configurable ports.LedgerConnector test double. It records
// call counts and can be told to fail specific operations, or to fail the first
// N burns (to exercise retry).
type MockLedger struct {
	NameVal string

	LockErr    error
	ReleaseErr error
	UnlockErr  error
	BurnErr    error // applied after BurnFailTimes failures are exhausted

	BurnFailTimes int // fail this many burns before succeeding
	HealthErr     error

	mu                                            sync.Mutex
	Locks, Releases, Burns, Unlocks, HealthChecks int
}

var _ ports.LedgerConnector = (*MockLedger)(nil)

func (m *MockLedger) Name() string { return m.NameVal }

func (m *MockLedger) Lock(_ context.Context, op ports.LedgerOp) (ports.LedgerReceipt, error) {
	m.mu.Lock()
	m.Locks++
	m.mu.Unlock()
	if m.LockErr != nil {
		return ports.LedgerReceipt{}, m.LockErr
	}
	return ports.LedgerReceipt{TxID: "lock-" + op.TransferID, Status: "committed"}, nil
}

func (m *MockLedger) Release(_ context.Context, op ports.LedgerOp) (ports.LedgerReceipt, error) {
	m.mu.Lock()
	m.Releases++
	m.mu.Unlock()
	if m.ReleaseErr != nil {
		return ports.LedgerReceipt{}, m.ReleaseErr
	}
	return ports.LedgerReceipt{TxID: "rel-" + op.TransferID, Status: "committed"}, nil
}

func (m *MockLedger) Burn(_ context.Context, op ports.LedgerOp) (ports.LedgerReceipt, error) {
	m.mu.Lock()
	m.Burns++
	n := m.Burns
	m.mu.Unlock()
	if n <= m.BurnFailTimes {
		return ports.LedgerReceipt{}, errBurnTransient
	}
	if m.BurnErr != nil {
		return ports.LedgerReceipt{}, m.BurnErr
	}
	return ports.LedgerReceipt{TxID: "burn-" + op.TransferID, Status: "committed"}, nil
}

func (m *MockLedger) Unlock(_ context.Context, op ports.LedgerOp) (ports.LedgerReceipt, error) {
	m.mu.Lock()
	m.Unlocks++
	m.mu.Unlock()
	if m.UnlockErr != nil {
		return ports.LedgerReceipt{}, m.UnlockErr
	}
	return ports.LedgerReceipt{TxID: "unlock-" + op.TransferID, Status: "committed"}, nil
}

func (m *MockLedger) Health(_ context.Context) error { return m.HealthErr }

type transientErr struct{}

func (transientErr) Error() string { return "transient ledger error" }

var errBurnTransient = transientErr{}
