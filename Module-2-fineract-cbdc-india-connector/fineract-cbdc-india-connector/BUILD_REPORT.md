# Build & verification report

This module was verified against its **real dependencies** before delivery.

## Results

| Check | Command | Result |
|-------|---------|--------|
| Compile | `go build ./...` | PASS (exit 0) |
| Vet | `go vet ./...` | PASS (exit 0) |
| Unit tests | `go test ./...` | PASS |
| Integration tests | `go test -tags integration ./test/integration/...` | PASS |
| Binary | `go build -o bin/india-connector ./cmd/server` | PASS (~17 MB static binary) |
| Formatting | `gofmt -l .` | clean (no files) |

## Runtime smoke test

- Starts and **fails fast** when required config is missing
  (`cbdc.base_url is required`).
- With env config set: `GET /healthz` → `200`, `GET /metrics` → `200`
  (Prometheus exposition), structured JSON logs with request-id correlation.
- **Graceful shutdown** on `SIGTERM` drains and exits cleanly.
- **Migrations**: when a database is configured the connector auto-applies
  pending migrations on startup; `-rollback N` reverts and exits. Without a DB,
  `-rollback` errors cleanly and the migration test skips.

## How to reproduce on your machine

```bash
go mod tidy          # creates go.sum (needs internet, one time)
go build ./...
go vet ./...
go test ./...
go test -tags integration ./test/integration/...
go build -o bin/india-connector ./cmd/server
```

## Note

`go.mod` ships **clean** with standard, canonical dependency paths — it pulls
directly from the public Go module proxy on any normal machine. `go.sum` is
produced by the first `go mod tidy`/`go build`; it is intentionally not shipped
because its checksums are created (and verified) against the live proxy.
