# SerialHub Makefile

.PHONY: build test test-coverage vet check clean deps help

# Binary names (Windows 用 .exe，Linux/macOS/WSL 无后缀)
ifeq ($(OS),Windows_NT)
SERIALHUB_BIN=bin/serialhub.exe
else
SERIALHUB_BIN=bin/serialhub
endif

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod

# Build the serialhub CLI
build:
	@echo "Building serialhub CLI..."
	@mkdir -p bin
	$(GOBUILD) -ldflags "-s -w" -o $(SERIALHUB_BIN) ./cmd/serialhub

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -cover -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Run go build check (type checking)
check:
	@echo "Running type check..."
	$(GOBUILD) ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf dist/
	rm -f coverage.out coverage.html

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

# 发布已全权交给 GitHub Actions（.github/workflows/release.yml，tag 驱动），
# 见 RELEASING.md。

# Display help
help:
	@echo "SerialHub Makefile"
	@echo ""
	@echo "开发命令:"
	@echo "  make build          - Build serialhub binary"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make vet            - Run go vet"
	@echo "  make check          - Run type check via go build"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo ""
	@echo "发布: 由 GitHub Actions 自动完成（推送 v* 标签触发），见 RELEASING.md"
