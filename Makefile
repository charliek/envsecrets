.PHONY: build test test-integration lint clean install fmt tidy check coverage docs docs-serve

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X github.com/charliek/envsecrets/internal/version.Version=$(VERSION) -X github.com/charliek/envsecrets/internal/version.GitCommit=$(GIT_COMMIT) -X github.com/charliek/envsecrets/internal/version.BuildDate=$(BUILD_DATE)"

# Build the binary
build:
	go build $(LDFLAGS) -o bin/envsecrets ./cmd/envsecrets

# Run all unit tests
test:
	go test -v ./...

# Run integration tests (requires Docker)
test-integration:
	go test -v -tags=integration ./test/integration/...

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf bin/ dist/ coverage.out coverage.html

# Install to local bin
install: build
	mkdir -p ~/.local/bin
	cp bin/envsecrets ~/.local/bin/envsecrets

# Format code
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Run all checks (lint + test)
check: lint test

# Run tests with coverage
coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Build documentation
docs:
	uv sync --group docs
	uv run mkdocs build

# Serve documentation locally
docs-serve:
	uv sync --group docs
	uv run mkdocs serve
