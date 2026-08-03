package client

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"
	"net/http"

	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/ports"
)

// CactiConnector is a concrete Hyperledger Cacti ledger-connector adapter. It
// speaks a small REST surface (lock/release/burn/unlock/health) over the shared
// resilient caller (retry + circuit breaker + auth). Each request carries the
// transfer id as an idempotency key so retries never double-apply.
type CactiConnector struct {
	name string
	c    *caller
}

var _ ports.LedgerConnector = (*CactiConnector)(nil)

// New builds a Cacti connector from configuration.
func New(cfg config.LedgerConfig) (*CactiConnector, error) {
	c, err := newCaller(cfg)
	if err != nil {
		return nil, err
	}
	name := cfg.Name
	if name == "" {
		name = "ledger"
	}
	return &CactiConnector{name: name, c: c}, nil
}

func (k *CactiConnector) Name() string { return k.name }

type opWire struct {
	TransferID  string `json:"transfer_id"`
	ReferenceID string `json:"reference_id"`
	Amount      string `json:"amount"`
	Asset       string `json:"asset"`
	Account     string `json:"account"`
}

type receiptWire struct {
	TxID   string `json:"tx_id"`
	Status string `json:"status"`
}

func toWire(req ports.LedgerOp) opWire {
	return opWire{
		TransferID:  req.TransferID,
		ReferenceID: req.ReferenceID,
		Amount:      req.Amount,
		Asset:       req.Asset,
		Account:     req.Account,
	}
}

func (k *CactiConnector) call(ctx context.Context, path string, req ports.LedgerOp) (ports.LedgerReceipt, error) {
	var out receiptWire
	if err := k.c.do(ctx, http.MethodPost, path, toWire(req), &out, ""); err != nil {
		return ports.LedgerReceipt{}, err
	}
	return ports.LedgerReceipt{TxID: out.TxID, Status: out.Status}, nil
}

func (k *CactiConnector) Lock(ctx context.Context, req ports.LedgerOp) (ports.LedgerReceipt, error) {
	return k.call(ctx, "/api/v1/lock", req)
}

func (k *CactiConnector) Release(ctx context.Context, req ports.LedgerOp) (ports.LedgerReceipt, error) {
	return k.call(ctx, "/api/v1/release", req)
}

func (k *CactiConnector) Burn(ctx context.Context, req ports.LedgerOp) (ports.LedgerReceipt, error) {
	return k.call(ctx, "/api/v1/burn", req)
}

func (k *CactiConnector) Unlock(ctx context.Context, req ports.LedgerOp) (ports.LedgerReceipt, error) {
	return k.call(ctx, "/api/v1/unlock", req)
}

func (k *CactiConnector) Health(ctx context.Context) error {
	return k.c.do(ctx, http.MethodGet, "/api/v1/health", nil, nil, "")
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_cacti_connector.log at runtime.
var _ = func() bool { flog.For("10_cacti_connector").Info("source file initialized"); return true }()
