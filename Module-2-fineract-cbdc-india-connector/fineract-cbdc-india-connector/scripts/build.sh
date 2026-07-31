#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo ">> go mod tidy"
go mod tidy

echo ">> go build"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/india-connector ./cmd/server

echo ">> built bin/india-connector"
