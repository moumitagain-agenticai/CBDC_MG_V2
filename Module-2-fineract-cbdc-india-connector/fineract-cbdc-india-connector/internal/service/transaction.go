package service

import (
	"context"

	"github.com/fineract/cbdc/india-connector/internal/domain"
	"github.com/fineract/cbdc/india-connector/internal/ports"
)

// GetTransaction returns a persisted transaction by its local id. When
// persistence is disabled the local record is unavailable.
func (c *Connector) GetTransaction(ctx context.Context, id string) (*domain.Transaction, error) {
	if id == "" {
		return nil, domain.NewValidationError("transaction id is required", nil)
	}
	if c.repo == nil {
		return nil, domain.NewNotFoundError("transaction persistence is disabled", nil)
	}
	return c.repo.GetByID(ctx, id)
}

// GetUpstreamStatus queries the sponsor bank for the live status of a
// previously submitted transaction.
func (c *Connector) GetUpstreamStatus(ctx context.Context, upstreamTxID string) (*ports.StatusResponse, error) {
	if upstreamTxID == "" {
		return nil, domain.NewValidationError("upstream transaction id is required", nil)
	}
	return c.client.GetTransactionStatus(ctx, upstreamTxID)
}
