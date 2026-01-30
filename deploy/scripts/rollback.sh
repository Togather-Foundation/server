#!/usr/bin/env bash
#
# rollback.sh - Togather Server Rollback Script
#
# Implements deployment rollback by switching traffic to previous version.
# Supports forced rollback and targeting specific versions.
#
# Usage:
#   ./rollback.sh ENVIRONMENT [options]
#
# Arguments:
#   ENVIRONMENT       Target environment: development, staging, or production
#
# Options:
#   --help           Show usage information
#   --force          Skip confirmations and force rollback
#   --version VER    Rollback to specific version (default: previous deployment)
#
# Exit Codes:
#   0   Success - rollback completed successfully
#   1   Configuration error - invalid arguments or rollback failed
#
# Reference: specs/001-deployment-infrastructure/spec.md
#            User Story 5 (US5) - Deployment Rollback

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROJECT_ROOT="$(cd "${DEPLOY_DIR}/.." && pwd)"
CONFIG_DIR="${DEPLOY_DIR}/config"
DOCKER_DIR="${DEPLOY_DIR}/docker"
STATE_FILE="${CONFIG_DIR}/deployment-state.json"
DEPLOYMENT_HISTORY_DIR="/var/lib/togather/deployments"
LOG_DIR="${HOME}/.togather/logs/rollbacks"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Rollback metadata
ROLLBACK_ID=""
ROLLBACK_LOG=""
ROLLBACK_TIMESTAMP=""

# ============================================================================
# LOGGING FUNCTIONS
# ============================================================================

# Initialize logging for this rollback
init_logging() {
    mkdir -p "${LOG_DIR}"
    ROLLBACK_TIMESTAMP=$(date +%Y%m%d_%H%M%S)
    ROLLBACK_ID="rollback_${ROLLBACK_TIMESTAMP}"
    ROLLBACK_LOG="${LOG_DIR}/${ROLLBACK_ID}.log"
    
    # Create log file
    touch "${ROLLBACK_LOG}"
    
    echo "Rollback started at $(date -u +"%Y-%m-%dT%H:%M:%SZ")" | tee -a "${ROLLBACK_LOG}"
    echo "Rollback ID: ${ROLLBACK_ID}" | tee -a "${ROLLBACK_LOG}"
}

# Log message with timestamp and level
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Color based on level
    local color="${NC}"
    case "$level" in
        ERROR)   color="${RED}" ;;
        SUCCESS) color="${GREEN}" ;;
        WARN)    color="${YELLOW}" ;;
        INFO)    color="${BLUE}" ;;
    esac
    
    # Log to file and stdout
    echo "${timestamp} [${level}] ${message}" >> "${ROLLBACK_LOG}"
    echo -e "${color}[${level}]${NC} ${message}"
}

# ============================================================================
# VALIDATION FUNCTIONS
# ============================================================================

# Check prerequisites
check_prerequisites() {
    log "INFO" "Checking prerequisites"
    
    # Check if deployment history directory exists
    if [[ ! -d "${DEPLOYMENT_HISTORY_DIR}" ]]; then
        log "ERROR" "Deployment history directory not found: ${DEPLOYMENT_HISTORY_DIR}"
        log "ERROR" "No deployment history available for rollback"
        return 1
    fi
    
    # Check if docker is available
    if ! command -v docker &> /dev/null; then
        log "ERROR" "docker command not found"
        return 1
    fi
    
    # Check if docker-compose is available
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log "ERROR" "docker-compose not found (neither standalone nor docker compose plugin)"
        return 1
    fi
    
    # Check if jq is available
    if ! command -v jq &> /dev/null; then
        log "ERROR" "jq command not found (required for JSON parsing)"
        return 1
    fi
    
    log "SUCCESS" "Prerequisites check passed"
    return 0
}

# ============================================================================
# ROLLBACK FUNCTIONS
# ============================================================================

