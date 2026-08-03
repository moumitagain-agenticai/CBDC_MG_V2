# EXPLANATION — fineract-cacti-bridge (Module 10)

## What this module is

A settlement coordinator for cross-chain CBDC transfers. Two central-bank (or
commercial-bank) ledgers can't share a single database transaction, so an
atomic-swap-style protocol is used instead: value is **locked** on the source,
**released** on the destination, then **burned** on the source. The bridge is the
stateful orchestrator that runs this protocol reliably in the presence of
partial failures and crashes.

## Why a saga, not a two-phase commit

The two ledgers are autonomous systems reached over the network via Hyperledger
Cacti connectors. There is no shared transaction manager and no way to hold locks
across both indefinitely. So the module uses the **saga** pattern: a sequence of
local operations, each with a compensating action, coordinated by a durable state
machine. The coordinator persists progress after every step; the compensation
path unwinds a source lock if the destination release fails.

## The state machine (`internal/domain/state_machine.go`)

Transitions are explicit and validated, which makes illegal moves impossible and
recovery trivial to reason about:

- `INITIATED → {LOCKED, FAILED}`
- `LOCKED → {RELEASED, COMPENSATING, FAILED}`
- `RELEASED → {BURNED, FAILED}` — deliberately **no** path back to compensation
- `COMPENSATING → {COMPENSATED, FAILED}`

The asymmetry around `RELEASED` is the core safety property: once the destination
is credited, rolling back would create money on one ledger without destroying it
on the other. So after release the saga only rolls forward.

## Correctness properties

- **No double-apply.** Every `LedgerOp` carries the transfer id as an idempotency
  key. Re-driving a step (retry or recovery) is safe because the connector
  deduplicates on that key.
- **No value creation/destruction across the pair.** Value exists as *locked on
  source* until it is *released on destination*; the burn only removes the
  already-locked source value. A pre-release failure unlocks the source, leaving
  both ledgers whole.
- **Crash recovery.** State is persisted after each transition. On startup
  `Recover()` lists non-terminal settlements and resumes each via the same
  `runForward` routine used for new transfers — the stage guards mean a
  `RELEASED` transfer only completes its burn, a `COMPENSATING` one only
  finishes unlocking, and so on.
- **Bounded burn retries with fresh recovery budget.** `burn_max_attempts` bounds
  each *invocation*; `burn_attempts` accumulates across invocations for
  telemetry. An exhausted burn leaves the transfer `RELEASED` (never rolled
  back); a later recovery run retries it with a fresh budget.

## Ports & adapters

The `Coordinator` depends only on two interfaces: `LedgerConnector` (Lock /
Release / Burn / Unlock / Health) and `SettlementRepository`. The real
`LedgerConnector` is a Cacti REST client built on the shared resilient caller
(retryable HTTP + circuit breaker + pluggable auth). The repository has an
in-memory implementation (default) and a Postgres one (durable, enables
recovery). Tests swap in a mock connector and the in-memory repository, so the
full saga — including compensation, retry, idempotency and recovery — is
verified without a network or database.

## Operational surface

`-rollback <id>` compensates a specific settlement and exits; `-migrate-rollback
N` reverses the last N database migrations; the HTTP API exposes initiate / fetch
/ rollback plus health and Prometheus metrics. Graceful shutdown drains the
server on SIGINT/SIGTERM.
