//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/fineract/cbdc/india-connector/internal/adapters/api"
	"github.com/fineract/cbdc/india-connector/internal/ports"
	"github.com/fineract/cbdc/india-connector/internal/service"
	"github.com/fineract/cbdc/india-connector/pkg/metrics"
	"github.com/fineract/cbdc/india-connector/test/mocks"
)

func testServer() *httptest.Server {
	svc := service.NewConnector(&mocks.MockCBDCClient{}, nil, metrics.New(), zap.NewNop())
	h := api.NewHandler(svc)
	router := api.NewRouter(h, metrics.New(), zap.NewNop(), 0)
	return httptest.NewServer(router)
}

func TestIssueEndpoint(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	body, _ := json.Marshal(ports.IssueRequest{
		WalletID:    "wallet-1",
		Amount:      "250",
		Currency:    "INR",
		ReferenceID: uuid.NewString(),
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/v1/issue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var out ports.IssueResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "CONFIRMED", out.Status)
}

func TestIssueEndpoint_BadRequest(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	// missing required fields
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/v1/issue", bytes.NewReader([]byte(`{"amount":"10"}`)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHealthz(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
