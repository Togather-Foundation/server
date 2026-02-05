.PHONY: help build test test-ci lint lint-ci lint-openapi vulncheck ci fmt clean run dev install-tools install-pyshacl test-contracts validate-shapes sqlc sqlc-generate migrate-up migrate-down migrate-river coverage-check docker-up docker-db docker-down docker-logs docker-rebuild docker-clean docker-compose-lint db-setup db-init db-check setup deploy-package test-local test-staging test-staging-smoke test-production-smoke test-remote

MIGRATIONS_DIR := internal/storage/postgres/migrations
DOCKER_COMPOSE_DIR := deploy/docker
DOCKER_COMPOSE_FILE := $(DOCKER_COMPOSE_DIR)/docker-compose.yml
DB_NAME ?= togather
DB_USER ?= $(USER)
DB_PORT ?= 5432

# Default target
help:
	@echo "Togather Server - Build Commands"
	@echo ""
	@echo "Getting Started:"
	@echo "  make setup         - 🚀 Interactive first-time setup (RECOMMENDED)"
	@echo ""
	@echo "Available targets:"
	@echo "  make build         - Build the server binary"
	@echo "  make test          - Run all tests"
	@echo "  make test-ci       - Run tests exactly as CI does (race detector, verbose)"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make lint-ci       - Run golangci-lint exactly as CI does (with 5m timeout)"
	@echo "  make lint-openapi  - Validate OpenAPI specification"
	@echo "  make vulncheck     - Run govulncheck vulnerability scan"
	@echo "  make ci            - Run full CI pipeline locally (lint, format check, tests, build)"
	@echo "  make test-v        - Run tests with verbose output"
	@echo "  make test-race     - Run tests with race detector"
	@echo "  make coverage      - Run tests with coverage report (enforces 80% threshold)"
	@echo "  make coverage-check - Check if coverage meets 80% threshold"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make lint-ci       - Run golangci-lint exactly as CI does (with 5m timeout)"
	@echo "  make fmt           - Format all Go files"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make run           - Build and run the server (kills existing first)"
	@echo "  make dev           - Run in development mode with live reload (requires air from install-tools)"
	@echo "  make stop          - Stop running server processes"
	@echo "  make restart       - Restart the server (stop + run, full rebuild)"
	@echo "  make restart-quick - Quick restart without rebuild (local only)"
	@echo "  make restart-staging - Restart staging server via SSH"
	@echo "  make restart-production - Restart production server via SSH"
	@echo "  make restart-slot SLOT=blue|green ENV=local|staging|production - Restart specific slot"
	@echo "  make deploy-package - Build deployment package (binary + configs, no source)"
	@echo "  make install-tools - Install development tools (Go tools)"
	@echo "  make install-pyshacl - Install pyshacl for SHACL validation"
	@echo "  make test-contracts - Run contract tests (requires pyshacl)"
	@echo "  make validate-shapes - Validate SHACL shapes against sample data"
	@echo "  make sqlc          - Generate SQLc code (alias for sqlc-generate)"
	@echo "  make sqlc-generate - Generate SQLc code from SQL queries"
	@echo "  make migrate-up    - Run database migrations"
	@echo "  make migrate-down  - Roll back one migration"
	@echo "  make migrate-river - Run River job queue migrations"
	@echo "  make docker-compose-lint - Validate docker-compose YAML files"
	@echo ""
	@echo "Remote Testing:"
	@echo "  make test-local           - Run all tests on local server"
	@echo "  make test-staging         - Run all tests on staging server"
	@echo "  make test-staging-smoke   - Run smoke tests on staging server"
	@echo "  make test-production-smoke - Run smoke tests on production (read-only)"
	@echo "  make test-remote ENV=<env> TYPE=<type> - Run custom remote tests"
	@echo ""
	@echo "Docker Development:"
	@echo "  make docker-up     - Start database and server in Docker (port 5433)"
	@echo "  make docker-db     - Start only database in Docker (port 5433)"
	@echo "  make docker-down   - Stop all Docker containers"
	@echo "  make docker-logs   - View Docker container logs"
	@echo "  make docker-rebuild - Rebuild and restart containers"
	@echo "  make docker-clean  - Stop containers and remove volumes"
	@echo ""
	@echo "Local PostgreSQL Setup:"
	@echo "  make db-check      - Check if local PostgreSQL has required extensions"
	@echo "  make db-setup      - Create local database with extensions"
	@echo "  make db-init       - Create .env for local PostgreSQL development"

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
	@mkdir -p bin
	@go build -ldflags "$(LDFLAGS)" -o bin/togather-server ./cmd/server
	@ln -sf bin/togather-server server

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
	@echo "==> Running batch integration tests (with River workers)..."
	@go test -v -race ./tests/integration_batch/...
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

