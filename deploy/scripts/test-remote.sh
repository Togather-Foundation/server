#!/usr/bin/env bash
#
# test-remote.sh - Remote Server Testing Wrapper
#
# Unified interface for running tests against local, staging, or production servers.
# Supports smoke tests, performance tests, and custom test scripts.
#
# Usage:
#   ./test-remote.sh <environment> [test-type]
#   
#   environment:  local, staging, production
#   test-type:    smoke (default), perf, all
#
# Examples:
#   ./test-remote.sh local              # Run smoke tests on local
#   ./test-remote.sh staging smoke      # Run smoke tests on staging
#   ./test-remote.sh production smoke   # Run smoke tests on production (read-only)
#   ./test-remote.sh staging perf       # Run performance tests on staging
#   ./test-remote.sh staging all        # Run all tests on staging
#
# Exit Codes:
#   0   All tests passed
#   1   One or more tests failed
#   2   Invalid arguments or configuration

set -euo pipefail

# Get script directory (save before config.sh overwrites it)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$SCRIPT_DIR"  # Preserve for performance-test.sh path
TESTING_DIR="${SCRIPT_DIR}/../testing"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ============================================================================
# Helper Functions
# ============================================================================

log() {
    local level="$1"
    shift
    local message="$*"
    
    case "$level" in
        ERROR)   echo -e "${RED}[ERROR]${NC} $message" >&2 ;;
        WARN)    echo -e "${YELLOW}[WARN]${NC} $message" ;;
        SUCCESS) echo -e "${GREEN}[SUCCESS]${NC} $message" ;;
        INFO)    echo -e "${BLUE}[INFO]${NC} $message" ;;
        HEADER)  echo -e "${CYAN}=== $message ===${NC}" ;;
        *)       echo "[${level}] $message" ;;
    esac
}

usage() {
    cat << EOF
Usage: $0 <environment> [test-type]

Arguments:
  environment    Target environment: local, staging, production
  test-type      Type of tests to run: smoke (default), perf, all

Examples:
  $0 local              # Run smoke tests on local server
  $0 staging smoke      # Run smoke tests on staging
  $0 production smoke   # Run smoke tests on production (read-only)
  $0 staging perf       # Run performance tests on staging
  $0 staging all        # Run all tests on staging

Exit Codes:
  0   All tests passed
  1   One or more tests failed
  2   Invalid arguments or configuration
EOF
    exit 2
}

# ============================================================================
# Main
# ============================================================================

main() {
    # Parse arguments
    if [ $# -lt 1 ]; then
        log "ERROR" "Missing required argument: environment"
        usage
    fi
    
    local environment="$1"
    local test_type="${2:-smoke}"
    
    # Validate environment
    case "$environment" in
        local|staging|production)
            ;;
        *)
            log "ERROR" "Invalid environment: $environment"
            usage
            ;;
    esac
    
    # Validate test type
    case "$test_type" in
        smoke|perf|all)
            ;;
        *)
            log "ERROR" "Invalid test type: $test_type"
            usage
            ;;
    esac
    
    log "HEADER" "Remote Testing: $environment ($test_type)"
    echo ""
    
    # Load environment config
    # shellcheck disable=SC1091
    if ! source "${TESTING_DIR}/config.sh" "$environment"; then
        log "ERROR" "Failed to load config for environment: $environment"
        exit 2
    fi
    
    echo ""
    
    # Production safety check
    if [ "$environment" = "production" ]; then
        if [ "$test_type" = "perf" ] || [ "$test_type" = "all" ]; then
            log "ERROR" "Performance tests are not allowed in production"
            log "ERROR" "Production only supports smoke tests (read-only)"
            exit 2
        fi
        
        log "WARN" "Testing production environment (read-only mode)"
        log "WARN" "No destructive operations will be performed"
        echo ""
    fi
    
    # Track overall status
    local tests_passed=true

    # Wait for service readiness before tests
    local wait_script="${SCRIPTS_DIR}/wait-for-health.sh"
    local wait_timeout="${WAIT_TIMEOUT:-60}"
    if [ "${SKIP_WAIT_FOR_HEALTH:-false}" != "true" ]; then
        if [ -x "${wait_script}" ]; then
            log "INFO" "Waiting for health before running tests (timeout: ${wait_timeout}s)"
            if ! "${wait_script}" --base-url "${BASE_URL}" --timeout "${wait_timeout}"; then
                log "ERROR" "Health wait timed out for ${BASE_URL}"
                exit 1
            fi
            echo ""
        else
            log "WARN" "Health wait script not found; skipping pre-test wait"
        fi
    fi
    
    # Run tests based on type
    case "$test_type" in
        smoke)
            log "INFO" "Running smoke tests..."
            echo ""
            if "${TESTING_DIR}/smoke-tests.sh" "$environment"; then
                log "SUCCESS" "Smoke tests passed"
            else
                log "ERROR" "Smoke tests failed"
                tests_passed=false
            fi
            ;;
            
        perf)
            log "INFO" "Running performance tests..."
            echo ""
            
            if [ "${ALLOW_LOAD_TESTING:-false}" != "true" ]; then
                log "ERROR" "Load testing is not allowed for environment: $environment"
                log "ERROR" "Set ALLOW_LOAD_TESTING=true in config if intentional"
                exit 2
            fi
            
            # Check if performance test script exists
            local perf_script="${SCRIPTS_DIR}/performance-test.sh"
            if [ ! -f "$perf_script" ]; then
                log "WARN" "Performance test script not found: $perf_script"
                log "INFO" "Skipping performance tests"
            else
                if "$perf_script" --profile light --url "$BASE_URL"; then
                    log "SUCCESS" "Performance tests passed"
                else
                    log "ERROR" "Performance tests failed"
                    tests_passed=false
                fi
            fi
            ;;
            
        all)
            log "INFO" "Running all tests..."
            echo ""
            
            # Smoke tests
            log "INFO" "Step 1/2: Running smoke tests..."
            echo ""
            if "${TESTING_DIR}/smoke-tests.sh" "$environment"; then
                log "SUCCESS" "Smoke tests passed"
            else
                log "ERROR" "Smoke tests failed"
                tests_passed=false
            fi
            
            echo ""
            echo ""
            
            # Performance tests (if allowed)
            if [ "${ALLOW_LOAD_TESTING:-false}" = "true" ]; then
                log "INFO" "Step 2/2: Running performance tests..."
                echo ""
                
                local perf_script="${SCRIPTS_DIR}/performance-test.sh"
                if [ ! -f "$perf_script" ]; then
                    log "WARN" "Performance test script not found: $perf_script"
                    log "INFO" "Skipping performance tests"
                else
                    if "$perf_script" --profile light --url "$BASE_URL"; then
                        log "SUCCESS" "Performance tests passed"
                    else
                        log "ERROR" "Performance tests failed"
                        tests_passed=false
                    fi
                fi
            else
                log "INFO" "Step 2/2: Skipping performance tests (not allowed for $environment)"
            fi
            ;;
    esac
    
    echo ""
    echo ""
    log "HEADER" "Testing Complete"
    
    if [ "$tests_passed" = true ]; then
        log "SUCCESS" "All tests passed for $environment"
        exit 0
    else
        log "ERROR" "Some tests failed for $environment"
        exit 1
    fi
}

# Run main
main "$@"
