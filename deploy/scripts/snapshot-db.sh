#!/usr/bin/env bash
#
# snapshot-db.sh - Database Snapshot Creation for Togather Server
#
# Creates compressed PostgreSQL database snapshots before migrations for rollback safety.
# Implements 7-day retention policy with automatic cleanup of expired snapshots.
#
# Usage:
#   ./snapshot-db.sh [options]
#
# Options:
#   --help              Show usage information
#   --reason REASON     Reason for snapshot (default: manual)
#   --retention-days N  Override default retention period (default: 7)
#   --dry-run           Validate configuration without creating snapshot
#   --list              List all existing snapshots
#   --cleanup           Clean up expired snapshots only (no new snapshot)
#
# Environment Variables (required):
#   DATABASE_URL            Full PostgreSQL connection string
#   POSTGRES_USER           Database user (for pg_dump auth)
#   POSTGRES_PASSWORD       Database password (for pg_dump auth)
#   POSTGRES_DB             Database name
#
# Environment Variables (optional):
#   SNAPSHOT_DIR            Directory for snapshots (default: /var/lib/togather/db-snapshots)
#   SNAPSHOT_RETENTION_DAYS Retention period in days (default: 7)
#   POSTGRES_HOST           Database host (default: extracted from DATABASE_URL)
#   POSTGRES_PORT           Database port (default: extracted from DATABASE_URL)
#
# Exit Codes:
#   0   Success - snapshot created and old snapshots cleaned up
#   1   Configuration error - missing environment variables or invalid parameters
#   2   Database connection error - cannot connect to PostgreSQL
#   3   Snapshot creation error - pg_dump failed or disk full
#   4   Cleanup error - failed to remove expired snapshots
#
# Reference: specs/001-deployment-infrastructure/spec.md FR-010
#            specs/001-deployment-infrastructure/data-model.md L350-L400
#            specs/001-deployment-infrastructure/deployment-scripts.md

set -euo pipefail

# Script metadata
SCRIPT_VERSION="1.0.0"
SCRIPT_NAME="$(basename "$0")"

# Configuration defaults
DEFAULT_SNAPSHOT_DIR="/var/lib/togather/db-snapshots"
DEFAULT_RETENTION_DAYS=7
DEFAULT_REASON="manual"

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
    local timestamp
    timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    
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

Database snapshot creation and management for Togather Server.
Creates compressed pg_dump snapshots with automatic retention management.

OPTIONS:
    --help                  Show this help message
    --reason REASON         Reason for snapshot (e.g., "pre-migration", "manual-backup")
    --retention-days N      Override retention period (default: $DEFAULT_RETENTION_DAYS days)
    --dry-run               Validate configuration without creating snapshot
    --list                  List all existing snapshots with details
    --cleanup               Clean up expired snapshots only (no new snapshot)

ENVIRONMENT VARIABLES (required):
    DATABASE_URL            PostgreSQL connection string
    POSTGRES_USER           Database user
    POSTGRES_PASSWORD       Database password
    POSTGRES_DB             Database name

ENVIRONMENT VARIABLES (optional):
    SNAPSHOT_DIR            Snapshot directory (default: $DEFAULT_SNAPSHOT_DIR)
    SNAPSHOT_RETENTION_DAYS Retention period (default: $DEFAULT_RETENTION_DAYS)
    POSTGRES_HOST           Database host (extracted from DATABASE_URL if not set)
    POSTGRES_PORT           Database port (extracted from DATABASE_URL if not set)

EXAMPLES:
    # Create snapshot before migration
    $SCRIPT_NAME --reason "pre-migration-v1.2.0"

    # List all snapshots
    $SCRIPT_NAME --list

    # Cleanup old snapshots only
    $SCRIPT_NAME --cleanup

    # Dry-run to validate configuration
    $SCRIPT_NAME --dry-run

EXIT CODES:
    0   Success
    1   Configuration error
    2   Database connection error
    3   Snapshot creation error
    4   Cleanup error

For more information: specs/001-deployment-infrastructure/spec.md
EOF
}

# ============================================================================
# Configuration Validation
# ============================================================================

