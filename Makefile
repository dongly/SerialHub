# SerialHub Makefile

.PHONY: build test vet clean install help

# Binary names
SERIALHUB_BIN=bin/serialhub.exe
SERVE_BIN=bin/serve.exe

# Go parameters
GOCMD=go
GOBUILD=$(`GOCMD`) build
GOTEST=$(`GOCMD`) test
GOVET=$(`GOCMD`) vet
GOMOD=$(`GOCMD`) mod

# Build the serialhub CLI
build-cli:
	@echo "Building serialhub CLI..."
	@mkdir -p bin
	$(`GOBUILD`) -o $(`SERIALHUB_BIN`) ./cmd/serialhub

# Build the serve HTTP service
build-serve:
	@echo "Building serve HTTP service..."
	@mkdir -p bin
	$(`GOBUILD`) -o $(`SERVE_BIN`) ./cmd/serve

# Build both binaries
build: build-cli build-serve

# Run tests
test:
	@echo "Running tests..."
	$(`GOTEST`) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(`GOTEST`) -cover -coverprofile=coverage.out ./...
	$(`GOCMD`) tool cover -html=coverage.out -o coverage.html

# Run go vet
vet:
	@echo "Running go vet..."
	$(`GOVET`) ./...

# Run go build check (type checking)
check:
	@echo "Running type check..."
	$(`GOBUILD`) ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(`GOMOD`) tidy
	$(`GOMOD`) download

# Display help
help:
	@echo "Available targets:"
	@echo "  make build          - Build both serialhub and serve binaries"
	@echo "  make build-cli      - Build serialhub CLI only"
	@echo "  make build-serve    - Build serve HTTP service only"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make vet            - Run go vet"
	@echo "  make check          - Run type check via go build"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo "  make help           - Display this help message"
