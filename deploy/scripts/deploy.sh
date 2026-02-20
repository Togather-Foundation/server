#!/usr/bin/env bash
#
# deploy.sh - Togather Server Deployment Script
#
# Implements blue-green zero-downtime deployment with automatic rollback.
# Orchestrates Docker builds, database migrations, health checks, and traffic switching.
#
# Usage:
#   ./deploy.sh ENVIRONMENT [options]
#
# Arguments:
#   ENVIRONMENT         Target environment: development, staging, or production
#
# Options:
#   --dry-run          Validate configuration without deploying
#   --skip-migrations  Skip database migration step
#   --force            Force deployment even if validations fail
#   --env-diff         Run environment variable audit only (no deploy)
#   --version          Show script version
#   --help             Show usage information
#
# Exit Codes:
#   0   Success - deployment completed successfully
#   1   Configuration error - invalid arguments, missing environment, or validation failure
#
# Reference: specs/001-deployment-infrastructure/spec.md

set -euo pipefail

# Script version
SCRIPT_VERSION="1.0.0"

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROJECT_ROOT="$(cd "${DEPLOY_DIR}/.." && pwd)"
CONFIG_DIR="${DEPLOY_DIR}/config"
DOCKER_DIR="${DEPLOY_DIR}/docker"
STATE_FILE="${CONFIG_DIR}/deployment-state.json"
DEPLOYMENT_CONFIG="${CONFIG_DIR}/deployment.yml"
LOG_DIR="${HOME}/.togather/logs/deployments"
DEPLOYMENT_HISTORY_DIR="/var/lib/togather/deployments"
LOCK_TIMEOUT=1800  # 30 minutes in seconds

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Deployment metadata
DEPLOYMENT_ID=""
DEPLOYMENT_LOG=""
GIT_COMMIT=""
GIT_SHORT_COMMIT=""
DEPLOYMENT_TIMESTAMP=""
DEPLOYED_BY="${USER}@$(hostname)"
SNAPSHOT_PATH=""  # Captured during create_db_snapshot for rollback history
COMPOSE_CMD="docker compose"  # Default to plugin mode, detected in validate_tool_versions()

# ============================================================================
# LOGGING FUNCTIONS
# ============================================================================

# Initialize logging for this deployment
init_logging() {
    local env="$1"
    DEPLOYMENT_TIMESTAMP=$(date -u +"%Y%m%d_%H%M%S")
    DEPLOYMENT_LOG="${LOG_DIR}/${env}_${DEPLOYMENT_TIMESTAMP}.log"
    
    # Create log directory if it doesn't exist
    mkdir -p "${LOG_DIR}"
    
    # Create log file
    touch "${DEPLOYMENT_LOG}"
    
    log "INFO" "Deployment log initialized: ${DEPLOYMENT_LOG}"
}

# Log message to both stdout and log file with secret sanitization
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.%3NZ")
    
    # Sanitize secrets from log message
    message=$(sanitize_secrets "$message")
    
    # Structured log format
    local log_entry=$(cat <<EOF
{"timestamp":"${timestamp}","level":"${level}","deployment_id":"${DEPLOYMENT_ID}","message":"${message}"}
EOF
)
    
    # Write to log file if initialized
    if [[ -n "${DEPLOYMENT_LOG:-}" ]]; then
        echo "${log_entry}" >> "${DEPLOYMENT_LOG}"
    fi
    
    # Print to stdout with color
    local color="${NC}"
    case "$level" in
        ERROR)   color="${RED}" ;;
        WARN)    color="${YELLOW}" ;;
        SUCCESS) color="${GREEN}" ;;
        INFO)    color="${BLUE}" ;;
    esac
    
    echo -e "${color}[${level}]${NC} ${message}"
}

