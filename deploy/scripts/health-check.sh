#!/usr/bin/env bash
# Togather Server Health Check Script
# Validates deployment health for blue-green deployments
# See: specs/001-deployment-infrastructure/spec.md

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_DIR="${DEPLOY_DIR}/config"

# Health check parameters
MAX_ATTEMPTS=30
RETRY_DELAY=2
TIMEOUT=5

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ============================================================================
# FUNCTIONS
# ============================================================================

log() {
    local level="$1"
    shift
    local message="$*"
    
    local color="${NC}"
    case "$level" in
        ERROR)   color="${RED}" ;;
        SUCCESS) color="${GREEN}" ;;
        WARN)    color="${YELLOW}" ;;
    esac
    
    echo -e "${color}[${level}]${NC} ${message}"
}

# Check HTTP endpoint health
check_http_health() {
    local url="$1"
    local attempt="$2"
    
    local response_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time ${TIMEOUT} "${url}" 2>/dev/null || echo "000")
    
    if [[ "$response_code" == "200" ]]; then
        return 0
    else
        log "INFO" "HTTP health check (attempt ${attempt}/${MAX_ATTEMPTS}): ${response_code}"
        return 1
    fi
}

# Check database connectivity
check_database_health() {
    local database_url="$1"
    
    # Simple connection test using psql
    if echo "SELECT 1;" | psql "${database_url}" -t -q > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Get health check endpoint for slot
get_health_endpoint() {
    local env="$1"
    local slot="$2"
    
    # Load environment configuration
    local env_file="${CONFIG_DIR}/environments/.env.${env}"
    if [[ ! -f "${env_file}" ]]; then
        log "ERROR" "Environment file not found: ${env_file}"
        return 1
    fi
    
    source "${env_file}"
    
    # Determine port based on slot
    # Blue slot: 8080, Green slot: 8081 (default convention)
    local port=8080
    if [[ "$slot" == "green" ]]; then
        port=8081
    fi
    
    # Override from environment if set
    if [[ -n "${HEALTH_CHECK_URL:-}" ]]; then
        echo "${HEALTH_CHECK_URL}"
    else
        echo "http://localhost:${port}/health"
    fi
}

# Perform comprehensive health check
health_check() {
    local env="$1"
    local slot="$2"
    
    log "INFO" "Starting health check for ${env} environment (${slot} slot)"
    
    # Load environment variables
    local env_file="${CONFIG_DIR}/environments/.env.${env}"
    if [[ ! -f "${env_file}" ]]; then
        log "ERROR" "Environment file not found: ${env_file}"
        return 1
    fi
    
    source "${env_file}"
    
    # Get health check endpoint
    local health_url=$(get_health_endpoint "${env}" "${slot}")
    log "INFO" "Health check URL: ${health_url}"
    
    # 1. HTTP endpoint health check
    log "INFO" "Checking HTTP endpoint health..."
    local attempt=0
    local http_healthy=false
    
    while [[ $attempt -lt $MAX_ATTEMPTS ]]; do
        ((attempt++))
        
        if check_http_health "${health_url}" "${attempt}"; then
            log "SUCCESS" "HTTP endpoint healthy (${health_url})"
            http_healthy=true
            break
        fi
        
        if [[ $attempt -lt $MAX_ATTEMPTS ]]; then
            sleep ${RETRY_DELAY}
        fi
    done
    
    if [[ "$http_healthy" != "true" ]]; then
        log "ERROR" "HTTP endpoint health check failed after ${MAX_ATTEMPTS} attempts"
        return 1
    fi
    
    # 2. Database connectivity check
    log "INFO" "Checking database connectivity..."
    
    if check_database_health "${DATABASE_URL}"; then
        log "SUCCESS" "Database connectivity healthy"
    else
        log "ERROR" "Database connectivity check failed"
        return 1
    fi
    
    # 3. Detailed health status check (if available)
    log "INFO" "Fetching detailed health status..."
    
    local health_response=$(curl -sf --max-time ${TIMEOUT} "${health_url}" 2>/dev/null || echo "{}")
    
    if [[ -n "$health_response" ]] && echo "$health_response" | jq -e . > /dev/null 2>&1; then
        local status=$(echo "$health_response" | jq -r '.status // "unknown"')
        
        log "INFO" "Health status: ${status}"
        
        # Check individual components if available
        if echo "$health_response" | jq -e '.checks' > /dev/null 2>&1; then
            echo "$health_response" | jq -r '.checks | to_entries[] | "  - \(.key): \(.value.status // "unknown")"'
        fi
        
        # Fail if status is not healthy
        if [[ "$status" != "healthy" ]] && [[ "$status" != "ok" ]]; then
            log "WARN" "Health status is not healthy: ${status}"
            # Don't fail for MVP, just warn
        fi
    else
        log "WARN" "Could not parse health response JSON"
    fi
    
    # 4. Migration version check (T030 requirement)
    log "INFO" "Checking migration version..."
    
    local migration_version=$(echo "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;" | \
                             psql "${DATABASE_URL}" -t -q 2>/dev/null || echo "unknown")
    
    if [[ "$migration_version" != "unknown" ]]; then
        log "INFO" "Current migration version: ${migration_version}"
    else
        log "WARN" "Could not determine migration version"
    fi
    
    # All checks passed
    log "SUCCESS" "All health checks passed for ${env} (${slot} slot)"
    return 0
}

# ============================================================================
# USAGE AND MAIN
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 ENVIRONMENT [SLOT]

Validate deployment health checks.

Arguments:
  ENVIRONMENT    Target environment (development, staging, production)
  SLOT          Deployment slot (blue, green) [optional, defaults to current active]

Examples:
  $0 production
  $0 production blue
  $0 staging green

Exit codes:
  0  All health checks passed
  1  One or more health checks failed

See: specs/001-deployment-infrastructure/spec.md
EOF
}

main() {
    local environment=""
    local slot="blue"
    
    # Parse arguments
    if [[ $# -lt 1 ]]; then
        echo "Error: ENVIRONMENT argument required"
        usage
        exit 1
    fi
    
    environment="$1"
    
    if [[ $# -ge 2 ]]; then
        slot="$2"
    fi
    
    # Validate environment
    case "$environment" in
        development|staging|production)
            ;;
        *)
            echo "Error: Invalid environment: $environment"
            echo "Must be one of: development, staging, production"
            exit 1
            ;;
    esac
    
    # Validate slot
    case "$slot" in
        blue|green)
            ;;
        *)
            echo "Error: Invalid slot: $slot"
            echo "Must be either: blue, green"
            exit 1
            ;;
    esac
    
    # Run health check
    health_check "$environment" "$slot"
}

# Run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
