# fineract-cbdc-india-connector

Module 2 of the E1 Cross-Border CBDC platform. It connects Apache Fineract to
the **Indian CBDC (e₹)** through a sponsor-bank API and exposes the standard
connector operations — **issue, transfer, lock, burn, redeem** — plus balance,
status, and health endpoints.

It is built to the same contract as **Module 1**
(`fineract-cbdc-connector-abstraction`): the connector service implements the
same five operations, so this module is a drop-in implementation of the shared
abstraction. It builds and runs stand-alone today, and wires into the monorepo
on top of Module 1 with no code changes (see the commented block in `go.mod`).

## Architecture

Hexagonal (ports & adapters):

```
cmd/server            entrypoint: config load, DI wiring, graceful shutdown
internal/domain       entities + typed error taxonomy (no leaking internals)
internal/ports        interfaces + DTOs (CBDCClient, TransactionRepository)
internal/service      business logic: validate → persist → call upstream → update
internal/adapters/
  api                 chi router, handlers, middleware (request-id, recover, logs)
  client              sponsor-bank HTTP client (retry + circuit breaker + auth)
  repository          PostgreSQL transaction audit trail (parameterized SQL)
internal/config       YAML + env config, validated fail-fast on startup
pkg/logger            zap structured logging
pkg/metrics           Prometheus metrics on a private registry
pkg/utils             money-safe amount validation (big.Rat, never float)
```

## Quickstart

```bash
# 1. dependencies + build (creates go.sum on first run)
make build            # or: go build ./cmd/server

# 2. configure
cp configs/config.example.yaml configs/config.yaml
cp .env.example .env   # put your sponsor-bank credentials here

# 3. run
make run               # or: go run ./cmd/server -config configs/config.yaml

# 4. test
make test              # unit tests
make itest             # integration tests (httptest against mocks)
```

With Docker:

```bash
docker compose -f deployments/docker-compose.yaml up --build
```

## Configuration

Config comes from `configs/config.yaml`, overridden by environment variables
(secrets should always come from env). Key variables:

| Env | Meaning |
|-----|---------|
| `CBDC_BASE_URL` | Sponsor-bank e₹ API base URL (required) |
| `CBDC_AUTH_MODE` | `apikey` \| `oauth2` \| `mtls` |
| `CBDC_API_KEY` | API key (apikey mode) |
| `CBDC_OAUTH_TOKEN_URL` / `CBDC_CLIENT_ID` / `CBDC_CLIENT_SECRET` | oauth2 mode |
| `DATABASE_DSN` | Postgres DSN; setting it enables the audit trail |
| `SERVER_PORT` | HTTP port (default 8080) |
| `LOG_LEVEL` | `debug` \| `info` \| `warn` \| `error` |

The server **fails fast** on startup if required config is missing or
inconsistent.

## Database migrations

When persistence is enabled (`DATABASE_DSN` set), the connector **runs pending
migrations automatically on startup** and records them in a `schema_migrations`
table. Migrations are defined in Go (`internal/adapters/repository/migration.go`)
with versioned up/down SQL.

```bash
# roll back the last migration and exit
go run ./cmd/server -rollback 1
```

`migrations/001_init.sql` is also provided as a standalone reference you can
apply manually (`psql "$DATABASE_DSN" -f migrations/001_init.sql`) if you prefer
to manage schema outside the app.

## API

Base path `/api/v1`. See `api/openapi.yaml` for the full spec.

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/issue` | Mint e₹ into a wallet |
| POST | `/api/v1/transfer` | Move e₹ between wallets |
| POST | `/api/v1/lock` | Reserve e₹ |
| POST | `/api/v1/burn` | Destroy e₹ |
| POST | `/api/v1/redeem` | Redeem e₹ to bank money |
| GET | `/api/v1/wallets/{walletID}/balance` | Wallet balance |
| GET | `/api/v1/transactions/{id}/status` | Upstream status |
| GET | `/api/v1/transactions/{id}` | Persisted transaction |
| GET | `/healthz` `/readyz` `/metrics` | Liveness / readiness / Prometheus |

Example:

```bash
curl -X POST localhost:8080/api/v1/issue \
  -H 'Content-Type: application/json' \
  -d '{"wallet_id":"w1","amount":"100.50","currency":"INR",
       "reference_id":"11111111-1111-4111-8111-111111111111"}'
```

Write operations are **idempotent** on `reference_id` (a UUIDv4): replaying the
same reference returns the original result instead of double-processing.

## Production features

Retry with backoff (`go-retryablehttp`), circuit breaker (`gobreaker`),
context timeouts, request-id correlation, structured logs, Prometheus metrics,
input validation (`validator/v10`), a stable error-code → HTTP-status mapping,
rate limiting, CORS, graceful shutdown, parameterized SQL, and a non-root
multi-stage Docker image.

## Note on `go.sum`

`go.sum` is generated on first `go mod tidy` / `go build` (it needs network to
fetch and checksum dependencies). Run `make deps` once in an environment with
internet access and commit the resulting `go.sum`. The dependency set in
`go.mod` is standard and pulls cleanly from the public Go proxy.