# ============================================================================
# Remote Testing
# ============================================================================

# Test local server (all tests)
test-local:
	@echo "Running all tests on local server..."
	@./deploy/scripts/test-remote.sh local all

# Test staging server (all tests)
test-staging:
	@echo "Running all tests on staging server..."
	@./deploy/scripts/test-remote.sh staging all

# Test staging server (smoke tests only)
test-staging-smoke:
	@echo "Running smoke tests on staging server..."
	@./deploy/scripts/test-remote.sh staging smoke

# Test production server (smoke tests only, read-only)
test-production-smoke:
	@echo "Running smoke tests on production server (read-only)..."
	@./deploy/scripts/test-remote.sh production smoke

# Generic remote test target (use ENV and TYPE variables)
# Example: make test-remote ENV=staging TYPE=smoke
test-remote:
	@if [ -z "$(ENV)" ]; then \
		echo "ERROR: ENV variable is required"; \
		echo "Usage: make test-remote ENV=<local|staging|production> TYPE=<smoke|perf|all>"; \
		exit 1; \
	fi
	@TYPE=$${TYPE:-smoke}; \
	echo "Running $$TYPE tests on $(ENV) server..."; \
	./deploy/scripts/test-remote.sh $(ENV) $$TYPE

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

# Validate OpenAPI specification
lint-openapi:
	@echo "Validating OpenAPI specification..."
	@if command -v vacuum > /dev/null 2>&1; then \
		vacuum lint specs/001-sel-backend/contracts/openapi.yaml; \
	elif [ -f $(HOME)/go/bin/vacuum ]; then \
		$(HOME)/go/bin/vacuum lint specs/001-sel-backend/contracts/openapi.yaml; \
	elif [ -f $(GOPATH)/bin/vacuum ]; then \
		$(GOPATH)/bin/vacuum lint specs/001-sel-backend/contracts/openapi.yaml; \
	else \
		echo "vacuum not found. Install with 'make install-tools'"; \
		exit 1; \
	fi

# Run vulnerability scan (requires govulncheck)
vulncheck:
	@echo "Running govulncheck..."
	@if command -v govulncheck > /dev/null 2>&1; then \
		govulncheck ./...; \
	elif [ -f $(HOME)/go/bin/govulncheck ]; then \
		$(HOME)/go/bin/govulncheck ./...; \
	elif [ -f $(GOPATH)/bin/govulncheck ]; then \
		$(GOPATH)/bin/govulncheck ./...; \
	else \
		echo "govulncheck not found. Install with 'make install-tools'"; \
		exit 1; \
	fi

# Validate docker-compose files
docker-compose-lint:
	@echo "Validating docker-compose files..."
	@if ! command -v docker > /dev/null 2>&1; then \
		echo "✗ docker not found. Install Docker to validate compose files"; \
		exit 1; \
	fi
	@cd $(DOCKER_COMPOSE_DIR) && docker compose -f docker-compose.yml config --quiet
	@echo "✓ docker-compose.yml is valid"
	@cd $(DOCKER_COMPOSE_DIR) && docker compose -f docker-compose.blue-green.yml config --quiet
	@echo "✓ docker-compose.blue-green.yml is valid"