# Sanitize secrets from strings (T023a - Secret sanitization)
# NOTE: This function provides best-effort redaction but is NOT foolproof.
# Avoid logging user-controlled input or environment variables directly.
sanitize_secrets() {
    local input="$1"
    
    # Redact DATABASE_URL passwords (handles special chars, stops at @ or end)
    # Pattern: postgresql://user:password@host -> postgresql://user:***REDACTED***@host
    input=$(echo "$input" | sed -E 's|(postgresql://[^:]+:)[^@]+(@)|\1***REDACTED***\2|g')
    
    # Redact key=value or key:value patterns (handles quoted values and special chars)
    # Match until whitespace, quote, or end of line
    input=$(echo "$input" | sed -E 's|(JWT_SECRET[=:])([^[:space:]"'\'']+)|\1***REDACTED***|g')
    
    # Redact quoted values: KEY="value with spaces" or KEY='value'
    input=$(echo "$input" | sed -E 's|(JWT_SECRET[=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    
    # Redact generic secret patterns (case-insensitive, handles unquoted and quoted)
    # Unquoted: password=value or password:value (stop at whitespace)
    input=$(echo "$input" | sed -E 's|([Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd][=:])([^[:space:]"'\'']+)|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|([Tt][Oo][Kk][Ee][Nn][=:])([^[:space:]"'\'']+)|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|([Ss][Ee][Cc][Rr][Ee][Tt][=:])([^[:space:]"'\'']+)|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|([Kk][Ee][Yy][=:])([^[:space:]"'\'']+)|\1***REDACTED***|g')
    
    # Quoted: password="value with spaces" or password='value'
    input=$(echo "$input" | sed -E 's|([Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd][=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|([Tt][Oo][Kk][Ee][Nn][=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|([Ss][Ee][Cc][Rr][Ee][Tt][=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|([Kk][Ee][Yy][=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    
    echo "$input"
}

# Portable file permissions check (Linux/macOS compatible)
# Returns octal permissions like "600" or "UNKNOWN" on error
get_file_perms() {
    local file="$1"
    
    # Try Linux (GNU) stat first
    if stat -L -c '%a' "$file" 2>/dev/null; then
        return 0
    fi
    
    # Try macOS (BSD) stat
    if stat -f '%Lp' "$file" 2>/dev/null; then
        return 0
    fi
    
    # Fallback: couldn't determine permissions
    # Use sentinel value that can't be real permissions
    echo "UNKNOWN"
    return 1
}

# update_state_file_atomic - Atomically updates deployment state file with fsync
# Implements atomic write pattern: write to temp -> fsync -> rename
# This prevents partial writes and ensures durability (T022 requirement)
# Args:
#   $@ - jq arguments (e.g., --arg key value '.field = $key')
# Returns:
#   0 on success, 1 on error
# Example:
#   update_state_file_atomic --arg status "deployed" '.deployments[-1].status = $status'
update_state_file_atomic() {
    # All arguments are passed to jq, with the last one being the expression
    local temp_file=$(mktemp)
    
    # Write to temp file
    if ! jq "$@" "${STATE_FILE}" > "$temp_file"; then
        log "ERROR" "Failed to update state file with jq"
        rm -f "$temp_file"
        return 1
    fi
    
    # Validate temp file before committing the update
    if ! validate_state_file "$temp_file"; then
        log "ERROR" "State file validation failed after update"
        log "ERROR" "This update would violate the schema, rolling back"
        rm -f "$temp_file"
        return 1
    fi
    
    # Sync temp file to disk
    sync "$temp_file" 2>/dev/null || fsync "$temp_file" 2>/dev/null || true
    
    # Atomic rename
    mv "$temp_file" "${STATE_FILE}"
    
    # Sync parent directory to ensure rename is durable
    sync "${CONFIG_DIR}" 2>/dev/null || true
    
    return 0
}

# Validate deployment state file structure
# Checks required fields and basic schema compliance
validate_state_file() {
    local state_file="$1"
    
    if [[ ! -f "$state_file" ]]; then
        log "ERROR" "State file does not exist: ${state_file}"
        return 1
    fi
    
    # Check if file is valid JSON
    if ! jq empty "$state_file" 2>/dev/null; then
        log "ERROR" "State file is not valid JSON: ${state_file}"
        return 1
    fi
    
    # Validate required top-level fields
    local required_fields=("environment" "lock")
    for field in "${required_fields[@]}"; do
        if ! jq -e "has(\"$field\")" "$state_file" >/dev/null 2>&1; then
            log "ERROR" "State file missing required field: ${field}"
            return 1
        fi
    done
    
    # Validate environment field is valid enum value
    local env=$(jq -r '.environment // ""' "$state_file")
    if [[ ! "$env" =~ ^(development|staging|production)$ ]]; then
        log "ERROR" "Invalid environment value: ${env} (must be development, staging, or production)"
        return 1
    fi
    
    # Validate lock structure
    if ! jq -e '.lock | has("locked")' "$state_file" >/dev/null 2>&1; then
        log "ERROR" "State file lock object missing 'locked' field"
        return 1
    fi
    
    local locked=$(jq -r '.lock.locked' "$state_file")
    if [[ "$locked" != "true" && "$locked" != "false" ]]; then
        log "ERROR" "State file lock.locked must be boolean (true/false), got: ${locked}"
        return 1
    fi
    
    # If lock is active, validate required lock fields
    if [[ "$locked" == "true" ]]; then
        local lock_fields=("lock_id" "locked_by" "locked_at" "lock_expires_at" "deployment_id")
        for field in "${lock_fields[@]}"; do
            if ! jq -e ".lock | has(\"$field\")" "$state_file" >/dev/null 2>&1; then
                log "ERROR" "Locked state missing required field: lock.${field}"
                return 1
            fi
            
            local value=$(jq -r ".lock.${field}" "$state_file")
            if [[ -z "$value" || "$value" == "null" ]]; then
                log "ERROR" "Locked state field cannot be empty: lock.${field}"
                return 1
            fi
        done
    fi
    
    # Validate current_deployment structure if present
    if jq -e '.current_deployment | type == "object"' "$state_file" >/dev/null 2>&1; then
        local deployment_fields=("id" "version" "git_commit" "deployed_at" "deployed_by" "active_slot" "health_status")
        for field in "${deployment_fields[@]}"; do
            if ! jq -e ".current_deployment | has(\"$field\")" "$state_file" >/dev/null 2>&1; then
                log "ERROR" "current_deployment missing required field: ${field}"
                return 1
            fi
        done
        
        # Validate active_slot is blue or green
        local slot=$(jq -r '.current_deployment.active_slot // ""' "$state_file")
        if [[ ! "$slot" =~ ^(blue|green)$ ]]; then
            log "ERROR" "Invalid active_slot value: ${slot} (must be blue or green)"
            return 1
        fi
        
        # Validate health_status is valid enum
        local health=$(jq -r '.current_deployment.health_status // ""' "$state_file")
        if [[ ! "$health" =~ ^(healthy|degraded|unhealthy|unknown)$ ]]; then
            log "ERROR" "Invalid health_status value: ${health}"
            return 1
        fi
    fi
    
    log "INFO" "State file validation passed: ${state_file}"
    return 0
}

# ============================================================================
# VALIDATION FUNCTIONS
# ============================================================================

# T014: Validate configuration files
validate_config() {
    local env="$1"
    log "INFO" "Validating configuration for environment: ${env}"
    
    # Check deployment config exists
    if [[ ! -f "${DEPLOYMENT_CONFIG}" ]]; then
        log "ERROR" "Deployment configuration not found: ${DEPLOYMENT_CONFIG}"
        return 1
    fi
    
    # Basic validation: Check if deployment.yml is valid YAML
    # Use Python to validate YAML syntax (available on all platforms)
    log "INFO" "Validating deployment.yml structure..."
    if ! python3 -c "import yaml; yaml.safe_load(open('${DEPLOYMENT_CONFIG}'))" 2>/dev/null; then
        log "ERROR" "deployment.yml is not valid YAML syntax"
        log "ERROR" "File: ${DEPLOYMENT_CONFIG}"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  1. Check YAML syntax with: python3 -c \"import yaml; yaml.safe_load(open('${DEPLOYMENT_CONFIG}'))\""
        log "ERROR" "  2. Verify indentation is consistent (use spaces, not tabs)"
        log "ERROR" "  3. See schema documentation: ${CONFIG_DIR}/deployment.schema.json"
        return 1
    fi
    
    log "INFO" "deployment.yml is valid YAML"
    log "INFO" "Note: See ${CONFIG_DIR}/deployment.schema.json for full schema documentation"
    
    # T037: Load environment configuration
    # For local deployments: use deploy/docker/.env or root .env
    # For remote deployments: symlinked at ${CONFIG_DIR}/environments/.env.${env} by remote deploy logic
    # Precedence: CLI env vars > shell env > .env file > deployment.yml defaults
    
    local env_file=""
    
    # Check for environment file in multiple locations
    if [[ -f "${CONFIG_DIR}/environments/.env.${env}" ]]; then
        # Remote deployment: symlinked from /opt/togather/.env.{env}
        env_file="${CONFIG_DIR}/environments/.env.${env}"
        log "INFO" "Using environment file: ${env_file}"
    elif [[ -f "${PROJECT_ROOT}/deploy/docker/.env" ]]; then
        # Local Docker deployment
        env_file="${PROJECT_ROOT}/deploy/docker/.env"
        log "INFO" "Using local Docker environment: ${env_file}"
    elif [[ -f "${PROJECT_ROOT}/.env" ]]; then
        # Local development (non-Docker)
        env_file="${PROJECT_ROOT}/.env"
        log "INFO" "Using local development environment: ${env_file}"
    else
        log "ERROR" "No environment configuration found"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  For local development:"
        log "ERROR" "    cp ${PROJECT_ROOT}/.env.example ${PROJECT_ROOT}/.env"
        log "ERROR" "    nano ${PROJECT_ROOT}/.env"
        log "ERROR" ""
        log "ERROR" "  For local Docker:"
        log "ERROR" "    cp ${PROJECT_ROOT}/deploy/docker/.env.example ${PROJECT_ROOT}/deploy/docker/.env"
        log "ERROR" "    nano ${PROJECT_ROOT}/deploy/docker/.env"
        log "ERROR" ""
        log "ERROR" "  For remote deployment:"
        log "ERROR" "    SSH to server and create /opt/togather/.env.${env}"
        log "ERROR" "    Example: cp /opt/togather/src/deploy/config/environments/.env.${env}.example /opt/togather/.env.${env}"
        return 1
    fi
    
    # T038: Check environment file permissions (MUST be 600 for security)
    local perms=$(get_file_perms "${env_file}")
    
    if [[ "${perms}" != "UNKNOWN" && "${perms}" != "600" ]]; then
        log "WARN" "Environment file has insecure permissions: ${perms} (expected: 600)"
        log "WARN" "Secrets may be readable by other users"
        log "WARN" "Fix with: chmod 600 ${env_file}"
        # Don't fail for local development, but warn
    fi
    
    # Save current env vars to detect overrides
    local saved_DATABASE_URL="${DATABASE_URL:-}"
    local saved_JWT_SECRET="${JWT_SECRET:-}"
    local saved_ENVIRONMENT="${ENVIRONMENT:-}"
    
    source "${env_file}"
    
    # Restore CLI/shell overrides (they take precedence over .env file)
    # Count overrides for generic logging (avoid revealing specific secret names)
    local override_count=0
    if [[ -n "${saved_DATABASE_URL}" ]]; then
        DATABASE_URL="${saved_DATABASE_URL}"
        ((override_count++))
    fi
    if [[ -n "${saved_JWT_SECRET}" ]]; then
        JWT_SECRET="${saved_JWT_SECRET}"
        ((override_count++))
    fi
    if [[ -n "${saved_ENVIRONMENT}" ]]; then
        ENVIRONMENT="${saved_ENVIRONMENT}"
        ((override_count++))
    fi
    
    # Generic logging to avoid revealing specific variable names
    if [[ $override_count -gt 0 ]]; then
        log "INFO" "Using ${override_count} environment variable(s) from shell/CLI (precedence: CLI > .env > defaults)"
    fi
    
    # T039: Validate required environment variables with clear remediation
    local required_vars=("ENVIRONMENT" "DATABASE_URL" "JWT_SECRET")
    local missing_vars=()
    
    for var in "${required_vars[@]}"; do
        if [[ -z "${!var:-}" ]]; then
            missing_vars+=("$var")
        fi
    done
    
    if [[ ${#missing_vars[@]} -gt 0 ]]; then
        log "ERROR" "Missing required environment variables:"
        for var in "${missing_vars[@]}"; do
            log "ERROR" "  - ${var}"
        done
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  1. Edit the environment file:"
        log "ERROR" "     ${EDITOR:-nano} ${env_file}"
        log "ERROR" "  2. Set values for: ${missing_vars[*]}"
        log "ERROR" "  3. Example formats:"
        log "ERROR" "     DATABASE_URL=postgresql://user:pass@host:5432/dbname"
        log "ERROR" "     JWT_SECRET=\$(openssl rand -hex 32)"
        log "ERROR" "     ENVIRONMENT=${env}"
        return 1
    fi
    
    # T035 & T039: Validate no CHANGE_ME placeholders with specific guidance
    if grep -q "CHANGE_ME" "${env_file}"; then
        log "ERROR" "Environment file contains CHANGE_ME placeholders"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  1. Find all CHANGE_ME values:"
        log "ERROR" "     grep CHANGE_ME ${env_file}"
        log "ERROR" "  2. Generate secure secrets:"
        log "ERROR" "     JWT_SECRET: openssl rand -hex 32"
        log "ERROR" "  3. Edit the file and replace placeholders:"
        log "ERROR" "     ${EDITOR:-nano} ${env_file}"
        log "ERROR" "  4. Verify no placeholders remain:"
        log "ERROR" "     grep -v '^#' ${env_file} | grep CHANGE_ME"
        return 1
    fi
    
    # T040: Run environment variable audit against template
    # Detects missing/extra vars between .env.example and actual .env file
    local audit_script="${SCRIPT_DIR}/env-audit.sh"
    if [[ -x "${audit_script}" ]]; then
        local audit_env="${env}"
        
        # Resolve the correct template to match the env file that was selected
        # This prevents mismatches (e.g., Docker env file vs staging template)
        local audit_template=""
        if [[ "${env_file}" == *"/deploy/docker/"* ]]; then
            audit_template="${PROJECT_ROOT}/deploy/docker/.env.example"
        elif [[ "${env_file}" == "${PROJECT_ROOT}/.env" ]]; then
            audit_template="${PROJECT_ROOT}/.env.example"
        fi
        
        log "INFO" "Running environment variable audit..."
        local audit_exit=0
        local audit_output=""
        local audit_args=("${audit_env}" --env-file "${env_file}" --quiet)
        [[ -n "${audit_template}" ]] && audit_args+=(--template "${audit_template}")
        audit_output=$("${audit_script}" "${audit_args[@]}" 2>&1) || audit_exit=$?
        
        if [[ -n "${audit_output}" ]]; then
            while IFS= read -r line; do
                log "INFO" "  ${line}"
            done <<< "${audit_output}"
        fi
        
        if [[ ${audit_exit} -eq 1 ]]; then
            log "ERROR" "Environment audit found missing required variables"
            log "ERROR" "Run: ${audit_script} ${audit_env} --env-file ${env_file}"
            return 1
        elif [[ ${audit_exit} -eq 2 ]]; then
            log "WARN" "Environment audit found missing optional variables"
            log "WARN" "Run: ${audit_script} ${audit_env} --env-file ${env_file}"
            log "WARN" "Continuing deployment (use --env-diff to review before deploying)"
        fi
    else
        log "WARN" "Environment audit script not found at ${audit_script}, skipping audit"
    fi
    
    # Validate deployment state file structure
    if [[ -f "${STATE_FILE}" ]]; then
        if ! validate_state_file "${STATE_FILE}"; then
            log "ERROR" "Deployment state file validation failed: ${STATE_FILE}"
            return 1
        fi
    else
        log "WARN" "Deployment state file does not exist yet: ${STATE_FILE}"
        log "INFO" "It will be created during the first deployment"
    fi
    
    log "SUCCESS" "Configuration validation passed"
    return 0
}

# T014a: Validate tool versions
validate_tool_versions() {
    log "INFO" "Validating deployment tool versions"
    
    local errors=0
    
    # Check docker version (>= 20.10)
    if ! command -v docker &> /dev/null; then
        log "ERROR" "docker not found in PATH"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  Install Docker Engine >= 20.10"
        log "ERROR" "  Ubuntu/Debian: https://docs.docker.com/engine/install/ubuntu/"
        log "ERROR" "  RHEL/CentOS: https://docs.docker.com/engine/install/centos/"
        log "ERROR" "  macOS: https://docs.docker.com/desktop/install/mac-install/"
        ((errors++))
    else
        local docker_version=$(docker --version | grep -oP '\d+\.\d+' | head -1)
        local docker_major=$(echo "$docker_version" | cut -d. -f1)
        local docker_minor=$(echo "$docker_version" | cut -d. -f2)
        
        if [[ $docker_major -lt 20 ]] || [[ $docker_major -eq 20 && $docker_minor -lt 10 ]]; then
            log "ERROR" "docker version $docker_version found, but >= 20.10 required"
            ((errors++))
        else
            log "INFO" "docker version $docker_version OK"
        fi
    fi
    
    # Check docker-compose version (>= 2.0)
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log "ERROR" "docker-compose not found (neither standalone nor docker compose plugin)"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  Install Docker Compose >= 2.0"
        log "ERROR" "  Plugin (recommended): docker compose comes with Docker Desktop"
        log "ERROR" "  Standalone: https://docs.docker.com/compose/install/standalone/"
        ((errors++))
    else
        COMPOSE_CMD="docker-compose"
        if ! command -v docker-compose &> /dev/null; then
            COMPOSE_CMD="docker compose"
        fi
        
        local compose_version=$(${COMPOSE_CMD} version --short 2>/dev/null || echo "0.0.0")
        local compose_major=$(echo "$compose_version" | cut -d. -f1 | tr -d 'v')
        
        if [[ $compose_major -lt 2 ]]; then
            log "ERROR" "docker-compose version $compose_version found, but >= 2.0 required"
            ((errors++))
        else
            log "INFO" "docker-compose version $compose_version OK"
        fi
    fi
    
    # Check golang-migrate
    if ! command -v migrate &> /dev/null; then
        log "ERROR" "golang-migrate (migrate command) not found in PATH"
        log "ERROR" "Install from: https://github.com/golang-migrate/migrate"
        ((errors++))
    else
        local migrate_version=$(migrate -version 2>&1 | head -1)
        log "INFO" "golang-migrate OK: $migrate_version"
    fi
    
    # Check jq
    if ! command -v jq &> /dev/null; then
        log "ERROR" "jq not found in PATH"
        log "ERROR" "Install with: sudo apt-get install jq (Debian/Ubuntu) or brew install jq (macOS)"
        ((errors++))
    else
        local jq_version=$(jq --version)
        log "INFO" "jq OK: $jq_version"
    fi
    
    # Check psql
    if ! command -v psql &> /dev/null; then
        log "ERROR" "psql (PostgreSQL client) not found in PATH"
        log "ERROR" "Install with: sudo apt-get install postgresql-client"
        ((errors++))
    else
        local psql_version=$(psql --version)
        log "INFO" "psql OK: $psql_version"
    fi
    
    if [[ $errors -gt 0 ]]; then
        log "ERROR" "Tool version validation failed with $errors error(s)"
        return 1
    fi
    
    log "SUCCESS" "All required tools found with correct versions"
    return 0
}

# T014b: Validate Git commit matches CI test results
validate_git_commit() {
    log "INFO" "Validating Git commit"
    
    # Get current Git commit
    if ! git rev-parse HEAD &> /dev/null; then
        log "ERROR" "Not in a Git repository"
        return 1
    fi
    
    GIT_COMMIT=$(git rev-parse HEAD)
    GIT_SHORT_COMMIT=$(git rev-parse --short=7 HEAD)
    
    # Check for uncommitted changes
    if ! git diff-index --quiet HEAD --; then
        log "WARN" "Uncommitted changes detected in working directory"
        log "WARN" "Deploying commit ${GIT_SHORT_COMMIT} but local files may differ"
        # Don't fail for MVP, but warn
    fi
    
    # Check if commit exists on remote (ensures it passed CI)
    local current_branch=$(git rev-parse --abbrev-ref HEAD)
    
    if ! git branch -r --contains "${GIT_COMMIT}" | grep -q .; then
        log "WARN" "Commit ${GIT_SHORT_COMMIT} not found on any remote branch"
        log "WARN" "This commit may not have passed CI tests"
        # Don't fail for MVP, allow local testing
    else
        log "INFO" "Commit ${GIT_SHORT_COMMIT} found on remote branches"
    fi
    
    log "SUCCESS" "Git commit validation passed: ${GIT_SHORT_COMMIT}"
    return 0
}

# ============================================================================
# DEPLOYMENT LOCK FUNCTIONS (T015)
# ============================================================================

# Generate ULID-like ID with high entropy to prevent collisions
# Format: prefix_timestamp(hex16)_random(hex24)
# Collision probability: ~1 in 2^96 even in same nanosecond
generate_id() {
    local prefix="$1"
    
    # Use nanosecond resolution timestamp (not all systems support %N)
    local timestamp_ns=$(date +%s%N 2>/dev/null || echo "$(date +%s)000000000")
    
    # Convert to hex (16 hex chars = 64 bits)
    local timestamp_hex=$(printf '%016x' "$timestamp_ns")
    
    # Generate 96 bits (24 hex chars) of cryptographic randomness
    # This provides ~2^96 possible values, making collisions astronomically unlikely
    local random=$(openssl rand -hex 12)
    
    echo "${prefix}_${timestamp_hex}_${random}"
}

# acquire_lock - Acquires deployment lock using atomic directory creation
# Implements distributed locking using POSIX-atomic mkdir operation
# Handles stale lock detection with configurable timeout (LOCK_TIMEOUT)
# Coordinates with state file to track lock ownership and metadata
# Args:
#   $1 - environment (e.g., "production", "staging")
# Returns:
#   0 on success (sets DEPLOYMENT_ID global), 1 if lock held by another process
# Side effects:
#   - Creates lock directory: /tmp/togather-deploy-${env}.lock
#   - Sets trap to cleanup lock on exit
#   - Updates state file with lock metadata
# Example:
#   acquire_lock "production" || { echo "Deploy in progress"; exit 1; }
acquire_lock() {
    local env="$1"
    local lock_dir="/tmp/togather-deploy-${env}.lock"
    
    log "INFO" "Acquiring deployment lock for ${env}"
    
    # Check if state file exists, create if this is first deployment
    # Extract environment from state file for lock directory name
    local env=$(jq -r '.environment // ""' "${STATE_FILE}" 2>/dev/null || echo "")
    
    if [[ ! -f "${STATE_FILE}" ]]; then
        log "INFO" "First deployment - creating initial state file"
        mkdir -p "$(dirname "${STATE_FILE}")"
        
        # Create initial empty state file with minimal valid JSON
        cat > "${STATE_FILE}" <<STATE_EOF
{
  "environment": "${env}",
  "lock": {
    "locked": false
  },
  "active_slot": "blue",
  "slots": {
    "blue": {"status": "inactive"},
    "green": {"status": "inactive"}
  },
  "last_deployment": null
}
STATE_EOF
        chmod 600 "${STATE_FILE}"
    fi
    
    # Try to create lock directory atomically (mkdir is atomic in POSIX)
    if mkdir "$lock_dir" 2>/dev/null; then
        # Lock acquired - set trap to cleanup on exit
        log "INFO" "Lock directory created: ${lock_dir}"
    else
        # Lock directory exists - check if stale
        log "INFO" "Lock directory exists, checking if stale"
        
        # Read lock info from state file
        local locked=$(jq -r '.lock.locked // false' "${STATE_FILE}")
        local locked_at=$(jq -r '.lock.locked_at // ""' "${STATE_FILE}")
        
        if [[ "$locked" != "true" ]] || [[ -z "$locked_at" ]]; then
            # Inconsistent state - lock dir exists but state file says unlocked
            log "WARN" "Inconsistent lock state detected"
            log "WARN" "Lock directory exists but state file shows unlocked"
            log "ERROR" "Manual intervention required: rm -rf ${lock_dir}"
            return 1
        fi
        
        # Parse lock timestamp with explicit error handling
        local locked_timestamp=$(date -d "$locked_at" +%s 2>/dev/null || echo "0")
        
        if [[ $locked_timestamp -eq 0 ]]; then
            log "WARN" "Could not parse lock timestamp: ${locked_at}"
            log "ERROR" "Lock may be corrupted, manual intervention required"
            log "ERROR" "To override: rm -rf ${lock_dir} && edit ${STATE_FILE}"
            return 1
        fi
        
        local now_timestamp=$(date +%s)
        local lock_age=$((now_timestamp - locked_timestamp))
        
        if [[ $lock_age -gt $LOCK_TIMEOUT ]]; then
            log "WARN" "Stale lock detected (age: ${lock_age}s > ${LOCK_TIMEOUT}s)"
            log "WARN" "Attempting to remove stale lock"
            
            if rmdir "$lock_dir" 2>/dev/null; then
                log "WARN" "Stale lock removed, retrying acquisition"
                # Retry lock acquisition
                if mkdir "$lock_dir" 2>/dev/null; then
                    log "INFO" "Lock acquired after removing stale lock"
                else
                    log "ERROR" "Failed to acquire lock after removing stale lock"
                    log "ERROR" "Another process may have acquired it first"
                    return 1
                fi
            else
                log "ERROR" "Failed to remove stale lock directory"
                log "ERROR" "Manual intervention required: rm -rf ${lock_dir}"
                return 1
            fi
        else
            local locked_by=$(jq -r '.lock.locked_by // "unknown"' "${STATE_FILE}")
            local deployment_id=$(jq -r '.lock.deployment_id // "unknown"' "${STATE_FILE}")
            log "ERROR" "Deployment already in progress"
            log "ERROR" "Locked by: ${locked_by}"
            log "ERROR" "Deployment ID: ${deployment_id}"
            log "ERROR" "Lock age: ${lock_age}s (timeout: ${LOCK_TIMEOUT}s)"
            log "ERROR" "Lock directory: ${lock_dir}"
            return 1
        fi
    fi
    
    # Generate lock ID and deployment ID
    DEPLOYMENT_ID=$(generate_id "dep")
    local lock_id=$(generate_id "lock")
    local lock_expires_at=$(date -u -d "+30 minutes" +"%Y-%m-%dT%H:%M:%SZ")
    local locked_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Update state file with lock (atomic update with fsync)
    update_state_file_atomic --arg lock_id "$lock_id" \
       --arg deployment_id "$DEPLOYMENT_ID" \
       --arg locked_by "$DEPLOYED_BY" \
       --arg locked_at "$locked_at" \
       --arg lock_expires_at "$lock_expires_at" \
       --arg pid "$$" \
       --arg hostname "$(hostname)" \
       '.lock = {
          locked: true,
          lock_id: $lock_id,
          deployment_id: $deployment_id,
          locked_by: $locked_by,
          locked_at: $locked_at,
          lock_expires_at: $lock_expires_at,
          pid: ($pid | tonumber),
          hostname: $hostname
        }' || {
        log "ERROR" "Failed to update state file with lock"
        rmdir "$lock_dir" 2>/dev/null || true
        return 1
    }
    
    log "SUCCESS" "Deployment lock acquired: ${DEPLOYMENT_ID}"
    return 0
}

# Release deployment lock
release_lock() {
    # Skip lock release if no lock was acquired (e.g., in remote deployment local context)
    if [[ -z "${DEPLOYMENT_ID}" ]]; then
        return 0
    fi

    log "INFO" "Releasing deployment lock"
    
    # Extract environment from state file for lock directory name
    local env=$(jq -r '.environment // ""' "${STATE_FILE}" 2>/dev/null || echo "")
    
    if [[ ! -f "${STATE_FILE}" ]]; then
        log "WARN" "State file not found, cannot release lock"
        return 0
    fi
    
    # Atomic state file update
    update_state_file_atomic '.lock = {locked: false}' || {
        log "WARN" "Failed to update state file when releasing lock"
        return 1
    }
    
    # Remove lock directory
    local lock_dir="/tmp/togather-deploy-${env}.lock"
    if ! rmdir "$lock_dir" 2>/dev/null; then
        log "WARN" "Failed to remove lock directory: ${lock_dir}"
        log "WARN" "Lock state updated but directory may need manual cleanup"
        # Don't fail - state file is already updated
    fi
    
    log "SUCCESS" "Deployment lock released"
    return 0
}

# Ensure lock is released on script exit
trap_exit() {
    local exit_code=$?
    
    if [[ $exit_code -ne 0 ]]; then
        log "ERROR" "Deployment failed with exit code: $exit_code"
    fi
    
    # Release lock
    release_lock
    
    exit $exit_code
}

trap trap_exit EXIT INT TERM

# ============================================================================
# DOCKER BUILD FUNCTIONS (T016)
# ============================================================================

# Validate Docker build arguments format
validate_build_args() {
    local git_commit="$1"
    local build_timestamp="$2"
    local version="$3"
    
    local errors=()
    
    # Validate GIT_COMMIT format (40-character hex string for full commit)
    if [[ -n "${git_commit}" ]] && [[ "${git_commit}" != "unknown" ]]; then
        if ! echo "${git_commit}" | grep -qE '^[0-9a-f]{40}$'; then
            errors+=("GIT_COMMIT must be 40-character hex string (got: ${git_commit})")
        fi
    fi
    
    # Validate BUILD_TIMESTAMP format (ISO8601: YYYY-MM-DDTHH:MM:SSZ)
    if [[ -n "${build_timestamp}" ]] && [[ "${build_timestamp}" != "unknown" ]]; then
        if ! echo "${build_timestamp}" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
            errors+=("BUILD_TIMESTAMP must be ISO8601 format YYYY-MM-DDTHH:MM:SSZ (got: ${build_timestamp})")
        fi
    fi
    
    # Validate VERSION format (semantic version or short commit)
    if [[ -n "${version}" ]] && [[ "${version}" != "dev" ]] && [[ "${version}" != "unknown" ]]; then
        # Accept semver (X.Y.Z) or short commit (7-char hex) or full commit (40-char hex)
        if ! echo "${version}" | grep -qE '^(v?[0-9]+\.[0-9]+\.[0-9]+|[0-9a-f]{7,40})$'; then
            errors+=("VERSION must be semver (X.Y.Z) or git commit hash (got: ${version})")
        fi
    fi
    
    # Report errors if any
    if [[ ${#errors[@]} -gt 0 ]]; then
        log "ERROR" "Docker build argument validation failed:"
        for error in "${errors[@]}"; do
            log "ERROR" "  - ${error}"
        done
        return 1
    fi
    
    log "INFO" "Build arguments validated successfully"
    return 0
}

# Generate web files (robots.txt, sitemap.xml) for deployment
generate_web_files() {
    local env="$1"
    
    log "INFO" "Generating web files for ${env}"
    
    # Determine domain based on environment
    local domain=""
    case "$env" in
        production)
            domain="togather.foundation"
            ;;
        staging)
            domain="staging.toronto.togather.foundation"
            ;;
        development)
            domain="localhost:8080"
            ;;
        *)
            log "ERROR" "Unknown environment: ${env}"
            return 1
            ;;
    esac
    
    log "INFO" "Using domain: ${domain}"
    
    # Build server binary if not present (needed for webfiles command)
    if [[ ! -f "${PROJECT_ROOT}/server" ]]; then
        log "INFO" "Building server binary for webfiles generation"
        cd "${PROJECT_ROOT}"
        if ! make build; then
            log "ERROR" "Failed to build server binary"
            return 1
        fi
    fi
    
    # Generate web files using server CLI
    cd "${PROJECT_ROOT}"
    if ! ./server webfiles --domain "${domain}" --output "${PROJECT_ROOT}/web"; then
        log "ERROR" "Failed to generate web files"
        return 1
    fi
    
    log "SUCCESS" "Web files generated for domain: ${domain}"
    return 0
}

# Build Docker image with version metadata
build_docker_image() {
    local env="$1"
    
    log "INFO" "Building Docker image for ${env}"
    
    local image_name="togather-server"
    local image_tag="${GIT_SHORT_COMMIT}"
    local build_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Determine domain based on environment
    local domain=""
    case "$env" in
        production)
            domain="togather.foundation"
            ;;
        staging)
            domain="staging.toronto.togather.foundation"
            ;;
        development)
            domain="localhost:8080"
            ;;
        *)
            log "WARN" "Unknown environment: ${env}, using default domain"
            domain="togather.foundation"
            ;;
    esac
    
    log "INFO" "Building with domain: ${domain}"
    
    # Validate build arguments before building
    if ! validate_build_args "${GIT_COMMIT}" "${build_timestamp}" "${GIT_SHORT_COMMIT}"; then
        log "ERROR" "Build argument validation failed"
        return 1
    fi
    
    # Build image with version metadata and domain
    cd "${PROJECT_ROOT}"
    
    if ! docker build \
        -f "${DOCKER_DIR}/Dockerfile" \
        -t "${image_name}:${image_tag}" \
        -t "${image_name}:latest" \
        --build-arg GIT_COMMIT="${GIT_COMMIT}" \
        --build-arg GIT_SHORT_COMMIT="${GIT_SHORT_COMMIT}" \
        --build-arg BUILD_TIMESTAMP="${build_timestamp}" \
        --build-arg VERSION="${GIT_SHORT_COMMIT}" \
        --build-arg DOMAIN="${domain}" \
        . ; then
        log "ERROR" "Docker image build failed"
        return 1
    fi
    
    log "SUCCESS" "Docker image built: ${image_name}:${image_tag}"
    return 0
}