validate_config() {
    log INFO "Validating configuration..."
    
    local errors=0
    
    # Required environment variables
    if [[ -z "${DATABASE_URL:-}" ]]; then
        log ERROR "DATABASE_URL environment variable is required"
        ((errors++))
    fi
    
    if [[ -z "${POSTGRES_USER:-}" ]]; then
        log ERROR "POSTGRES_USER environment variable is required"
        ((errors++))
    fi
    
    if [[ -z "${POSTGRES_PASSWORD:-}" ]]; then
        log ERROR "POSTGRES_PASSWORD environment variable is required"
        ((errors++))
    fi
    
    if [[ -z "${POSTGRES_DB:-}" ]]; then
        log ERROR "POSTGRES_DB environment variable is required"
        ((errors++))
    fi
    
    # Check for pg_dump
    if ! command -v pg_dump &> /dev/null; then
        log ERROR "pg_dump command not found. Install postgresql-client."
        ((errors++))
    fi
    
    # Check for gzip
    if ! command -v gzip &> /dev/null; then
        log ERROR "gzip command not found. Install gzip."
        ((errors++))
    fi
    
    # Validate snapshot directory
    if [[ ! -d "$SNAPSHOT_DIR" ]]; then
        log WARN "Snapshot directory does not exist: $SNAPSHOT_DIR"
        log INFO "Creating snapshot directory..."
        if ! mkdir -p "$SNAPSHOT_DIR" 2>/dev/null; then
            log ERROR "Failed to create snapshot directory: $SNAPSHOT_DIR"
            log ERROR "Check permissions and parent directory exists"
            ((errors++))
        else
            log SUCCESS "Created snapshot directory: $SNAPSHOT_DIR"
        fi
    fi
    
    # Check write permissions
    if [[ ! -w "$SNAPSHOT_DIR" ]]; then
        log ERROR "No write permission for snapshot directory: $SNAPSHOT_DIR"
        ((errors++))
    fi
    
    # Check disk space (require at least 1GB free)
    local free_space
    free_space=$(df -BG "$SNAPSHOT_DIR" | tail -1 | awk '{print $4}' | sed 's/G//')
    if [[ "$free_space" -lt 1 ]]; then
        log WARN "Low disk space: ${free_space}GB free in $SNAPSHOT_DIR"
        log WARN "Recommend at least 1GB free for database snapshots"
    fi
    
    if [[ $errors -gt 0 ]]; then
        log ERROR "Configuration validation failed with $errors error(s)"
        return 1
    fi
    
    log SUCCESS "Configuration validated successfully"
    return 0
}

# ============================================================================
# Database Connection
# ============================================================================

extract_db_params() {
    # Extract host and port from DATABASE_URL if not explicitly set
    # Format: postgresql://user:pass@host:port/database?params
    
    if [[ -z "${POSTGRES_HOST:-}" ]] || [[ -z "${POSTGRES_PORT:-}" ]]; then
        # Extract from DATABASE_URL
        local url="${DATABASE_URL}"
        
        # Remove protocol
        url="${url#postgresql://}"
        url="${url#postgres://}"
        
        # Remove user:pass@ prefix
        url="${url#*@}"
        
        # Extract host:port
        local host_port="${url%%/*}"
        
        # Split host and port
        if [[ "$host_port" == *":"* ]]; then
            POSTGRES_HOST="${POSTGRES_HOST:-${host_port%:*}}"
            POSTGRES_PORT="${POSTGRES_PORT:-${host_port#*:}}"
        else
            POSTGRES_HOST="${POSTGRES_HOST:-$host_port}"
            POSTGRES_PORT="${POSTGRES_PORT:-5432}"
        fi
    fi
    
    export POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
    export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
}

test_db_connection() {
    log INFO "Testing database connection..."
    
    extract_db_params
    
    # Use pg_isready for lightweight connection test
    if command -v pg_isready &> /dev/null; then
        if ! PGPASSWORD="$POSTGRES_PASSWORD" pg_isready \
            -h "$POSTGRES_HOST" \
            -p "$POSTGRES_PORT" \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB" \
            -t 5 &> /dev/null; then
            log ERROR "Database connection failed"
            log ERROR "Host: $POSTGRES_HOST:$POSTGRES_PORT"
            log ERROR "Database: $POSTGRES_DB"
            log ERROR "User: $POSTGRES_USER"
            return 2
        fi
    else
        # Fallback to psql if pg_isready not available
        if ! PGPASSWORD="$POSTGRES_PASSWORD" psql \
            -h "$POSTGRES_HOST" \
            -p "$POSTGRES_PORT" \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB" \
            -c "SELECT 1" &> /dev/null; then
            log ERROR "Database connection failed"
            return 2
        fi
    fi
    
    log SUCCESS "Database connection successful"
    return 0
}

# ============================================================================
# Snapshot Creation
# ============================================================================