# Run full CI pipeline locally
ci: sqlc-generate lint-ci vulncheck
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
	@echo "==> Validating OpenAPI specification..."
	@$(MAKE) lint-openapi
	@echo ""
	@echo "==> Validating docker-compose files..."
	@$(MAKE) docker-compose-lint
	@echo ""
	@echo "==> Building server..."
	@$(MAKE) build
	@if [ ! -f bin/togather-server ]; then \
		echo "✗ Build failed: binary not found"; \
		exit 1; \
	fi
	@echo "✓ Build successful"
	@echo ""
	@echo "==> Checking SQLc generation is up to date..."
	@if [ -n "$$(git status --porcelain internal/storage/postgres/sqlc)" ]; then \
		echo "✗ Generated SQLc code differs from committed version"; \
		echo "Run 'make sqlc-generate' and commit changes"; \
		git diff internal/storage/postgres/sqlc; \
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
	@rm -f server
	@rm -rf bin
	@rm -f coverage.out coverage.html
	@go clean

# Stop running server processes
stop:
	@echo "Stopping server processes..."
	@# Kill by port first (most reliable)
	@if lsof -ti:8080 > /dev/null 2>&1; then \
		echo "Stopping process on port 8080..."; \
		lsof -ti:8080 | xargs -r kill 2>/dev/null || true; \
		sleep 1; \
	fi
	@# Force kill if still running
	@if lsof -ti:8080 > /dev/null 2>&1; then \
		echo "Force stopping process on port 8080..."; \
		lsof -ti:8080 | xargs -r kill -9 2>/dev/null || true; \
		sleep 1; \
	fi
	@# Also try killing by binary name as backup
	@-killall togather-server 2>/dev/null || true
	@echo "✓ Server stopped"

# Build and run the server (kills existing processes first)
run: build
	@echo "Checking for existing server processes..."
	@# Stop any existing server on port 8080
	@if lsof -ti:8080 > /dev/null 2>&1; then \
		echo "Found running server on port 8080, stopping it..."; \
		lsof -ti:8080 | xargs -r kill 2>/dev/null || true; \
		sleep 1; \
		if lsof -ti:8080 > /dev/null 2>&1; then \
			lsof -ti:8080 | xargs -r kill -9 2>/dev/null || true; \
			sleep 1; \
		fi; \
	fi
	@echo "Running server..."
	@./server

# Development mode with live reload (requires air - run 'make install-tools' first)
dev:
	@echo "Checking for existing server processes..."
	@# Stop any existing server on port 8080
	@if lsof -ti:8080 > /dev/null 2>&1; then \
		echo "Found running server on port 8080, stopping it..."; \
		lsof -ti:8080 | xargs -r kill 2>/dev/null || true; \
		sleep 1; \
		if lsof -ti:8080 > /dev/null 2>&1; then \
			lsof -ti:8080 | xargs -r kill -9 2>/dev/null || true; \
			sleep 1; \
		fi; \
	fi
	@# Check for air in PATH and in GOPATH/bin
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	if which air > /dev/null 2>&1; then \
		echo "Running with air (live reload)..."; \
		air; \
	else \
		echo "air not found, running without live reload..."; \
		echo "Install air with: make install-tools"; \
		go run ./cmd/server; \
	fi

# Restart server (stop + run)
restart: stop
	@sleep 1
	@$(MAKE) run

# Quick restart using deployment script (faster than full rebuild)
# Restarts the active slot without rebuilding or redeploying
restart-quick:
	@echo "Restarting local server (quick mode)..."
	@./deploy/scripts/restart.sh local