# ============================================================================
# DATABASE FUNCTIONS (T017, T018)
# ============================================================================

# Create database snapshot before migrations (T017, T041)
create_db_snapshot() {
    local env="$1"
    
    log "INFO" "Creating database snapshot before migrations"
    
    # Enable snapshot validation by default for production safety
    # This adds ~2-5s but catches corrupt snapshots early
    export VALIDATE_SNAPSHOT="${VALIDATE_SNAPSHOT:-true}"
    
    # Use CLI for snapshot creation
    local server_binary="${PROJECT_ROOT}/server"
    
    if [[ ! -f "${server_binary}" ]]; then
        log "WARN" "server binary not found, skipping automated snapshot"
        log "WARN" "Consider creating a manual database backup before migrations"
        log "WARN" "  pg_dump -U togather -h localhost -p 5433 togather > backup.sql"
        SNAPSHOT_PATH=""
        return 0
    fi
    
    # Create snapshot using CLI
    log "INFO" "Creating database snapshot before deployment"
    
    local snapshot_output
    snapshot_output=$("${server_binary}" snapshot create --reason "pre-deploy-${env}" --format json 2>&1)
    local snapshot_status=$?
    
    if [[ $snapshot_status -ne 0 ]]; then
        log "ERROR" "Database snapshot creation failed: ${snapshot_output}"
        log "ERROR" "Aborting deployment to prevent data loss"
        return 1
    fi
    
    # Extract snapshot path from JSON output
    SNAPSHOT_PATH=$(echo "${snapshot_output}" | jq -r '.snapshot_path // empty')
    
    if [[ -z "${SNAPSHOT_PATH}" ]]; then
        log "WARN" "Could not extract snapshot path from output"
        SNAPSHOT_PATH="unknown"
    fi
    
    log "SUCCESS" "Database snapshot created: ${SNAPSHOT_PATH}"
    return 0
}

