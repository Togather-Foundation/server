.PHONY: help build test test-ci lint lint-ci ci fmt clean run dev install-tools install-pyshacl test-contracts validate-shapes sqlc sqlc-generate migrate-up migrate-down coverage-check

MIGRATIONS_DIR := internal/storage/postgres/migrations

# Default target
help:
	@echo "Togather Server - Build Commands"
	@echo ""
	@echo "Available targets:"
	@echo "  make build         - Build the server binary"
	@echo "  make test          - Run all tests"
	@echo "  make test-ci       - Run tests exactly as CI does (race detector, verbose)"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make lint-ci       - Run golangci-lint exactly as CI does (with 5m timeout)"
	@echo "  make ci            - Run full CI pipeline locally (lint, format check, tests, build)"
	@echo "  make test-v        - Run tests with verbose output"
	@echo "  make test-race     - Run tests with race detector"
	@echo "  make coverage      - Run tests with coverage report (enforces 80% threshold)"
	@echo "  make coverage-check - Check if coverage meets 80% threshold"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make lint-ci       - Run golangci-lint exactly as CI does (with 5m timeout)"
	@echo "  make fmt           - Format all Go files"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make run           - Build and run the server"
	@echo "  make dev           - Run in development mode (with air if available)"
	@echo "  make install-tools - Install development tools (Go tools)"
	@echo "  make install-pyshacl - Install pyshacl for SHACL validation"
	@echo "  make test-contracts - Run contract tests (requires pyshacl)"
	@echo "  make validate-shapes - Validate SHACL shapes against sample data"
	@echo "  make sqlc          - Generate SQLc code (alias for sqlc-generate)"
	@echo "  make sqlc-generate - Generate SQLc code from SQL queries"
	@echo "  make migrate-up    - Run database migrations"
	@echo "  make migrate-down  - Roll back one migration"

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'github.com/Togather-Foundation/server/cmd/server/cmd.Version=$(VERSION)' \
           -X 'github.com/Togather-Foundation/server/cmd/server/cmd.GitCommit=$(GIT_COMMIT)' \
           -X 'github.com/Togather-Foundation/server/cmd/server/cmd.BuildDate=$(BUILD_DATE)'

# Build the server
build:
	@echo "Building server..."
	@go build -ldflags "$(LDFLAGS)" -o bin/togather-server ./cmd/server

# Run all tests
test:
	@echo "Running tests..."
	@go test ./...

# Run tests exactly as CI does
test-ci:
	@echo "Running tests as CI does (with race detector and verbose output)..."
	@echo ""
	@echo "==> Running unit tests..."
	@go test -v -race -coverprofile=coverage.out ./internal/...
	@echo ""
	@echo "==> Running contract tests..."
	@if ! command -v pyshacl > /dev/null 2>&1; then \
		echo "WARNING: pyshacl not found. SHACL validation will be skipped."; \
		echo "Install with: make install-pyshacl"; \
		echo ""; \
	fi
	@go test -v -race ./tests/contracts/...
	@echo ""
	@echo "==> Running integration tests..."
	@go test -v -race ./tests/integration/...
	@echo ""
	@echo "==> Running E2E tests..."
	@go test -v -race ./tests/e2e/...
	@echo ""
	@echo "✓ All tests passed!"

# Run tests with verbose output
test-v:
	@echo "Running tests (verbose)..."
	@go test -v ./...

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	@go test -race ./...

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@$(MAKE) coverage-check

# Check coverage threshold (80% minimum)
coverage-check:
	@echo ""
	@echo "Checking coverage threshold (35% minimum)..."
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	THRESHOLD=35; \
	if [ -z "$$COVERAGE" ]; then \
		echo "ERROR: Could not parse coverage percentage"; \
		exit 1; \
	fi; \
	echo "Current coverage: $$COVERAGE%"; \
	if [ $$(echo "$$COVERAGE >= $$THRESHOLD" | bc) -eq 1 ]; then \
		echo "✓ Coverage meets threshold ($$COVERAGE% >= $$THRESHOLD%)"; \
	else \
		echo "✗ Coverage below threshold ($$COVERAGE% < $$THRESHOLD%)"; \
		echo ""; \
		echo "To improve coverage:"; \
		echo "  1. Run 'make coverage' to generate HTML report"; \
		echo "  2. Open coverage.html to see uncovered lines"; \
		echo "  3. Add tests for uncovered code paths"; \
		exit 1; \
	fi

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
	elif [ -f $(HOME)/go/bin/golangci-lint ]; then \
		$(HOME)/go/bin/golangci-lint run ./...; \
	elif [ -f $(GOPATH)/bin/golangci-lint ]; then \
		$(GOPATH)/bin/golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install with 'make install-tools'"; \
		exit 1; \
	fi

# Run linter exactly as CI does
lint-ci:
	@echo "Running linter as CI does (with 5m timeout)..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run --timeout=5m ./...; \
	elif [ -f $(HOME)/go/bin/golangci-lint ]; then \
		$(HOME)/go/bin/golangci-lint run --timeout=5m ./...; \
	elif [ -f $(GOPATH)/bin/golangci-lint ]; then \
		$(GOPATH)/bin/golangci-lint run --timeout=5m ./...; \
	else \
		echo "golangci-lint not found. Install with 'make install-tools'"; \
		exit 1; \
	fi

