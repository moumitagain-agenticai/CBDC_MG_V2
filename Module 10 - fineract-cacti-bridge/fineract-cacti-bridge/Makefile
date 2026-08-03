.PHONY: help deps build run test itest vet lint docker clean

BIN := bin/cacti-bridge

help:
	@echo "Targets:"
	@echo "  deps    - go mod tidy (populates go.sum)"
	@echo "  build   - compile the server binary to $(BIN)"
	@echo "  run     - run the server (uses configs/config.yaml)"
	@echo "  test    - unit tests"
	@echo "  itest   - integration tests (build tag: integration)"
	@echo "  vet     - go vet"
	@echo "  lint    - golangci-lint (if installed)"
	@echo "  docker  - build the docker image"
	@echo "  clean   - remove build artifacts"

deps:
	go mod tidy

build: deps
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/server

run:
	go run ./cmd/server -config configs/config.yaml

test:
	go test ./... -count=1

itest:
	go test -tags integration ./test/integration/... -count=1

vet:
	go vet ./...

lint:
	golangci-lint run ./... || echo "golangci-lint not installed; skipping"

docker:
	docker build -f deployments/Dockerfile -t fineract-cacti-bridge:latest .

clean:
	rm -rf bin