# run_migrations - Executes database migrations with locking and rollback support
# Implements safe migration execution with atomic locking to prevent concurrent runs
# Detects dirty migration state and provides detailed recovery instructions
# Validates migration success and updates deployment metadata
# Args:
#   $1 - environment (e.g., "production", "staging")
# Returns:
#   0 on success, 1 on failure (dirty state, lock conflict, or migration error)
# Side effects:
#   - Creates migration lock: /tmp/togather-migration-${env}.lock
#   - Sources .env.${env} to get DATABASE_URL
#   - Runs golang-migrate CLI against database
# Error recovery:
#   On failure, provides rollback instructions referencing snapshot created by create_db_snapshot
# Example:
#   create_db_snapshot production && run_migrations production
run_migrations() {
    local env="$1"
    local migration_lock_dir="/tmp/togather-migration-${env}.lock"
    
    log "INFO" "Executing database migrations"
    
    # Load environment to get DATABASE_URL
    # Use same logic as pre_flight_checks for environment file discovery
    local env_file=""
    
    if [[ -f "${CONFIG_DIR}/environments/.env.${env}" ]]; then
        env_file="${CONFIG_DIR}/environments/.env.${env}"
    elif [[ -f "${PROJECT_ROOT}/deploy/docker/.env" ]]; then
        env_file="${PROJECT_ROOT}/deploy/docker/.env"
    elif [[ -f "${PROJECT_ROOT}/.env" ]]; then
        env_file="${PROJECT_ROOT}/.env"
    else
        log "ERROR" "No environment configuration found for migrations"
        log "ERROR" "Cannot determine DATABASE_URL"
        return 1
    fi
    
    log "INFO" "Sourcing environment from: ${env_file}"
    source "${env_file}"
    
    local migrations_dir="${PROJECT_ROOT}/internal/storage/postgres/migrations"
    
    if [[ ! -d "${migrations_dir}" ]]; then
        log "ERROR" "Migrations directory not found: ${migrations_dir}"
        return 1
    fi
    
    # T032: Acquire migration lock atomically to prevent concurrent migrations
    if mkdir "$migration_lock_dir" 2>/dev/null; then
        # Lock acquired - set trap to cleanup on ALL exit paths
        # Note: RETURN is function-scoped only, removed to ensure cleanup on script exit
        # Cleanup will be handled explicitly at function exit to avoid unbound variable error
        log "INFO" "Migration lock acquired"
    else
        log "ERROR" "Migration lock directory already exists: ${migration_lock_dir}"
        log "ERROR" "Another migration may be in progress"
        log "ERROR" "If stale, remove: rm -rf ${migration_lock_dir}"
        return 1
    fi
    
    # Check current migration version
    local current_version=$(migrate -path "${migrations_dir}" -database "${DATABASE_URL}" version 2>&1 || echo "none")
    log "INFO" "Current migration version: ${current_version}"
    
    # Check for dirty migration state
    if echo "$current_version" | grep -q "dirty"; then
        log "ERROR" "Database is in dirty migration state"
        log "ERROR" "A previous migration failed and left the database in an inconsistent state"
        log "ERROR" "Manual intervention required:"
        log "ERROR" "  1. Review the failed migration in: ${migrations_dir}"
        log "ERROR" "  2. Fix the database manually or restore from snapshot"
        log "ERROR" "  3. Force migration version: migrate -path ${migrations_dir} -database \$DATABASE_URL force <version>"
        rmdir "$migration_lock_dir" 2>/dev/null || true
        return 1
    fi
    
    # Run migrations
    log "INFO" "Running forward migrations..."
    if ! migrate -path "${migrations_dir}" -database "${DATABASE_URL}" up; then
        # T031: Migration failure detected - provide rollback instructions
        log "ERROR" "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log "ERROR" "MIGRATION FAILED"
        log "ERROR" "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log "ERROR" ""
        log "ERROR" "Database migrations encountered an error."
        log "ERROR" "The database may be in an inconsistent state."
        log "ERROR" ""
        log "ERROR" "ROLLBACK OPTIONS:"
        log "ERROR" ""
        log "ERROR" "Option 1: Restore from automatic snapshot (RECOMMENDED)"
        log "ERROR" "  The most recent snapshot was created before this migration."
        log "ERROR" "  To restore:"
        log "ERROR" "    server snapshot list  # Find the latest snapshot"
        log "ERROR" "    # Restore snapshot (requires manual confirmation):"
        log "ERROR" "    psql \$DATABASE_URL < /var/lib/togather/db-snapshots/<snapshot-file>"
        log "ERROR" ""
        log "ERROR" "Option 2: Manual migration rollback"
        log "ERROR" "  1. Check migration status:"
        log "ERROR" "     migrate -path ${migrations_dir} -database \$DATABASE_URL version"
        log "ERROR" "  2. Rollback one migration:"
        log "ERROR" "     migrate -path ${migrations_dir} -database \$DATABASE_URL down 1"
        log "ERROR" ""
        log "ERROR" "Option 3: Fix dirty migration state"
        log "ERROR" "  If the migration left the database in 'dirty' state:"
        log "ERROR" "     migrate -path ${migrations_dir} -database \$DATABASE_URL force <version>"
        log "ERROR" ""
        log "ERROR" "After restoring/fixing, review the failed migration in:"
        log "ERROR" "  ${migrations_dir}"
        log "ERROR" ""
        log "ERROR" "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        
        # Explicit lock cleanup
        rmdir "$migration_lock_dir" 2>/dev/null || true
        return 1
    fi
    
    # Get new migration version
    local new_version=$(migrate -path "${migrations_dir}" -database "${DATABASE_URL}" version 2>&1 || echo "none")
    log "INFO" "New migration version: ${new_version}"
    
    if [[ "$current_version" != "$new_version" ]]; then
        log "SUCCESS" "Database migrations completed successfully"
        log "INFO" "Migrated from version ${current_version} to ${new_version}"
    else
        log "INFO" "No new migrations to apply (already at version ${new_version})"
    fi
    
    # Explicit lock cleanup
    rmdir "$migration_lock_dir" 2>/dev/null || true
    log "INFO" "Migration lock released"
    return 0
}

