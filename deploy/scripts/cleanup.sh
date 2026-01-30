#!/usr/bin/env bash
#
# cleanup.sh - Deployment Artifact Cleanup for Togather Server
#
# Cleans up old deployment artifacts including Docker images, database snapshots,
# and deployment logs to free up disk space and maintain system hygiene.
#
# Usage:
#   ./cleanup.sh [options]
#
# Options:
#   --help              Show usage information
#   --dry-run           Show what would be cleaned without actually deleting
#   --force             Skip confirmation prompts
#   --keep-images N     Number of Docker images to keep per tag pattern (default: 3)
#   --keep-snapshots N  Number of database snapshots to keep (default: 7)
#   --keep-logs-days N  Number of days of logs to keep (default: 30)
#   --all               Clean all artifact types (default if none specified)
#   --images-only       Clean only Docker images
#   --snapshots-only    Clean only database snapshots
#   --logs-only         Clean only deployment logs
#
# Environment Variables (optional):
#   SNAPSHOT_DIR            Directory for snapshots (default: /var/lib/togather/db-snapshots)
#   DEPLOYMENT_HISTORY_DIR  Directory for deployment history (default: /var/lib/togather/deployments)
#   LOG_DIR                 Directory for deployment logs (default: $HOME/.togather/logs/deployments)
#
# Exit Codes:
#   0   Success - cleanup completed
#   1   Configuration error - invalid arguments or missing dependencies
#   2   Cleanup error - failed to remove some artifacts
#
# Reference: specs/001-deployment-infrastructure/tasks.md T072

set -euo pipefail

# Script metadata
SCRIPT_VERSION="1.0.0"
SCRIPT_NAME="$(basename "$0")"

# Configuration defaults
DEFAULT_SNAPSHOT_DIR="/var/lib/togather/db-snapshots"
DEFAULT_DEPLOYMENT_HISTORY_DIR="/var/lib/togather/deployments"
DEFAULT_LOG_DIR="${HOME}/.togather/logs"
DEFAULT_KEEP_IMAGES=3
DEFAULT_KEEP_SNAPSHOTS=7
DEFAULT_KEEP_LOGS_DAYS=30

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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
        SUCCESS) echo -e "${GREEN}[SUCCESS]${NC} $message" ;;
        INFO)    echo -e "${BLUE}[INFO]${NC} $message" ;;
        *)       echo "[${level}] $message" ;;
    esac
}

show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME [options]

Cleanup deployment artifacts to free up disk space and maintain system hygiene.

OPTIONS:
    --help                  Show this help message
    --dry-run               Show what would be cleaned without deleting
    --force                 Skip confirmation prompts
    --keep-images N         Number of Docker images to keep (default: $DEFAULT_KEEP_IMAGES)
    --keep-snapshots N      Number of database snapshots to keep (default: $DEFAULT_KEEP_SNAPSHOTS)
    --keep-logs-days N      Number of days of logs to keep (default: $DEFAULT_KEEP_LOGS_DAYS)
    --all                   Clean all artifact types (default)
    --images-only           Clean only Docker images
    --snapshots-only        Clean only database snapshots
    --logs-only             Clean only deployment logs

ENVIRONMENT VARIABLES (optional):
    SNAPSHOT_DIR            Snapshot directory (default: /var/lib/togather/db-snapshots)
    DEPLOYMENT_HISTORY_DIR  Deployment history directory (default: /var/lib/togather/deployments)
    LOG_DIR                 Logs directory (default: \$HOME/.togather/logs)

EXAMPLES:
    # Dry run to see what would be cleaned
    $SCRIPT_NAME --dry-run

    # Clean all artifacts with default retention
    $SCRIPT_NAME --force

    # Clean only old logs (keep 7 days)
    $SCRIPT_NAME --logs-only --keep-logs-days 7

    # Keep only last 2 Docker images
    $SCRIPT_NAME --images-only --keep-images 2 --force

EXIT CODES:
    0   Success
    1   Configuration error
    2   Cleanup error

