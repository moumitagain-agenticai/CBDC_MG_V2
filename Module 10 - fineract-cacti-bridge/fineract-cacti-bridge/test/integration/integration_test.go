//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/fineract/cacti-bridge/internal/adapters/api"
	"github.com/fineract/cacti-bridge/internal/adapters/client"
	"github.com/fineract/cacti-bridge/internal/adapters/repository"
	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/service"
	"github.com/fineract/cacti-bridge/pkg/metrics"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// fakeSource serves lock/burn/unlock/health. It records unlock calls so the test
// can assert compensation ran.
func fakeSource(unlockCalled *bool) *httptest.Server {
	mux := http.NewServeMux()
	ok := func(tx string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]any{"tx_id": tx, "status": "committed"})
		}
	}
	mux.HandleFunc("/api/v1/lock", ok("lock-tx"))
	mux.HandleFunc("/api/v1/burn", ok("burn-tx"))
	mux.HandleFunc("/api/v1/unlock", func(w http.ResponseWriter, r *http.Request) {
		*unlockCalled = true
		writeJSON(w, map[string]any{"tx_id": "unlock-tx", "status": "committed"})
	})
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	return httptest.NewServer(mux)
}

// fakeDest serves release/health; releaseFails toggles a 502 on release.
func fakeDest(releaseFails bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/release", func(w http.ResponseWriter, r *http.Request) {
		if releaseFails {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"ledger rejected"}`))
			return
		}
		writeJSON(w, map[string]any{"tx_id": "rel-tx", "status": "committed"})
	})
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	return httptest.NewServer(mux)
}

func lcfg(name, url string) config.LedgerConfig {
	return config.LedgerConfig{
		Name: name, BaseURL: url, AuthMode: "apikey", APIKey: "k",
		Timeout: 2 * time.Second, MaxRetries: 0, BreakerMaxReq: 3, BreakerName: name,
	}
}

func testServer(t *testing.T, srcURL, dstURL string) *httptest.Server {
	t.Helper()
	src, err := client.New(lcfg("corda-uae", srcURL))
	require.NoError(t, err)
	dst, err := client.New(lcfg("besu-eu", dstURL))
	require.NoError(t, err)
	scfg := config.Default().Settlement
	coord := service.NewCoordinator(src, dst, repository.NewMemory(), scfg, metrics.New(), zap.NewNop())
	return httptest.NewServer(api.NewRouter(api.NewHandler(coord, 1<<20), metrics.New(), zap.NewNop(), 0))
}

func do(t *testing.T, method, url, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = bytes.NewReader([]byte(body))
	}
	req, _ := http.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

func TestFullFlow_HappyPath(t *testing.T) {
	unlocked := false
	src := fakeSource(&unlocked)
	defer src.Close()
	dst := fakeDest(false)
	defer dst.Close()
	srv := testServer(t, src.URL, dst.URL)
	defer srv.Close()

	code, res := do(t, http.MethodPost, srv.URL+"/api/v1/settlements", `{"reference_id":"IT-1","amount":"100.00","asset":"eAED","source_ledger":"corda-uae","dest_ledger":"besu-eu","sender":"acct-a","recipient":"acct-b"}`)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "BURNED", res["status"])
	assert.NotEmpty(t, res["lock_tx_id"])
	assert.NotEmpty(t, res["release_tx_id"])
	assert.NotEmpty(t, res["burn_tx_id"])
	assert.False(t, unlocked, "unlock must not be called on the happy path")

	id := res["id"].(string)
	code, res = do(t, http.MethodGet, srv.URL+"/api/v1/settlements/"+id, "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "BURNED", res["status"])
}

func TestFullFlow_ReleaseFails_Compensated(t *testing.T) {
	unlocked := false
	src := fakeSource(&unlocked)
	defer src.Close()
	dst := fakeDest(true) // release returns 502
	defer dst.Close()
	srv := testServer(t, src.URL, dst.URL)
	defer srv.Close()

	code, res := do(t, http.MethodPost, srv.URL+"/api/v1/settlements", `{"reference_id":"IT-2","amount":"50.00","asset":"eAED","source_ledger":"corda-uae","dest_ledger":"besu-eu","sender":"acct-a","recipient":"acct-b"}`)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "COMPENSATED", res["status"])
	assert.NotEmpty(t, res["unlock_tx_id"])
	assert.True(t, unlocked, "compensation must unlock the source")
}

func TestSettle_BadRequest(t *testing.T) {
	unlocked := false
	src := fakeSource(&unlocked)
	defer src.Close()
	dst := fakeDest(false)
	defer dst.Close()
	srv := testServer(t, src.URL, dst.URL)
	defer srv.Close()

	// same source and dest ledger -> validation error
	code, _ := do(t, http.MethodPost, srv.URL+"/api/v1/settlements", `{"reference_id":"IT-3","amount":"1","asset":"eAED","source_ledger":"x","dest_ledger":"x","sender":"a","recipient":"b"}`)
	assert.Equal(t, http.StatusBadRequest, code)
}