# ============================================================================
# BLUE-GREEN DEPLOYMENT FUNCTIONS (T019, T021)
# ============================================================================

# Get current active slot (blue or green)
get_active_slot() {
    # Extract environment from state file for lock directory name
    local env=$(jq -r '.environment // ""' "${STATE_FILE}" 2>/dev/null || echo "")
    
    if [[ ! -f "${STATE_FILE}" ]]; then
        echo "blue"  # Default to blue for first deployment
        return 0
    fi
    
    jq -r '.current_deployment.active_slot // "blue"' "${STATE_FILE}"
}

# Get inactive slot (opposite of active)
get_inactive_slot() {
    local active_slot=$(get_active_slot)
    
    if [[ "$active_slot" == "blue" ]]; then
        echo "green"
    else
        echo "blue"
    fi
}

# deploy_to_slot - Deploys application to specified blue/green slot
# Orchestrates container deployment using docker-compose for zero-downtime releases
# Deploys to inactive slot by default, allowing health checks before traffic switch
# Args:
#   $1 - environment (e.g., "production", "staging")
#   $2 - slot (optional: "blue" or "green", defaults to inactive slot)
# Returns:
#   0 on success, 1 on docker-compose failure
# Side effects:
#   - Exports COMPOSE_PROJECT_NAME, DEPLOYMENT_SLOT, IMAGE_TAG, ENV_FILE
#   - Recreates and starts docker container for specified slot with new image
# Example:
#   deploy_to_slot production blue  # Deploy to blue slot explicitly
#   deploy_to_slot production        # Deploy to inactive slot (auto-detected)

# cleanup_slot_orphans - Removes orphaned containers and processes for a slot
# Handles edge case where Docker container failed to start but left processes running
# This can happen when container creation fails but the entrypoint already started
# Args:
#   $1 - slot ("blue" or "green")
# Returns:
#   0 always (cleanup is best-effort, failures are logged but not fatal)
# Side effects:
#   - Removes containers in Created/Dead/Exited state
#   - Kills processes holding ports 8081 (blue) or 8082 (green)
cleanup_slot_orphans() {
    local slot="$1"
    local container_name="togather-server-${slot}"
    local port=8081
    
    if [[ "${slot}" == "green" ]]; then
        port=8082
    fi
    
    # Remove container if it exists in non-running state
    local container_state=$(docker inspect --format='{{.State.Status}}' "${container_name}" 2>/dev/null || echo "")
    if [[ -n "${container_state}" ]] && [[ "${container_state}" != "running" ]]; then
        log "INFO" "Removing ${slot} container in ${container_state} state"
        docker rm -f "${container_name}" 2>/dev/null || true
    fi
    
    # Kill any processes holding the port (safety net for orphaned processes)
    # This handles cases where Docker's process cleanup failed
    local pid=$(lsof -ti tcp:${port} 2>/dev/null || true)
    if [[ -n "${pid}" ]]; then
        log "WARN" "Found process ${pid} holding port ${port}, killing it"
        kill -9 ${pid} 2>/dev/null || true
        sleep 0.5  # Give OS time to release the port
    fi
    
    return 0
}

deploy_to_slot() {
    local env="$1"
    local slot="${2:-$(get_inactive_slot)}"  # Accept slot as parameter or determine it
    
    # Clean up any orphaned containers or processes from previous failed deployments
    cleanup_slot_orphans "${slot}"

    log "INFO" "Deploying to ${slot} slot"
    
    # Use same logic as pre_flight_checks for environment file discovery
    local env_file=""
    
    if [[ -f "${CONFIG_DIR}/environments/.env.${env}" ]]; then
        # Remote deployment: symlinked from /opt/togather/.env.{env}
        env_file="${CONFIG_DIR}/environments/.env.${env}"
    elif [[ -f "${PROJECT_ROOT}/deploy/docker/.env" ]]; then
        # Local Docker deployment
        env_file="${PROJECT_ROOT}/deploy/docker/.env"
    else
        log "ERROR" "No environment configuration found for deployment"
        log "ERROR" "For local: create deploy/docker/.env"
        log "ERROR" "For remote: ensure /opt/togather/.env.${env} exists on server"
        return 1
    fi
    
    log "INFO" "Using environment file: ${env_file}"
    
    local compose_file="${DOCKER_DIR}/docker-compose.blue-green.yml"
    local image_tag="${GIT_SHORT_COMMIT}"
    
    # Set environment variables for docker-compose
    export COMPOSE_PROJECT_NAME="togather-${env}"
    export DEPLOYMENT_SLOT="${slot}"
    export IMAGE_TAG="${image_tag}"
    export ENV_FILE="${env_file}"
    
    # Source .env file to make variables available for docker-compose interpolation
    # Docker Compose requires variables in shell environment for ${VAR} syntax
    set -a; source "${env_file}"; set +a
    
    # Deploy to slot using docker-compose
    cd "${DOCKER_DIR}"
    
    if ! ${COMPOSE_CMD} -f "${compose_file}" up -d --no-deps --force-recreate "togather-${slot}"; then
        log "ERROR" "Failed to deploy to ${slot} slot"
        return 1
    fi
    
    log "SUCCESS" "Deployed to ${slot} slot"
    
    # Ensure monitoring services are running (grafana, prometheus)
    # These are in the 'monitoring' profile and won't start unless explicitly requested
    log "INFO" "Ensuring monitoring services are available..."
    if ${COMPOSE_CMD} -f "${compose_file}" --profile monitoring up -d grafana prometheus 2>/dev/null; then
        log "SUCCESS" "Monitoring services started (Grafana, Prometheus)"
    else
        log "WARN" "Failed to start monitoring services (non-fatal)"
        log "WARN" "Monitoring may not be available in admin UI"
        log "WARN" "To start manually: cd ${DOCKER_DIR} && docker compose --profile monitoring up -d"
    fi
    
    # Note: No Caddy reload needed here (as of T043)
    # Caddyfiles configured with "transport http { keepalive off }" to prevent
    # stale connection pooling. This eliminates the need for manual reloads and
    # ensures zero-downtime static file updates during blue-green deployments.
    
    return 0
}

