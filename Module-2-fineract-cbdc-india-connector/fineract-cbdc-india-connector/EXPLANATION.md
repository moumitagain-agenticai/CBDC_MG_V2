# EXPLANATION — how this module works (plain language)

This document explains the module in simple terms, so you can understand and
present it without reading every file.

## What is this module for?

Fineract is the core banking system. It does not know how to talk to India's
digital rupee (e₹). This module is the **translator / adapter** in between.
Fineract (or any caller) sends a simple HTTP request like "issue 100 e₹ to
wallet X", and this module talks to the **sponsor bank's e₹ API** to actually
do it, then reports back what happened.

The five core actions are:

- **Issue**  — create new e₹ into a wallet.
- **Transfer** — move e₹ from one wallet to another.
- **Lock** — freeze/reserve e₹ (used during settlement).
- **Burn** — destroy e₹.
- **Redeem** — convert e₹ back into normal bank money.

## How a request flows (issue as example)

1. A caller sends `POST /api/v1/issue` with a JSON body.
2. **api layer** decodes the JSON and passes it to the service.
3. **service layer** validates it (amount must be a positive number, currency
   must be 3 letters, reference id must be a UUID). If invalid → `400`.
4. It checks **idempotency**: if this reference id was already processed, it
   returns the old result instead of doing it twice.
5. It writes a `PENDING` record to the database (if a database is configured).
6. It calls the **sponsor-bank client**, which sends the real HTTP request with
   retries and a circuit breaker for safety.
7. On success it updates the record to `CONFIRMED` and returns the result.
   On failure it marks it `FAILED` and returns a clear error.
8. Metrics and a structured log line are recorded for the whole thing.

## Why "ports and adapters" (hexagonal)?

The **service** (the brain) only depends on **interfaces** (called *ports*),
not on concrete details. The real HTTP client and the real database are
**adapters** that plug into those ports. Benefits:

- We can test the brain with a fake client (see `test/mocks`) — no real bank
  needed.
- We can swap PostgreSQL for something else, or the sponsor bank for another
  provider, without touching the business logic.
- It matches Module 1's design, so all connectors look and behave the same.

## The safety features, in one line each

- **Retries**: transient network blips are retried automatically.
- **Circuit breaker**: if the bank keeps failing, we stop hammering it and fail
  fast for a while, then recover.
- **Idempotency**: the same reference id never double-charges.
- **Validation**: bad input is rejected before we call the bank.
- **Money as text**: amounts use exact decimals, never floating point.
- **Typed errors**: every failure maps to a stable code and HTTP status.
- **Graceful shutdown**: on stop, in-flight requests finish before exit.
- **Metrics + logs**: everything is observable in Prometheus and JSON logs.

## Relationship to Module 1

Module 1 defines the *standard interface* every CBDC connector must implement.
This module (India) implements exactly those five operations with the same
shapes, so it is a real, working example of that abstraction. It is built to
run on its own now, and to snap onto Module 1 later (one commented block in
`go.mod`) with no code changes.

## What was verified

`go build ./...`, `go vet ./...`, unit tests, and integration tests all pass,
and the server binary starts, serves `/healthz` and `/metrics`, and shuts down
cleanly on a signal.
