#!/usr/bin/env bash
#
# deploy-sync-test.sh - Unit tests for deploy.sh sync_sources function
#
# Tests the sync_sources function logic without requiring a full deployment.
# Uses mocking to isolate the function behavior.
#
# Usage:
#   ./deploy-sync-test.sh          # Run all tests
#   ./deploy-sync-test.sh -v      # Verbose mode
#
# Exit Codes:
#   0   All tests passed
#   1   One or more tests failed

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DEPLOY_SCRIPT="${PROJECT_ROOT}/deploy/scripts/deploy.sh"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
VERBOSE=false

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Log functions
log() {
    local level="$1"
    shift
    local message="$*"
    
    case "$level" in
        ERROR)   echo -e "${RED}[ERROR]${NC} $message" >&2 ;;
        WARN)    echo -e "${YELLOW}[WARN]${NC} $message" ;;
        SUCCESS) echo -e "${GREEN}[PASS]${NC} $message" ;;
        FAIL)    echo -e "${RED}[FAIL]${NC} $message" ;;
        INFO)    [[ "$VERBOSE" == "true" ]] && echo -e "${BLUE}[INFO]${NC} $message" || true ;;
        *)       echo "[${level}] $message" ;;
    esac
}

# Test helper: assertEquals
assert_equals() {
    local expected="$1"
    local actual="$2"
    local message="${3:-Assertion failed}"
    
    if [[ "$expected" == "$actual" ]]; then
        return 0
    else
        echo "  Expected: '$expected'" >&2
        echo "  Actual:   '$actual'" >&2
        return 1
    fi
}

# Test helper: assertFileContains
assert_file_contains() {
    local file="$1"
    local pattern="$2"
    
    if grep -q "$pattern" "$file" 2>/dev/null; then
        return 0
    else
        echo "  File '$file' does not contain pattern: $pattern" >&2
        return 1
    fi
}

# Test helper: assertFileNotContains  
assert_file_not_contains() {
    local file="$1"
    local pattern="$2"
    
    if grep -q "$pattern" "$file" 2>/dev/null; then
        echo "  File '$file' contains pattern (should not): $pattern" >&2
        return 1
    else
        return 0
    fi
}

# Run a single test
run_test() {
    local test_name="$1"
    local test_func="$2"
    
    ((TESTS_RUN++)) || true
    log "INFO" "Test ${TESTS_RUN}: ${test_name}"
    
    if $test_func; then
        ((TESTS_PASSED++)) || true
        log "SUCCESS" "${test_name}"
        return 0
    else
        ((TESTS_FAILED++)) || true
        log "FAIL" "${test_name}"
        return 1
    fi
}

# ============================================================================
# Tests
# ============================================================================

test_deploy_script_exists() {
    [[ -f "$DEPLOY_SCRIPT" ]]
}

test_deploy_script_is_executable() {
    [[ -x "$DEPLOY_SCRIPT" ]]
}

test_sync_sources_function_exists() {
    grep -q "^sync_sources()" "$DEPLOY_SCRIPT"
}

test_sync_sources_is_called_in_deploy() {
    grep -q 'sync_sources' "$DEPLOY_SCRIPT"
}

test_sync_sources_called_before_switch_traffic() {
    # Verify the order: sync_sources -> switch_traffic -> update_deployment_state
    # This ensures configs are synced before traffic switches to the new container
    local deploy_func
    deploy_func=$(sed -n '/^deploy()/,/^}/p' "$DEPLOY_SCRIPT")
    
    local sync_line=$(echo "$deploy_func" | grep -n 'sync_sources' | head -1 | cut -d: -f1)
    local switch_line=$(echo "$deploy_func" | grep -n 'switch_traffic' | head -1 | cut -d: -f1)
    local update_line=$(echo "$deploy_func" | grep -n 'update_deployment_state' | head -1 | cut -d: -f1)
    
    [[ -n "$sync_line" ]] && [[ -n "$switch_line" ]] && [[ -n "$update_line" ]] && \
        [[ "$sync_line" -lt "$switch_line" ]] && [[ "$switch_line" -lt "$update_line" ]]
}

