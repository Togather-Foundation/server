#!/usr/bin/env bash
#
# Cleanup Review Queue Test Fixtures
#
# This script removes review queue test events created by setup_fixtures.sh.
# It deletes events with specific ULID patterns used by the test fixtures.
#
# Usage:
#   tests/e2e/cleanup_fixtures.sh
#
# Environment:
#   DATABASE_URL - Database connection string (required)
#
# Returns:
#   0 on success, 1 on failure

set -e  # Exit on error
set -o pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

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

    # Check if psql is available
    if ! command -v psql &> /dev/null; then
        log_error "psql command not found. Install PostgreSQL client tools."
        exit 1
    fi
}

# Cleanup test fixtures from database
cleanup_fixtures() {
    log_info "Cleaning up review queue test fixtures..."
    
    # Delete review queue entries for test events
    # Note: We use ULID patterns that are unlikely to match real events
    # Test events from fixtures.go should use recognizable patterns
    
    local sql="
    BEGIN;
    
    -- Delete review queue entries for events with test patterns
    DELETE FROM event_review_queue
    WHERE event_id IN (
        SELECT id FROM events 
        WHERE ulid LIKE 'TESTRQ%' 
           OR name LIKE '%(Late Night)%'
           OR name LIKE '%(Online)%' AND source_id LIKE '%evt-novenue-%'
    );
    
    -- Delete event occurrences
    DELETE FROM event_occurrences
    WHERE event_id IN (
        SELECT id FROM events 
        WHERE ulid LIKE 'TESTRQ%'
           OR name LIKE '%(Late Night)%'
           OR name LIKE '%(Online)%' AND source_id LIKE '%evt-novenue-%'
    );
    
    -- Delete events
    DELETE FROM events
    WHERE ulid LIKE 'TESTRQ%'
       OR (name LIKE '%(Late Night)%')
       OR (name LIKE '%(Online)%' AND source_id LIKE '%evt-novenue-%');
    
    COMMIT;
    
    SELECT 'Cleanup completed' as result;
    "
    
    if echo "$sql" | psql "$DATABASE_URL" &> /dev/null; then
        log_info "Cleaned up test fixtures successfully"
        return 0
    else
        log_error "Failed to cleanup fixtures"
        return 1
    fi
}

# Main execution
main() {
    echo "========================================"
    echo "Cleanup Review Queue Test Fixtures"
    echo "========================================"
    echo ""
    
    # Run cleanup steps
    check_prerequisites
    cleanup_fixtures
    
    echo ""
    echo "========================================"
    log_info "Cleanup complete"
    echo "========================================"
    echo ""
    
    return 0
}

# Run main
main "$@"
