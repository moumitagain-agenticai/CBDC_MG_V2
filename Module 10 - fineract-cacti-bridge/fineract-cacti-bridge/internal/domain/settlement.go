package domain

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"time"

	"github.com/fineract/cacti-bridge/pkg/utils"
)

// Transfer is a cross-chain settlement coordinated via the lock-release-burn
// saga. It is the durable saga record: every step advances the status and
// records the corresponding ledger receipt so a crash can be recovered.
type Transfer struct {
	ID           string         `json:"id"`
	ReferenceID  string         `json:"reference_id"`
	Status       TransferStatus `json:"status"`
	Amount       string         `json:"amount"`
	Asset        string         `json:"asset"`
	SourceLedger string         `json:"source_ledger"`
	DestLedger   string         `json:"dest_ledger"`
	Sender       string         `json:"sender"`
	Recipient    string         `json:"recipient"`

	LockTxID    string `json:"lock_tx_id,omitempty"`
	ReleaseTxID string `json:"release_tx_id,omitempty"`
	BurnTxID    string `json:"burn_tx_id,omitempty"`
	UnlockTxID  string `json:"unlock_tx_id,omitempty"`

	BurnAttempts  int    `json:"burn_attempts"`
	FailureReason string `json:"failure_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SettleRequest initiates a cross-chain settlement.
type SettleRequest struct {
	ReferenceID  string `json:"reference_id"`
	Amount       string `json:"amount"`
	Asset        string `json:"asset"`
	SourceLedger string `json:"source_ledger"`
	DestLedger   string `json:"dest_ledger"`
	Sender       string `json:"sender"`
	Recipient    string `json:"recipient"`
}

// Validate checks a settle request.
func (r SettleRequest) Validate() error {
	if r.ReferenceID == "" {
		return NewValidationError("reference_id is required", nil)
	}
	if !utils.IsPositiveAmount(r.Amount) {
		return NewValidationError("amount must be a positive decimal", nil)
	}
	if r.Asset == "" {
		return NewValidationError("asset is required", nil)
	}
	if r.SourceLedger == "" || r.DestLedger == "" {
		return NewValidationError("source_ledger and dest_ledger are required", nil)
	}
	if r.SourceLedger == r.DestLedger {
		return NewValidationError("source_ledger and dest_ledger must differ", nil)
	}
	if r.Sender == "" || r.Recipient == "" {
		return NewValidationError("sender and recipient are required", nil)
	}
	return nil
}

// NewTransfer builds an INITIATED transfer from a validated request.
func NewTransfer(id string, r SettleRequest, now time.Time) *Transfer {
	return &Transfer{
		ID:           id,
		ReferenceID:  r.ReferenceID,
		Status:       StatusInitiated,
		Amount:       r.Amount,
		Asset:        r.Asset,
		SourceLedger: r.SourceLedger,
		DestLedger:   r.DestLedger,
		Sender:       r.Sender,
		Recipient:    r.Recipient,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_settlement.log at runtime.
var _ = func() bool { flog.For("10_settlement").Info("source file initialized"); return true }()