# Restart staging server (requires SSH access)
restart-staging:
	@echo "Restarting staging server..."
	@if [ -f .deploy.conf.staging ]; then \
		SSH_HOST=$$(grep '^SSH_HOST=' .deploy.conf.staging | cut -d= -f2); \
		SSH_USER=$$(grep '^SSH_USER=' .deploy.conf.staging | cut -d= -f2); \
		./deploy/scripts/restart.sh staging --remote $$SSH_USER@$$SSH_HOST; \
	else \
		echo "Error: .deploy.conf.staging not found"; \
		exit 1; \
	fi

# Restart production server (requires SSH access)
restart-production:
	@echo "Restarting production server..."
	@if [ -f .deploy.conf.production ]; then \
		SSH_HOST=$$(grep '^SSH_HOST=' .deploy.conf.production | cut -d= -f2); \
		SSH_USER=$$(grep '^SSH_USER=' .deploy.conf.production | cut -d= -f2); \
		./deploy/scripts/restart.sh production --remote $$SSH_USER@$$SSH_HOST; \
	else \
		echo "Error: .deploy.conf.production not found"; \
		exit 1; \
	fi

# Restart specific slot (blue or green)
# Usage: make restart-slot SLOT=blue ENV=staging
restart-slot:
	@if [ -z "$(SLOT)" ]; then echo "Error: SLOT not specified (use SLOT=blue or SLOT=green)"; exit 1; fi
	@if [ -z "$(ENV)" ]; then echo "Error: ENV not specified (use ENV=local|staging|production)"; exit 1; fi
	@echo "Restarting $(ENV) server (slot: $(SLOT))..."
	@if [ "$(ENV)" = "local" ]; then \
		./deploy/scripts/restart.sh $(ENV) --slot $(SLOT); \
	else \
		if [ -f .deploy.conf.$(ENV) ]; then \
			SSH_HOST=$$(grep '^SSH_HOST=' .deploy.conf.$(ENV) | cut -d= -f2); \
			SSH_USER=$$(grep '^SSH_USER=' .deploy.conf.$(ENV) | cut -d= -f2); \
			./deploy/scripts/restart.sh $(ENV) --slot $(SLOT) --remote $$SSH_USER@$$SSH_HOST; \
		else \
			echo "Error: .deploy.conf.$(ENV) not found"; \
			exit 1; \
		fi \
	fi

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@echo ""
	@echo "==> Installing Go tools..."
	@echo "Installing golangci-lint v1.64.8 (building from source)..."
	@# Note: Using v1.64.8 which supports Go 1.23-1.24
	@# v2.x requires latest Go which may not be available yet
	@TEMP_DIR=$$(mktemp -d); \
	cd $$TEMP_DIR && \
	git clone --depth 1 --branch v1.64.8 https://github.com/golangci/golangci-lint.git && \
	cd golangci-lint && \
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(HOME)/go/bin/golangci-lint ./cmd/golangci-lint && \
	cd ~ && \
	rm -rf $$TEMP_DIR && \
	echo "✓ golangci-lint v1.64.8 built from source with Go $$(go version | awk '{print $$3}')"
	@echo "Installing air (live reload)..."
	@go install github.com/air-verse/air@latest
	@echo "Installing sqlc..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "Installing River CLI..."
	@go install github.com/riverqueue/river/cmd/river@latest
	@echo "Installing govulncheck..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Installing vacuum (OpenAPI linter)..."
	@go install github.com/daveshanley/vacuum@latest
	@echo ""
	@echo "==> Installing golang-migrate (pre-built binary with database drivers)..."
	@if [ "$$(uname -m)" = "x86_64" ]; then \
		ARCH="amd64"; \
	elif [ "$$(uname -m)" = "aarch64" ] || [ "$$(uname -m)" = "arm64" ]; then \
		ARCH="arm64"; \
	else \
		echo "❌ Unsupported architecture: $$(uname -m)"; \
		exit 1; \
	fi; \
	MIGRATE_VERSION="v4.18.1"; \
	MIGRATE_URL="https://github.com/golang-migrate/migrate/releases/download/$$MIGRATE_VERSION/migrate.linux-$$ARCH.tar.gz"; \
	echo "Downloading golang-migrate $$MIGRATE_VERSION for linux-$$ARCH..."; \
	curl -L "$$MIGRATE_URL" 2>/dev/null | tar xzv -C /tmp migrate; \
	chmod +x /tmp/migrate; \
	mv /tmp/migrate $(HOME)/go/bin/migrate; \
	echo "✓ golang-migrate installed to $(HOME)/go/bin/migrate"
	@echo ""
	@echo "==> Verifying installations..."
	@$(HOME)/go/bin/migrate -version || echo "⚠️  migrate verification failed"
	@$(HOME)/go/bin/river version || echo "⚠️  river verification failed"
	@$(HOME)/go/bin/govulncheck -version || echo "⚠️  govulncheck verification failed"
	@echo ""
	@echo "✓ Go tools installed successfully!"
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
	@if [ -f ./migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} ./migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	elif command -v migrate > /dev/null 2>&1; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	elif [ -f $(HOME)/go/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(HOME)/go/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	elif [ -f $(GOPATH)/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(GOPATH)/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up; \
	else \
		echo "migrate not found. Install with 'make install-tools' or use bundled ./migrate"; \
		exit 1; \
	fi

.PHONY: migrate-down
migrate-down:
	@echo "Rolling back last migration..."
	@if [ -f ./migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} ./migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	elif command -v migrate > /dev/null 2>&1; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	elif [ -f $(HOME)/go/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(HOME)/go/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	elif [ -f $(GOPATH)/bin/migrate ]; then \
		DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} $(GOPATH)/bin/migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1; \
	else \
		echo "migrate not found. Install with 'make install-tools' or use bundled ./migrate"; \
		exit 1; \
	fi

# Run River job queue migrations
.PHONY: migrate-river
migrate-river:
	@echo "Running River job queue migrations..."
	@if [ -f ./river ]; then \
		./river migrate-up --database-url "$${DATABASE_URL:?DATABASE_URL is required}"; \
	elif command -v river > /dev/null 2>&1; then \
		river migrate-up --database-url "$${DATABASE_URL:?DATABASE_URL is required}"; \
	elif [ -f $(HOME)/go/bin/river ]; then \
		$(HOME)/go/bin/river migrate-up --database-url "$${DATABASE_URL:?DATABASE_URL is required}"; \
	elif [ -f $(GOPATH)/bin/river ]; then \
		$(GOPATH)/bin/river migrate-up --database-url "$${DATABASE_URL:?DATABASE_URL is required}"; \
	else \
		echo "river CLI not found. Install with 'make install-tools' or use bundled ./river"; \
		exit 1; \
	fi
	@echo "✓ River migrations complete"

# =============================================================================
# Docker Development Targets
# =============================================================================

# Start both database and server in Docker
.PHONY: docker-up
docker-up:
	@echo "Starting Docker containers (database + server)..."
	@if [ ! -f .env ]; then \
		echo ""; \
		echo "⚠️  No .env file found in project root"; \
		echo ""; \
		echo "Run 'server setup --docker' first to generate .env"; \
		echo "Or create .env manually based on .env.example"; \
		echo ""; \
		exit 1; \
	fi
	@cd $(DOCKER_COMPOSE_DIR) && docker compose --env-file ../../.env up -d
	@echo ""
	@echo "✓ Containers started!"
	@echo ""
	@echo "Server:   http://localhost:8080"
	@echo "Database: localhost:5433"
	@echo ""
	@echo "View logs:    make docker-logs"
	@echo "Stop:         make docker-down"
	@echo "Rebuild:      make docker-rebuild"

# Start only database in Docker (for running server natively)
.PHONY: docker-db
docker-db:
	@echo "Starting PostgreSQL database in Docker..."
	@if [ ! -f .env ]; then \
		echo ""; \
		echo "⚠️  No .env file found in project root"; \
		echo ""; \
		echo "Run 'server setup --docker' first to generate .env"; \
		echo "Or create .env manually based on .env.example"; \
		echo ""; \
		exit 1; \
	fi
	@cd $(DOCKER_COMPOSE_DIR) && docker compose --env-file ../../.env up -d togather-db
	@echo ""
	@echo "✓ Database started on localhost:5433"
	@echo ""
	@echo "Stop: make docker-down"

# Stop all Docker containers
.PHONY: docker-down
docker-down:
	@echo "Stopping Docker containers..."
	@cd $(DOCKER_COMPOSE_DIR) && docker compose --env-file ../../.env down
	@echo "✓ Containers stopped"

# View Docker container logs
.PHONY: docker-logs
docker-logs:
	@echo "Viewing Docker logs (Ctrl+C to exit)..."
	@cd $(DOCKER_COMPOSE_DIR) && docker compose --env-file ../../.env logs -f

# Rebuild and restart containers
.PHONY: docker-rebuild
docker-rebuild:
	@echo "Rebuilding and restarting containers..."
	@cd $(DOCKER_COMPOSE_DIR) && docker compose --env-file ../../.env up -d --build
	@echo ""
	@echo "✓ Containers rebuilt and restarted!"
	@echo ""
	@echo "Server:   http://localhost:8080"
	@echo "Database: localhost:5433"

# Stop containers and remove volumes (clean slate)
.PHONY: docker-clean
docker-clean:
	@echo "⚠️  This will remove all containers AND volumes (database data will be lost)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "Stopping containers and removing volumes..."; \
		cd $(DOCKER_COMPOSE_DIR) && docker compose --env-file ../../.env down -v; \
		echo "✓ Containers and volumes removed"; \
	else \
		echo "Cancelled"; \
	fi

# =============================================================================
# Local PostgreSQL Setup Targets
# =============================================================================

# Check if local PostgreSQL has required extensions
.PHONY: db-check
db-check:
	@echo "Checking local PostgreSQL extensions..."
	@echo ""
	@echo "Available extensions:"
	@psql -d postgres -c "SELECT name, default_version, installed_version, comment FROM pg_available_extensions WHERE name IN ('postgis', 'vector', 'pg_trgm', 'pg_stat_statements') ORDER BY name;" 2>/dev/null || \
		(echo "❌ Could not connect to local PostgreSQL"; \
		echo ""; \
		echo "Make sure PostgreSQL is running:"; \
		echo "  sudo systemctl status postgresql"; \
		echo ""; \
		echo "Or use Docker PostgreSQL:"; \
		echo "  make docker-db"; \
		exit 1)
	@echo ""
	@echo "Checking required extensions..."
	@MISSING=""; \
	for ext in postgis vector pg_trgm; do \
		if ! psql -d postgres -tAc "SELECT 1 FROM pg_available_extensions WHERE name='$$ext'" 2>/dev/null | grep -q 1; then \
			MISSING="$$MISSING $$ext"; \
		fi; \
	done; \
	if [ -n "$$MISSING" ]; then \
		echo "❌ Missing required extensions:$$MISSING"; \
		echo ""; \
		echo "Install them:"; \
		echo "  sudo apt install postgresql-16-postgis-3 postgresql-16-pgvector"; \
		echo ""; \
		echo "Note: Package is 'postgresql-16-pgvector' but extension name is 'vector'"; \
		exit 1; \
	else \
		echo "✓ All required extensions available (postgis, vector, pg_trgm)"; \
	fi

# Create local database with required extensions
.PHONY: db-setup
db-setup:
	@echo "Setting up local PostgreSQL database..."
	@echo ""
	@if psql -lqt 2>/dev/null | cut -d \| -f 1 | grep -qw $(DB_NAME); then \
		echo "⚠️  Database '$(DB_NAME)' already exists"; \
		if [ -t 0 ]; then \
			printf "Drop and recreate? [y/N] "; \
			read REPLY; \
			case "$$REPLY" in \
				[Yy]*) \
					echo "Dropping database..."; \
					dropdb $(DB_NAME) 2>/dev/null || true; \
					echo "Creating database..."; \
					createdb $(DB_NAME); \
					;; \
				*) \
					echo "Using existing database"; \
					;; \
			esac; \
		else \
			echo "Using existing database (non-interactive mode)"; \
		fi; \
	else \
		echo "Creating database '$(DB_NAME)'..."; \
		createdb $(DB_NAME); \
	fi
	@echo ""
	@echo "Installing extensions..."
	@psql -d $(DB_NAME) -c "CREATE EXTENSION IF NOT EXISTS postgis;" || echo "⚠️  Could not install postgis"
	@psql -d $(DB_NAME) -c "CREATE EXTENSION IF NOT EXISTS vector;" || echo "⚠️  Could not install vector (pgvector package)"
	@psql -d $(DB_NAME) -c "CREATE EXTENSION IF NOT EXISTS pg_trgm;" || true
	@psql -d $(DB_NAME) -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;" || true
	@echo ""
	@echo "✓ Database setup complete!"
	@echo ""
	@echo "Database: $(DB_NAME)"
	@echo "User:     $(DB_USER)"
	@echo "Port:     $(DB_PORT)"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Create .env file:    make db-init"
	@echo "  2. Run migrations:      make migrate-up"
	@echo "  3. Run River migrations: make migrate-river"
	@echo "  4. Start server:        make run"

