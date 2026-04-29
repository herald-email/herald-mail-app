# Makefile for Herald

RELEASE_ENV ?= .herald-release.env
ifneq (,$(wildcard $(RELEASE_ENV)))
include $(RELEASE_ENV)
export
endif

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_LDFLAGS := -X github.com/herald-email/herald-mail-app/internal/version.Version=$(VERSION) -X github.com/herald-email/herald-mail-app/internal/version.Commit=$(COMMIT) -X github.com/herald-email/herald-mail-app/internal/version.Date=$(DATE)
OAUTH_LDFLAGS := -X github.com/herald-email/herald-mail-app/internal/oauth.defaultClientID=$(HERALD_GOOGLE_CLIENT_ID) -X github.com/herald-email/herald-mail-app/internal/oauth.defaultClientSecret=$(HERALD_GOOGLE_CLIENT_SECRET)
GO_LDFLAGS := -s -w $(VERSION_LDFLAGS) $(EXTRA_LDFLAGS)

.PHONY: build build-ssh build-mcp build-release-local docs-media run clean test deps fmt vet install-hooks

# Build the application
build:
	@mkdir -p bin
	@go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/herald ./cmd/herald

# Build the legacy SSH server compatibility wrapper
build-ssh:
	@mkdir -p bin
	@go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/herald-ssh-server ./cmd/herald-ssh-server

# Build the legacy MCP server compatibility wrapper
build-mcp:
	@mkdir -p bin
	@go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/herald-mcp-server ./cmd/herald-mcp-server

# Build all local binaries with OAuth defaults from .herald-release.env.
build-release-local:
	@if [ -z "$$HERALD_GOOGLE_CLIENT_ID" ] || [ -z "$$HERALD_GOOGLE_CLIENT_SECRET" ]; then \
		echo "Missing HERALD_GOOGLE_CLIENT_ID or HERALD_GOOGLE_CLIENT_SECRET."; \
		echo "Copy .herald-release.env.example to $(RELEASE_ENV) and fill in local values."; \
		exit 1; \
	fi
	@$(MAKE) build build-mcp build-ssh EXTRA_LDFLAGS="$(OAUTH_LDFLAGS)"

# Regenerate documentation screenshots and demo GIFs.
docs-media: build build-mcp
	demos/generate-doc-media.sh

# Run the application
run:
	go run ./cmd/herald

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# Install repository-managed Git hooks
install-hooks:
	git config core.hooksPath .githooks

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Build for multiple platforms
build-all: clean
	@mkdir -p bin
	@GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/herald-linux-amd64 ./cmd/herald
	@GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/herald-darwin-amd64 ./cmd/herald
	@GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/herald-windows-amd64.exe ./cmd/herald

# Development setup
dev-setup: deps fmt vet test

# Production build
prod-build: dev-setup build