See: specs/001-deployment-infrastructure/tasks.md T072
EOF
}

# ============================================================================
# Cleanup Functions
# ============================================================================

cleanup_docker_images() {
    local keep_count="$1"
    local dry_run="$2"
    
    log "INFO" "Cleaning up Docker images (keeping last ${keep_count})..."
    
    if ! command -v docker &> /dev/null; then
        log "WARN" "Docker not found, skipping Docker image cleanup"
        return 0
    fi
    
    # Find togather images
    local images=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep -E "togather|server" || true)
    
    if [[ -z "$images" ]]; then
        log "INFO" "No togather Docker images found"
        return 0
    fi
    
    log "INFO" "Found $(echo "$images" | wc -l) togather images"
    
    # Get unique repositories
    local repos=$(echo "$images" | cut -d: -f1 | sort -u)
    
    local total_removed=0
    local total_size=0
    
    for repo in $repos; do
        # Get images for this repo, sorted by creation time (newest first)
        local repo_images=$(docker images "$repo" --format "{{.ID}} {{.CreatedAt}} {{.Size}}" | sort -rk2)
        local image_count=$(echo "$repo_images" | wc -l)
        
        if [[ $image_count -le $keep_count ]]; then
            log "INFO" "Repository ${repo}: keeping all ${image_count} images"
            continue
        fi
        
        # Skip the first N images (keep_count), remove the rest
        local to_remove=$(echo "$repo_images" | tail -n +$((keep_count + 1)))
        
        if [[ -z "$to_remove" ]]; then
            continue
        fi
        
        while IFS= read -r line; do
            local image_id=$(echo "$line" | awk '{print $1}')
            local image_date=$(echo "$line" | awk '{print $2, $3}')
            local image_size=$(echo "$line" | awk '{print $4}')
            
            if [[ "$dry_run" == "true" ]]; then
                log "INFO" "Would remove image ${image_id} (${image_size}, created ${image_date})"
            else
                log "INFO" "Removing image ${image_id} (${image_size}, created ${image_date})"
                if docker rmi "$image_id" 2>/dev/null; then
                    ((total_removed++)) || true
                else
                    log "WARN" "Failed to remove image ${image_id} (may be in use)"
                fi
            fi
        done <<< "$to_remove"
    done
    
    if [[ "$dry_run" == "false" && $total_removed -gt 0 ]]; then
        log "SUCCESS" "Removed ${total_removed} old Docker images"
        # Prune dangling images
        docker image prune -f >/dev/null 2>&1 || true
    elif [[ "$dry_run" == "true" ]]; then
        log "INFO" "Dry run: would remove old images from repositories"
    else
        log "INFO" "No images needed cleanup"
    fi
    
    return 0
}

cleanup_snapshots() {
    local keep_count="$1"
    local dry_run="$2"
    local snapshot_dir="$3"
    
    log "INFO" "Cleaning up database snapshots (keeping last ${keep_count})..."
    
    if [[ ! -d "$snapshot_dir" ]]; then
        log "INFO" "Snapshot directory does not exist: ${snapshot_dir}"
        return 0
    fi
    
    # Find snapshot files (*.sql.gz)
    local snapshots=$(find "$snapshot_dir" -name "*.sql.gz" -type f -printf "%T@ %p\n" 2>/dev/null | sort -rn || true)
    
    if [[ -z "$snapshots" ]]; then
        log "INFO" "No snapshots found in ${snapshot_dir}"
        return 0
    fi
    
    local snapshot_count=$(echo "$snapshots" | wc -l)
    log "INFO" "Found ${snapshot_count} snapshots"
    
    if [[ $snapshot_count -le $keep_count ]]; then
        log "INFO" "Keeping all ${snapshot_count} snapshots"
        return 0
    fi
    
    # Skip the first N snapshots, remove the rest
    local to_remove=$(echo "$snapshots" | tail -n +$((keep_count + 1)))
    local removed_count=0
    local total_size=0
    
    while IFS= read -r line; do
        local snapshot_path=$(echo "$line" | awk '{print $2}')
        local snapshot_name=$(basename "$snapshot_path")
        local snapshot_size=$(du -h "$snapshot_path" 2>/dev/null | cut -f1 || echo "unknown")
        
        if [[ "$dry_run" == "true" ]]; then
            log "INFO" "Would remove snapshot: ${snapshot_name} (${snapshot_size})"
        else
            log "INFO" "Removing snapshot: ${snapshot_name} (${snapshot_size})"
            if rm -f "$snapshot_path"; then
                ((removed_count++)) || true
            else
                log "WARN" "Failed to remove snapshot: ${snapshot_path}"
            fi
        fi
    done <<< "$to_remove"
    
    if [[ "$dry_run" == "false" && $removed_count -gt 0 ]]; then
        log "SUCCESS" "Removed ${removed_count} old snapshots"
    elif [[ "$dry_run" == "true" ]]; then
        log "INFO" "Dry run: would remove ${removed_count} snapshots"
    fi
    
    return 0
}

