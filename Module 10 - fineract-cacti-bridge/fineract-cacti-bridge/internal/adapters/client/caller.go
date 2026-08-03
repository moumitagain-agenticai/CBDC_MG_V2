package client

import "github.com/fineract/cacti-bridge/pkg/flog"
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

	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/domain"
)

// caller is the shared HTTP engine used by every scheme provider: a retrying
// client wrapped in a circuit breaker with pluggable app authentication.
type caller struct {
	baseURL string
	http    *retryablehttp.Client
	breaker *gobreaker.CircuitBreaker
	auth    authorizer
}

func newCaller(cfg config.LedgerConfig) (*caller, error) {
	rc := retryablehttp.NewClient()
	rc.RetryMax = cfg.MaxRetries
	rc.HTTPClient.Timeout = cfg.Timeout
	rc.Logger = nil

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        cfg.BreakerName,
		MaxRequests: cfg.BreakerMaxReq,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 5 },
	})
	auth, err := newAuthorizer(cfg)
	if err != nil {
		return nil, err
	}
	return &caller{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		http:    rc,
		breaker: cb,
		auth:    auth,
	}, nil
}

// do executes a call through the breaker. When bearer is non-empty it is used as
// the Authorization header; otherwise app-level auth is applied.
func (c *caller) do(ctx context.Context, method, path string, body, out any, bearer string) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return domain.NewInternalError("marshal request", err)
		}
		payload = b
	}
	_, err := c.breaker.Execute(func() (any, error) {
		return nil, c.roundtrip(ctx, method, path, payload, out, bearer)
	})
	if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
		return domain.NewCircuitOpenError("provider circuit open", err)
	}
	return err
}

func (c *caller) roundtrip(ctx context.Context, method, path string, payload []byte, out any, bearer string) error {
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
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	} else if err := c.auth.apply(ctx, req); err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return domain.NewTimeoutError("provider timeout", err)
		}
		return domain.NewUpstreamError("provider request failed", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return mapHTTPError(resp.StatusCode, respBody)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return domain.NewUpstreamError("decode provider response", err)
		}
	}
	return nil
}

func mapHTTPError(status int, body []byte) error {
	msg := fmt.Sprintf("provider returned %d", status)
	if snippet := strings.TrimSpace(string(body)); snippet != "" {
		msg = msg + ": " + snippet
	}
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return domain.NewUnauthorizedError(msg, nil)
	case status == http.StatusNotFound:
		return domain.NewNotFoundError(msg, nil)
	case status == http.StatusConflict:
		return domain.NewConflictError(msg, nil)
	default:
		return domain.NewUpstreamError(msg, nil)
	}
}

// ---- authentication strategies ----

type authorizer interface {
	apply(ctx context.Context, req *retryablehttp.Request) error
}

func newAuthorizer(cfg config.LedgerConfig) (authorizer, error) {
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
	a.expiry = time.Now().Add(ttl - 30*time.Second)
	return a.token, nil
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_caller.log at runtime.
var _ = func() bool { flog.For("10_caller").Info("source file initialized"); return true }()