# T042, T048: Get previous deployment from history or specific version
get_previous_deployment() {
    local env="$1"
    local target_version="${2:-}"  # Optional: specific version to rollback to
    local env_dir="${DEPLOYMENT_HISTORY_DIR}/${env}"
    
    log "INFO" "Looking for deployment in ${env} environment"
    
    # Check if environment history exists
    if [[ ! -d "${env_dir}" ]]; then
        log "ERROR" "No deployment history found for environment: ${env}"
        return 1
    fi
    
    local target_file=""
    
    # T048: If specific version requested, find it
    if [[ -n "${target_version}" ]]; then
        log "INFO" "Searching for specific version: ${target_version}"
        
        # Search for deployment with matching version
        for deployment_file in "${env_dir}"/*.json; do
            if [[ -f "${deployment_file}" && ! -L "${deployment_file}" ]]; then
                local file_version=$(jq -r '.version' "${deployment_file}" 2>/dev/null)
                if [[ "${file_version}" == "${target_version}" ]]; then
                    target_file="${deployment_file}"
                    log "INFO" "Found deployment for version ${target_version}"
                    break
                fi
            fi
        done
        
        if [[ -z "${target_file}" ]]; then
            log "ERROR" "No deployment found for version: ${target_version}"
            log "ERROR" "Available versions:"
            for deployment_file in "${env_dir}"/*.json; do
                if [[ -f "${deployment_file}" && ! -L "${deployment_file}" ]]; then
                    local ver=$(jq -r '.version' "${deployment_file}" 2>/dev/null)
                    local date=$(jq -r '.deployed_at' "${deployment_file}" 2>/dev/null)
                    log "ERROR" "  - ${ver} (deployed at ${date})"
                fi
            done
            return 1
        fi
    else
        # Use previous deployment link
        local previous_link="${env_dir}/previous.json"
        
        # Check if previous deployment link exists
        if [[ ! -L "${previous_link}" ]]; then
            log "ERROR" "No previous deployment found for environment: ${env}"
            log "ERROR" "Previous deployment link does not exist: ${previous_link}"
            return 1
        fi
        
        # Resolve symlink and check if file exists
        target_file=$(readlink -f "${previous_link}")
        if [[ ! -f "${target_file}" ]]; then
            log "ERROR" "Previous deployment file not found: ${target_file}"
            return 1
        fi
        
        log "INFO" "Previous deployment found: $(basename "${target_file}")"
    fi
    
    # Parse and display deployment info
    local prev_version=$(jq -r '.version' "${target_file}")
    local prev_deployed_at=$(jq -r '.deployed_at' "${target_file}")
    local prev_deployed_by=$(jq -r '.deployed_by' "${target_file}")
    local prev_status=$(jq -r '.status' "${target_file}")
    
    log "INFO" "  Version: ${prev_version}"
    log "INFO" "  Deployed at: ${prev_deployed_at}"
    log "INFO" "  Deployed by: ${prev_deployed_by}"
    log "INFO" "  Status: ${prev_status}"
    
    # Return the path to target deployment file
    echo "${target_file}"
    return 0
}

# Get current deployment info
get_current_deployment() {
    local env="$1"
    local env_dir="${DEPLOYMENT_HISTORY_DIR}/${env}"
    local current_link="${env_dir}/current.json"
    
    if [[ ! -L "${current_link}" ]]; then
        log "WARN" "No current deployment link found"
        return 1
    fi
    
    local current_file=$(readlink -f "${current_link}")
    if [[ ! -f "${current_file}" ]]; then
        log "WARN" "Current deployment file not found: ${current_file}"
        return 1
    fi
    
    echo "${current_file}"
    return 0
}

# T043: Switch Docker image to previous version
switch_docker_image() {
    local env="$1"
    local previous_deployment_file="$2"
    
    log "INFO" "Switching Docker image to previous version"
    
    # Extract deployment info from previous deployment
    local docker_image=$(jq -r '.docker_image' "${previous_deployment_file}")
    local compose_project=$(jq -r '.compose_project' "${previous_deployment_file}")
    local git_commit=$(jq -r '.git_commit' "${previous_deployment_file}")
    local prev_slot=$(jq -r '.active_slot' "${previous_deployment_file}")
    
    if [[ -z "${docker_image}" || "${docker_image}" == "null" ]]; then
        log "ERROR" "Docker image not found in previous deployment"
        return 1
    fi
    
    log "INFO" "Target Docker image: ${docker_image}"
    log "INFO" "Target slot: ${prev_slot}"
    
    # Check if Docker image exists locally
    if ! docker image inspect "${docker_image}" &> /dev/null; then
        log "WARN" "Docker image not found locally: ${docker_image}"
        log "INFO" "Attempting to rebuild from Git commit: ${git_commit}"
        
        # Try to rebuild the image from the previous commit
        cd "${PROJECT_ROOT}"
        
        # Checkout previous commit (detached HEAD)
        if ! git checkout "${git_commit}" 2>&1 | tee -a "${ROLLBACK_LOG}"; then
            log "ERROR" "Failed to checkout Git commit: ${git_commit}"
            return 1
        fi
        
        # Extract image name and tag
        local image_name="${docker_image%:*}"
        local image_tag="${docker_image##*:}"
        
        # Rebuild image
        if ! docker build \
            -f "${DOCKER_DIR}/Dockerfile" \
            -t "${docker_image}" \
            --build-arg GIT_COMMIT="${git_commit}" \
            --build-arg GIT_SHORT_COMMIT="${image_tag}" \
            . 2>&1 | tee -a "${ROLLBACK_LOG}"; then
            log "ERROR" "Failed to rebuild Docker image"
            # Return to original branch
            git checkout - &> /dev/null || true
            return 1
        fi
        
        # Return to original branch
        git checkout - &> /dev/null || true
        
        log "SUCCESS" "Docker image rebuilt: ${docker_image}"
    else
        log "SUCCESS" "Docker image found locally: ${docker_image}"
    fi
    
    return 0
}

# Get current active slot
get_active_slot() {
    if [[ ! -f "${STATE_FILE}" ]]; then
        echo "blue"  # Default to blue
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

# T044: Deploy previous version to inactive slot and switch traffic
rollback_deployment() {
    local env="$1"
    local previous_deployment_file="$2"
    
    log "INFO" "Rolling back deployment"
    
    # Extract deployment configuration
    local docker_image=$(jq -r '.docker_image' "${previous_deployment_file}")
    local image_tag="${docker_image##*:}"
    local env_file="${CONFIG_DIR}/environments/.env.${env}"
    local compose_file="${DOCKER_DIR}/docker-compose.blue-green.yml"
    
    # Determine target slot (opposite of current)
    local target_slot=$(get_inactive_slot)
    
    log "INFO" "Deploying previous version to ${target_slot} slot"
    
    # Set environment variables for docker-compose
    export COMPOSE_PROJECT_NAME="togather-${env}"
    export DEPLOYMENT_SLOT="${target_slot}"
    export IMAGE_TAG="${image_tag}"
    export ENV_FILE="${env_file}"
    
    # Deploy to target slot
    cd "${DOCKER_DIR}"
    
    if ! docker-compose -f "${compose_file}" up -d "${target_slot}" 2>&1 | tee -a "${ROLLBACK_LOG}"; then
        log "ERROR" "Failed to deploy previous version to ${target_slot} slot"
        return 1
    fi
    
    log "SUCCESS" "Previous version deployed to ${target_slot} slot"
    
    # Wait a few seconds for container to initialize
    log "INFO" "Waiting 5 seconds for container initialization"
    sleep 5
    
    return 0
}

# T045: Validate health checks after rollback
validate_rollback_health() {
    local env="$1"
    local slot="$2"
    
    log "INFO" "Validating health checks for ${slot} slot"
    
    # Use health-check.sh script if available
    local health_check_script="${SCRIPT_DIR}/health-check.sh"
    
    if [[ ! -f "${health_check_script}" ]]; then
        log "WARN" "health-check.sh not found, skipping health validation"
        log "WARN" "Manual health check recommended"
        return 0
    fi
    
    # Run health checks
    if ! bash "${health_check_script}" "${env}" "${slot}" 2>&1 | tee -a "${ROLLBACK_LOG}"; then
        log "ERROR" "Health checks failed for ${slot} slot"
        log "ERROR" "Rollback may not be successful - manual verification required"
        return 1
    fi
    
    log "SUCCESS" "Health checks passed for ${slot} slot"
    return 0
}

# Switch traffic to rolled-back slot
switch_traffic() {
    local env="$1"
    local target_slot="$2"
    
    log "INFO" "Switching traffic to ${target_slot} slot"
    
    local nginx_container="togather-${env}-nginx"
    
    # Check if nginx container exists
    if ! docker ps -q -f name="${nginx_container}" | grep -q .; then
        log "WARN" "Nginx container not found: ${nginx_container}"
        log "WARN" "Skipping traffic switch (direct access only)"
        return 0
    fi
    
    # Reload nginx configuration
    if ! docker exec "${nginx_container}" nginx -s reload 2>&1 | tee -a "${ROLLBACK_LOG}"; then
        log "ERROR" "Failed to reload nginx configuration"
        return 1
    fi
    
    log "SUCCESS" "Traffic switched to ${target_slot} slot"
    return 0
}

# Update deployment state after rollback
update_deployment_state() {
    local env="$1"
    local slot="$2"
    local previous_deployment_file="$3"
    
    log "INFO" "Updating deployment state"
    
    # Extract info from previous deployment
    local version=$(jq -r '.version' "${previous_deployment_file}")
    local git_commit=$(jq -r '.git_commit' "${previous_deployment_file}")
    local deployment_id=$(jq -r '.deployment_id' "${previous_deployment_file}")
    
    # Create or update state file
    mkdir -p "$(dirname "${STATE_FILE}")"
    
    cat > "${STATE_FILE}" <<EOF
{
  "current_deployment": {
    "deployment_id": "${deployment_id}",
    "version": "${version}",
    "git_commit": "${git_commit}",
    "environment": "${env}",
    "active_slot": "${slot}",
    "deployed_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
    "deployed_by": "${USER}@$(hostname)",
    "rollback": true,
    "rollback_id": "${ROLLBACK_ID}"
  }
}
EOF
    
    log "SUCCESS" "Deployment state updated"
    return 0
}

# ============================================================================
# MAIN ROLLBACK FUNCTION
# ============================================================================

# T047, T048, T049: Rollback with optional flags
rollback() {
    local env="$1"
    local force_rollback="${2:-false}"
    local target_version="${3:-}"
    
    log "INFO" "Starting rollback for environment: ${env}"
    
    if [[ "${force_rollback}" == "true" ]]; then
        log "WARN" "Force mode enabled - skipping interactive confirmation"
    fi
    
    if [[ -n "${target_version}" ]]; then
        log "INFO" "Target version: ${target_version}"
    fi
    
    # Check prerequisites
    check_prerequisites || return 1
    
    # T042, T048: Get previous deployment or specific version
    local previous_deployment_file
    previous_deployment_file=$(get_previous_deployment "${env}" "${target_version}") || return 1
    
    # Show current deployment for comparison
    log "INFO" "Current deployment:"
    local current_deployment_file
    current_deployment_file=$(get_current_deployment "${env}") || log "WARN" "No current deployment found"
    
    if [[ -n "${current_deployment_file}" ]]; then
        local curr_version=$(jq -r '.version' "${current_deployment_file}")
        local curr_deployed_at=$(jq -r '.deployed_at' "${current_deployment_file}")
        log "INFO" "  Version: ${curr_version}"
        log "INFO" "  Deployed at: ${curr_deployed_at}"
    fi
    
    # T049: Check for database snapshot and provide restore instructions
    local snapshot_path=$(jq -r '.snapshot_path' "${previous_deployment_file}")
    if [[ -n "${snapshot_path}" && "${snapshot_path}" != "null" && "${snapshot_path}" != "none" ]]; then
        echo ""
        log "INFO" "Database snapshot available: ${snapshot_path}"
        log "WARN" "If database migrations were applied after this deployment, you may need to restore the database"
        log "INFO" "To restore database snapshot, run:"
        log "INFO" "  pg_restore -d \$DATABASE_URL --clean --if-exists ${snapshot_path}"
        log "WARN" "Database restore requires manual confirmation and should be done carefully"
        echo ""
    else
        log "WARN" "No database snapshot associated with this deployment"
        log "WARN" "If schema changes occurred, manual database migration may be required"
    fi
    
    # T046, T047: Confirm rollback (skip if --force)
    if [[ "${force_rollback}" != "true" ]]; then
        echo ""
        echo -e "${YELLOW}WARNING: This will rollback the deployment to the target version.${NC}"
        echo -e "${YELLOW}Current version will be stopped and replaced.${NC}"
        echo ""
        read -p "Do you want to continue? (yes/no): " confirm
        
        if [[ "${confirm}" != "yes" ]]; then
            log "INFO" "Rollback cancelled by user"
            return 1
        fi
    fi
    
    # T043: Switch Docker image
    switch_docker_image "${env}" "${previous_deployment_file}" || return 1
    
    # T044: Deploy to inactive slot
    rollback_deployment "${env}" "${previous_deployment_file}" || return 1
    
    # Determine which slot we deployed to
    local target_slot=$(get_inactive_slot)
    
    # T045: Validate health
    validate_rollback_health "${env}" "${target_slot}" || return 1
    
    # Switch traffic
    switch_traffic "${env}" "${target_slot}" || return 1
    
    # Update state
    update_deployment_state "${env}" "${target_slot}" "${previous_deployment_file}" || return 1
    
    log "SUCCESS" "Rollback completed successfully"
    log "INFO" "Rollback ID: ${ROLLBACK_ID}"
    log "INFO" "Rollback log: ${ROLLBACK_LOG}"
    
    # Remind about database snapshot if available
    if [[ -n "${snapshot_path}" && "${snapshot_path}" != "null" && "${snapshot_path}" != "none" ]]; then
        echo ""
        log "WARN" "Don't forget to restore database snapshot if needed:"
        log "INFO" "  pg_restore -d \$DATABASE_URL --clean --if-exists ${snapshot_path}"
    fi
    
    return 0
}

# ============================================================================
# USAGE AND MAIN
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 [OPTIONS] ENVIRONMENT

Rollback Togather server to previous deployment version.

Arguments:
  ENVIRONMENT         Target environment (development, staging, production)

Options:
  --force             Skip interactive confirmation prompt (T047)
  --version VERSION   Rollback to specific version instead of previous (T048)
  --help              Show this help message and exit

Examples:
  $0 development              Rollback to previous version (interactive)
  $0 production --force       Rollback without confirmation
  $0 staging --version abc123 Rollback to specific Git commit version

Notes:
  - Requires previous deployment history in ${DEPLOYMENT_HISTORY_DIR}
  - Uses blue-green deployment strategy for zero-downtime rollback
  - Validates health checks before completing rollback
  - Logs are saved to ${LOG_DIR}
  - Database snapshots are shown but must be restored manually (T049)

Database Restore (T049):
  If the target deployment has an associated database snapshot, you will be
  shown instructions to restore it. Database restore requires manual confirmation
  and should be done carefully to avoid data loss.
  
  Example restore command:
    pg_restore -d \$DATABASE_URL --clean --if-exists /path/to/snapshot.pgdump

EOF
}

main() {
    # Parse arguments
    if [[ $# -eq 0 ]]; then
        usage
        exit 1  # Configuration error
    fi
    
    local env=""
    local force_rollback="false"
    local target_version=""
    
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help)
                usage
                exit 0  # Success
                ;;
            --force)
                force_rollback="true"
                shift
                ;;
            --version)
                if [[ -z "${2:-}" || "${2}" == --* ]]; then
                    echo "ERROR: --version requires a version argument" >&2
                    echo "" >&2
                    usage
                    exit 1  # Configuration error
                fi
                target_version="$2"
                shift 2
                ;;
            -*)
                echo "ERROR: Unknown option: $1" >&2
                echo "" >&2
                usage
                exit 1  # Configuration error
                ;;
            *)
                env="$1"
                shift
                ;;
        esac
    done
    
    # Validate environment
    if [[ -z "${env}" ]]; then
        echo "ERROR: ENVIRONMENT argument required" >&2
        echo "" >&2
        usage
        exit 1  # Configuration error
    fi
    
    # Initialize logging
    init_logging
    
    # Execute rollback with flags
    if rollback "${env}" "${force_rollback}" "${target_version}"; then
        exit 0  # Success
    else
        log "ERROR" "Rollback failed"
        exit 1  # Rollback operation failed
    fi
}

# Run main if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
