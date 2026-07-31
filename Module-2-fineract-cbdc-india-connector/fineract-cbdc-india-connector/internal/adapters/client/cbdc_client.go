package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker"

	"github.com/fineract/cbdc/india-connector/internal/config"
	"github.com/fineract/cbdc/india-connector/internal/domain"
	"github.com/fineract/cbdc/india-connector/internal/ports"
)

// CBDCClient is the concrete sponsor-bank e₹ API adapter. It wraps a retrying
// HTTP client with a circuit breaker and pluggable authentication.
type CBDCClient struct {
	baseURL string
	http    *retryablehttp.Client
	breaker *gobreaker.CircuitBreaker
	auth    authorizer
}

// compile-time assertion that CBDCClient satisfies the port.
var _ ports.CBDCClient = (*CBDCClient)(nil)

// New builds a CBDCClient from configuration.
func New(cfg config.CBDCConfig) (*CBDCClient, error) {
	rc := retryablehttp.NewClient()
	rc.RetryMax = cfg.MaxRetries
	rc.HTTPClient.Timeout = cfg.Timeout
	rc.Logger = nil // silence default stdout logging; app uses zap

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        cfg.BreakerName,
		MaxRequests: cfg.BreakerMaxReq,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= 5
		},
	})

	auth, err := newAuthorizer(cfg)
	if err != nil {
		return nil, err
	}

	return &CBDCClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		http:    rc,
		breaker: cb,
		auth:    auth,
	}, nil
}

func (c *CBDCClient) Issue(ctx context.Context, req ports.IssueRequest) (*ports.IssueResponse, error) {
	var out ports.IssueResponse
	if err := c.postJSON(ctx, "/v1/issue", req, &out.OperationResult); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) Transfer(ctx context.Context, req ports.TransferRequest) (*ports.TransferResponse, error) {
	var out ports.TransferResponse
	if err := c.postJSON(ctx, "/v1/transfer", req, &out.OperationResult); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) Lock(ctx context.Context, req ports.LockRequest) (*ports.LockResponse, error) {
	var out ports.LockResponse
	if err := c.postJSON(ctx, "/v1/lock", req, &out.OperationResult); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) Burn(ctx context.Context, req ports.BurnRequest) (*ports.BurnResponse, error) {
	var out ports.BurnResponse
	if err := c.postJSON(ctx, "/v1/burn", req, &out.OperationResult); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) Redeem(ctx context.Context, req ports.RedeemRequest) (*ports.RedeemResponse, error) {
	var out ports.RedeemResponse
	if err := c.postJSON(ctx, "/v1/redeem", req, &out.OperationResult); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) GetBalance(ctx context.Context, walletID string) (*ports.BalanceResponse, error) {
	var out ports.BalanceResponse
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/balance"
	if err := c.getJSON(ctx, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) GetTransactionStatus(ctx context.Context, upstreamTxID string) (*ports.StatusResponse, error) {
	var out ports.StatusResponse
	path := "/v1/transactions/" + url.PathEscape(upstreamTxID)
	if err := c.getJSON(ctx, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *CBDCClient) HealthCheck(ctx context.Context) error {
	return c.getJSON(ctx, "/v1/health", nil)
}

// postJSON marshals body, executes the call through the breaker, and decodes
// the response into out (which may be nil to discard the body).
func (c *CBDCClient) postJSON(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.NewInternalError("marshal request", err)
	}
	return c.do(ctx, http.MethodPost, path, payload, out)
}

func (c *CBDCClient) getJSON(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *CBDCClient) do(ctx context.Context, method, path string, payload []byte, out any) error {
	_, err := c.breaker.Execute(func() (any, error) {
		return nil, c.roundtrip(ctx, method, path, payload, out)
	})
	if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
		return domain.NewCircuitOpenError("sponsor bank circuit open", err)
	}
	return err
}

func (c *CBDCClient) roundtrip(ctx context.Context, method, path string, payload []byte, out any) error {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := retryablehttp.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return domain.NewInternalError("build request", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.auth.apply(ctx, req); err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return domain.NewTimeoutError("sponsor bank timeout", err)
		}
		return domain.NewUpstreamError("sponsor bank request failed", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 400 {
		return mapHTTPError(resp.StatusCode, respBody)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return domain.NewUpstreamError("decode sponsor bank response", err)
		}
	}
	return nil
}

// mapHTTPError converts an upstream HTTP error status into a typed domain error.
func mapHTTPError(status int, body []byte) error {
	msg := fmt.Sprintf("sponsor bank returned %d", status)
	snippet := strings.TrimSpace(string(body))
	if snippet != "" {
		msg = msg + ": " + snippet
	}
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return domain.NewUnauthorizedError(msg, nil)
	case status == http.StatusNotFound:
		return domain.NewNotFoundError(msg, nil)
	case status == http.StatusConflict:
		return domain.NewConflictError(msg, nil)
	case status >= 500:
		return domain.NewUpstreamError(msg, nil)
	default:
		return domain.NewUpstreamError(msg, nil)
	}
}

// ---- authentication strategies ----

type authorizer interface {
	apply(ctx context.Context, req *retryablehttp.Request) error
}

func newAuthorizer(cfg config.CBDCConfig) (authorizer, error) {
	switch cfg.AuthMode {
	case "apikey":
		return &apiKeyAuth{key: cfg.APIKey}, nil
	case "oauth2":
		return &oauthAuth{
			tokenURL:     cfg.OAuthTokenURL,
			clientID:     cfg.ClientID,
			clientSecret: cfg.ClientSecret,
			hc:           &http.Client{Timeout: cfg.Timeout},
		}, nil
	case "mtls":
		// Client certificates are configured on the HTTP transport at deploy
		// time; no per-request header is needed.
		return &noopAuth{}, nil
	default:
		return nil, domain.NewInternalError("unsupported auth mode: "+cfg.AuthMode, nil)
	}
}

type noopAuth struct{}

func (noopAuth) apply(context.Context, *retryablehttp.Request) error { return nil }

type apiKeyAuth struct{ key string }

func (a *apiKeyAuth) apply(_ context.Context, req *retryablehttp.Request) error {
	req.Header.Set("X-API-Key", a.key)
	return nil
}

// oauthAuth implements OAuth2 client-credentials with simple token caching.
type oauthAuth struct {
	tokenURL     string
	clientID     string
	clientSecret string
	hc           *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (a *oauthAuth) apply(ctx context.Context, req *retryablehttp.Request) error {
	tok, err := a.getToken(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}

func (a *oauthAuth) getToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && time.Now().Before(a.expiry) {
		return a.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", a.clientID)
	form.Set("client_secret", a.clientSecret)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", domain.NewInternalError("build token request", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.hc.Do(httpReq)
	if err != nil {
		return "", domain.NewUpstreamError("oauth token request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", domain.NewUnauthorizedError(fmt.Sprintf("oauth token endpoint returned %d", resp.StatusCode), nil)
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", domain.NewUpstreamError("decode oauth token", err)
	}
	if tr.AccessToken == "" {
		return "", domain.NewUnauthorizedError("empty access token from oauth endpoint", nil)
	}

	a.token = tr.AccessToken
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	// Refresh 30s early to avoid using a token that expires mid-flight.
	a.expiry = time.Now().Add(ttl - 30*time.Second)
	return a.token, nil
}
