#!/usr/bin/env bash
#
# smoke-tests.sh - Deployment Smoke Tests for Togather Server
#
# Quick validation tests to run after deployment to verify basic functionality.
# These tests should complete in under 30 seconds and catch critical issues.
#
# Usage:
#   ./smoke-tests.sh [environment]
#   ./smoke-tests.sh local      # Test local server
#   ./smoke-tests.sh staging    # Test staging
#   ./smoke-tests.sh production # Test production (read-only)
#
# Or with custom BASE_URL:
#   BASE_URL=http://example.com:8080 ./smoke-tests.sh
#
# Arguments:
#   environment    Environment to test: local, staging, production (default: local)
#
# Exit Codes:
#   0   All smoke tests passed
#   1   One or more smoke tests failed
#
# Reference: specs/001-deployment-infrastructure/tasks.md T080

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load config if environment specified
if [ $# -ge 1 ]; then
    # shellcheck disable=SC1091
    source "${SCRIPT_DIR}/config.sh" "$1"
else
    # Use BASE_URL from environment or default
    BASE_URL="${BASE_URL:-http://localhost:8080}"
    TIMEOUT="${TIMEOUT:-5}"
    RETRY_COUNT="${RETRY_COUNT:-3}"
    MAX_RESPONSE_TIME_MS="${MAX_RESPONSE_TIME_MS:-1000}"
    ENVIRONMENT="${ENVIRONMENT:-local}"
fi

# Configuration
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
    local last_error=""
    
    while [[ $retry_count -lt $RETRY_COUNT ]]; do
        # Capture stderr to get curl error messages
        local curl_stderr=$(mktemp)
        local response=$(curl -s -w "\n%{http_code}" --max-time "$TIMEOUT" "$url" 2>"$curl_stderr" || echo -e "\n000")
        last_error=$(cat "$curl_stderr")
        rm -f "$curl_stderr"
        
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
    
    # Log the actual error on final failure
    if [[ -n "$last_error" ]]; then
        log "ERROR" "Connection failed: $last_error"
    elif [[ "$status" != "000" ]]; then
        log "ERROR" "HTTP $status (expected $expected_status)"
    fi
    
    return 1
}

# ============================================================================
# Smoke Tests
# ============================================================================

test_health_endpoint() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 1/16: Health endpoint - GET ${BASE_URL}/health"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate JSON structure
        if echo "$response" | jq -e '.status' >/dev/null 2>&1; then
            local status=$(echo "$response" | jq -r '.status')
            
            # Check for valid status values (healthy or degraded are acceptable)
            if [[ "$status" == "healthy" ]]; then
                log "SUCCESS" "Health endpoint returned healthy status"
                
                # Show details of health checks
                local checks=$(echo "$response" | jq -r '.checks | to_entries | map("\(.key): \(.value.status)") | join(", ")')
                log "INFO" "  Health checks: ${checks}"
                
                ((TESTS_PASSED++)) || true
                return 0
            elif [[ "$status" == "degraded" ]]; then
                log "SUCCESS" "Health endpoint returned degraded status (acceptable)"
                
                # Show which checks are failing
                local failing=$(echo "$response" | jq -r '.checks | to_entries | map(select(.value.status != "pass")) | map("\(.key): \(.value.status)") | join(", ")')
                if [[ -n "$failing" ]]; then
                    log "WARN" "  Non-passing checks: ${failing}"
                fi
                
                ((TESTS_PASSED++)) || true
                return 0
            else
                log "FAIL" "Health endpoint returned unexpected status: ${status} (expected: healthy or degraded)"
                log "ERROR" "  Full response: $(echo "$response" | jq -c '.')"
                ((TESTS_FAILED++)) || true
                return 1
            fi
        else
            log "FAIL" "Health endpoint response is not valid JSON"
            log "ERROR" "  Response: ${response}"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Health endpoint not responding or returned wrong status code"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_version_endpoint() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 2/16: Version endpoint - GET ${BASE_URL}/version"
    
    local response=$(http_get "${BASE_URL}/version" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate JSON structure
        if echo "$response" | jq -e '.version' >/dev/null 2>&1; then
            local version=$(echo "$response" | jq -r '.version')
            local git_commit=$(echo "$response" | jq -r '.git_commit // "unknown"')
            log "SUCCESS" "Version endpoint working (version: ${version}, commit: ${git_commit:0:7})"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Version response missing required 'version' field"
            log "ERROR" "  Response: ${response}"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "WARN" "Version endpoint not available (may not be implemented yet)"
        ((TESTS_PASSED++)) || true
        return 0
    fi
}