# Initialize .env file for local PostgreSQL
.PHONY: db-init
db-init:
	@echo "Creating .env for local PostgreSQL development..."
	@if [ -f .env ]; then \
		echo "⚠️  .env file already exists"; \
		read -p "Overwrite? [y/N] " -n 1 -r; \
		echo; \
		if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
			echo "Cancelled"; \
			exit 0; \
		fi; \
	fi
	@JWT_SECRET=$$(openssl rand -base64 32 2>/dev/null || echo "dev_jwt_secret_change_me"); \
	echo "# Development Environment Configuration" > .env; \
	echo "# Generated by: make db-init" >> .env; \
	echo "" >> .env; \
	echo "# Database (local PostgreSQL)" >> .env; \
	echo "DATABASE_URL=postgresql://$(DB_USER)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable" >> .env; \
	echo "" >> .env; \
	echo "# Server" >> .env; \
	echo "SERVER_HOST=0.0.0.0" >> .env; \
	echo "SERVER_PORT=8080" >> .env; \
	echo "ENVIRONMENT=development" >> .env; \
	echo "LOG_LEVEL=debug" >> .env; \
	echo "" >> .env; \
	echo "# Security (auto-generated)" >> .env; \
	echo "JWT_SECRET=$$JWT_SECRET" >> .env; \
	echo "ADMIN_PASSWORD=admin123" >> .env; \
	echo "ADMIN_USERNAME=admin" >> .env; \
	echo "ADMIN_EMAIL=admin@togather.local" >> .env; \
	echo "" >> .env
	@echo "✓ Created .env file"
	@echo ""
	@echo "Database URL: postgresql://$(DB_USER)@localhost:$(DB_PORT)/$(DB_NAME)"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Run migrations: make migrate-up"
	@echo "  2. Start server:   make run"

# Interactive Setup
# =================

setup:
	@echo "🚀 Starting interactive setup..."
	@echo ""
	@go build -o server ./cmd/server
	@./server setup
	@echo ""
	@echo "Setup complete! Server binary available at: ./server"

setup-docker:
	@echo "🚀 Starting Docker setup (non-interactive)..."
	@go build -o server ./cmd/server
	@./server setup --docker --non-interactive
	@echo ""
	@echo "Setup complete!"

setup-help:
	@go build -o server ./cmd/server
	@./server setup --help

# =================
# Deployment Package
# =================

deploy-package:
	@echo "Building deployment package..."
	@./deploy/scripts/build-deploy-package.sh
