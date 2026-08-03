#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo ">> unit tests"
go test ./... -count=1

echo ">> integration tests"
go test -tags integration ./test/integration/... -count=1

echo ">> vet"
go vet ./...