cleanup_logs() {
    local keep_days="$1"
    local dry_run="$2"
    local log_dir="$3"
    
    log "INFO" "Cleaning up deployment logs (keeping last ${keep_days} days)..."
    
    if [[ ! -d "$log_dir" ]]; then
        log "INFO" "Log directory does not exist: ${log_dir}"
        return 0
    fi
    
    # Find log files older than N days
    local old_logs=$(find "$log_dir" -type f -name "*.log" -mtime +${keep_days} 2>/dev/null || true)
    
    if [[ -z "$old_logs" ]]; then
        log "INFO" "No old logs found (older than ${keep_days} days)"
        return 0
    fi
    
    local log_count=$(echo "$old_logs" | wc -l)
    log "INFO" "Found ${log_count} logs older than ${keep_days} days"
    
    local removed_count=0
    local total_size=0
    
    while IFS= read -r log_file; do
        if [[ -z "$log_file" ]]; then
            continue
        fi
        
        local log_name=$(basename "$log_file")
        local log_size=$(du -h "$log_file" 2>/dev/null | cut -f1 || echo "unknown")
        local log_age=$(find "$log_file" -printf "%Td days\n" 2>/dev/null || echo "unknown")
        
        if [[ "$dry_run" == "true" ]]; then
            log "INFO" "Would remove log: ${log_name} (${log_size}, ${log_age} old)"
        else
            log "INFO" "Removing log: ${log_name} (${log_size}, ${log_age} old)"
            if rm -f "$log_file"; then
                ((removed_count++)) || true
            else
                log "WARN" "Failed to remove log: ${log_file}"
            fi
        fi
    done <<< "$old_logs"
    
    if [[ "$dry_run" == "false" && $removed_count -gt 0 ]]; then
        log "SUCCESS" "Removed ${removed_count} old logs"
    elif [[ "$dry_run" == "true" ]]; then
        log "INFO" "Dry run: would remove ${removed_count} logs"
    fi
    
    return 0
}

# ============================================================================
# Main
# ============================================================================