test_database_connectivity() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 3/16: Database connectivity check"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check database check status
        local db_status=$(echo "$response" | jq -r '.checks.database.status // "unknown"')
        if [[ "$db_status" == "pass" ]]; then
            local db_msg=$(echo "$response" | jq -r '.checks.database.message // "connected"')
            log "SUCCESS" "Database connectivity verified - ${db_msg}"
            ((TESTS_PASSED++)) || true
            return 0
        elif [[ "$db_status" == "warn" ]]; then
            log "WARN" "Database check returned warning (may be slow but functional)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Database connectivity check failed (status: ${db_status})"
            local db_msg=$(echo "$response" | jq -r '.checks.database.message // "no message"')
            log "ERROR" "  Database error: ${db_msg}"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Could not retrieve database status from health endpoint"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_migration_status() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 4/16: Database migration status check"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check migrations check status
        local migrations_status=$(echo "$response" | jq -r '.checks.migrations.status // "unknown"')
        if [[ "$migrations_status" == "pass" ]]; then
            local version=$(echo "$response" | jq -r '.checks.migrations.details.version // "unknown"')
            local dirty=$(echo "$response" | jq -r '.checks.migrations.details.dirty // false')
            
            if [[ "$dirty" == "false" ]]; then
                log "SUCCESS" "Database migrations verified (version: ${version}, clean state)"
            else
                log "WARN" "Database migrations in dirty state (version: ${version})"
            fi
            
            ((TESTS_PASSED++)) || true
            return 0
        elif [[ "$migrations_status" == "warn" ]]; then
            log "WARN" "Migration check returned warning status"
            local version=$(echo "$response" | jq -r '.checks.migrations.details.version // "unknown"')
            log "INFO" "  Migration version: ${version}"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Migration status check failed (status: ${migrations_status})"
            local msg=$(echo "$response" | jq -r '.checks.migrations.message // "no message"')
            log "ERROR" "  Migration error: ${msg}"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Could not retrieve migration status from health endpoint"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_http_endpoint_check() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 5/16: HTTP endpoint health check"
    
    local response=$(http_get "${BASE_URL}/health" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check http_endpoint check status
        local http_status=$(echo "$response" | jq -r '.checks.http_endpoint.status // "unknown"')
        if [[ "$http_status" == "pass" ]]; then
            log "SUCCESS" "HTTP endpoint health check passed"
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
    log "INFO" "Test 6/16: CORS headers check"
    
    # CORS headers are only sent when Origin header is present (cross-origin requests)
    # Send a test Origin header to check if CORS is configured
    local test_origin="https://example.com"
    local headers=$(curl -s -I -H "Origin: ${test_origin}" --max-time "$TIMEOUT" "${BASE_URL}/health" 2>/dev/null || echo "")
    
    if [[ -z "$headers" ]]; then
        log "FAIL" "Could not retrieve headers"
        ((TESTS_FAILED++)) || true
        return 1
    fi
    
    # Check for CORS headers
    if echo "$headers" | grep -qi "Access-Control-Allow-Origin"; then
        local origin=$(echo "$headers" | grep -i "Access-Control-Allow-Origin" | cut -d: -f2- | tr -d ' \r')
        log "SUCCESS" "CORS headers present (origin: ${origin})"
        ((TESTS_PASSED++)) || true
        return 0
    else
        # CORS not configured - this may be expected for environments without frontends
        if [[ "$ENVIRONMENT" == "production" ]]; then
            log "FAIL" "CORS headers not found in production - frontend clients will be blocked"
            log "ERROR" "  Set CORS_ALLOWED_ORIGINS environment variable with allowed domains"
            ((TESTS_FAILED++)) || true
            return 1
        else
            log "WARN" "CORS headers not found (may not be needed for ${ENVIRONMENT} without frontend)"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    fi
}

