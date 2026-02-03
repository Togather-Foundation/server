#!/usr/bin/env bash
# unlock.sh - Safely release deployment locks
# 
# This script provides safe lock management with proper checks:
# - Shows current lock status
# - Checks if deployment process is still running
# - Provides options to wait or force unlock
#
# Usage:
#   ./unlock.sh [ENVIRONMENT]           # Show lock status
#   ./unlock.sh [ENVIRONMENT] --force   # Force unlock (dangerous!)
#   ./unlock.sh [ENVIRONMENT] --wait    # Wait for lock to be released

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Import configuration
STATE_FILE="${PROJECT_ROOT}/deploy/config/deployment-state.json"

# Parse arguments
ENVIRONMENT="${1:-}"
ACTION="${2:-status}"

if [[ -z "${ENVIRONMENT}" ]]; then
    echo "Usage: $0 ENVIRONMENT [--force|--wait|--status]"
    echo ""
    echo "Environments: development, staging, production"
    echo ""
    echo "Options:"
    echo "  --status    Show lock status (default)"
    echo "  --force     Force unlock (DANGEROUS - use only if deployment crashed)"
    echo "  --wait      Wait for lock to be released naturally"
    echo ""
    echo "Examples:"
    echo "  $0 staging                  # Show lock status"
    echo "  $0 staging --wait           # Wait for deployment to finish"
    echo "  $0 staging --force          # Force unlock (last resort)"
    exit 1
fi

# Validate environment
case "${ENVIRONMENT}" in
    development|staging|production) ;;
    *)
        echo -e "${RED}ERROR: Invalid environment '${ENVIRONMENT}'${NC}"
        echo "Must be one of: development, staging, production"
        exit 1
        ;;
esac

LOCK_DIR="/tmp/togather-deploy-${ENVIRONMENT}.lock"

# Show lock status
show_status() {
    echo -e "${BLUE}Deployment Lock Status: ${ENVIRONMENT}${NC}"
    echo "============================================"
    
    # Check lock directory
    if [[ -d "${LOCK_DIR}" ]]; then
        echo -e "${YELLOW}Lock directory exists:${NC} ${LOCK_DIR}"
        ls -ld "${LOCK_DIR}"
    else
        echo -e "${GREEN}Lock directory: Not found (unlocked)${NC}"
    fi
    
    echo ""
    
    # Check state file
    if [[ -f "${STATE_FILE}" ]]; then
        echo -e "${BLUE}State file info:${NC}"
        local locked=$(jq -r '.lock.locked // false' "${STATE_FILE}")
        local locked_by=$(jq -r '.lock.locked_by // "N/A"' "${STATE_FILE}")
        local locked_at=$(jq -r '.lock.locked_at // "N/A"' "${STATE_FILE}")
        local deployment_id=$(jq -r '.lock.deployment_id // "N/A"' "${STATE_FILE}")
        local pid=$(jq -r '.lock.pid // "N/A"' "${STATE_FILE}")
        local hostname=$(jq -r '.lock.hostname // "N/A"' "${STATE_FILE}")
        
        echo "  Locked: ${locked}"
        echo "  Locked by: ${locked_by}"
        echo "  Locked at: ${locked_at}"
        echo "  Deployment ID: ${deployment_id}"
        echo "  PID: ${pid}"
        echo "  Hostname: ${hostname}"
        
        # Calculate lock age
        if [[ "${locked_at}" != "N/A" ]] && [[ "${locked}" == "true" ]]; then
            local locked_timestamp=$(date -d "${locked_at}" +%s 2>/dev/null || echo "0")
            if [[ ${locked_timestamp} -gt 0 ]]; then
                local now_timestamp=$(date +%s)
                local lock_age=$((now_timestamp - locked_timestamp))
                local minutes=$((lock_age / 60))
                local seconds=$((lock_age % 60))
                echo "  Lock age: ${minutes}m ${seconds}s"
                
                # Warn if stale (>30 minutes)
                if [[ ${lock_age} -gt 1800 ]]; then
                    echo -e "  ${YELLOW}⚠ Lock is stale (>30 minutes)${NC}"
                fi
            fi
        fi
        
        # Check if process is still running
        if [[ "${pid}" != "N/A" ]] && [[ "${hostname}" == "$(hostname)" ]]; then
            if ps -p "${pid}" > /dev/null 2>&1; then
                local cmd=$(ps -p "${pid}" -o comm= 2>/dev/null || echo "unknown")
                echo -e "  ${GREEN}✓ Process ${pid} is still running: ${cmd}${NC}"
            else
                echo -e "  ${YELLOW}⚠ Process ${pid} is not running (deployment may have crashed)${NC}"
            fi
        fi
    else
        echo -e "${YELLOW}State file not found:${NC} ${STATE_FILE}"
    fi
    
    echo ""
}