main() {
    local dry_run=false
    local force=false
    local keep_images=$DEFAULT_KEEP_IMAGES
    local keep_snapshots=$DEFAULT_KEEP_SNAPSHOTS
    local keep_logs_days=$DEFAULT_KEEP_LOGS_DAYS
    local clean_images=false
    local clean_snapshots=false
    local clean_logs=false
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)
                show_usage
                exit 0  # Success
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            --force)
                force=true
                shift
                ;;
            --keep-images)
                if [[ -z "${2:-}" || ! "${2}" =~ ^[0-9]+$ ]]; then
                    log "ERROR" "--keep-images requires a positive integer"
                    exit 1  # Configuration error
                fi
                keep_images="$2"
                shift 2
                ;;
            --keep-snapshots)
                if [[ -z "${2:-}" || ! "${2}" =~ ^[0-9]+$ ]]; then
                    log "ERROR" "--keep-snapshots requires a positive integer"
                    exit 1  # Configuration error
                fi
                keep_snapshots="$2"
                shift 2
                ;;
            --keep-logs-days)
                if [[ -z "${2:-}" || ! "${2}" =~ ^[0-9]+$ ]]; then
                    log "ERROR" "--keep-logs-days requires a positive integer"
                    exit 1  # Configuration error
                fi
                keep_logs_days="$2"
                shift 2
                ;;
            --all)
                clean_images=true
                clean_snapshots=true
                clean_logs=true
                shift
                ;;
            --images-only)
                clean_images=true
                shift
                ;;
            --snapshots-only)
                clean_snapshots=true
                shift
                ;;
            --logs-only)
                clean_logs=true
                shift
                ;;
            *)
                log "ERROR" "Unknown option: $1"
                echo "" >&2
                show_usage
                exit 1  # Configuration error
                ;;
        esac
    done
    
    # If no specific cleanup type selected, default to all
    if [[ "$clean_images" == false && "$clean_snapshots" == false && "$clean_logs" == false ]]; then
        clean_images=true
        clean_snapshots=true
        clean_logs=true
    fi
    
    # Set directories from environment or defaults
    SNAPSHOT_DIR="${SNAPSHOT_DIR:-$DEFAULT_SNAPSHOT_DIR}"
    DEPLOYMENT_HISTORY_DIR="${DEPLOYMENT_HISTORY_DIR:-$DEFAULT_DEPLOYMENT_HISTORY_DIR}"
    LOG_DIR="${LOG_DIR:-$DEFAULT_LOG_DIR}"
    
    # Show header
    log "INFO" "Togather Deployment Cleanup Tool v${SCRIPT_VERSION}"
    if [[ "$dry_run" == "true" ]]; then
        log "INFO" "DRY RUN MODE - No changes will be made"
    fi
    echo ""
    
    # Show configuration
    log "INFO" "Cleanup configuration:"
    [[ "$clean_images" == "true" ]] && log "INFO" "  - Docker images: keep last ${keep_images}"
    [[ "$clean_snapshots" == "true" ]] && log "INFO" "  - Database snapshots: keep last ${keep_snapshots}"
    [[ "$clean_logs" == "true" ]] && log "INFO" "  - Deployment logs: keep last ${keep_logs_days} days"
    echo ""
    
    # Confirmation prompt (unless --force or --dry-run)
    if [[ "$force" == "false" && "$dry_run" == "false" ]]; then
        read -p "Proceed with cleanup? [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log "INFO" "Cleanup cancelled"
            exit 0  # Success
        fi
    fi
    
    local errors=0
    
    # Execute cleanup tasks
    if [[ "$clean_images" == "true" ]]; then
        if ! cleanup_docker_images "$keep_images" "$dry_run"; then
            log "ERROR" "Docker image cleanup failed"
            ((errors++)) || true
        fi
        echo ""
    fi
    
    if [[ "$clean_snapshots" == "true" ]]; then
        if ! cleanup_snapshots "$keep_snapshots" "$dry_run" "$SNAPSHOT_DIR"; then
            log "ERROR" "Snapshot cleanup failed"
            ((errors++)) || true
        fi
        echo ""
    fi
    
    if [[ "$clean_logs" == "true" ]]; then
        if ! cleanup_logs "$keep_logs_days" "$dry_run" "$LOG_DIR"; then
            log "ERROR" "Log cleanup failed"
            ((errors++)) || true
        fi
        echo ""
    fi
    
    # Summary
    if [[ $errors -gt 0 ]]; then
        log "ERROR" "Cleanup completed with ${errors} errors"
        exit 2  # Cleanup error
    elif [[ "$dry_run" == "true" ]]; then
        log "SUCCESS" "Dry run completed - no changes made"
        exit 0  # Success
    else
        log "SUCCESS" "Cleanup completed successfully"
        exit 0  # Success
    fi
}

# Run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
