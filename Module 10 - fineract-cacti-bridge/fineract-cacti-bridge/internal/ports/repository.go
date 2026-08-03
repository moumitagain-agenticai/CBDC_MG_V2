package ports

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"

	"github.com/fineract/cacti-bridge/internal/domain"
)

// SettlementRepository is the outbound port for durable saga state. Persisting
// after each step is what makes the lock-release-burn saga crash-recoverable.
type SettlementRepository interface {
	// Save inserts a new transfer, or returns a Conflict if the reference_id
	// already exists (idempotent initiation).
	Save(ctx context.Context, t *domain.Transfer) error
	// Update persists the current state of an existing transfer.
	Update(ctx context.Context, t *domain.Transfer) error
	// Get returns a transfer by id.
	Get(ctx context.Context, id string) (*domain.Transfer, error)
	// GetByReference returns a transfer by client reference id, or NotFound.
	GetByReference(ctx context.Context, referenceID string) (*domain.Transfer, error)
	// ListInFlight returns transfers in non-terminal states, for recovery.
	ListInFlight(ctx context.Context) ([]domain.Transfer, error)
	// Ping checks repository liveness.
	Ping(ctx context.Context) error
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_repository.log at runtime.
var _ = func() bool { flog.For("10_repository").Info("source file initialized"); return true }()
