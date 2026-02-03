#!/usr/bin/env bash
#
# smoke_test.sh - Deployment Smoke Tests for Togather Server
#
# Quick validation tests to run after deployment to verify basic functionality.
# These tests should complete in under 30 seconds and catch critical issues.
#
# Usage:
#   ./smoke_test.sh [BASE_URL]
#
# Arguments:
#   BASE_URL    Base URL of deployed server (default: http://localhost:8080)
#
# Exit Codes:
#   0   All smoke tests passed
#   1   One or more smoke tests failed
#
# Reference: specs/001-deployment-infrastructure/tasks.md T080

set -euo pipefail

# Configuration
BASE_URL="${1:-http://localhost:8080}"
TIMEOUT=5
RETRY_COUNT=3
RETRY_DELAY=2

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

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
        SUCCESS) echo -e "${GREEN}[PASS]${NC} $message" ;;
        FAIL)    echo -e "${RED}[FAIL]${NC} $message" ;;
        INFO)    echo -e "${BLUE}[INFO]${NC} $message" ;;
        *)       echo "[${level}] $message" ;;
    esac
}

# HTTP request with retries
http_get() {
    local url="$1"
    local expected_status="${2:-200}"
    local retry_count=0
    
    while [[ $retry_count -lt $RETRY_COUNT ]]; do
        local response=$(curl -s -w "\n%{http_code}" --max-time "$TIMEOUT" "$url" 2>/dev/null || echo -e "\n000")
        local body=$(echo "$response" | head -n -1)
        local status=$(echo "$response" | tail -n 1)
        
        if [[ "$status" == "$expected_status" ]]; then
            echo "$body"
            return 0
        fi
        
        ((retry_count++)) || true
        if [[ $retry_count -lt $RETRY_COUNT ]]; then
            sleep "$RETRY_DELAY"
        fi
    done
    
    return 1
}

# ============================================================================
# Smoke Tests
# ============================================================================

