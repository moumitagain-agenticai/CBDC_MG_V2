package ports

import "github.com/fineract/cacti-bridge/pkg/flog"
import "context"

// LedgerConnector is the outbound port for a Hyperledger Cacti ledger connector.
// The coordinator drives two connectors (source and destination) through the
// lock-release-burn saga. Each operation is idempotent on TransferID so that
// retries and crash-recovery never double-apply a ledger action.
type LedgerConnector interface {
	// Name identifies the ledger (e.g. "corda-uae", "besu-eu").
	Name() string
	// Lock escrows value on the source ledger.
	Lock(ctx context.Context, req LedgerOp) (LedgerReceipt, error)
	// Release credits value on the destination ledger.
	Release(ctx context.Context, req LedgerOp) (LedgerReceipt, error)
	// Burn destroys the locked source value, finalising the transfer.
	Burn(ctx context.Context, req LedgerOp) (LedgerReceipt, error)
	// Unlock refunds a lock on the source ledger (compensation).
	Unlock(ctx context.Context, req LedgerOp) (LedgerReceipt, error)
	// Health reports connector reachability for readiness checks.
	Health(ctx context.Context) error
}

// LedgerOp is a single ledger instruction. TransferID is the idempotency key.
type LedgerOp struct {
	TransferID  string
	ReferenceID string
	Amount      string
	Asset       string
	Account     string
}

// LedgerReceipt is a connector's acknowledgement of a ledger operation.
type LedgerReceipt struct {
	TxID   string
	Status string
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_ledger.log at runtime.
var _ = func() bool { flog.For("10_ledger").Info("source file initialized"); return true }()
