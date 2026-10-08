# Kestrel — Development Makefile
# Requires: Go 1.26+, Node 22+, npm

BINARY      := kestrel
BUILD_DIR   := build
WEB_DIR     := web
GO_CMD      := go
NPM_CMD     := npm

.PHONY: all build build-web test vet lint clean run dev help

## all: Build backend binary + frontend (default target)
all: build-web build

## build: Compile the Go binary
build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GO_CMD) build -o $(BUILD_DIR)/$(BINARY) ./cmd/server

## build-web: Build the React SPA
build-web:
	@echo "Building frontend..."
	@cd $(WEB_DIR) && $(NPM_CMD) install --silent && $(NPM_CMD) run build

## run: Run the server (dev mode, no HTTPS)
run: build-web
	$(GO_CMD) run ./cmd/server

## run-https: Run the server with self-signed TLS
run-https: build-web
	$(GO_CMD) run ./cmd/server --https

## test: Run all Go unit tests (CGO_ENABLED=1 required for SQLite-backed tests)
test:
	CGO_ENABLED=1 $(GO_CMD) test ./... -v -count=1 -timeout 60s

## test-short: Run tests without network calls
test-short:
	$(GO_CMD) test ./... -v -short -count=1 -timeout 30s

## vet: Run go vet
vet:
	$(GO_CMD) vet ./...

## fmt: Format Go source
fmt:
	$(GO_CMD) fmt ./...

## lint: Run golangci-lint (install separately: https://golangci-lint.run)
lint:
	golangci-lint run ./...

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules

## dev: Run backend with hot-reload (requires 'air': go install github.com/air-verse/air@latest)
dev:
	air

## web-dev: Run Vite dev server (proxy to backend on :8080)
web-dev:
	@cd $(WEB_DIR) && $(NPM_CMD) run dev

## help: Show this help message
help:
	@echo "Kestrel development targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
