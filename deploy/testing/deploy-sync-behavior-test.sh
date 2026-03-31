#!/usr/bin/env bash
#
# deploy-sync-behavior-test.sh - Behavior-level tests for sync_sources()
#
# Tests the runtime behavior of the sync_sources function by mocking
# the docker command and verifying exit codes and log output.
#
# Usage:
#   ./deploy-sync-behavior-test.sh          # Run all tests
#   ./deploy-sync-behavior-test.sh -v      # Verbose mode
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
# Test Harness: Extract and execute sync_sources with mocked docker
# ============================================================================

# Extract the sync_sources function body from deploy.sh
SYNC_SOURCES_FUNC=$(sed -n '/^sync_sources()/,/^}/p' "$DEPLOY_SCRIPT")

run_sync_sources_test() {
    local env="$1"
    local slot="${2:-blue}"
    local docker_ps_output="${3:-togather-server-${slot}}"
    local docker_exec_output="$4"
    local docker_exec_exit="${5:-0}"
    local docker_ps_exit="${6:-0}"

    # Create a temporary script that contains:
    # 1. Our mock log function
    # 2. Our mock docker function
    # 3. The sync_sources function
    # 4. A call to sync_sources
    local tmp_script
    tmp_script=$(mktemp /tmp/sync-test-XXXXXX.sh)

    cat > "$tmp_script" <<'TESTSCRIPT'
#!/usr/bin/env bash
set -euo pipefail

# Log function - captures output to a file for inspection
LOG_FILE="${LOG_OUTPUT_FILE:-/dev/stderr}"
log() {
    local level="$1"
    shift
    local message="$*"
    echo "[${level}] ${message}" >> "$LOG_FILE"
}

# Docker mock - behavior controlled by env vars
docker() {
    local cmd="$1"
    shift
    case "$cmd" in
        ps)
            echo "${DOCKER_PS_OUTPUT:-}"
            return "${DOCKER_PS_EXIT:-0}"
            ;;
        exec)
            # Skip the container name and command args, output the mock response
            echo "${DOCKER_EXEC_OUTPUT:-}"
            return "${DOCKER_EXEC_EXIT:-0}"
            ;;
        *)
            echo "docker mock: unknown command $cmd" >&2
            return 1
            ;;
    esac
}
TESTSCRIPT

    # Append the sync_sources function
    echo "" >> "$tmp_script"
    echo "$SYNC_SOURCES_FUNC" >> "$tmp_script"

    # Append the call to sync_sources
    echo "" >> "$tmp_script"
    echo "sync_sources '${env}' '${slot}'" >> "$tmp_script"
    echo "exit \$?" >> "$tmp_script"

    chmod +x "$tmp_script"

    # Create a temp file to capture log output
    local log_file
    log_file=$(mktemp /tmp/sync-log-XXXXXX.txt)

    # Run the test script
    local result=0
    LOG_OUTPUT_FILE="$log_file" \
    DOCKER_PS_OUTPUT="$docker_ps_output" \
    DOCKER_PS_EXIT="$docker_ps_exit" \
    DOCKER_EXEC_OUTPUT="$docker_exec_output" \
    DOCKER_EXEC_EXIT="$docker_exec_exit" \
    bash "$tmp_script" >> "$log_file" 2>&1 || result=$?

    # Export log content for test assertions
    TEST_LOG_CONTENT=$(cat "$log_file")

    # Cleanup
    rm -f "$tmp_script" "$log_file"

    return $result
}

# ============================================================================
# Tests
# ============================================================================

# Test 1: Success case - exit 0 in staging
test_success_staging() {
    local result=0
    run_sync_sources_test "staging" "blue" "togather-server-blue" \
        '{"sources_found": 1, "created": 1, "updated": 0, "total": 1, "warnings": 0, "errors": 0}' 0 || result=$?
    [[ $result -eq 0 ]]
}

# Test 2: Success case - exit 0 in production
test_success_production() {
    local result=0
    run_sync_sources_test "production" "green" "togather-server-green" \
        '{"sources_found": 2, "created": 1, "updated": 1, "total": 2, "warnings": 0, "errors": 0}' 0 || result=$?
    [[ $result -eq 0 ]]
}

# Test 3: Success case - exit 0 in development
test_success_development() {
    local result=0
    run_sync_sources_test "development" "blue" "togather-server-blue" \
        '{"sources_found": 1, "created": 1, "updated": 0, "total": 1, "warnings": 0, "errors": 0}' 0 || result=$?
    [[ $result -eq 0 ]]
}

