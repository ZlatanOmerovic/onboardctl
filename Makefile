# onboardctl — developer Makefile.

BIN        := onboardctl
PKG        := ./cmd/onboardctl
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/ZlatanOmerovic/onboardctl/internal/cli.Version=$(VERSION) \
	-X github.com/ZlatanOmerovic/onboardctl/internal/cli.Commit=$(COMMIT) \
	-X github.com/ZlatanOmerovic/onboardctl/internal/cli.BuildDate=$(BUILD_DATE)

.PHONY: help build install test vet fmt tidy lint run clean

help: ## List targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary locally
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

install: ## Install the binary to $GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' $(PKG)

test: ## Run unit tests
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format code
	gofmt -w .

tidy: ## Tidy modules
	go mod tidy

lint: ## Run golangci-lint (install separately: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

run: build ## Build and run
	./$(BIN)

clean: ## Remove build artefacts
	rm -f $(BIN)
	rm -rf dist/
