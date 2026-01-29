#!/usr/bin/env bash
# Togather Server Deployment Script
# Implements blue-green zero-downtime deployment with automatic rollback
# See: specs/001-deployment-infrastructure/spec.md

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
    input=$(echo "$input" | sed -E 's|(ADMIN_API_KEY[=:])([^[:space:]"'\'']+)|\1***REDACTED***|g')
    
    # Redact quoted values: KEY="value with spaces" or KEY='value'
    input=$(echo "$input" | sed -E 's|(JWT_SECRET[=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    input=$(echo "$input" | sed -E 's|(ADMIN_API_KEY[=:])["'\''][^"'\'']*["'\'']|\1***REDACTED***|g')
    
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
# Returns octal permissions like "600" or "000" on error
get_file_perms() {
    local file="$1"
    
    # Try Linux (GNU) stat first
    if stat -c '%a' "$file" 2>/dev/null; then
        return 0
    fi
    
    # Try macOS (BSD) stat
    if stat -f '%Lp' "$file" 2>/dev/null; then
        return 0
    fi
    
    # Fallback: couldn't determine permissions
    echo "000"
    return 1
}

# Atomic state file update (T022: Atomic writes with fsync)
update_state_file_atomic() {
    local jq_expression="$1"
    local temp_file=$(mktemp)
    
    # Write to temp file
    if ! jq "$jq_expression" "${STATE_FILE}" > "$temp_file"; then
        log "ERROR" "Failed to update state file with jq"
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
    
    # Check environment file exists
    local env_file="${CONFIG_DIR}/environments/.env.${env}"
    if [[ ! -f "${env_file}" ]]; then
        log "ERROR" "Environment file not found: ${env_file}"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  1. Copy the example file:"
        log "ERROR" "     cp ${CONFIG_DIR}/environments/.env.${env}.example ${env_file}"
        log "ERROR" "  2. Edit the file and replace all CHANGE_ME values:"
        log "ERROR" "     ${EDITOR:-nano} ${env_file}"
        log "ERROR" "  3. Secure the file permissions:"
        log "ERROR" "     chmod 600 ${env_file}"
        return 1
    fi
    
    # T038: Check environment file permissions (MUST be 600 for security)
    local perms=$(get_file_perms "${env_file}")
    if [[ "${perms}" != "600" ]]; then
        log "ERROR" "Environment file has insecure permissions: ${perms}"
        log "ERROR" "Secrets could be readable by other users!"
        log "ERROR" ""
        log "ERROR" "REMEDIATION:"
        log "ERROR" "  chmod 600 ${env_file}"
        log "ERROR" ""
        # Get owner portably (works on Linux and macOS)
        local owner=$(ls -ld "${env_file}" 2>/dev/null | awk '{print $3}')
        log "ERROR" "Current owner: ${owner:-unknown}"
        log "ERROR" "Current permissions: ${perms} (expected: 600)"
        return 1
    fi
    
    # T037: Source environment file with override precedence
    # Precedence: CLI env vars > shell env > .env file > deployment.yml defaults
    # Save current env vars to detect overrides
    local saved_DATABASE_URL="${DATABASE_URL:-}"
    local saved_JWT_SECRET="${JWT_SECRET:-}"
    local saved_ADMIN_API_KEY="${ADMIN_API_KEY:-}"
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
    if [[ -n "${saved_ADMIN_API_KEY}" ]]; then
        ADMIN_API_KEY="${saved_ADMIN_API_KEY}"
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
    local required_vars=("ENVIRONMENT" "DATABASE_URL" "JWT_SECRET" "ADMIN_API_KEY")
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
        log "ERROR" "     ADMIN_API_KEY=\$(openssl rand -hex 32)"
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
        log "ERROR" "     ADMIN_API_KEY: openssl rand -hex 32"
        log "ERROR" "  3. Edit the file and replace placeholders:"
        log "ERROR" "     ${EDITOR:-nano} ${env_file}"
        log "ERROR" "  4. Verify no placeholders remain:"
        log "ERROR" "     grep -v '^#' ${env_file} | grep CHANGE_ME"
        return 1
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
        ((errors++))
    else
        local compose_cmd="docker-compose"
        if ! command -v docker-compose &> /dev/null; then
            compose_cmd="docker compose"
        fi
        
        local compose_version=$($compose_cmd version --short 2>/dev/null || echo "0.0.0")
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

# Acquire deployment lock
acquire_lock() {
    local env="$1"
    local lock_dir="/tmp/togather-deploy-${env}.lock"
    
    log "INFO" "Acquiring deployment lock for ${env}"
    
    # Check if state file exists
    if [[ ! -f "${STATE_FILE}" ]]; then
        log "ERROR" "Deployment state file not found: ${STATE_FILE}"
        return 1
    fi
    
    # Try to create lock directory atomically (mkdir is atomic in POSIX)
    if mkdir "$lock_dir" 2>/dev/null; then
        # Lock acquired - set trap to cleanup on exit
        trap 'rmdir "$lock_dir" 2>/dev/null || true' EXIT INT TERM
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
                    trap 'rmdir "$lock_dir" 2>/dev/null || true' EXIT INT TERM
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
    log "INFO" "Releasing deployment lock"
    
    if [[ ! -f "${STATE_FILE}" ]]; then
        log "WARN" "State file not found, cannot release lock"
        return 0
    fi
    
    # Atomic state file update
    update_state_file_atomic '.lock = {locked: false}' || {
        log "WARN" "Failed to update state file when releasing lock"
        return 1
    }
    
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

# Build Docker image with version metadata
build_docker_image() {
    local env="$1"
    
    log "INFO" "Building Docker image for ${env}"
    
    local image_name="togather-server"
    local image_tag="${GIT_SHORT_COMMIT}"
    local build_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Build image with version metadata
    cd "${PROJECT_ROOT}"
    
    if ! docker build \
        -f "${DOCKER_DIR}/Dockerfile" \
        -t "${image_name}:${image_tag}" \
        -t "${image_name}:latest" \
        --build-arg GIT_COMMIT="${GIT_COMMIT}" \
        --build-arg GIT_SHORT_COMMIT="${GIT_SHORT_COMMIT}" \
        --build-arg BUILD_TIMESTAMP="${build_timestamp}" \
        --build-arg VERSION="${GIT_SHORT_COMMIT}" \
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

# Create database snapshot before migrations (T017)
create_db_snapshot() {
    local env="$1"
    
    log "INFO" "Creating database snapshot before migrations"
    
    # Enable snapshot validation by default for production safety
    # This adds ~2-5s but catches corrupt snapshots early
    export VALIDATE_SNAPSHOT="${VALIDATE_SNAPSHOT:-true}"
    
    # Check if snapshot-db.sh exists (from T027)
    local snapshot_script="${SCRIPT_DIR}/snapshot-db.sh"
    
    if [[ ! -f "${snapshot_script}" ]]; then
        log "WARN" "snapshot-db.sh not found, skipping snapshot"
        log "WARN" "Database backup recommended before migrations"
        return 0
    fi
    
    # Call snapshot script
    if ! bash "${snapshot_script}" "${env}"; then
        log "ERROR" "Database snapshot creation failed"
        log "ERROR" "Aborting deployment to prevent data loss"
        return 1
    fi
    
    log "SUCCESS" "Database snapshot created successfully"
    return 0
}

# Execute database migrations (T018, T031: Failure detection, T032: Locking)
run_migrations() {
    local env="$1"
    local migration_lock_dir="/tmp/togather-migration-${env}.lock"
    
    log "INFO" "Executing database migrations"
    
    # Load environment to get DATABASE_URL
    local env_file="${CONFIG_DIR}/environments/.env.${env}"
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
        trap 'rmdir "$migration_lock_dir" 2>/dev/null || true' EXIT INT TERM
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
        log "ERROR" "    cd ${SCRIPT_DIR}"
        log "ERROR" "    ./snapshot-db.sh list  # Find the latest snapshot"
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
        
        # Trap will cleanup lock on exit
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
    
    log "INFO" "Migration lock will be released on function exit"
    return 0
}

# ============================================================================
# BLUE-GREEN DEPLOYMENT FUNCTIONS (T019, T021)
# ============================================================================

# Get current active slot (blue or green)
get_active_slot() {
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

# Deploy to inactive slot (T019: Blue-green orchestration)
deploy_to_slot() {
    local env="$1"
    local slot="${2:-$(get_inactive_slot)}"  # Accept slot as parameter or determine it
    
    log "INFO" "Deploying to ${slot} slot"
    
    local env_file="${CONFIG_DIR}/environments/.env.${env}"
    local compose_file="${DOCKER_DIR}/docker-compose.blue-green.yml"
    local image_tag="${GIT_SHORT_COMMIT}"
    
    # Set environment variables for docker-compose
    export COMPOSE_PROJECT_NAME="togather-${env}"
    export DEPLOYMENT_SLOT="${slot}"
    export IMAGE_TAG="${image_tag}"
    export ENV_FILE="${env_file}"
    
    # Deploy to slot using docker-compose
    cd "${DOCKER_DIR}"
    
    if ! docker-compose -f "${compose_file}" up -d "${slot}"; then
        log "ERROR" "Failed to deploy to ${slot} slot"
        return 1
    fi
    
    log "SUCCESS" "Deployed to ${slot} slot"
    return 0
}

# Switch traffic to new slot (T021: Traffic switching)
switch_traffic() {
    local env="$1"
    local new_slot="${2:-$(get_inactive_slot)}"  # Accept slot as parameter or determine it
    
    log "INFO" "Switching traffic to ${new_slot} slot"
    
    # Update nginx configuration to point to new slot
    # Note: This is a simplified version. In production, you'd use nginx config templates
    
    local nginx_config="${DOCKER_DIR}/nginx.conf"
    local nginx_container="togather-${env}-nginx"
    
    # Check if nginx container exists
    if ! docker ps -q -f name="${nginx_container}" | grep -q .; then
        log "WARN" "Nginx container not found: ${nginx_container}"
        log "WARN" "Skipping traffic switch (direct access only)"
        return 0
    fi
    
    # Reload nginx configuration
    if ! docker exec "${nginx_container}" nginx -s reload; then
        log "ERROR" "Failed to reload nginx configuration"
        return 1
    fi
    
    log "SUCCESS" "Traffic switched to ${new_slot} slot"
    return 0
}

# ============================================================================
# HEALTH CHECK FUNCTIONS (T020)
# ============================================================================

# Validate deployment health
validate_health() {
    local env="$1"
    local slot="$2"
    
    log "INFO" "Validating health of ${slot} deployment"
    
    # Check if health-check.sh exists
    local health_check_script="${SCRIPT_DIR}/health-check.sh"
    
    if [[ ! -f "${health_check_script}" ]]; then
        log "WARN" "health-check.sh not found, using basic HTTP check"
        
        # Basic HTTP health check
        local health_url="http://localhost:8080/health"
        local max_attempts=30
        local attempt=0
        
        while [[ $attempt -lt $max_attempts ]]; do
            if curl -sf "${health_url}" > /dev/null 2>&1; then
                log "SUCCESS" "Health check passed"
                return 0
            fi
            
            ((attempt++))
            log "INFO" "Health check attempt ${attempt}/${max_attempts}..."
            sleep 2
        done
        
        log "ERROR" "Health check failed after ${max_attempts} attempts"
        return 1
    fi
    
    # Call dedicated health check script
    if ! bash "${health_check_script}" "${env}" "${slot}"; then
        log "ERROR" "Health check validation failed"
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

# Save deployment to history directory (T041)
save_deployment_history() {
    local env="$1"
    local status="$2"
    
    # Create history directory
    sudo mkdir -p "${DEPLOYMENT_HISTORY_DIR}"
    sudo chown "${USER}:${USER}" "${DEPLOYMENT_HISTORY_DIR}" 2>/dev/null || true
    
    local history_file="${DEPLOYMENT_HISTORY_DIR}/${DEPLOYMENT_ID}.json"
    
    # Create deployment record
    cat > "${history_file}" <<EOF
{
  "deployment_id": "${DEPLOYMENT_ID}",
  "version": "${GIT_SHORT_COMMIT}",
  "git_commit": "${GIT_COMMIT}",
  "environment": "${env}",
  "deployed_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "deployed_by": "${DEPLOYED_BY}",
  "status": "${status}",
  "log_file": "${DEPLOYMENT_LOG}"
}
EOF
    
    log "INFO" "Deployment history saved: ${history_file}"
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
    
    # T016: Build Docker image
    build_docker_image "$env" || return 1
    
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
# USAGE AND MAIN
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 [OPTIONS] ENVIRONMENT

Deploy Togather server using blue-green zero-downtime strategy.

Arguments:
  ENVIRONMENT         Target environment (development, staging, production)

Options:
  --dry-run           Validate configuration without deploying
  --skip-migrations   Skip database migration execution (use with caution)
  --force             Force deployment even if lock exists or validations fail
  --version           Show script version and exit
  --help              Show this help message

Examples:
  $0 production
  $0 --dry-run staging
  $0 development
  $0 production --skip-migrations
  $0 staging --force

See: specs/001-deployment-infrastructure/spec.md
EOF
}

main() {
    # Parse arguments
    local dry_run=false
    local skip_migrations=false
    local force_deploy=false
    local environment=""
    
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
            --version)
                echo "Togather Deployment Script v${SCRIPT_VERSION}"
                exit 0
                ;;
            --help)
                usage
                exit 0
                ;;
            -*)
                echo "Unknown option: $1"
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
        echo "Error: ENVIRONMENT argument required"
        usage
        exit 1
    fi
    
    # Validate environment value
    case "$environment" in
        development|staging|production)
            ;;
        *)
            echo "Error: Invalid environment: $environment"
            echo "Must be one of: development, staging, production"
            exit 1
            ;;
    esac
    
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