# switch_traffic - Switches load balancer traffic to new deployment slot
# Implements zero-downtime traffic cutover for blue-green deployments
# Updates Caddy configuration to route traffic to newly deployed slot
# Should only be called after successful health checks on new slot
# Args:
#   $1 - environment (e.g., "production", "staging")
#   $2 - new_slot (optional: "blue" or "green", defaults to inactive slot)
# Returns:
#   0 on success, 1 on Caddy reload failure
# Side effects:
#   - Updates Caddyfile configuration
#   - Reloads Caddy container to apply config changes
# Notes:
#   - This is simplified version; production uses Caddy config templates
#   - Traffic switch is atomic (Caddy reload is zero-downtime)
# Example:
#   validate_health production blue && switch_traffic production blue
# T021: Switch traffic to new slot
switch_traffic() {
    local env="$1"
    local target_slot="$2"
    
    log "INFO" "Switching traffic to ${target_slot} slot"
    
    # Determine target port based on slot
    local target_port=8081
    if [[ "${target_slot}" == "green" ]]; then
        target_port=8082
    fi
    
    # Check if Caddy is running
    if ! systemctl is-active --quiet caddy; then
        log "WARN" "Caddy service not running"
        log "WARN" "Skipping traffic switch (deployments will still work on direct ports)"
        return 0  # Non-fatal - deployments can work without Caddy
    fi

    local caddyfile="/etc/caddy/Caddyfile"
    
    # Check if Caddyfile exists
    if [[ ! -f "${caddyfile}" ]]; then
        log "WARN" "Caddyfile not found at ${caddyfile}"
        log "WARN" "Skipping traffic switch (deployments will still work on direct ports)"
        return 0  # Non-fatal
    fi

    # Track whether we need to reload Caddy
    local needs_reload=false

    # Sync environment Caddyfile if available
    # IMPORTANT: When syncing from repo, preserve the live port/slot settings.
    # The repo Caddyfile has a hardcoded default port (usually 8081/blue), but
    # the live Caddyfile may point to a different slot. Without preserving this,
    # the sync overwrites the live port, and then the "already pointing" check
    # short-circuits without reloading — leaving Caddy's in-memory config stale.
    local caddy_source="${CONFIG_DIR}/environments/Caddyfile.${env}"
    if [[ -f "${caddy_source}" ]]; then
        if ! cmp -s "${caddy_source}" "${caddyfile}" 2>/dev/null; then
            log "INFO" "Syncing Caddyfile from ${caddy_source}"
            
            # Capture current live port/slot before overwriting
            local live_port=$(sed -n '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/p' "${caddyfile}" | grep -oP 'reverse_proxy\s+localhost:\K\d+' | head -1)
            local live_slot=$(sed -n '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/p' "${caddyfile}" | grep -oP 'X-Togather-Slot "\K[^"]+' | head -1)
            
            # Sync the full Caddyfile from repo (picks up structural changes)
            sudo cp "${caddy_source}" "${caddyfile}"
            
            # Restore live port/slot if they were set and differ from repo defaults
            if [[ -n "${live_port}" ]]; then
                sudo sed -i '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/ s/reverse_proxy localhost:[0-9]\+/reverse_proxy localhost:'"${live_port}"'/' "${caddyfile}"
            fi
            if [[ -n "${live_slot}" ]]; then
                sudo sed -i '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/ s/header_down X-Togather-Slot "[^"]*"/header_down X-Togather-Slot "'"${live_slot}"'"/' "${caddyfile}"
            fi
            
            # Caddyfile structure changed — must reload even if port stays the same
            needs_reload=true
            log "INFO" "Caddyfile synced (preserved live port: ${live_port:-unknown}, slot: ${live_slot:-unknown})"
        fi
    else
        log "WARN" "Caddyfile source not found: ${caddy_source}"
        log "WARN" "Skipping Caddyfile sync"
    fi

    if ! sudo caddy validate --config "${caddyfile}" &> /dev/null; then
        log "ERROR" "Caddyfile validation failed before traffic switch"
        return 1
    fi
    
    # Verify markers exist before attempting replacement
    if ! grep -q "# BLUE_GREEN_SLOT_START" "${caddyfile}" || ! grep -q "# BLUE_GREEN_SLOT_END" "${caddyfile}"; then
        log "ERROR" "Caddyfile missing BLUE_GREEN_SLOT markers - cannot update automatically"
        log "ERROR" "Please add markers around the main reverse_proxy block"
        return 1
    fi
    
    # Determine current active port (from blue-green managed section only)
    local current_port=$(sed -n '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/p' "${caddyfile}" | grep -oP 'reverse_proxy\s+localhost:\K\d+' | head -1)
    log "INFO" "Current Caddyfile port: ${current_port}"
    log "INFO" "Target port: ${target_port}"
    
    # Create backup of Caddyfile (always, before any modifications)
    local backup_file="${caddyfile}.backup.$(date +%Y%m%d_%H%M%S)"
    sudo cp "${caddyfile}" "${backup_file}"
    log "INFO" "Created Caddyfile backup: ${backup_file}"
    
    # Update port/slot if Caddyfile doesn't already point to target
    if [[ "${current_port}" != "${target_port}" ]]; then
        # Update Caddyfile with new port and slot name (only within marked section)
        log "INFO" "Updating Caddyfile to point to localhost:${target_port} (${target_slot})"
        sudo sed -i '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/ s/reverse_proxy localhost:[0-9]\+/reverse_proxy localhost:'"${target_port}"'/' "${caddyfile}"
        sudo sed -i '/# BLUE_GREEN_SLOT_START/,/# BLUE_GREEN_SLOT_END/ s/header_down X-Togather-Slot "[^"]*"/header_down X-Togather-Slot "'"${target_slot}"'"/' "${caddyfile}"
        needs_reload=true
    else
        log "INFO" "Caddyfile already pointing to ${target_slot} slot (port ${target_port})"
    fi
    
    # Reload Caddy if any configuration changes were made
    if [[ "$needs_reload" == "true" ]]; then
        # Validate Caddyfile syntax before reload
        if ! sudo caddy validate --config "${caddyfile}" &> /dev/null; then
            log "ERROR" "Caddyfile validation failed after update"
            log "ERROR" "Restoring backup: ${backup_file}"
            sudo cp "${backup_file}" "${caddyfile}"
            return 1
        fi
        
        log "SUCCESS" "Caddyfile validated"
        
        # Reload Caddy using admin API for reliable, synchronous config reload.
        # The admin API (localhost:2019) applies the new config and returns only
        # after the reload is complete, unlike systemctl reload which sends SIGUSR1
        # asynchronously and can return before Caddy actually applies the config.
        log "INFO" "Reloading Caddy configuration..."
        if sudo caddy reload --config "${caddyfile}" --force 2>/dev/null; then
            log "SUCCESS" "Caddy reloaded via admin API"
        else
            # Fallback to systemctl reload if admin API fails (e.g., admin API disabled)
            log "WARN" "Caddy admin API reload failed, falling back to systemctl reload"
            if sudo systemctl reload caddy; then
                log "SUCCESS" "Caddy reloaded via systemctl"
                # systemctl reload is async — give Caddy time to apply config
                sleep 1
            else
                log "ERROR" "Caddy reload failed"
                log "ERROR" "Restoring backup: ${backup_file}"
                sudo cp "${backup_file}" "${caddyfile}"
                sudo systemctl reload caddy
                return 1
            fi
        fi
    else
        log "INFO" "No Caddy configuration changes needed, skipping reload"
    fi
    
    # Always verify traffic is serving the correct version, even if no reload
    # was needed. This catches cases where Caddy's in-memory config diverged
    # from the Caddyfile (e.g., after a failed previous reload).
    log "INFO" "Verifying traffic serves correct version (${GIT_SHORT_COMMIT})..."
    
    # Determine domain from NODE_DOMAIN or construct from env parameter
    local domain="${NODE_DOMAIN:-${env}.toronto.togather.foundation}"
    
    local max_attempts=10
    local attempt=1
    local sleep_duration=2
    
    while [[ $attempt -le $max_attempts ]]; do
        # Check via actual HTTPS endpoint (what users hit)
        local live_version=$(curl -sf "https://${domain}/version" | jq -r '.git_commit // .version // empty' 2>/dev/null || echo "")
        
        if [[ "$live_version" == "$GIT_COMMIT" ]] || [[ "$live_version" == "$GIT_SHORT_COMMIT" ]]; then
            log "SUCCESS" "Traffic verified: serving version ${live_version} from ${target_slot} slot"
            log "INFO" "Caddy is routing https://${domain} to localhost:${target_port}"
            return 0
        fi
        
        log "WARN" "Verification attempt ${attempt}/${max_attempts}: Version mismatch"
        log "WARN" "  Expected: ${GIT_SHORT_COMMIT} (${GIT_COMMIT})"
        log "WARN" "  Got: ${live_version:-<no response>}"
        
        # If no reload was done but version is wrong, Caddy's in-memory config
        # is stale — force a reload to recover
        if [[ "$needs_reload" == "false" ]] && [[ $attempt -eq 3 ]]; then
            log "WARN" "Version mismatch without config change — Caddy in-memory config may be stale"
            log "INFO" "Forcing Caddy reload to recover..."
            if sudo caddy reload --config "${caddyfile}" --force 2>/dev/null; then
                log "INFO" "Recovery reload completed via admin API"
            else
                sudo systemctl reload caddy && sleep 1
                log "INFO" "Recovery reload completed via systemctl"
            fi
        fi
        
        if [[ $attempt -lt $max_attempts ]]; then
            log "INFO" "Waiting ${sleep_duration}s for Caddy to propagate..."
            sleep $sleep_duration
        fi
        
        ((attempt++))
    done
    
    # Verification failed - rollback
    log "ERROR" "Traffic switch verification FAILED after ${max_attempts} attempts"
    log "ERROR" "HTTPS endpoint still serving wrong version"
    log "ERROR" "Rolling back Caddyfile to prevent user-facing issues"
    sudo cp "${backup_file}" "${caddyfile}"
    if sudo caddy reload --config "${caddyfile}" --force 2>/dev/null; then
        log "INFO" "Rollback reload completed via admin API"
    else
        sudo systemctl reload caddy
    fi
    return 1
}

