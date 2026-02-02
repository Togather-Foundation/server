#!/usr/bin/env bash
# deploy/testing/config.sh
# Configuration loader for testing scripts
# Usage: source deploy/testing/config.sh <environment>
#        Environment: local, staging, production

set -euo pipefail

# Get script directory (absolute path)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENVIRONMENTS_DIR="${SCRIPT_DIR}/environments"

# Determine environment
if [ $# -ge 1 ]; then
    TEST_ENVIRONMENT="$1"
elif [ -n "${TEST_ENVIRONMENT:-}" ]; then
    # Already set in environment
    true
else
    TEST_ENVIRONMENT="local"
fi

# Validate environment
case "$TEST_ENVIRONMENT" in
    local|staging|production)
        ;;
    *)
        echo "❌ Invalid environment: $TEST_ENVIRONMENT" >&2
        echo "Valid options: local, staging, production" >&2
        return 1 2>/dev/null || exit 1
        ;;
esac

# Load environment config
CONFIG_FILE="${ENVIRONMENTS_DIR}/${TEST_ENVIRONMENT}.test.env"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "❌ Config file not found: $CONFIG_FILE" >&2
    return 1 2>/dev/null || exit 1
fi

echo "📋 Loading test config: $TEST_ENVIRONMENT"
# shellcheck disable=SC1090
source "$CONFIG_FILE"

# Validate required variables
: "${BASE_URL:?BASE_URL must be set in config}"
: "${ENVIRONMENT:?ENVIRONMENT must be set in config}"
: "${TIMEOUT:?TIMEOUT must be set in config}"

# Set defaults for optional variables
RETRY_COUNT="${RETRY_COUNT:-3}"
MAX_RESPONSE_TIME_MS="${MAX_RESPONSE_TIME_MS:-500}"
ALLOW_DESTRUCTIVE="${ALLOW_DESTRUCTIVE:-false}"
ALLOW_LOAD_TESTING="${ALLOW_LOAD_TESTING:-false}"
ALLOW_MIGRATION_TESTS="${ALLOW_MIGRATION_TESTS:-false}"
AUTO_CLEANUP="${AUTO_CLEANUP:-false}"

# Production safety check
if [ "$ENVIRONMENT" = "production" ]; then
    if [ "$ALLOW_DESTRUCTIVE" = "true" ]; then
        echo "⚠️  WARNING: ALLOW_DESTRUCTIVE=true in production config!" >&2
        echo "⚠️  This is dangerous and should never be enabled." >&2
        echo "⚠️  Overriding to false for safety." >&2
        ALLOW_DESTRUCTIVE=false
    fi
    if [ "$ALLOW_LOAD_TESTING" = "true" ]; then
        echo "⚠️  WARNING: ALLOW_LOAD_TESTING=true in production config!" >&2
        echo "⚠️  Overriding to false for safety." >&2
        ALLOW_LOAD_TESTING=false
    fi
fi

# Export all config variables
export TEST_ENVIRONMENT
export BASE_URL
export ENVIRONMENT
export TIMEOUT
export RETRY_COUNT
export MAX_RESPONSE_TIME_MS
export ALLOW_DESTRUCTIVE
export ALLOW_LOAD_TESTING
export ALLOW_MIGRATION_TESTS
export AUTO_CLEANUP
export API_KEY
export JWT_TOKEN

# Display loaded config (without secrets)
echo "  Environment: $ENVIRONMENT"
echo "  Base URL: $BASE_URL"
echo "  Timeout: ${TIMEOUT}s"
echo "  Destructive ops: $ALLOW_DESTRUCTIVE"
echo "  Load testing: $ALLOW_LOAD_TESTING"

# Return success
return 0 2>/dev/null || exit 0