# Wait for lock to be released
wait_for_unlock() {
    echo -e "${BLUE}Waiting for deployment to complete...${NC}"
    echo "Press Ctrl+C to cancel"
    echo ""
    
    local timeout=3600  # 1 hour
    local elapsed=0
    local interval=5
    
    while [[ ${elapsed} -lt ${timeout} ]]; do
        if [[ ! -d "${LOCK_DIR}" ]]; then
            echo -e "${GREEN}✓ Lock released!${NC}"
            return 0
        fi
        
        local minutes=$((elapsed / 60))
        local seconds=$((elapsed % 60))
        echo -ne "\rWaiting... ${minutes}m ${seconds}s"
        
        sleep ${interval}
        elapsed=$((elapsed + interval))
    done
    
    echo ""
    echo -e "${RED}Timeout after 1 hour. Lock still exists.${NC}"
    echo "Consider using --force if deployment crashed."
    return 1
}

# Force unlock
force_unlock() {
    echo -e "${RED}⚠ FORCE UNLOCK - This is dangerous!${NC}"
    echo ""
    echo "This will:"
    echo "  1. Remove lock directory: ${LOCK_DIR}"
    echo "  2. Update state file to unlocked"
    echo ""
    echo "Only do this if:"
    echo "  - The deployment process crashed"
    echo "  - You've verified no deployment is running"
    echo "  - The lock is stale (>30 minutes old)"
    echo ""
    read -p "Are you sure you want to force unlock? (type 'yes' to confirm): " confirm
    
    if [[ "${confirm}" != "yes" ]]; then
        echo "Cancelled."
        exit 0
    fi
    
    # Remove lock directory
    if [[ -d "${LOCK_DIR}" ]]; then
        if rmdir "${LOCK_DIR}" 2>/dev/null; then
            echo -e "${GREEN}✓ Lock directory removed${NC}"
        else
            echo -e "${YELLOW}⚠ Could not remove lock directory (may not be empty)${NC}"
            echo "  Trying force remove..."
            if rm -rf "${LOCK_DIR}"; then
                echo -e "${GREEN}✓ Lock directory force removed${NC}"
            else
                echo -e "${RED}ERROR: Failed to remove lock directory${NC}"
                exit 1
            fi
        fi
    else
        echo "Lock directory does not exist (already removed)"
    fi
    
    # Update state file
    if [[ -f "${STATE_FILE}" ]]; then
        local temp_file=$(mktemp)
        if jq '.lock.locked = false | .lock.deployment_id = null' "${STATE_FILE}" > "${temp_file}"; then
            mv "${temp_file}" "${STATE_FILE}"
            echo -e "${GREEN}✓ State file updated to unlocked${NC}"
        else
            rm -f "${temp_file}"
            echo -e "${RED}ERROR: Failed to update state file${NC}"
            exit 1
        fi
    else
        echo "State file does not exist"
    fi
    
    echo ""
    echo -e "${GREEN}✓ Lock forcefully released${NC}"
    echo ""
    echo "Next deployment will proceed normally."
}

# Main logic
case "${ACTION}" in
    --status|status)
        show_status
        ;;
    --wait|wait)
        show_status
        wait_for_unlock
        ;;
    --force|force)
        show_status
        echo ""
        force_unlock
        ;;
    *)
        echo -e "${RED}ERROR: Invalid action '${ACTION}'${NC}"
        echo "Use: --status, --wait, or --force"
        exit 1
        ;;
esac