create_snapshot() {
    local reason="$1"
    local timestamp
    timestamp="$(date -u +"%Y%m%d_%H%M%S")"
    
    local snapshot_name="togather_${POSTGRES_DB}_${timestamp}_${reason}.sql.gz"
    local snapshot_path="$SNAPSHOT_DIR/$snapshot_name"
    local metadata_path="${snapshot_path%.sql.gz}.meta.json"
    
    log INFO "Creating database snapshot..."
    log INFO "Database: $POSTGRES_DB"
    log INFO "Reason: $reason"
    log INFO "Output: $snapshot_path"
    
    # Create snapshot metadata
    local git_commit="${GIT_COMMIT:-unknown}"
    local deployment_id="${DEPLOYMENT_ID:-manual}"
    
    cat > "$metadata_path" << EOF
{
  "snapshot_name": "$snapshot_name",
  "database": "$POSTGRES_DB",
  "host": "$POSTGRES_HOST",
  "port": "$POSTGRES_PORT",
  "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "reason": "$reason",
  "git_commit": "$git_commit",
  "deployment_id": "$deployment_id",
  "retention_days": $RETENTION_DAYS,
  "expires_at": "$(date -u -d "+${RETENTION_DAYS} days" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v +${RETENTION_DAYS}d +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")"
}
EOF
    
    # Create compressed snapshot using pg_dump
    log INFO "Running pg_dump (this may take several minutes for large databases)..."
    
    local start_time
    start_time=$(date +%s)
    
    if ! PGPASSWORD="$POSTGRES_PASSWORD" pg_dump \
        -h "$POSTGRES_HOST" \
        -p "$POSTGRES_PORT" \
        -U "$POSTGRES_USER" \
        -d "$POSTGRES_DB" \
        --format=plain \
        --no-owner \
        --no-acl \
        --clean \
        --if-exists \
        --verbose \
        2>&1 | gzip > "$snapshot_path"; then
        log ERROR "pg_dump failed"
        rm -f "$snapshot_path" "$metadata_path"
        return 3
    fi
    
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    # Get snapshot size
    local size_bytes
    size_bytes=$(stat -f%z "$snapshot_path" 2>/dev/null || stat -c%s "$snapshot_path" 2>/dev/null || echo "0")
    local size_mb=$((size_bytes / 1024 / 1024))
    
    log SUCCESS "Snapshot created successfully"
    log INFO "Size: ${size_mb}MB (compressed)"
    log INFO "Duration: ${duration}s"
    log INFO "Location: $snapshot_path"
    log INFO "Metadata: $metadata_path"
    
    # Update metadata with actual size and duration
    local tmp_meta="${metadata_path}.tmp"
    jq --arg size "$size_mb" --arg duration "$duration" \
        '. + {size_mb: ($size | tonumber), duration_seconds: ($duration | tonumber)}' \
        "$metadata_path" > "$tmp_meta" && mv "$tmp_meta" "$metadata_path"
    
    echo "$snapshot_path"
}

# ============================================================================
# Snapshot Cleanup
# ============================================================================

list_snapshots() {
    log INFO "Listing snapshots in $SNAPSHOT_DIR"
    
    if [[ ! -d "$SNAPSHOT_DIR" ]]; then
        log WARN "Snapshot directory does not exist: $SNAPSHOT_DIR"
        return 0
    fi
    
    local snapshots
    snapshots=$(find "$SNAPSHOT_DIR" -maxdepth 1 -name "*.sql.gz" -type f | sort -r)
    
    if [[ -z "$snapshots" ]]; then
        log INFO "No snapshots found"
        return 0
    fi
    
    echo ""
    printf "%-40s %-15s %-12s %-20s %s\n" "SNAPSHOT" "SIZE" "AGE" "EXPIRES" "REASON"
    echo "--------------------------------------------------------------------------------------------------------"
    
    local now
    now=$(date +%s)
    
    while IFS= read -r snapshot_path; do
        local name
        name=$(basename "$snapshot_path")
        local meta_path="${snapshot_path%.sql.gz}.meta.json"
        
        # Get file size
        local size_bytes
        size_bytes=$(stat -f%z "$snapshot_path" 2>/dev/null || stat -c%s "$snapshot_path" 2>/dev/null || echo "0")
        local size_mb=$((size_bytes / 1024 / 1024))
        
        # Get file age
        local file_time
        file_time=$(stat -f%m "$snapshot_path" 2>/dev/null || stat -c%Y "$snapshot_path" 2>/dev/null || echo "$now")
        local age_days=$(( (now - file_time) / 86400 ))
        
        # Get metadata if available
        local reason="unknown"
        local retention=7
        if [[ -f "$meta_path" ]]; then
            reason=$(jq -r '.reason // "unknown"' "$meta_path" 2>/dev/null || echo "unknown")
            retention=$(jq -r '.retention_days // 7' "$meta_path" 2>/dev/null || echo "7")
        fi
        
        local expires_in=$((retention - age_days))
        local expires_text
        if [[ $expires_in -le 0 ]]; then
            expires_text="${RED}EXPIRED${NC}"
        elif [[ $expires_in -le 2 ]]; then
            expires_text="${YELLOW}${expires_in}d${NC}"
        else
            expires_text="${expires_in}d"
        fi
        
        printf "%-40s %-15s %-12s %-30s %s\n" \
            "$name" \
            "${size_mb}MB" \
            "${age_days}d" \
            "$expires_text" \
            "$reason"
    done <<< "$snapshots"
    
    echo ""
}