# ============================================================================
# HEALTH CHECK FUNCTIONS (T020)
# ============================================================================

# Validate deployment health
validate_health() {
    local env="$1"
    local slot="$2"
    
    log "INFO" "Validating health of ${slot} deployment"
    
    local wait_script="${SCRIPT_DIR}/wait-for-health.sh"
    local timeout_seconds="${HEALTH_WAIT_TIMEOUT:-60}"
    
    if [[ ! -x "${wait_script}" ]]; then
        log "ERROR" "Health wait script not found or not executable: ${wait_script}"
        return 1
    fi
    
    log "INFO" "Waiting for health (timeout: ${timeout_seconds}s)"
    if ! "${wait_script}" --slot "${slot}" --timeout "${timeout_seconds}"; then
        log "ERROR" "Health check validation failed for ${slot} slot"
        return 1
    fi
    
    log "SUCCESS" "Health check validation passed"
    return 0
}

# ============================================================================
# STATE TRACKING FUNCTIONS (T022)
# ============================================================================

# Update deployment state
update_deployment_state() {
    local env="$1"
    local status="$2"
    local slot=$(get_inactive_slot)
    
    log "INFO" "Updating deployment state: ${status}"
    
    local deployed_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Move current deployment to previous (atomic update with fsync)
    update_state_file_atomic --arg deployment_id "$DEPLOYMENT_ID" \
       --arg version "$GIT_SHORT_COMMIT" \
       --arg git_commit "$GIT_COMMIT" \
       --arg deployed_at "$deployed_at" \
       --arg deployed_by "$DEPLOYED_BY" \
       --arg slot "$slot" \
       --arg status "$status" \
       --arg env "$env" \
       '.previous_deployment = .current_deployment |
        .current_deployment = {
          id: $deployment_id,
          version: $version,
          git_commit: $git_commit,
          deployed_at: $deployed_at,
          deployed_by: $deployed_by,
          active_slot: $slot,
          health_status: "unknown",
          status: $status,
          environment: $env
        }' || return 1
    
    log "SUCCESS" "Deployment state updated"
    
    # Save deployment history
    save_deployment_history "$env" "$status"
    
    return 0
}

# Save deployment to history directory (T041: deployment history tracking)
save_deployment_history() {
    local env="$1"
    local status="$2"
    
    # Create history directory
    sudo mkdir -p "${DEPLOYMENT_HISTORY_DIR}/${env}"
    sudo chown -R "${USER}:${USER}" "${DEPLOYMENT_HISTORY_DIR}" 2>/dev/null || true
    
    local history_file="${DEPLOYMENT_HISTORY_DIR}/${env}/${DEPLOYMENT_ID}.json"
    local image_name="togather-server"
    local image_tag="${GIT_SHORT_COMMIT}"
    local active_slot=$(get_active_slot)
    
    # Create deployment record with all rollback-relevant information
    cat > "${history_file}" <<EOF
{
  "deployment_id": "${DEPLOYMENT_ID}",
  "version": "${GIT_SHORT_COMMIT}",
  "git_commit": "${GIT_COMMIT}",
  "environment": "${env}",
  "deployed_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "deployed_by": "${DEPLOYED_BY}",
  "status": "${status}",
  "log_file": "${DEPLOYMENT_LOG}",
  "docker_image": "${image_name}:${image_tag}",
  "compose_project": "togather-${env}",
  "active_slot": "${active_slot}",
  "snapshot_path": "${SNAPSHOT_PATH:-none}"
}
EOF
    
    log "INFO" "Deployment history saved: ${history_file}"
    
    # Update symlinks for rollback (T041: previous/current tracking)
    # Move current -> previous, then set new current
    local env_dir="${DEPLOYMENT_HISTORY_DIR}/${env}"
    local current_link="${env_dir}/current.json"
    local previous_link="${env_dir}/previous.json"
    
    # If current exists, copy it to previous (not symlink, actual copy for safety)
    if [[ -L "${current_link}" ]]; then
        local current_target=$(readlink "${current_link}")
        if [[ -f "${current_target}" ]]; then
            rm -f "${previous_link}"
            ln -s "$(basename "${current_target}")" "${previous_link}"
            log "INFO" "Previous deployment link updated"
        fi
    fi
    
    # Update current to point to new deployment
    rm -f "${current_link}"
    ln -s "$(basename "${history_file}")" "${current_link}"
    log "INFO" "Current deployment link updated"
    
    return 0
}

# ============================================================================
# MAIN DEPLOYMENT FUNCTION
# ============================================================================

deploy() {
    local env="$1"
    local skip_migrations="${2:-false}"
    local force_deploy="${3:-false}"
    
    log "INFO" "Starting deployment to ${env}"
    log "INFO" "Operator: ${DEPLOYED_BY}"
    
    if [[ "$skip_migrations" == "true" ]]; then
        log "WARN" "Skipping database migrations (--skip-migrations flag)"
    fi
    
    if [[ "$force_deploy" == "true" ]]; then
        log "WARN" "Force deployment mode enabled (--force flag)"
    fi
    
    # T014, T014a, T014b: Validation phase
    validate_config "$env" || return 1
    validate_tool_versions || return 1
    validate_git_commit || return 1
    
    # T015: Acquire deployment lock (skip if --force)
    if [[ "$force_deploy" != "true" ]]; then
        acquire_lock "$env" || return 1
    else
        log "WARN" "Skipping deployment lock (--force flag)"
        DEPLOYMENT_ID="forced_$(date +%s)_${GIT_SHORT_COMMIT}"
    fi
    
    # T016: Build Docker image (web files generated during build via DOMAIN build arg)
    build_docker_image "$env" || return 1
    
    # Pre-flight check: Validate migrations directory exists before snapshot
    # Fail fast to avoid wasting time/storage on unnecessary snapshot
    if [[ "$skip_migrations" != "true" ]]; then
        local migrations_dir="${PROJECT_ROOT}/internal/storage/postgres/migrations"
        if [[ ! -d "${migrations_dir}" ]]; then
            log "ERROR" "Migrations directory not found: ${migrations_dir}"
            log "ERROR" "Cannot proceed with deployment - migrations required"
            return 1
        fi
        log "INFO" "Migrations directory validated: ${migrations_dir}"
    fi
    
    # T017: Create database snapshot (skip if --skip-migrations)
    if [[ "$skip_migrations" != "true" ]]; then
        create_db_snapshot "$env" || return 1
    else
        log "WARN" "Skipping database snapshot (no migrations to run)"
    fi
    
    # T018: Run database migrations (skip if --skip-migrations)
    if [[ "$skip_migrations" != "true" ]]; then
        run_migrations "$env" || return 1
    else
        log "WARN" "Skipping database migrations (--skip-migrations flag)"
    fi
    
    # Determine target slot once to avoid race conditions
    local target_slot=$(get_inactive_slot)
    log "INFO" "Target deployment slot: ${target_slot}"
    
    # T019: Deploy to inactive slot (blue-green)
    deploy_to_slot "$env" "$target_slot" || return 1
    
    # T020: Validate health checks
    validate_health "$env" "$target_slot" || return 1
    
    # T021: Switch traffic to new slot
    switch_traffic "$env" "$target_slot" || return 1
    
    # T022: Update deployment state
    update_deployment_state "$env" "active" || return 1
    
    log "SUCCESS" "Deployment completed successfully"
    log "INFO" "Deployment ID: ${DEPLOYMENT_ID}"
    log "INFO" "Version: ${GIT_SHORT_COMMIT}"
    log "INFO" "Active slot: ${target_slot}"
    
    return 0
}


# ============================================================================
# REMOTE DEPLOYMENT
# ============================================================================