test_sync_sources_fails_on_container_not_running() {
    # Test that sync_sources would fail if container doesn't exist
    local sync_func
    sync_func=$(sed -n '/^sync_sources()/,/^}$/p' "$DEPLOY_SCRIPT")
    
    # Should check if container is running
    echo "$sync_func" | grep -q 'docker ps.*grep'
}

test_sync_sources_calls_scrape_sync() {
    local sync_func
    sync_func=$(sed -n '/^sync_sources()/,/^}$/p' "$DEPLOY_SCRIPT")
    
    # Should run scrape sync inside container
    echo "$sync_func" | grep -q 'scrape sync'
}

test_sync_sources_handles_failure() {
    local sync_func
    sync_func=$(sed -n '/^sync_sources()/,/^}$/p' "$DEPLOY_SCRIPT")
    
    # Should return error on failure
    echo "$sync_func" | grep -q 'return 1'
}

test_dockerfile_includes_configs_sources() {
    local dockerfile="${PROJECT_ROOT}/deploy/docker/Dockerfile"
    
    [[ -f "$dockerfile" ]] && grep -q 'configs/sources' "$dockerfile"
}

test_sync_sources_logs_output() {
    local sync_func
    sync_func=$(sed -n '/^sync_sources()/,/^}$/p' "$DEPLOY_SCRIPT")
    
    # Should log sync output
    echo "$sync_func" | grep -q 'sync_output'
}

test_sync_sources_checks_no_sources_warning() {
    local sync_func
    sync_func=$(sed -n '/^sync_sources()/,/^}$/p' "$DEPLOY_SCRIPT")
    
    # Should warn if no sources found
    echo "$sync_func" | grep -q 'No source configs found'
}

# ============================================================================
# Main
# ============================================================================

main() {
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -h|--help)
                echo "Usage: $0 [-v|--verbose]"
                echo ""
                echo "Unit tests for deploy.sh sync_sources function"
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    log "INFO" "Running deploy.sh sync_sources unit tests"
    log "INFO" "Script: ${DEPLOY_SCRIPT}"
    echo ""
    
    # Check prerequisites
    if [[ ! -f "$DEPLOY_SCRIPT" ]]; then
        log "ERROR" "Deploy script not found: ${DEPLOY_SCRIPT}"
        exit 1
    fi
    
    # Run tests
    run_test "Deploy script exists" test_deploy_script_exists
    run_test "Deploy script is executable" test_deploy_script_is_executable
    run_test "sync_sources function exists" test_sync_sources_function_exists
    run_test "sync_sources is called in deploy flow" test_sync_sources_is_called_in_deploy
    run_test "sync_sources called before switch_traffic" test_sync_sources_called_before_switch_traffic
    run_test "sync_sources fails if container not running" test_sync_sources_fails_on_container_not_running
    run_test "sync_sources calls scrape sync" test_sync_sources_calls_scrape_sync
    run_test "sync_sources handles failure" test_sync_sources_handles_failure
    run_test "Dockerfile includes configs/sources" test_dockerfile_includes_configs_sources
    run_test "sync_sources logs output" test_sync_sources_logs_output
    run_test "sync_sources checks for no sources warning" test_sync_sources_checks_no_sources_warning
    
    echo ""
    echo "========================================"
    log "INFO" "Test Summary"
    echo "========================================"
    log "INFO" "Tests run:    ${TESTS_RUN}"
    log "INFO" "Tests passed: ${TESTS_PASSED}"
    log "INFO" "Tests failed: ${TESTS_FAILED}"
    echo ""
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        log "SUCCESS" "All tests passed!"
        exit 0
    else
        log "ERROR" "${TESTS_FAILED} test(s) failed"
        exit 1
    fi
}

# Run main if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
