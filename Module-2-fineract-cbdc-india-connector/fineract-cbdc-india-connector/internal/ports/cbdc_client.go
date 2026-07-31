package ports

import "context"

// CBDCClient is the outbound port for the Indian CBDC (e₹) sponsor-bank API.
// Adapters in internal/adapters/client implement it. The operation set matches
// the Module 1 CbdcConnectorInterface so the India connector is a drop-in
// implementation of the shared abstraction.
type CBDCClient interface {
	Issue(ctx context.Context, req IssueRequest) (*IssueResponse, error)
	Transfer(ctx context.Context, req TransferRequest) (*TransferResponse, error)
	Lock(ctx context.Context, req LockRequest) (*LockResponse, error)
	Burn(ctx context.Context, req BurnRequest) (*BurnResponse, error)
	Redeem(ctx context.Context, req RedeemRequest) (*RedeemResponse, error)

	GetBalance(ctx context.Context, walletID string) (*BalanceResponse, error)
	GetTransactionStatus(ctx context.Context, upstreamTxID string) (*StatusResponse, error)
	HealthCheck(ctx context.Context) error
}

// ---- Request DTOs (validated at the API boundary) ----

// IssueRequest requests minting of new CBDC into a wallet.
type IssueRequest struct {
	WalletID    string `json:"wallet_id" validate:"required"`
	Amount      string `json:"amount" validate:"required,numeric"`
	Currency    string `json:"currency" validate:"required,len=3"`
	ReferenceID string `json:"reference_id" validate:"required,uuid4"`
}

// TransferRequest moves CBDC between two wallets.
type TransferRequest struct {
	SourceWallet      string `json:"source_wallet" validate:"required"`
	DestinationWallet string `json:"destination_wallet" validate:"required,nefield=SourceWallet"`
	Amount            string `json:"amount" validate:"required,numeric"`
	Currency          string `json:"currency" validate:"required,len=3"`
	ReferenceID       string `json:"reference_id" validate:"required,uuid4"`
}

// LockRequest reserves CBDC in a wallet (e.g. for settlement legs).
type LockRequest struct {
	WalletID    string `json:"wallet_id" validate:"required"`
	Amount      string `json:"amount" validate:"required,numeric"`
	Currency    string `json:"currency" validate:"required,len=3"`
	ReferenceID string `json:"reference_id" validate:"required,uuid4"`
}

// BurnRequest destroys locked/held CBDC (e.g. on redemption).
type BurnRequest struct {
	WalletID    string `json:"wallet_id" validate:"required"`
	Amount      string `json:"amount" validate:"required,numeric"`
	Currency    string `json:"currency" validate:"required,len=3"`
	ReferenceID string `json:"reference_id" validate:"required,uuid4"`
}

// RedeemRequest converts CBDC back to commercial bank money.
type RedeemRequest struct {
	WalletID    string `json:"wallet_id" validate:"required"`
	Amount      string `json:"amount" validate:"required,numeric"`
	Currency    string `json:"currency" validate:"required,len=3"`
	ReferenceID string `json:"reference_id" validate:"required,uuid4"`
}

// ---- Response DTOs ----

// OperationResult is the common shape returned by write operations.
type OperationResult struct {
	UpstreamTxID string `json:"upstream_tx_id"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
}

type IssueResponse struct{ OperationResult }
type TransferResponse struct{ OperationResult }
type LockResponse struct{ OperationResult }
type BurnResponse struct{ OperationResult }
type RedeemResponse struct{ OperationResult }

// BalanceResponse is the sponsor-bank balance for a wallet.
type BalanceResponse struct {
	WalletID  string `json:"wallet_id"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
	Currency  string `json:"currency"`
}

// StatusResponse is the upstream status for a previously submitted transaction.
type StatusResponse struct {
	UpstreamTxID string `json:"upstream_tx_id"`
	Status       string `json:"status"`
}
