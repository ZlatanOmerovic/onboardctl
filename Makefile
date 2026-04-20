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

.PHONY: help build install test vet fmt tidy lint run clean extras release-dry

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

fmt: ## Format code (gofmt -s + goimports if available)
	gofmt -s -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -local github.com/ZlatanOmerovic/onboardctl -w .; \
	else \
		echo "goimports not found — run: go install golang.org/x/tools/cmd/goimports@latest"; \
	fi

tidy: ## Tidy modules
	go mod tidy

lint: ## Run golangci-lint (install separately: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

run: build ## Build and run
	./$(BIN)

clean: ## Remove build artefacts
	rm -f $(BIN)
	rm -rf dist/

extras: ## Generate shell completions + manpages under dist/extras/
	@rm -rf dist/extras
	@mkdir -p dist
	go run ./cmd/gen dist/extras

release-dry: extras ## Locally simulate a release without publishing (needs goreleaser)
	@command -v goreleaser >/dev/null || { echo "install goreleaser: https://goreleaser.com/install"; exit 1; }
	goreleaser release --snapshot --clean --skip=publish,sign
