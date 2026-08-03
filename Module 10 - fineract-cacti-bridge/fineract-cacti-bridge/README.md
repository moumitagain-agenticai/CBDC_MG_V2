# fineract-cacti-bridge

Module 10 of the Fineract CBDC platform: a **cross-chain settlement coordinator**.
It moves value between two ledgers (e.g. a Corda-based eAED ledger and a Besu-based
digital-euro ledger) through Hyperledger **Cacti** connectors, using a durable
**lock → release → burn** saga with compensation, explicit rollback, and
crash recovery.

## The saga

```
INITIATED --lock(source)--> LOCKED --release(dest)--> RELEASED --burn(source)--> BURNED
                              |
                              | release fails
                              v
                        COMPENSATING --unlock(source)--> COMPENSATED
```

1. **Lock** escrows value on the **source** ledger.
2. **Release** credits value on the **destination** ledger.
3. **Burn** destroys the locked source value, finalising the transfer.

**Point of no return:** once `RELEASED`, the destination is already credited, so
the saga only rolls *forward* — the burn is retried and, if it can't complete, the
transfer stays `RELEASED` (recoverable) rather than being rolled back. Before
`RELEASED`, a failure (or an explicit rollback request) **compensates** by
unlocking the source.

**Idempotency & recovery:** every ledger operation carries the transfer id as an
idempotency key, and state is persisted after each step. On startup the
coordinator resumes every in-flight settlement to a terminal state, so a crash
mid-saga is healed automatically (with the Postgres repository).

## API

| Method | Path                                 | Purpose                              |
|--------|--------------------------------------|--------------------------------------|
| POST   | `/api/v1/settlements`                | Initiate a settlement (idempotent)   |
| GET    | `/api/v1/settlements/{id}`           | Fetch a settlement                   |
| POST   | `/api/v1/settlements/{id}/rollback`  | Compensate an in-flight settlement   |
| GET    | `/healthz` `/readyz` `/metrics`      | Liveness / readiness / Prometheus    |

See `api/openapi.yaml` for the full contract.

## Configuration

Config is layered: `configs/config.yaml` (see `configs/config.example.yaml`),
overridden by environment (see `.env.example`). Two connectors must be
configured — `ledgers.source` and `ledgers.dest`. Setting `DATABASE_DSN` enables
durable Postgres storage and crash recovery; otherwise an in-memory repository is
used. Config is validated fail-fast on startup.

## Running

```bash
make deps        # go mod tidy (generates go.sum)
make test        # unit tests
make itest       # integration tests (fake Cacti connectors over httptest)
make build       # -> bin/cacti-bridge
make run         # start the server

# operational one-shots
./bin/cacti-bridge -rollback <settlement-id>     # compensate a specific settlement
./bin/cacti-bridge -migrate-rollback 1           # roll back the last DB migration
```

## Layout

```
cmd/server            entrypoint (wiring, recovery, graceful shutdown)
internal/domain       Transfer, statuses, state machine, errors
internal/ports        LedgerConnector + SettlementRepository interfaces
internal/service      Coordinator (saga), recovery, health
internal/adapters
  client              Cacti connector HTTP client (retry + breaker + auth)
  repository          in-memory + Postgres, versioned migrations
  api                 chi router, handlers, middleware
pkg                   logger (zap), metrics (Prometheus), utils, flog
```

## Per-source-file logging (Logrus)

Alongside the primary structured logger (zap, for console/DI), this module ships
a **Logrus** per-source-file logger in `pkg/flog`. Every source file writes its
own log file under `logs/`, named `10_<file>.log` (module number + source file
name), e.g. `10_main.log`, `10_coordinator.log`, `10_migration.log`. Each file
registers itself with the logger and a Logrus hook routes entries to the matching
file (JSON formatted). The `logs/` directory is created on startup and is
git-ignored. `go.sum` for the added `github.com/sirupsen/logrus` dependency is
generated on the first `go mod tidy`.
