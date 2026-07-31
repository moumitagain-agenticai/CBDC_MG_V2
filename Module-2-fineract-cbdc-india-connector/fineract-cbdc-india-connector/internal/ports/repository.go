package ports

import (
	"context"

	"github.com/fineract/cbdc/india-connector/internal/domain"
)

// TransactionRepository is the outbound port for persisting the immutable
// transaction audit trail. Implementations live in
// internal/adapters/repository.
type TransactionRepository interface {
	// Save persists a new transaction record.
	Save(ctx context.Context, tx *domain.Transaction) error
	// UpdateStatus transitions a transaction to a new status, recording the
	// upstream id and any failure reason.
	UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, upstreamTxID, failureReason string) error
	// GetByReferenceID enables idempotency checks on write operations.
	GetByReferenceID(ctx context.Context, referenceID string) (*domain.Transaction, error)
	// GetByID fetches a single transaction.
	GetByID(ctx context.Context, id string) (*domain.Transaction, error)
	// Ping verifies connectivity for readiness checks.
	Ping(ctx context.Context) error
}
