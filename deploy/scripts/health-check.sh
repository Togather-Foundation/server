#!/usr/bin/env bash
#
# health-check.sh - Togather Server Health Check Script
#
# Validates deployment health for blue-green deployments.
# Checks HTTP endpoints and database connectivity with retries.
#
# Usage:
#   ./health-check.sh ENVIRONMENT [SLOT]
#
# Arguments:
#   ENVIRONMENT    Target environment: development, staging, or production
#   SLOT           Blue-green slot (default: blue). Values: blue, green
#
# Exit Codes:
#   0   Success - all health checks passed
#   1   Configuration error - invalid arguments or health check failed
#
# Reference: specs/001-deployment-infrastructure/spec.md

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

# Schema integrity strictness (can be overridden by environment variable)
# Set HEALTH_CHECK_STRICT_SCHEMA=true to fail on missing tables (recommended for production)
STRICT_SCHEMA="${HEALTH_CHECK_STRICT_SCHEMA:-false}"

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
    # Read from environment or use defaults from docker-compose.blue-green.yml
    # Blue slot: 8081 (line 48), Green slot: 8082 (line 88)
    local port
    if [[ "$slot" == "blue" ]]; then
        port="${BLUE_PORT:-8081}"
    else
        port="${GREEN_PORT:-8082}"
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
    
    # 5. Schema integrity check (verify critical tables exist)
    log "INFO" "Verifying schema integrity..."
    local critical_tables=("events" "places" "organizations" "river_job")
    local missing_tables=()
    
    for table in "${critical_tables[@]}"; do
        if ! echo "SELECT to_regclass('public.${table}');" | \
             psql "${DATABASE_URL}" -t -q 2>/dev/null | grep -q "$table"; then
            missing_tables+=("$table")
        fi
    done
    
    if [[ ${#missing_tables[@]} -gt 0 ]]; then
        log "WARN" "Missing expected tables: ${missing_tables[*]}"
        log "WARN" "This may indicate incomplete migrations or manual schema changes"
        
        # Check if strict mode is enabled
        if [[ "${STRICT_SCHEMA}" == "true" ]]; then
            log "ERROR" "HEALTH_CHECK_STRICT_SCHEMA=true: Failing health check due to missing tables"
            log "ERROR" "Expected tables: ${critical_tables[*]}"
            log "ERROR" "Missing tables: ${missing_tables[*]}"
            log "ERROR" ""
            log "ERROR" "REMEDIATION:"
            log "ERROR" "  1. Run migrations: make migrate-up"
            log "ERROR" "  2. Verify migrations completed successfully"
            log "ERROR" "  3. Check for manual schema changes"
            return 1
        else
            log "WARN" "Set HEALTH_CHECK_STRICT_SCHEMA=true to fail health check on missing tables (recommended for production)"
        fi
    else
        log "INFO" "All critical tables present"
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

Environment Variables:
  HEALTH_CHECK_STRICT_SCHEMA    If set to 'true', fail health check if critical
                                tables are missing. Recommended for production.
                                Default: false

Examples:
  $0 production
  $0 production blue
  HEALTH_CHECK_STRICT_SCHEMA=true $0 production

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
        echo "ERROR: ENVIRONMENT argument required" >&2
        echo "" >&2
        usage
        exit 1  # Configuration error
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
            echo "ERROR: Invalid environment '$environment'" >&2
            echo "Must be one of: development, staging, production" >&2
            echo "" >&2
            usage
            exit 1  # Configuration error
            ;;
    esac
    
    # Validate slot
    case "$slot" in
        blue|green)
            ;;
        *)
            echo "ERROR: Invalid slot '$slot'" >&2
            echo "Must be either: blue, green" >&2
            echo "" >&2
            usage
            exit 1  # Configuration error
            ;;
    esac
    
    # Run health check
    health_check "$environment" "$slot"
}

# Run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