# Deploy to a remote server via SSH
# This function runs on the local machine and orchestrates remote deployment
deploy_remote() {
    local env="$1"
    local remote_host="$2"
    local target_commit="${3:-${GIT_COMMIT}}"
    local repo_dir="/opt/togather/src"
    
    log "INFO" "Remote deployment to ${remote_host}"
    log "INFO" "Target environment: ${env}"
    log "INFO" "Local commit: ${GIT_SHORT_COMMIT}"
    
    # Validate local git state first
    validate_git_commit || return 1
    
    # Auto-detect repository URL from git remote
    local repo_url=$(git remote get-url origin 2>/dev/null)
    if [[ -z "$repo_url" ]]; then
        log "ERROR" "Cannot detect git remote URL"
        log "ERROR" "Run: git remote add origin <url>"
        return 1
    fi
    
    
    log "INFO" "Repository URL: ${repo_url}"
    log "INFO" "Target commit: ${target_commit}"
    
    # Build remote command to execute
    local remote_cmd=$(cat <<'REMOTE_EOF'
set -euo pipefail

REPO_DIR="/opt/togather/src"
REPO_URL="__REPO_URL__"
TARGET_COMMIT="__TARGET_COMMIT__"
ENVIRONMENT="__ENVIRONMENT__"

echo "→ Remote deployment starting..."
echo "  Environment: ${ENVIRONMENT}"
echo "  Repository: ${REPO_URL}"
echo "  Commit: ${TARGET_COMMIT}"
echo ""

# Ensure repo directory exists and is owned by current user
if [ ! -d "${REPO_DIR}/.git" ]; then
    echo "→ Repository not found, cloning to ${REPO_DIR}..."
    sudo mkdir -p "${REPO_DIR}"
    sudo chown ${USER}:${USER} "${REPO_DIR}"
    git clone "${REPO_URL}" "${REPO_DIR}"
    cd "${REPO_DIR}"
else
    echo "→ Repository found, updating..."
    cd "${REPO_DIR}"
    git fetch origin
fi

# Checkout target commit
echo "→ Checking out commit ${TARGET_COMMIT}..."
git checkout "${TARGET_COMMIT}"

# Verify we're on the right commit
CURRENT_COMMIT=$(git rev-parse HEAD)
if [[ "${CURRENT_COMMIT}" != "${TARGET_COMMIT}"* ]]; then
    echo "ERROR: Failed to checkout ${TARGET_COMMIT}"
    echo "Current commit: ${CURRENT_COMMIT}"
    exit 1
fi

# Setup environment configuration
echo "→ Setting up environment configuration..."
CONFIG_DIR="${REPO_DIR}/deploy/config/environments"
PERSISTENT_ENV="/opt/togather/.env.${ENVIRONMENT}"

# Check if persistent env file exists
if [ ! -f "${PERSISTENT_ENV}" ]; then
    # Fall back to shared .env file from install.sh
    if [ -f "/opt/togather/.env" ]; then
        echo "  Using shared environment file: /opt/togather/.env"
        PERSISTENT_ENV="/opt/togather/.env"
    else
        echo "ERROR: No environment configuration found"
        echo ""
        echo "Create one of:"
        echo "  - ${PERSISTENT_ENV} (environment-specific)"
        echo "  - /opt/togather/.env (shared)"
        echo ""
        echo "Copy from example:"
        echo "  cp ${CONFIG_DIR}/.env.${ENVIRONMENT}.example ${PERSISTENT_ENV}"
        echo "  nano ${PERSISTENT_ENV}  # Edit configuration"
        echo "  chmod 600 ${PERSISTENT_ENV}"
        exit 1
    fi
fi

# Symlink persistent env file to expected location
ln -sf "${PERSISTENT_ENV}" "${CONFIG_DIR}/.env.${ENVIRONMENT}"
# Ensure target file has secure permissions
chmod 600 "${PERSISTENT_ENV}"
# Also create symlink at project root for docker-compose.yml compatibility
ln -sf "${PERSISTENT_ENV}" "${REPO_DIR}/.env"
echo "  Linked ${PERSISTENT_ENV} → ${CONFIG_DIR}/.env.${ENVIRONMENT}"


echo "→ Running deploy.sh on remote server..."
echo ""
./deploy/scripts/deploy.sh "${ENVIRONMENT}"
REMOTE_EOF
)
    
    # Substitute variables in remote command
    remote_cmd="${remote_cmd//__REPO_URL__/${repo_url}}"
    remote_cmd="${remote_cmd//__TARGET_COMMIT__/${target_commit}}"
    remote_cmd="${remote_cmd//__ENVIRONMENT__/${env}}"
    
    # Execute remotely via SSH
    log "INFO" "Connecting to ${remote_host}..."
    echo ""
    
    if ssh -t "${remote_host}" "${remote_cmd}"; then
        log "SUCCESS" "Remote deployment completed successfully"
        return 0
    else
        local exit_code=$?
        log "ERROR" "Remote deployment failed (exit code: ${exit_code})"
        return 1
    fi
}

# ============================================================================
# USAGE AND MAIN
# ============================================================================


usage() {
    cat << USAGE_EOF
Usage: ./deploy/scripts/deploy.sh [OPTIONS] ENVIRONMENT

Deploy Togather server using blue-green zero-downtime strategy.

Arguments:
  ENVIRONMENT         Target environment (development, staging, production)

Options:
  --remote USER@HOST  Deploy to remote server via SSH
  --version COMMIT    Deploy specific git commit/tag (default: current HEAD)
  --branch BRANCH     Deploy latest commit from specified branch (resolves to commit hash)
  --dry-run           Validate configuration without deploying
  --env-diff          Run environment variable audit only (compare .env vs template)
  --skip-migrations   Skip database migration execution (use with caution)
  --force             Force deployment even if lock exists or validations fail
  --help              Show this help message

Examples:
  # Deploy current HEAD commit (RECOMMENDED for feature branches)
  ./deploy/scripts/deploy.sh staging --version HEAD

  # Deploy current commit to production (local)
  ./deploy/scripts/deploy.sh production --version $(git rev-parse HEAD)

  # Deploy latest commit from feature branch
  ./deploy/scripts/deploy.sh staging --branch feature/user-administration

  # Deploy to remote staging server with current HEAD
  ./deploy/scripts/deploy.sh staging --remote deploy@staging.example.com --version HEAD

  # Deploy specific version to remote production
  ./deploy/scripts/deploy.sh production --remote deploy@prod.example.com --version v1.2.3

  # Dry-run validation
  ./deploy/scripts/deploy.sh --dry-run staging

  # Check for .env drift (missing/extra variables)
  ./deploy/scripts/deploy.sh staging --env-diff

See: specs/001-deployment-infrastructure/spec.md
USAGE_EOF
}

main() {
    # Parse arguments
    local dry_run=false
    local skip_migrations=false
    local force_deploy=false
    local env_diff_only=false
    local environment=""
    local remote_host=""
    local target_version=""
    local target_branch=""
    
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run)
                dry_run=true
                shift
                ;;
            --skip-migrations)
                skip_migrations=true
                shift
                ;;
            --force)
                force_deploy=true
                shift
                ;;
            --env-diff)
                env_diff_only=true
                shift
                ;;
            --remote)
                if [[ -z "${2:-}" || "${2:-}" == --* ]]; then
                    echo "ERROR: --remote requires user@host argument" >&2
                    exit 1
                fi
                remote_host="$2"
                shift 2
                ;;
            --version)
                if [[ "${2:-}" == "--help" || "${2:-}" == "-h" ]]; then
                    echo "Togather Deployment Script v${SCRIPT_VERSION}"
                    exit 0
                elif [[ -z "${2:-}" || "${2:-}" == --* ]]; then
                    echo "ERROR: --version requires commit/tag argument" >&2
                    exit 1
                fi
                target_version="$2"
                shift 2
                ;;
            --branch)
                if [[ -z "${2:-}" || "${2:-}" == --* ]]; then
                    echo "ERROR: --branch requires branch name argument" >&2
                    exit 1
                fi
                target_branch="$2"
                shift 2
                ;;
            --help)
                usage
                exit 0
                ;;
            -*)
                echo "ERROR: Unknown option: $1" >&2
                echo "" >&2
                usage
                exit 1
                ;;
            *)
                environment="$1"
                shift
                ;;
        esac
    done
    
    # Validate environment argument
    if [[ -z "$environment" ]]; then
        echo "ERROR: ENVIRONMENT argument required" >&2
        echo "" >&2
        usage
        exit 1
    fi
    
    # Check for conflicting options
    if [[ -n "$target_version" && -n "$target_branch" ]]; then
        echo "ERROR: Cannot specify both --version and --branch" >&2
        exit 1
    fi
    
    # Resolve branch to commit hash if --branch specified
    if [[ -n "$target_branch" ]]; then
        echo "→ Resolving branch '${target_branch}' to commit hash"
        target_version=$(git rev-parse "origin/${target_branch}" 2>/dev/null || git rev-parse "${target_branch}" 2>/dev/null)
        if [[ -z "$target_version" ]]; then
            echo "ERROR: Cannot resolve branch: ${target_branch}" >&2
            echo "Make sure the branch exists locally or on origin" >&2
            exit 1
        fi
        echo "  Branch '${target_branch}' resolved to: ${target_version}"
    fi
    
    # Validate environment value
    case "$environment" in
        development|staging|production)
            ;;
        *)
            echo "ERROR: Invalid environment '$environment'" >&2
            echo "Must be one of: development, staging, production" >&2
            echo "" >&2
            usage
            exit 1
            ;;
    esac
    
    # Load deployment configuration if available
    DEPLOY_CONF="${PROJECT_ROOT}/.deploy.conf.${environment}"
    if [[ -f "$DEPLOY_CONF" ]]; then
        echo "→ Loading deployment configuration from ${DEPLOY_CONF}"
        # shellcheck disable=SC1090
        source "$DEPLOY_CONF"
        
        # Export variables for use in this script and subprocesses
        export NODE_DOMAIN SSH_HOST SSH_USER CITY REGION ENVIRONMENT
        export BLUE_GREEN_ENABLED HEALTH_CHECK_TIMEOUT
        export RATE_LIMIT_PUBLIC RATE_LIMIT_AGENT RATE_LIMIT_ADMIN RATE_LIMIT_LOGIN RATE_LIMIT_FEDERATION
        export PERF_ADMIN_API_KEY PERF_AGENT_API_KEY
        
        # Use SSH_HOST for remote_host if not explicitly provided
        if [[ -z "$remote_host" && -n "${SSH_HOST:-}" ]]; then
            if [[ -n "${SSH_USER:-}" ]]; then
                remote_host="${SSH_USER}@${SSH_HOST}"
            else
                remote_host="${SSH_HOST}"
            fi
        fi
        
        echo "  NODE_DOMAIN: ${NODE_DOMAIN:-not set}"
        echo "  Remote host: ${remote_host:-not set}"
    fi
    
    # If --env-diff specified, run env audit only and exit
    # Exit directly via trap removal to avoid "Deployment failed" message for non-zero audit codes
    if [[ "$env_diff_only" == "true" ]]; then
        local audit_script="${SCRIPT_DIR}/env-audit.sh"
        if [[ ! -x "${audit_script}" ]]; then
            echo "ERROR: Environment audit script not found at ${audit_script}" >&2
            exit 1
        fi
        trap - EXIT INT TERM  # Remove deploy trap -- this is not a deploy
        "${audit_script}" "${environment}"
        exit $?
    fi
    
    # If --remote specified, delegate to remote execution
    if [[ -n "$remote_host" ]]; then
        # Initialize logging for local tracking
        init_logging "$environment"
        
        # Validate local git first
        validate_git_commit || exit 1
        
        # Determine target commit
        local target_commit="${GIT_COMMIT}"
        if [[ -n "$target_version" ]]; then
            # Resolve version to commit hash
            target_commit=$(git rev-parse "${target_version}" 2>/dev/null)
            if [[ -z "$target_commit" ]]; then
                log "ERROR" "Cannot resolve version: ${target_version}"
                exit 1
            fi
            log "INFO" "Version override: ${target_version} -> ${target_commit}"
        fi
        
        deploy_remote "$environment" "$remote_host" "$target_commit"
        exit $?
    fi
    
    # Initialize logging
    init_logging "$environment"
    
    # Dry run mode
    if [[ "$dry_run" == "true" ]]; then
        log "INFO" "DRY RUN MODE - No changes will be made"
        validate_config "$environment" || exit 1
        validate_tool_versions || exit 1
        validate_git_commit || exit 1
        log "SUCCESS" "Dry run validation passed"
        exit 0
    fi
    
    # Run deployment
    deploy "$environment" "$skip_migrations" "$force_deploy"
}

# Run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
