#!/usr/bin/env bash
#
# Setup Review Queue Test Fixtures
#
# This script generates and ingests review queue test events using the
# Go-based fixture generation and ingestion commands.
#
# Usage:
#   tests/e2e/setup_fixtures.sh [count] [seed]
#
# Arguments:
#   count  - Number of review queue events to generate (default: 5)
#   seed   - Random seed for reproducibility (default: random)
#
# Environment:
#   DATABASE_URL - Database connection string (required)
#   BASE_URL     - Server URL for ingestion (default: http://localhost:8080)
#
# Returns:
#   0 on success, 1 on failure

set -e  # Exit on error
set -o pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Configuration
COUNT="${1:-5}"
SEED="${2:-}"
FIXTURE_FILE="${TMPDIR:-/tmp}/review-queue-fixtures-$$.json"
BASE_URL="${BASE_URL:-http://localhost:8080}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}✓${NC} $*"
}

log_error() {
    echo -e "${RED}✗${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $*"
}

# Check prerequisites
check_prerequisites() {
    if [ -z "$DATABASE_URL" ]; then
        log_error "DATABASE_URL environment variable is required"
        log_info "Run: source .env"
        exit 1
    fi

    # Check if server binary exists (try both locations)
    if [ -f "$PROJECT_ROOT/bin/togather-server" ]; then
        SERVER_BIN="$PROJECT_ROOT/bin/togather-server"
    elif [ -f "$PROJECT_ROOT/server" ]; then
        SERVER_BIN="$PROJECT_ROOT/server"
    else
        log_error "Server binary not found. Run: make build"
        exit 1
    fi
}

# Generate review queue fixtures
generate_fixtures() {
    log_info "Generating $COUNT review queue fixtures..."
    
    local gen_cmd="\"$SERVER_BIN\" generate \"$FIXTURE_FILE\" --count $COUNT --review-queue"
    
    if [ -n "$SEED" ]; then
        gen_cmd="$gen_cmd --seed $SEED"
    fi
    
    if ! eval "$gen_cmd"; then
        log_error "Failed to generate fixtures"
        return 1
    fi
    
    log_info "Generated fixtures to: $FIXTURE_FILE"
    return 0
}

# Ingest fixtures into database
ingest_fixtures() {
    log_info "Ingesting fixtures into database..."
    
    if ! "$SERVER_BIN" ingest "$FIXTURE_FILE"; then
        log_error "Failed to ingest fixtures"
        return 1
    fi
    
    log_info "Ingested $COUNT fixtures successfully"
    return 0
}

# Cleanup temporary files
cleanup() {
    if [ -f "$FIXTURE_FILE" ]; then
        rm -f "$FIXTURE_FILE"
        log_info "Cleaned up temporary fixture file"
    fi
}

# Main execution
main() {
    echo "========================================"
    echo "Setup Review Queue Test Fixtures"
    echo "========================================"
    echo ""
    
    # Register cleanup trap
    trap cleanup EXIT
    
    # Run setup steps
    check_prerequisites
    generate_fixtures
    ingest_fixtures
    
    echo ""
    echo "========================================"
    log_info "Review queue fixtures setup complete"
    echo "========================================"
    echo ""
    echo "Fixture file: $FIXTURE_FILE"
    echo "Events generated: $COUNT"
    echo ""
    echo "Next steps:"
    echo "  - Run tests: uvx --from playwright --with playwright python tests/e2e/test_review_queue.py"
    echo "  - View review queue: open $BASE_URL/admin/review-queue"
    echo "  - Cleanup: tests/e2e/cleanup_fixtures.sh"
    echo ""
    
    return 0
}

# Run main
main "$@"
