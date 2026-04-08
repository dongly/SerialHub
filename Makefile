# SerialHub Makefile

.PHONY: build test vet clean install help release release-windows release-linux

# Binary names
SERIALHUB_BIN=bin/serialhub.exe

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod

# Version from code
VERSION=$(shell grep -E 'const\s+baseVersion\s*=' cmd/serialhub/main.go | sed -E 's/.*"([^"]+)".*/\1/')

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

# 发布构建命令

# Windows 发布构建 (使用 PowerShell)
release-windows:
	@echo "Building Windows release..."
	@powershell -ExecutionPolicy Bypass -File scripts\build-release.ps1

# Linux/macOS 发布构建 (使用 Bash)
release-linux:
	@echo "Building Linux/macOS release..."
	@bash scripts/build-release.sh

# 通用发布命令 (自动检测平台)
release:
ifeq ($(OS),Windows_NT)
	$(MAKE) release-windows
else
	$(MAKE) release-linux
endif

# 带版本号的发布构建
release-version:
ifeq ($(OS),Windows_NT)
	@powershell -ExecutionPolicy Bypass -File scripts\build-release.ps1 -Version "$(VERSION)"
else
	@bash scripts/build-release.sh "$(VERSION)"
endif

# 快速发布 (跳过测试)
release-fast:
ifeq ($(OS),Windows_NT)
	@powershell -ExecutionPolicy Bypass -File scripts\build-release.ps1 -SkipTests -SkipVet
else
	@bash scripts/build-release.sh --skip-tests --skip-vet
endif

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
	@echo "发布命令:"
	@echo "  make release           - Build release for current platform"
	@echo "  make release-windows   - Build Windows release (.zip)"
	@echo "  make release-linux     - Build Linux/macOS release (.tar.gz)"
	@echo "  make release-version   - Build release with version from code"
	@echo "  make release-fast      - Build release (skip tests and vet)"
	@echo ""
	@echo "当前版本: $(VERSION)"
