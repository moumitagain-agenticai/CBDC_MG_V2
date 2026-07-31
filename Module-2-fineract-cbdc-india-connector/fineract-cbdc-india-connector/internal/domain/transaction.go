package domain

import "time"

// OperationType enumerates the CBDC operations supported by the India connector.
// These mirror the Module 1 CbdcConnectorInterface operation set.
type OperationType string

const (
	OpIssue    OperationType = "ISSUE"
	OpTransfer OperationType = "TRANSFER"
	OpLock     OperationType = "LOCK"
	OpBurn     OperationType = "BURN"
	OpRedeem   OperationType = "REDEEM"
)

// TransactionStatus enumerates the lifecycle states of a CBDC transaction.
type TransactionStatus string

const (
	StatusPending   TransactionStatus = "PENDING"
	StatusConfirmed TransactionStatus = "CONFIRMED"
	StatusFailed    TransactionStatus = "FAILED"
	StatusReversed  TransactionStatus = "REVERSED"
)

// Transaction is the immutable record persisted for every CBDC operation.
type Transaction struct {
	ID            string            `json:"id"`
	ReferenceID   string            `json:"reference_id"`
	Operation     OperationType     `json:"operation"`
	Status        TransactionStatus `json:"status"`
	SourceWallet  string            `json:"source_wallet,omitempty"`
	DestWallet    string            `json:"destination_wallet,omitempty"`
	Amount        string            `json:"amount"`
	Currency      string            `json:"currency"`
	UpstreamTxID  string            `json:"upstream_tx_id,omitempty"`
	FailureReason string            `json:"failure_reason,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Currency for the Indian CBDC (digital rupee, e₹).
const CurrencyINR = "INR"
