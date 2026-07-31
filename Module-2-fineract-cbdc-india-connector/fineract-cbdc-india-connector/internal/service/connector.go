package service

import (
	"context"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/fineract/cbdc/india-connector/internal/domain"
	"github.com/fineract/cbdc/india-connector/internal/ports"
	"github.com/fineract/cbdc/india-connector/pkg/metrics"
	"github.com/fineract/cbdc/india-connector/pkg/utils"
)

// Connector is the core application service. It orchestrates validation,
// idempotent persistence, the upstream sponsor-bank call, status updates and
// metrics for every CBDC operation.
type Connector struct {
	client   ports.CBDCClient
	repo     ports.TransactionRepository // optional; nil disables persistence
	metrics  *metrics.Metrics
	log      *zap.Logger
	validate *validator.Validate
}

// NewConnector wires the service. repo may be nil when persistence is disabled.
func NewConnector(client ports.CBDCClient, repo ports.TransactionRepository, m *metrics.Metrics, log *zap.Logger) *Connector {
	return &Connector{
		client:   client,
		repo:     repo,
		metrics:  m,
		log:      log,
		validate: validator.New(),
	}
}

// ---- public operations (match Module 1 CbdcConnectorInterface) ----

func (c *Connector) Issue(ctx context.Context, req ports.IssueRequest) (*ports.IssueResponse, error) {
	if err := c.validateWrite(domain.OpIssue, req, req.Amount); err != nil {
		return nil, err
	}
	res, err := c.runWrite(ctx, writeParams{
		op: domain.OpIssue, referenceID: req.ReferenceID,
		dest: req.WalletID, amount: req.Amount, currency: req.Currency,
	}, func(ctx context.Context) (ports.OperationResult, error) {
		r, err := c.client.Issue(ctx, req)
		if err != nil {
			return ports.OperationResult{}, err
		}
		return r.OperationResult, nil
	})
	if err != nil {
		return nil, err
	}
	return &ports.IssueResponse{OperationResult: *res}, nil
}

func (c *Connector) Transfer(ctx context.Context, req ports.TransferRequest) (*ports.TransferResponse, error) {
	if err := c.validateWrite(domain.OpTransfer, req, req.Amount); err != nil {
		return nil, err
	}
	res, err := c.runWrite(ctx, writeParams{
		op: domain.OpTransfer, referenceID: req.ReferenceID,
		source: req.SourceWallet, dest: req.DestinationWallet,
		amount: req.Amount, currency: req.Currency,
	}, func(ctx context.Context) (ports.OperationResult, error) {
		r, err := c.client.Transfer(ctx, req)
		if err != nil {
			return ports.OperationResult{}, err
		}
		return r.OperationResult, nil
	})
	if err != nil {
		return nil, err
	}
	return &ports.TransferResponse{OperationResult: *res}, nil
}

func (c *Connector) Lock(ctx context.Context, req ports.LockRequest) (*ports.LockResponse, error) {
	if err := c.validateWrite(domain.OpLock, req, req.Amount); err != nil {
		return nil, err
	}
	res, err := c.runWrite(ctx, writeParams{
		op: domain.OpLock, referenceID: req.ReferenceID,
		source: req.WalletID, amount: req.Amount, currency: req.Currency,
	}, func(ctx context.Context) (ports.OperationResult, error) {
		r, err := c.client.Lock(ctx, req)
		if err != nil {
			return ports.OperationResult{}, err
		}
		return r.OperationResult, nil
	})
	if err != nil {
		return nil, err
	}
	return &ports.LockResponse{OperationResult: *res}, nil
}

func (c *Connector) Burn(ctx context.Context, req ports.BurnRequest) (*ports.BurnResponse, error) {
	if err := c.validateWrite(domain.OpBurn, req, req.Amount); err != nil {
		return nil, err
	}
	res, err := c.runWrite(ctx, writeParams{
		op: domain.OpBurn, referenceID: req.ReferenceID,
		source: req.WalletID, amount: req.Amount, currency: req.Currency,
	}, func(ctx context.Context) (ports.OperationResult, error) {
		r, err := c.client.Burn(ctx, req)
		if err != nil {
			return ports.OperationResult{}, err
		}
		return r.OperationResult, nil
	})
	if err != nil {
		return nil, err
	}
	return &ports.BurnResponse{OperationResult: *res}, nil
}

