// Package mocks provides in-memory test doubles for the connector ports.
package mocks

import (
	"context"

	"github.com/fineract/cbdc/india-connector/internal/ports"
)

// MockCBDCClient is a configurable test double for ports.CBDCClient. Any func
// left nil returns a canned success, so tests only set the behavior they care
// about.
type MockCBDCClient struct {
	IssueFunc    func(ctx context.Context, req ports.IssueRequest) (*ports.IssueResponse, error)
	TransferFunc func(ctx context.Context, req ports.TransferRequest) (*ports.TransferResponse, error)
	LockFunc     func(ctx context.Context, req ports.LockRequest) (*ports.LockResponse, error)
	BurnFunc     func(ctx context.Context, req ports.BurnRequest) (*ports.BurnResponse, error)
	RedeemFunc   func(ctx context.Context, req ports.RedeemRequest) (*ports.RedeemResponse, error)
	BalanceFunc  func(ctx context.Context, walletID string) (*ports.BalanceResponse, error)
	StatusFunc   func(ctx context.Context, upstreamTxID string) (*ports.StatusResponse, error)
	HealthFunc   func(ctx context.Context) error
}

var _ ports.CBDCClient = (*MockCBDCClient)(nil)

func okResult() ports.OperationResult {
	return ports.OperationResult{UpstreamTxID: "upstream-tx-1", Status: "CONFIRMED"}
}

func (m *MockCBDCClient) Issue(ctx context.Context, req ports.IssueRequest) (*ports.IssueResponse, error) {
	if m.IssueFunc != nil {
		return m.IssueFunc(ctx, req)
	}
	return &ports.IssueResponse{OperationResult: okResult()}, nil
}

func (m *MockCBDCClient) Transfer(ctx context.Context, req ports.TransferRequest) (*ports.TransferResponse, error) {
	if m.TransferFunc != nil {
		return m.TransferFunc(ctx, req)
	}
	return &ports.TransferResponse{OperationResult: okResult()}, nil
}

func (m *MockCBDCClient) Lock(ctx context.Context, req ports.LockRequest) (*ports.LockResponse, error) {
	if m.LockFunc != nil {
		return m.LockFunc(ctx, req)
	}
	return &ports.LockResponse{OperationResult: okResult()}, nil
}

func (m *MockCBDCClient) Burn(ctx context.Context, req ports.BurnRequest) (*ports.BurnResponse, error) {
	if m.BurnFunc != nil {
		return m.BurnFunc(ctx, req)
	}
	return &ports.BurnResponse{OperationResult: okResult()}, nil
}

func (m *MockCBDCClient) Redeem(ctx context.Context, req ports.RedeemRequest) (*ports.RedeemResponse, error) {
	if m.RedeemFunc != nil {
		return m.RedeemFunc(ctx, req)
	}
	return &ports.RedeemResponse{OperationResult: okResult()}, nil
}

func (m *MockCBDCClient) GetBalance(ctx context.Context, walletID string) (*ports.BalanceResponse, error) {
	if m.BalanceFunc != nil {
		return m.BalanceFunc(ctx, walletID)
	}
	return &ports.BalanceResponse{WalletID: walletID, Available: "1000", Locked: "0", Currency: "INR"}, nil
}

func (m *MockCBDCClient) GetTransactionStatus(ctx context.Context, upstreamTxID string) (*ports.StatusResponse, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx, upstreamTxID)
	}
	return &ports.StatusResponse{UpstreamTxID: upstreamTxID, Status: "CONFIRMED"}, nil
}

func (m *MockCBDCClient) HealthCheck(ctx context.Context) error {
	if m.HealthFunc != nil {
		return m.HealthFunc(ctx)
	}
	return nil
}
