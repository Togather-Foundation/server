#!/usr/bin/env bash
# restart.sh - Quick restart script for Togather server
#
# Provides fast restart without full rebuild/deploy cycle.
# Useful for:
# - Applying .env changes
# - Quick iteration during development
# - Restarting after configuration updates
#
# Usage:
#   ./restart.sh [OPTIONS] [ENVIRONMENT]
#
# Arguments:
#   ENVIRONMENT     Target environment (local, staging, production)
#                   Default: local
#
# Options:
#   --slot SLOT     Restart specific slot (blue, green, both)
#                   Default: active slot only
#   --hard          Hard restart (stop + start) instead of graceful
#   --remote HOST   Restart on remote server via SSH
#   --help          Show this help message
#
# Examples:
#   # Restart local development server
#   ./restart.sh local
#
#   # Restart staging server (active slot)
#   ./restart.sh staging --remote deploy@staging.example.com
#
#   # Restart both blue and green slots on staging
#   ./restart.sh staging --slot both --remote deploy@staging.example.com
#
#   # Hard restart (stop + start) staging green slot
#   ./restart.sh staging --slot green --hard --remote deploy@staging.example.com

set -euo pipefail

# Script directory (handle both local execution and remote piped execution)
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
else
    # Remote execution via pipe - try common locations
    if [[ -d "/opt/togather/src" ]]; then
        PROJECT_ROOT="/opt/togather/src"
        SCRIPT_DIR="$PROJECT_ROOT/deploy/scripts"
    elif [[ -f "deploy/config/deployment.yml" ]]; then
        PROJECT_ROOT="$(pwd)"
        SCRIPT_DIR="$PROJECT_ROOT/deploy/scripts"
    else
        echo "ERROR: Cannot determine project root directory"
        exit 1
    fi
fi

# Logging function
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%dT%H:%M:%S%z')
    
    case "$level" in
        INFO)
            echo -e "\033[0;34m[INFO]\033[0m $message"
            ;;
        SUCCESS)
            echo -e "\033[0;32m[SUCCESS]\033[0m $message"
            ;;
        WARN)
            echo -e "\033[1;33m[WARN]\033[0m $message"
            ;;
        ERROR)
            echo -e "\033[0;31m[ERROR]\033[0m $message"
            ;;
        *)
            echo "[$level] $message"
            ;;
    esac
}

# Default values
ENVIRONMENT="${ENVIRONMENT:-local}"
SLOT="active"
RESTART_MODE="graceful"
REMOTE_HOST=""
RESTART_TIMEOUT=30

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --slot)
            SLOT="$2"
            shift 2
            ;;
        --hard)
            RESTART_MODE="hard"
            shift
            ;;
        --remote)
            REMOTE_HOST="$2"
            shift 2
            ;;
        --timeout)
            RESTART_TIMEOUT="$2"
            shift 2
            ;;
        --help)
            grep '^#' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        -*)
            echo "Unknown option: $1"
            echo "Run with --help for usage information"
            exit 1
            ;;
        *)
            ENVIRONMENT="$1"
            shift
            ;;
    esac
done

# Validate environment
if [[ ! "$ENVIRONMENT" =~ ^(local|staging|production)$ ]]; then
    log "ERROR" "Invalid environment: $ENVIRONMENT (must be local, staging, or production)"
    exit 1
fi

# Validate slot
if [[ ! "$SLOT" =~ ^(active|blue|green|both)$ ]]; then
    log "ERROR" "Invalid slot: $SLOT (must be active, blue, green, or both)"
    exit 1
fi

# ============================================================================
# Remote execution
# ============================================================================

if [[ -n "$REMOTE_HOST" ]]; then
    log "INFO" "Restarting server on remote host: $REMOTE_HOST"
    
    # Build arguments for remote execution
    REMOTE_ARGS=("$ENVIRONMENT" "--slot" "$SLOT")
    if [[ "$RESTART_MODE" == "hard" ]]; then
        REMOTE_ARGS+=("--hard")
    fi
    
    # Copy script to remote and execute
    ssh "$REMOTE_HOST" "bash -s -- ${REMOTE_ARGS[*]}" < "$0"
    
    exit $?
fi