# Test 4: Warnings cause failure in staging
test_warnings_fail_staging() {
    local result=0
    run_sync_sources_test "staging" "blue" "togather-server-blue" \
        '{"sources_found": 1, "created": 0, "updated": 1, "total": 1, "warnings": 1, "errors": 0}' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 5: Warnings cause failure in production
test_warnings_fail_production() {
    local result=0
    run_sync_sources_test "production" "green" "togather-server-green" \
        '{"sources_found": 1, "created": 0, "updated": 1, "total": 1, "warnings": 2, "errors": 0}' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 6: Warnings only log in development (exit 0)
test_warnings_warn_development() {
    local result=0
    run_sync_sources_test "development" "blue" "togather-server-blue" \
        '{"sources_found": 1, "created": 0, "updated": 1, "total": 1, "warnings": 1, "errors": 0}' 0 || result=$?
    [[ $result -eq 0 ]]
}

# Test 7: Total 0 causes failure in staging
test_total_zero_fail_staging() {
    local result=0
    run_sync_sources_test "staging" "blue" "togather-server-blue" \
        '{"sources_found": 0, "created": 0, "updated": 0, "total": 0, "warnings": 0, "errors": 0}' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 8: Total 0 causes failure in production
test_total_zero_fail_production() {
    local result=0
    run_sync_sources_test "production" "green" "togather-server-green" \
        '{"sources_found": 0, "created": 0, "updated": 0, "total": 0, "warnings": 0, "errors": 0}' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 9: Total 0 only warns in development (exit 0)
test_total_zero_warn_development() {
    local result=0
    run_sync_sources_test "development" "blue" "togather-server-blue" \
        '{"sources_found": 0, "created": 0, "updated": 0, "total": 0, "warnings": 0, "errors": 0}' 0 || result=$?
    [[ $result -eq 0 ]]
}

# Test 10: Malformed JSON causes failure in staging
test_malformed_json_fail_staging() {
    local result=0
    run_sync_sources_test "staging" "blue" "togather-server-blue" \
        '{"sources_found": 1, oops broken' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 11: Malformed JSON causes failure in production
test_malformed_json_fail_production() {
    local result=0
    run_sync_sources_test "production" "green" "togather-server-green" \
        '{"sources_found": 1, oops broken' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 12: Malformed JSON causes failure in development (total=-1 sentinel)
test_malformed_json_fail_development() {
    local result=0
    run_sync_sources_test "development" "blue" "togather-server-blue" \
        '{"sources_found": 1, oops broken' 0 || result=$?
    # Malformed JSON results in total=-1 which is <= 0, so even dev should handle it
    # Looking at the code: total=-1 triggers the "non-numeric or sentinel" check
    # In dev, it only warns, so exit 0
    [[ $result -eq 0 ]]
}

# Test 13: Errors cause failure in staging
test_errors_fail_staging() {
    local result=0
    run_sync_sources_test "staging" "blue" "togather-server-blue" \
        '{"sources_found": 1, "created": 0, "updated": 0, "total": 1, "warnings": 0, "errors": 1}' 0 || result=$?
    [[ $result -eq 1 ]]
}

# Test 14: Errors only warn in development
test_errors_warn_development() {
    local result=0
    run_sync_sources_test "development" "blue" "togather-server-blue" \
        '{"sources_found": 1, "created": 0, "updated": 1, "total": 1, "warnings": 0, "errors": 1}' 0 || result=$?
    [[ $result -eq 0 ]]
}

# Test 15: Container not running causes failure
test_container_not_running() {
    local result=0
    run_sync_sources_test "staging" "blue" "" "" 0 1 || result=$?
    [[ $result -eq 1 ]]
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
                echo "Behavior-level tests for deploy.sh sync_sources function"
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    log "INFO" "Running sync_sources behavior tests"
    log "INFO" "Deploy script: ${DEPLOY_SCRIPT}"
    echo ""

    # Check prerequisites
    if [[ ! -f "$DEPLOY_SCRIPT" ]]; then
        log "ERROR" "Deploy script not found: ${DEPLOY_SCRIPT}"
        exit 1
    fi

    if ! command -v jq &> /dev/null; then
        log "ERROR" "jq is required but not installed"
        exit 1
    fi

    # Run tests
    run_test "Success in staging (exit 0)" test_success_staging
    run_test "Success in production (exit 0)" test_success_production
    run_test "Success in development (exit 0)" test_success_development
    run_test "Warnings fail in staging (exit 1)" test_warnings_fail_staging
    run_test "Warnings fail in production (exit 1)" test_warnings_fail_production
    run_test "Warnings warn in development (exit 0)" test_warnings_warn_development
    run_test "Total 0 fails in staging (exit 1)" test_total_zero_fail_staging
    run_test "Total 0 fails in production (exit 1)" test_total_zero_fail_production
    run_test "Total 0 warns in development (exit 0)" test_total_zero_warn_development
    run_test "Malformed JSON fails in staging (exit 1)" test_malformed_json_fail_staging
    run_test "Malformed JSON fails in production (exit 1)" test_malformed_json_fail_production
    run_test "Malformed JSON warns in development (exit 0)" test_malformed_json_fail_development
    run_test "Errors fail in staging (exit 1)" test_errors_fail_staging
    run_test "Errors warn in development (exit 0)" test_errors_warn_development
    run_test "Container not running fails (exit 1)" test_container_not_running

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
