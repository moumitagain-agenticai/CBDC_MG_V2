# BUILD REPORT — fineract-cacti-bridge (Module 10)

Toolchain: Go 1.22. Module path: `github.com/fineract/cacti-bridge`.

## Verification

| Check | Command | Result |
|-------|---------|--------|
| Build | `go build ./...` | pass |
| Vet | `go vet ./...` | pass |
| Unit tests | `go test ./...` | pass |
| Integration tests | `go test -tags integration ./test/integration/...` | pass |
| Formatting | `gofmt -l .` | clean |

## Test coverage of the saga

Unit (`internal/service`, mock connectors + in-memory repo):
- happy path lock→release→burn ⇒ `BURNED`
- lock fails ⇒ `FAILED`, destination never touched
- release fails ⇒ `COMPENSATING`→unlock⇒`COMPENSATED`
- burn fails twice then succeeds ⇒ `BURNED` (3 burn calls)
- burn exhausted ⇒ stays `RELEASED`, `burn_attempts=4`, "burn pending"
- idempotent initiation (same `reference_id` ⇒ same transfer, saga runs once)
- rollback of a `RELEASED` transfer ⇒ refused (409 / Conflict)
- **recovery** resumes a `RELEASED` transfer to `BURNED` with a healthy source
- validation error on bad request

Domain (`internal/domain`): legal/illegal transition matrix, idempotent no-op,
illegal transition leaves state unchanged, status helper classification.

Integration (`test/integration`, real Cacti client over `httptest` fake source
and destination connectors): full happy-path flow ⇒ `BURNED` (no unlock);
destination `502` on release ⇒ `COMPENSATED` with `unlock_tx_id`; same
source/dest ledger ⇒ `400`.

## Notes

- Ship `go.mod` is clean (canonical module paths, no replace directives). Run
  `make deps` (`go mod tidy`) once to generate `go.sum`.
- Nothing in this module is pushed to any remote; it is delivered as a
  self-contained zip.
