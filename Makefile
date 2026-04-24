# Makefile for Herald

.PHONY: build build-ssh build-mcp run clean test deps fmt vet install-hooks

# Build the application
build:
	go build -o bin/herald ./main.go

# Build the SSH server
build-ssh:
	go build -o bin/herald-ssh-server ./cmd/herald-ssh-server

# Build the MCP server
build-mcp:
	go build -o bin/herald-mcp-server ./cmd/herald-mcp-server

# Run the application
run:
	go run ./main.go

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
	GOOS=linux GOARCH=amd64 go build -o bin/herald-linux-amd64 ./main.go
	GOOS=darwin GOARCH=amd64 go build -o bin/herald-darwin-amd64 ./main.go
	GOOS=windows GOARCH=amd64 go build -o bin/herald-windows-amd64.exe ./main.go

# Development setup
dev-setup: deps fmt vet test

# Production build
prod-build: dev-setup build