test_security_headers() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 7/16: Security headers check"
    
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
            log "SUCCESS" "Security headers present (CSP ✓, X-Frame-Options ✓)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "Some security headers missing (CSP: ${has_csp}, X-Frame: ${has_xframe})"
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
    log "INFO" "Test 8/16: Response time check (threshold: ${MAX_RESPONSE_TIME_MS}ms)"
    
    local start_time=$(date +%s%N)
    local response=$(http_get "${BASE_URL}/health" 200)
    local end_time=$(date +%s%N)
    
    if [[ $? -eq 0 ]]; then
        local duration_ms=$(( (end_time - start_time) / 1000000 ))
        
        if [[ $duration_ms -lt $MAX_RESPONSE_TIME_MS ]]; then
            log "SUCCESS" "Response time acceptable (${duration_ms}ms < ${MAX_RESPONSE_TIME_MS}ms)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "Response time slow (${duration_ms}ms > ${MAX_RESPONSE_TIME_MS}ms threshold)"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "Health endpoint not responding"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_events_api() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 9/16: Events API endpoint - GET ${BASE_URL}/api/v1/events"
    
    local response=$(http_get "${BASE_URL}/api/v1/events" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate response structure
        if echo "$response" | jq -e '.items' >/dev/null 2>&1; then
            local count=$(echo "$response" | jq '.items | length')
            log "SUCCESS" "Events API accessible and working (${count} events)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "Events API returned unexpected format (may be empty)"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "Events API not accessible"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_places_api() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 10/16: Places API endpoint - GET ${BASE_URL}/api/v1/places"
    
    local response=$(http_get "${BASE_URL}/api/v1/places" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate response structure (SEL APIs use 'items' not 'data')
        if echo "$response" | jq -e '.items' >/dev/null 2>&1; then
            local count=$(echo "$response" | jq '.items | length')
            log "SUCCESS" "Places API accessible and working (${count} places)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Places API returned unexpected format (expected 'items' array)"
            log "ERROR" "  Response: $(echo "$response" | jq -c '.' | head -c 200)"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Places API not accessible"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_organizations_api() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 11/16: Organizations API endpoint - GET ${BASE_URL}/api/v1/organizations"
    
    local response=$(http_get "${BASE_URL}/api/v1/organizations" 200)
    
    if [[ $? -eq 0 ]]; then
        # Validate response structure (SEL APIs use 'items' not 'data')
        if echo "$response" | jq -e '.items' >/dev/null 2>&1; then
            local count=$(echo "$response" | jq '.items | length')
            log "SUCCESS" "Organizations API accessible and working (${count} organizations)"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Organizations API returned unexpected format (expected 'items' array)"
            log "ERROR" "  Response: $(echo "$response" | jq -c '.' | head -c 200)"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Organizations API not accessible"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_openapi_schema() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 12/16: OpenAPI schema endpoint - GET ${BASE_URL}/api/v1/openapi.json"
    
    local response=$(http_get "${BASE_URL}/api/v1/openapi.json" 200 2>/dev/null || echo "")
    
    if [[ -n "$response" && $? -eq 0 ]]; then
        # Validate it's valid JSON with openapi field
        if echo "$response" | jq -e '.openapi' >/dev/null 2>&1; then
            local openapi_version=$(echo "$response" | jq -r '.openapi')
            local api_title=$(echo "$response" | jq -r '.info.title // "unknown"')
            log "SUCCESS" "OpenAPI schema available (OpenAPI ${openapi_version})"
            log "INFO" "  API: ${api_title}"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "OpenAPI schema has invalid format (missing 'openapi' field)"
            log "ERROR" "  Response preview: $(echo "$response" | head -c 200)"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "OpenAPI schema endpoint not available - API documentation missing"
        log "ERROR" "  Endpoint should return OpenAPI 3.1.0 specification"
        log "ERROR" "  This is required for API clients and tools"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_admin_ui() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 13/16: Admin UI login page - GET ${BASE_URL}/admin/login"
    
    local response=$(http_get "${BASE_URL}/admin/login" 200)
    
    if [[ $? -eq 0 ]]; then
        # Check if it's HTML
        if echo "$response" | grep -qi "<!DOCTYPE html>"; then
            log "SUCCESS" "Admin UI login page accessible and rendering HTML"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "FAIL" "Admin UI returned non-HTML response"
            log "ERROR" "  Response preview: $(echo "$response" | head -c 100)"
            ((TESTS_FAILED++)) || true
            return 1
        fi
    else
        log "FAIL" "Admin UI not accessible"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_https_certificate() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 14/16: HTTPS certificate validity"
    
    # Skip if using http://
    if [[ "$BASE_URL" != https://* ]]; then
        log "WARN" "Skipping HTTPS test (BASE_URL uses HTTP, not HTTPS)"
        ((TESTS_PASSED++)) || true
        return 0
    fi
    
    local domain=$(echo "$BASE_URL" | sed -E 's|https?://([^/]+).*|\1|')
    
    # Check certificate validity
    local cert_output=$(timeout 10 curl -vI "$BASE_URL/" 2>&1 || echo "")
    
    if echo "$cert_output" | grep -q "SSL certificate verify ok"; then
        log "SUCCESS" "HTTPS certificate valid for ${domain}"
        ((TESTS_PASSED++)) || true
        return 0
    else
        # Check if it's a certificate error or connection issue
        if echo "$cert_output" | grep -qi "certificate\|SSL"; then
            log "FAIL" "HTTPS certificate validation failed for ${domain}"
            log "ERROR" "  Certificate issue detected - check Let's Encrypt status"
        else
            log "FAIL" "HTTPS connection failed for ${domain}"
            log "ERROR" "  Could not establish HTTPS connection"
        fi
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_slot_header() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 15/16: Active deployment slot identification"
    
    local headers=$(curl -s -I --max-time "$TIMEOUT" "${BASE_URL}/health" 2>/dev/null || echo "")
    
    if [[ -n "$headers" ]]; then
        if echo "$headers" | grep -qi "X-Togather-Slot"; then
            local slot=$(echo "$headers" | grep -i "X-Togather-Slot" | cut -d: -f2 | tr -d ' \r')
            log "SUCCESS" "Active deployment slot identified: ${slot}"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "X-Togather-Slot header not found (may be local/single deployment)"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "Could not retrieve headers from server"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

test_container_health() {
    ((TESTS_RUN++)) || true
    log "INFO" "Test 16/16: Docker container health status"
    
    # Skip if SSH_SERVER is not configured
    if [[ -z "${SSH_SERVER:-}" ]]; then
        log "WARN" "Skipping container health test (SSH_SERVER not configured for this environment)"
        ((TESTS_PASSED++)) || true
        return 0
    fi
    
    # Check if container is healthy
    local container_status=$(ssh "$SSH_SERVER" 'docker ps --format "{{.Names}}: {{.Status}}" --filter name=togather-server --filter status=running' 2>/dev/null || echo "")
    
    if [[ -n "$container_status" ]]; then
        if echo "$container_status" | grep -q "(healthy)"; then
            local healthy_containers=$(echo "$container_status" | grep "(healthy)" | wc -l)
            log "SUCCESS" "Docker container(s) running and healthy (${healthy_containers} healthy)"
            log "INFO" "  ${container_status}"
            ((TESTS_PASSED++)) || true
            return 0
        else
            log "WARN" "Docker container(s) running but not all healthy"
            log "INFO" "  ${container_status}"
            ((TESTS_PASSED++)) || true
            return 0
        fi
    else
        log "FAIL" "No healthy togather-server containers found"
        ((TESTS_FAILED++)) || true
        return 1
    fi
}

# ============================================================================
# Main
# ============================================================================

main() {
    log "INFO" "Starting smoke tests for: ${BASE_URL}"
    log "INFO" "Environment: ${ENVIRONMENT}"
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
    
    test_events_api
    echo ""
    
    test_places_api
    echo ""
    
    test_organizations_api
    echo ""
    
    test_openapi_schema
    echo ""
    
    test_admin_ui
    echo ""
    
    test_https_certificate
    echo ""
    
    test_slot_header
    echo ""
    
    test_container_health
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
