#!/bin/bash
# test-deployment.sh - Automated deployment testing
# Usage: ./test-deployment.sh <environment> <server> <domain>

set -euo pipefail

ENVIRONMENT="${1:-staging}"
SERVER="${2:-deploy@staging}"
DOMAIN="${3:-staging.toronto.togather.foundation}"

echo "========================================"
echo "Testing deployment on $DOMAIN"
echo "Environment: $ENVIRONMENT"
echo "Server: $SERVER"
echo "========================================"
echo ""

FAILED=0
CHECKS=0

run_check() {
    local name="$1"
    local command="$2"
    CHECKS=$((CHECKS + 1))
    
    if eval "$command" > /dev/null 2>&1; then
        echo "✓ $name"
        return 0
    else
        echo "✗ $name"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

# Health check
echo "=== Health Checks ==="
run_check "Health endpoint responds" \
    "curl -sf --max-time 10 'https://$DOMAIN/health'"

run_check "Health status is healthy or degraded" \
    "curl -sf 'https://$DOMAIN/health' | jq -e '.status == \"healthy\" or .status == \"degraded\"'"

# API checks
echo ""
echo "=== API Checks ==="
run_check "Events API accessible" \
    "curl -sf --max-time 10 'https://$DOMAIN/api/v1/events'"

run_check "Places API accessible" \
    "curl -sf --max-time 10 'https://$DOMAIN/api/v1/places'"

run_check "Organizations API accessible" \
    "curl -sf --max-time 10 'https://$DOMAIN/api/v1/organizations'"

run_check "OpenAPI schema available" \
    "curl -sf --max-time 10 'https://$DOMAIN/openapi.json' | jq -e '.openapi'"

# Admin UI check
echo ""
echo "=== Admin UI Checks ==="
run_check "Admin login page loads" \
    "curl -sf --max-time 10 'https://$DOMAIN/admin/login' | grep -q '<!DOCTYPE html>'"

# Container health
echo ""
echo "=== Container Checks ==="
run_check "Container is running and healthy" \
    "ssh '$SERVER' 'docker ps --format \"{{.Status}}\" --filter name=togather-server' | grep -q '(healthy)'"

# SSL check
echo ""
echo "=== Security Checks ==="
run_check "HTTPS certificate valid" \
    "curl -vI https://$DOMAIN/ 2>&1 | grep -q 'SSL certificate verify ok'"

# Slot verification
echo ""
echo "=== Deployment State ==="
ACTIVE_SLOT=$(curl -sI "https://$DOMAIN/health" 2>/dev/null | grep -i "X-Togather-Slot" | cut -d: -f2 | tr -d ' \r' || echo "unknown")
echo "Active slot: $ACTIVE_SLOT"

VERSION=$(curl -s "https://$DOMAIN/health" 2>/dev/null | jq -r '.version // "unknown"')
echo "Version: $VERSION"

# Final result
echo ""
echo "========================================"
echo "Test Results: $((CHECKS - FAILED))/$CHECKS passed"
if [ $FAILED -eq 0 ]; then
    echo "✅ All checks passed!"
    echo "========================================"
    exit 0
else
    echo "❌ $FAILED checks failed"
    echo "========================================"
    
    # Show container logs for debugging
    echo ""
    echo "Recent container logs:"
    ssh "$SERVER" "docker logs togather-server-$ACTIVE_SLOT --tail 20 2>&1 || echo 'Could not fetch logs'"
    
    exit 1
fi