test_health_endpoint() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing health endpoint: GET ${BASE_URL}/health"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate JSON structure
        if echo "$response" | jq -e '.status' >/dev/null 2>&1; then
            local status=$(echo "$response" | jq -r '.status')
            if [[ "$status" == "pass" || "$status" == "warn" ]]; then
                log "SUCCESS" "Health check passed (status: ${status})"
                ((TESTS_PASSED++)) || true
                return 0
            else
                log "FAIL" "Health check returned unhealthy status: ${status}"
                ((TESTS_FAILED++)) || true
                return 1
            fi
        else
            log "FAIL" "Health check response is not valid JSON"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Health endpoint not responding"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_version_endpoint() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing version endpoint: GET ${BASE_URL}/version"
    
    local response=$(http_get "${BASE_URL}/version" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate JSON structure
        if echo "$response" | jq -e '.version' >/dev/null 2>&1; then
            local version=$(echo "$response" | jq -r '.version')
            local git_commit=$(echo "$response" | jq -r '.git_commit // "unknown"')
            log "SUCCESS" "Version endpoint passed (version: ${version}, commit: ${git_commit})"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Version response missing required fields"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Version endpoint not responding"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_database_connectivity() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing database connectivity via health check"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check database check status
        local db_status=$(echo "$response" | jq -r '.checks.database.status // "unknown"')
        if [[ "$db_status" == "pass" ]]; then
            log "SUCCESS" "Database connectivity verified"
            ((TESTS_PASSED++)) || true
            return 0
        elif [[ "$db_status" == "warn" ]]; then
            log "WARN" "Database check returned warning status"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Database connectivity check failed (status: ${db_status})"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Could not retrieve database status"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_migration_status() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing migration status via health check"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check migrations check status
        local migrations_status=$(echo "$response" | jq -r '.checks.migrations.status // "unknown"')
        if [[ "$migrations_status" == "pass" ]]; then
            log "SUCCESS" "Database migrations verified"
            ((TESTS_PASSED++)) || true
            return 0
        elif [[ "$migrations_status" == "warn" ]]; then
            log "WARN" "Migration check returned warning status"
            local observed_version=$(echo "$response" | jq -r '.checks.migrations.observed_value // "unknown"')
            log "INFO" "Migration version: ${observed_version}"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Migration status check failed (status: ${migrations_status})"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Could not retrieve migration status"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_http_endpoint_check() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing HTTP endpoint health check"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check http_endpoint check status
        local http_status=$(echo "$response" | jq -r '.checks.http_endpoint.status // "unknown"')
        if [[ "$http_status" == "pass" ]]; then
            log "SUCCESS" "HTTP endpoint health check verified"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "HTTP endpoint health check failed (status: ${http_status})"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Could not retrieve HTTP endpoint status"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_cors_headers() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing CORS headers"
    
    local headers=$(curl -s -I --max-time "$TIMEOUT" "${BASE_URL}/health" 2>/dev/null || echo "")
    
    if [[ -n "$headers" ]]; then
        if echo "$headers" | grep -qi "Access-Control-Allow-Origin"; then
            log "SUCCESS" "CORS headers present"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "CORS headers not found (may be intentional for non-OPTIONS requests)"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "Could not retrieve headers"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_security_headers() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing security headers"
    
    local headers=$(curl -s -I --max-time "$TIMEOUT" "${BASE_URL}/health" 2>/dev/null || echo "")
    
    if [[ -n "$headers" ]]; then
        local has_csp=false
        local has_xframe=false
        
        if echo "$headers" | grep -qi "Content-Security-Policy"; then
            has_csp=true
        fi
        
        if echo "$headers" | grep -qi "X-Frame-Options"; then
            has_xframe=true
        fi
        
        if [[ "$has_csp" == "true" && "$has_xframe" == "true" ]]; then
            log "SUCCESS" "Security headers present (CSP, X-Frame-Options)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "Some security headers missing (CSP: ${has_csp}, X-Frame-Options: ${has_xframe})"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "Could not retrieve headers"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_response_time() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing health endpoint response time"
    
    local start_time=$(date +%s%N)
    local response=$(http_get "${BASE_URL}/health" 200)
    local end_time=$(date +%s%N)
    
    if [[ $? -eq 0 ]]; then
        local duration_ms=$(( (end_time - start_time) / 1000000 ))
        
        if [[ $duration_ms -lt 1000 ]]; then
            log "SUCCESS" "Response time acceptable (${duration_ms}ms)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "Response time slow (${duration_ms}ms > 1000ms)"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "Health endpoint not responding"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_api_docs_endpoint() {
    ((TESTS_RUN++)) || true
    log "INFO" "Testing API docs endpoint: GET ${BASE_URL}/api/docs"
    
    local response=$(http_get "${BASE_URL}/api/docs" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate HTML structure and Scalar UI presence
        local has_title=false
        local has_scalar=false
        local has_openapi_ref=false
        
        if echo "$response" | grep -q "Togather API Documentation"; then
            has_title=true
        fi
        
        if echo "$response" | grep -q "scalar-standalone.js"; then
            has_scalar=true
        fi
        
        if echo "$response" | grep -q "/api/v1/openapi.json"; then
            has_openapi_ref=true
        fi
        
        if [[ "$has_title" == "true" && "$has_scalar" == "true" && "$has_openapi_ref" == "true" ]]; then
            log "SUCCESS" "API docs endpoint verified (Scalar UI detected)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "API docs missing required elements (title: ${has_title}, scalar: ${has_scalar}, openapi: ${has_openapi_ref})"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "API docs endpoint not responding"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

# ============================================================================
# Main
# ============================================================================

main() {
    log "INFO" "Starting smoke tests for: ${BASE_URL}"
    log "INFO" "Timeout: ${TIMEOUT}s, Retries: ${RETRY_COUNT}"
    echo ""
    
    # Check dependencies
    if ! command -v curl &> /dev/null; then
        log "ERROR" "curl is required but not installed"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        log "ERROR" "jq is required but not installed"
        exit 1
    fi
    
    # Run smoke tests
    local start_time=$(date +%s)
    
    test_health_endpoint
    echo ""
    
    test_version_endpoint
    echo ""
    
    test_database_connectivity
    echo ""
    
    test_migration_status
    echo ""
    
    test_http_endpoint_check
    echo ""
    
    test_cors_headers
    echo ""
    
    test_security_headers
    echo ""
    
    test_response_time
    echo ""
    
    test_api_docs_endpoint
    echo ""
    
    local end_time=$(date +%s)
    local total_duration=$((end_time - start_time))
    
    # Summary
    echo "========================================"
    log "INFO" "Smoke Test Summary"
    echo "========================================"
    log "INFO" "Tests run:    ${TESTS_RUN}"
    log "INFO" "Tests passed: ${TESTS_PASSED}"
    log "INFO" "Tests failed: ${TESTS_FAILED}"
    log "INFO" "Duration:     ${total_duration}s"
    echo ""
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        log "SUCCESS" "All smoke tests passed!"
        exit 0
    else
        log "ERROR" "${TESTS_FAILED} test(s) failed"
        exit 1
    fi
}

# Run main if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main
fi