# ============================================================================
# Local restart
# ============================================================================

log "INFO" "Restarting $ENVIRONMENT server (slot: $SLOT, mode: $RESTART_MODE)"

# Load deployment configuration
DEPLOY_CONFIG="$PROJECT_ROOT/deploy/config/deployment.yml"
if [[ ! -f "$DEPLOY_CONFIG" ]]; then
    log "ERROR" "Deployment configuration not found: $DEPLOY_CONFIG"
    exit 1
fi

# Determine project name
PROJECT_NAME="togather-${ENVIRONMENT}"

# Get active slot if needed
if [[ "$SLOT" == "active" ]]; then
    STATE_FILE="$PROJECT_ROOT/deploy/config/deployment-state.json"
    if [[ -f "$STATE_FILE" ]]; then
        ACTIVE_SLOT=$(jq -r ".environments.${ENVIRONMENT}.active_slot // \"blue\"" "$STATE_FILE")
        log "INFO" "Active slot: $ACTIVE_SLOT"
        SLOT="$ACTIVE_SLOT"
    else
        log "WARN" "State file not found, defaulting to blue slot"
        SLOT="blue"
    fi
fi

# Determine containers to restart
CONTAINERS=()
case "$SLOT" in
    blue)
        CONTAINERS=("togather-server-blue")
        ;;
    green)
        CONTAINERS=("togather-server-green")
        ;;
    both)
        CONTAINERS=("togather-server-blue" "togather-server-green")
        ;;
esac

# ============================================================================
# Restart containers
# ============================================================================

restart_container() {
    local container_name="$1"
    local mode="$2"
    
    # Check if container exists
    if ! docker ps -a --format '{{.Names}}' | grep -q "^${container_name}$"; then
        log "WARN" "Container $container_name does not exist, skipping"
        return 0
    fi
    
    log "INFO" "Restarting container: $container_name (mode: $mode)"
    
    if [[ "$mode" == "graceful" ]]; then
        # Graceful restart: docker restart (sends SIGTERM, waits, then SIGKILL)
        if docker restart -t "$RESTART_TIMEOUT" "$container_name"; then
            log "SUCCESS" "Container $container_name restarted gracefully"
        else
            log "ERROR" "Failed to restart container $container_name"
            return 1
        fi
    else
        # Hard restart: stop + start
        log "INFO" "Stopping container $container_name..."
        if ! docker stop -t "$RESTART_TIMEOUT" "$container_name" 2>/dev/null; then
            log "WARN" "Container was not running or failed to stop gracefully"
        fi
        
        log "INFO" "Starting container $container_name..."
        if docker start "$container_name"; then
            log "SUCCESS" "Container $container_name started"
        else
            log "ERROR" "Failed to start container $container_name"
            return 1
        fi
    fi
    
    # Wait for health check
    log "INFO" "Waiting for container health check..."
    local max_attempts=30
    local attempt=0
    
    while [[ $attempt -lt $max_attempts ]]; do
        local health_status=$(docker inspect --format='{{.State.Health.Status}}' "$container_name" 2>/dev/null || echo "unknown")
        
        if [[ "$health_status" == "healthy" ]]; then
            log "SUCCESS" "Container $container_name is healthy"
            return 0
        elif [[ "$health_status" == "unhealthy" ]]; then
            log "ERROR" "Container $container_name is unhealthy"
            return 1
        fi
        
        attempt=$((attempt + 1))
        sleep 1
    done
    
    log "WARN" "Health check timed out for container $container_name"
    return 0  # Don't fail on timeout, container might not have health check
}

# Restart each container
for container in "${CONTAINERS[@]}"; do
    if ! restart_container "$container" "$RESTART_MODE"; then
        log "ERROR" "Failed to restart container: $container"
        exit 1
    fi
done

log "SUCCESS" "Server restart completed successfully"
log "INFO" "Restarted containers: ${CONTAINERS[*]}"

# Show container status
echo ""
log "INFO" "Container status:"
for container in "${CONTAINERS[@]}"; do
    if docker ps --format '{{.Names}}\t{{.Status}}' | grep -q "^${container}"; then
        docker ps --format '{{.Names}}\t{{.Status}}' | grep "^${container}"
    fi
done