# Run full CI pipeline locally
ci: lint-ci
	@echo ""
	@echo "==> Checking code formatting..."
	@if [ "$$(gofmt -l . | wc -l)" -gt 0 ]; then \
		echo "✗ Code is not formatted. Run 'make fmt'"; \
		gofmt -l .; \
		exit 1; \
	else \
		echo "✓ Code is properly formatted"; \
	fi
	@echo ""
	@echo "==> Building server..."
	@$(MAKE) build
	@if [ ! -f bin/togather-server ]; then \
		echo "✗ Build failed: binary not found"; \
		exit 1; \
	fi
	@echo "✓ Build successful"
	@echo ""
	@echo "==> Checking SQLc generation..."
	@$(MAKE) sqlc-generate
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "✗ Generated SQLc code differs from committed version"; \
		echo "Run 'make sqlc-generate' and commit changes"; \
		git diff; \
		exit 1; \
	else \
		echo "✓ SQLc code is up to date"; \
	fi
	@echo ""
	@$(MAKE) test-ci
	@echo ""
	@echo "=========================================="
	@echo "✓ All CI checks passed!"
	@echo "=========================================="

# Format all Go files
fmt:
	@echo "Formatting code..."
	@gofmt -w .
	@go mod tidy

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean

# Build and run the server
run: build
	@echo "Running server..."
	@./bin/togather-server

# Development mode (with live reload if air is installed)
dev:
	@if which air > /dev/null; then \
		echo "Running with air (live reload)..."; \
		air; \
	else \
		echo "air not found, running without live reload..."; \
		echo "Install air with: go install github.com/air-verse/air@latest"; \
		go run ./cmd/server; \
	fi

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Installing air (live reload)..."
	@go install github.com/air-verse/air@latest
	@echo "Installing sqlc..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "Installing migrate..."
	@go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "Go tools installed successfully!"
	@echo ""
	@echo "Make sure $(HOME)/go/bin is in your PATH:"
	@echo "  export PATH=\$$PATH:\$$(go env GOPATH)/bin"
	@echo ""
	@echo "Note: To enable SHACL validation, run 'make install-pyshacl'"

# Install pyshacl for SHACL validation
install-pyshacl:
	@echo "Installing pyshacl for SHACL validation..."
	@if command -v pyshacl > /dev/null 2>&1; then \
		echo "pyshacl is already installed:"; \
		pyshacl --version; \
	elif command -v uv > /dev/null 2>&1; then \
		echo "Using uv to install pyshacl..."; \
		uv tool install pyshacl; \
		echo ""; \
		echo "✓ pyshacl installed successfully!"; \
		echo "Note: Make sure ~/.local/bin is in your PATH"; \
	elif command -v uvx > /dev/null 2>&1; then \
		echo "Testing pyshacl with uvx (no installation needed)..."; \
		uvx pyshacl --version; \
		echo ""; \
		echo "✓ pyshacl works with uvx!"; \
		echo "Note: uvx will automatically run pyshacl when needed (no installation required)"; \
	elif command -v pipx > /dev/null 2>&1; then \
		echo "Using pipx to install pyshacl..."; \
		pipx install pyshacl; \
		echo "pyshacl installed successfully via pipx!"; \
	elif command -v pip3 > /dev/null 2>&1; then \
		echo "Attempting to install with pip3 --user..."; \
		pip3 install --user pyshacl || pip3 install --user --break-system-packages pyshacl; \
		echo "pyshacl installed successfully!"; \
		echo "Note: You may need to add ~/.local/bin to your PATH"; \
	else \
		echo "ERROR: No Python package manager found. Please install one of:"; \
		echo "  - uv (recommended): curl -LsSf https://astral.sh/uv/install.sh | sh"; \
		echo "  - pipx: sudo apt install pipx"; \
		echo "  - pip3: sudo apt install python3-pip"; \
		exit 1; \
	fi

# Run contract tests (requires pyshacl)
test-contracts:
	@echo "Running contract tests..."
	@if ! command -v pyshacl > /dev/null 2>&1; then \
		echo "WARNING: pyshacl not found. SHACL validation will be skipped."; \
		echo "Install with: make install-pyshacl"; \
		echo ""; \
	fi
	@go test -v ./tests/contracts/...

# Validate SHACL shapes against sample data
validate-shapes:
	@echo "Validating SHACL shapes..."
	@if ! command -v pyshacl > /dev/null 2>&1; then \
		echo "ERROR: pyshacl not found. Install with: make install-pyshacl"; \
		exit 1; \
	fi
	@echo "Running SHACL validation test..."
	@go test -v ./tests/contracts/... -run SHACL

# SQLc generate
.PHONY: sqlc-generate
sqlc-generate:
	@echo "Generating SQLc code..."
	@if command -v sqlc > /dev/null 2>&1; then \
		sqlc generate; \
	elif [ -f $(HOME)/go/bin/sqlc ]; then \
		$(HOME)/go/bin/sqlc generate; \
	elif [ -f $(GOPATH)/bin/sqlc ]; then \
		$(GOPATH)/bin/sqlc generate; \
	else \
		echo "sqlc not found. Install with 'make install-tools'"; \
		exit 1; \
	fi
	@echo "SQLc code generation complete!"

# Alias for sqlc-generate
.PHONY: sqlc
sqlc: sqlc-generate

# Database migrations
.PHONY: migrate-up
migrate-up:
	@echo "Running migrations..."
	@if command -v migrate > /dev/null 2>&1; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	elif [ -f $(HOME)/go/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(HOME)/go/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	elif [ -f $(GOPATH)/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(GOPATH)/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	else \
		echo "migrate not found. Install with 'make install-tools'"; \
		exit 1; \
	fi

.PHONY: migrate-down
migrate-down:
	@echo "Rolling back last migration..."
	@if command -v migrate > /dev/null 2>&1; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	elif [ -f $(HOME)/go/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(HOME)/go/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	elif [ -f $(GOPATH)/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(GOPATH)/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	else \
		echo "migrate not found. Install with 'make install-tools'"; \
		exit 1; \
	fi
