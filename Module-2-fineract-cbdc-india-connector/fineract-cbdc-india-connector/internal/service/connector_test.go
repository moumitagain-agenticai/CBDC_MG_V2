package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/fineract/cbdc/india-connector/internal/domain"
	"github.com/fineract/cbdc/india-connector/internal/ports"
	"github.com/fineract/cbdc/india-connector/internal/service"
	"github.com/fineract/cbdc/india-connector/pkg/metrics"
	"github.com/fineract/cbdc/india-connector/test/mocks"
)

func newSvc(client ports.CBDCClient) *service.Connector {
	return service.NewConnector(client, nil, metrics.New(), zap.NewNop())
}

func validIssue() ports.IssueRequest {
	return ports.IssueRequest{
		WalletID:    "wallet-1",
		Amount:      "100.50",
		Currency:    "INR",
		ReferenceID: uuid.NewString(),
	}
}

func TestIssue_Success(t *testing.T) {
	svc := newSvc(&mocks.MockCBDCClient{})
	res, err := svc.Issue(context.Background(), validIssue())
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "upstream-tx-1", res.UpstreamTxID)
	assert.Equal(t, "CONFIRMED", res.Status)
}

func TestIssue_ValidationError(t *testing.T) {
	svc := newSvc(&mocks.MockCBDCClient{})
	bad := validIssue()
	bad.Amount = "not-a-number"
	_, err := svc.Issue(context.Background(), bad)

	require.Error(t, err)
	de := domain.AsDomainError(err)
	assert.Equal(t, domain.CodeValidation, de.Code)
}

func TestIssue_MissingReferenceID(t *testing.T) {
	svc := newSvc(&mocks.MockCBDCClient{})
	bad := validIssue()
	bad.ReferenceID = "" // fails uuid4 validation
	_, err := svc.Issue(context.Background(), bad)

	require.Error(t, err)
	assert.Equal(t, domain.CodeValidation, domain.AsDomainError(err).Code)
}

func TestTransfer_UpstreamErrorPropagates(t *testing.T) {
	client := &mocks.MockCBDCClient{
		TransferFunc: func(ctx context.Context, req ports.TransferRequest) (*ports.TransferResponse, error) {
			return nil, domain.NewUpstreamError("sponsor bank 503", nil)
		},
	}
	svc := newSvc(client)

	req := ports.TransferRequest{
		SourceWallet:      "wallet-a",
		DestinationWallet: "wallet-b",
		Amount:            "50",
		Currency:          "INR",
		ReferenceID:       uuid.NewString(),
	}
	_, err := svc.Transfer(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, domain.CodeUpstream, domain.AsDomainError(err).Code)
}

func TestTransfer_SameWalletRejected(t *testing.T) {
	svc := newSvc(&mocks.MockCBDCClient{})
	req := ports.TransferRequest{
		SourceWallet:      "wallet-x",
		DestinationWallet: "wallet-x", // nefield validation must reject
		Amount:            "10",
		Currency:          "INR",
		ReferenceID:       uuid.NewString(),
	}
	_, err := svc.Transfer(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, domain.CodeValidation, domain.AsDomainError(err).Code)
}

func TestGetBalance(t *testing.T) {
	svc := newSvc(&mocks.MockCBDCClient{})
	res, err := svc.GetBalance(context.Background(), "wallet-9")
	require.NoError(t, err)
	assert.Equal(t, "wallet-9", res.WalletID)
	assert.Equal(t, "INR", res.Currency)
}