cleanup_expired_snapshots() {
    log INFO "Cleaning up expired snapshots (retention: ${RETENTION_DAYS} days)..."
    
    if [[ ! -d "$SNAPSHOT_DIR" ]]; then
        log WARN "Snapshot directory does not exist: $SNAPSHOT_DIR"
        return 0
    fi
    
    local snapshots
    snapshots=$(find "$SNAPSHOT_DIR" -maxdepth 1 -name "*.sql.gz" -type f)
    
    if [[ -z "$snapshots" ]]; then
        log INFO "No snapshots found to clean up"
        return 0
    fi
    
    local now
    now=$(date +%s)
    local deleted_count=0
    local freed_bytes=0
    
    while IFS= read -r snapshot_path; do
        local meta_path="${snapshot_path%.sql.gz}.meta.json"
        
        # Get file age
        local file_time
        file_time=$(stat -f%m "$snapshot_path" 2>/dev/null || stat -c%Y "$snapshot_path" 2>/dev/null || echo "$now")
        local age_days=$(( (now - file_time) / 86400 ))
        
        # Get retention period from metadata
        local retention=$RETENTION_DAYS
        if [[ -f "$meta_path" ]]; then
            retention=$(jq -r '.retention_days // 7' "$meta_path" 2>/dev/null || echo "$RETENTION_DAYS")
        fi
        
        if [[ $age_days -gt $retention ]]; then
            local size_bytes
            size_bytes=$(stat -f%z "$snapshot_path" 2>/dev/null || stat -c%s "$snapshot_path" 2>/dev/null || echo "0")
            
            log INFO "Deleting expired snapshot (${age_days}d old): $(basename "$snapshot_path")"
            
            if rm -f "$snapshot_path" "$meta_path"; then
                ((deleted_count++))
                freed_bytes=$((freed_bytes + size_bytes))
            else
                log WARN "Failed to delete: $snapshot_path"
            fi
        fi
    done <<< "$snapshots"
    
    if [[ $deleted_count -gt 0 ]]; then
        local freed_mb=$((freed_bytes / 1024 / 1024))
        log SUCCESS "Deleted $deleted_count expired snapshot(s), freed ${freed_mb}MB"
    else
        log INFO "No expired snapshots to delete"
    fi
    
    return 0
}

# ============================================================================
# Main Function
# ============================================================================

main() {
    # Parse command-line arguments
    local reason="$DEFAULT_REASON"
    local dry_run=false
    local list_only=false
    local cleanup_only=false
    
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)
                show_usage
                exit 0
                ;;
            --reason)
                reason="$2"
                shift 2
                ;;
            --retention-days)
                if [[ "$2" =~ ^[0-9]+$ ]] && [[ "$2" -gt 0 ]]; then
                    RETENTION_DAYS="$2"
                else
                    log ERROR "Invalid retention days: $2 (must be positive integer)"
                    exit 1
                fi
                shift 2
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            --list)
                list_only=true
                shift
                ;;
            --cleanup)
                cleanup_only=true
                shift
                ;;
            *)
                log ERROR "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    # Set defaults from environment
    SNAPSHOT_DIR="${SNAPSHOT_DIR:-$DEFAULT_SNAPSHOT_DIR}"
    RETENTION_DAYS="${SNAPSHOT_RETENTION_DAYS:-$DEFAULT_RETENTION_DAYS}"
    
    # Show header
    log INFO "Togather Database Snapshot Tool v$SCRIPT_VERSION"
    log INFO "Snapshot directory: $SNAPSHOT_DIR"
    log INFO "Retention policy: $RETENTION_DAYS days"
    echo ""
    
    # Handle list-only mode
    if [[ "$list_only" == true ]]; then
        list_snapshots
        exit 0
    fi
    
    # Validate configuration
    if ! validate_config; then
        exit 1
    fi
    
    # Dry-run mode: validate and exit
    if [[ "$dry_run" == true ]]; then
        log INFO "Dry-run mode: validating database connection..."
        if ! test_db_connection; then
            exit 2
        fi
        log SUCCESS "Dry-run validation passed (no snapshot created)"
        exit 0
    fi
    
    # Cleanup-only mode
    if [[ "$cleanup_only" == true ]]; then
        if ! cleanup_expired_snapshots; then
            exit 4
        fi
        exit 0
    fi
    
    # Test database connection
    if ! test_db_connection; then
        exit 2
    fi
    
    # Create snapshot
    if ! create_snapshot "$reason"; then
        exit 3
    fi
    
    # Cleanup old snapshots
    if ! cleanup_expired_snapshots; then
        log WARN "Snapshot created but cleanup failed"
        exit 4
    fi
    
    log SUCCESS "Database snapshot completed successfully"
    exit 0
}

# Run main function
main "$@"