func (c *Connector) Redeem(ctx context.Context, req ports.RedeemRequest) (*ports.RedeemResponse, error) {
	if err := c.validateWrite(domain.OpRedeem, req, req.Amount); err != nil {
		return nil, err
	}
	res, err := c.runWrite(ctx, writeParams{
		op: domain.OpRedeem, referenceID: req.ReferenceID,
		source: req.WalletID, amount: req.Amount, currency: req.Currency,
	}, func(ctx context.Context) (ports.OperationResult, error) {
		r, err := c.client.Redeem(ctx, req)
		if err != nil {
			return ports.OperationResult{}, err
		}
		return r.OperationResult, nil
	})
	if err != nil {
		return nil, err
	}
	return &ports.RedeemResponse{OperationResult: *res}, nil
}

// GetBalance returns the sponsor-bank balance for a wallet.
func (c *Connector) GetBalance(ctx context.Context, walletID string) (*ports.BalanceResponse, error) {
	if walletID == "" {
		return nil, domain.NewValidationError("wallet_id is required", nil)
	}
	return c.client.GetBalance(ctx, walletID)
}

// ---- internal helpers ----

type writeParams struct {
	op          domain.OperationType
	referenceID string
	source      string
	dest        string
	amount      string
	currency    string
}

// validateWrite runs struct validation plus a positive-amount check and records
// a metric on failure.
func (c *Connector) validateWrite(op domain.OperationType, req any, amount string) error {
	if err := c.validate.Struct(req); err != nil {
		verr := domain.NewValidationError("request validation failed", err)
		c.recordErr(op, verr)
		return verr
	}
	if !utils.IsPositiveAmount(amount) {
		verr := domain.NewValidationError("amount must be a positive decimal", nil)
		c.recordErr(op, verr)
		return verr
	}
	return nil
}

// runWrite handles idempotency, pending persistence, the upstream call, and the
// terminal status update shared by all write operations.
func (c *Connector) runWrite(ctx context.Context, p writeParams, call func(context.Context) (ports.OperationResult, error)) (*ports.OperationResult, error) {
	start := time.Now()
	defer func() {
		c.metrics.OpDuration.WithLabelValues(string(p.op)).Observe(time.Since(start).Seconds())
	}()

	// Idempotency: a prior transaction with the same reference id short-circuits.
	if p.referenceID != "" && c.repo != nil {
		if existing, err := c.repo.GetByReferenceID(ctx, p.referenceID); err == nil && existing != nil {
			c.log.Info("idempotent replay",
				zap.String("operation", string(p.op)),
				zap.String("reference_id", p.referenceID))
			return &ports.OperationResult{
				UpstreamTxID: existing.UpstreamTxID,
				Status:       string(existing.Status),
				Message:      "idempotent replay",
			}, nil
		}
	}

	tx := &domain.Transaction{
		ID:           uuid.NewString(),
		ReferenceID:  p.referenceID,
		Operation:    p.op,
		Status:       domain.StatusPending,
		SourceWallet: p.source,
		DestWallet:   p.dest,
		Amount:       p.amount,
		Currency:     p.currency,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if c.repo != nil {
		if err := c.repo.Save(ctx, tx); err != nil {
			c.log.Error("failed to persist pending transaction", zap.Error(err))
			c.recordErr(p.op, err)
			return nil, err
		}
	}

	res, err := call(ctx)
	if err != nil {
		c.log.Warn("upstream operation failed",
			zap.String("operation", string(p.op)),
			zap.String("tx_id", tx.ID),
			zap.Error(err))
		c.recordErr(p.op, err)
		if c.repo != nil {
			_ = c.repo.UpdateStatus(ctx, tx.ID, domain.StatusFailed, "", err.Error())
		}
		return nil, err
	}

	if c.repo != nil {
		_ = c.repo.UpdateStatus(ctx, tx.ID, domain.StatusConfirmed, res.UpstreamTxID, "")
	}
	c.metrics.OpsTotal.WithLabelValues(string(p.op), string(domain.StatusConfirmed)).Inc()
	c.log.Info("operation confirmed",
		zap.String("operation", string(p.op)),
		zap.String("tx_id", tx.ID),
		zap.String("upstream_tx_id", res.UpstreamTxID))
	return &res, nil
}

// recordErr increments the error and failed-operation counters.
func (c *Connector) recordErr(op domain.OperationType, err error) {
	de := domain.AsDomainError(err)
	c.metrics.OpErrorsTotal.WithLabelValues(string(op), string(de.Code)).Inc()
	c.metrics.OpsTotal.WithLabelValues(string(op), string(domain.StatusFailed)).Inc()
}
