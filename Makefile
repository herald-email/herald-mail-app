# Makefile for Mail Processor

.PHONY: build run clean test deps fmt vet

# Build the application
build:
	go build -o bin/herald ./main.go

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