#!/usr/bin/env bash
#
# remote.sh - Run a server CLI command on a remote environment (staging/production).
#
# Finds the active blue/green Docker container on the remote host and forwards
# the server subcommand into it via SSH + docker exec.  Reads SSH_HOST and
# SSH_USER from .deploy.conf.<env>.
#
# Usage:
#   scripts/remote.sh staging token-exchange         # When TOGATHER_ADMIN_API_KEY is set, runs over HTTPS (no SSH needed)
#   scripts/remote.sh staging admin-token --duration 8h
#   scripts/remote.sh staging api-key create my-agent --role admin
#   scripts/remote.sh staging scrape failures
#   scripts/remote.sh staging scrape sync
#   scripts/remote.sh production events
#
# Shortcuts (optional Makefile targets or aliases):
#   make remote-staging CMD="token-exchange"
#   make remote-production CMD="scrape failures"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<EOF
Usage: $0 <env> <server args...>

Run a server CLI command on a remote environment.

Arguments:
  env           Target environment: staging or production
  server args   Arguments to forward to the server binary (e.g. token-exchange, admin-token --duration 8h)

Examples:
  $0 staging token-exchange         # When TOGATHER_ADMIN_API_KEY is set, runs over HTTPS (no SSH needed)
  $0 staging admin-token --duration 8h
  $0 staging api-key create my-agent --role admin
  $0 staging scrape failures
  $0 staging scrape sync
  $0 production events
EOF
    exit 1
}

if [ $# -lt 2 ]; then
    usage
fi

ENV="$1"
shift

if [ "$ENV" != "staging" ] && [ "$ENV" != "production" ]; then
    echo "Error: environment must be 'staging' or 'production', got '$ENV'"
    exit 1
fi

CONF_FILE="$PROJECT_ROOT/.deploy.conf.$ENV"
if [ ! -f "$CONF_FILE" ]; then
    echo "Error: $CONF_FILE not found"
    exit 1
fi

SSH_HOST=$(grep '^SSH_HOST=' "$CONF_FILE" | cut -d= -f2)
SSH_USER=$(grep '^SSH_USER=' "$CONF_FILE" | cut -d= -f2)

if [ -z "$SSH_HOST" ]; then
    echo "Error: SSH_HOST not found in $CONF_FILE"
    exit 1
fi

REMOTE="${SSH_USER}@${SSH_HOST}"
if [ "$SSH_USER" = "" ]; then
    REMOTE="$SSH_HOST"
fi

# For token-exchange, if TOGATHER_ADMIN_API_KEY is set locally, we can exchange
# over HTTPS without SSH (no need to reach into the container).
FIRST_ARG="$1"
if [ "$FIRST_ARG" = "token-exchange" ] && [ -n "${TOGATHER_ADMIN_API_KEY:-}" ]; then
    BASE_URL="${TOGATHER_BASE_URL:-}"
    if [ -z "$BASE_URL" ]; then
        BASE_URL=$(grep -E '^(export )?TOGATHER_BASE_URL=' ".agent-keys/${ENV}" 2>/dev/null | cut -d= -f2- | tr -d '"' | xargs)
    fi
    if [ -z "$BASE_URL" ]; then
        echo "Error: TOGATHER_BASE_URL not set and not found in .agent-keys/${ENV}"
        exit 1
    fi
    RESPONSE=$(curl -s -X POST -H "Authorization: Bearer $TOGATHER_ADMIN_API_KEY" "$BASE_URL/api/v1/auth/token")
    TOKEN=$(echo "$RESPONSE" | jq -r '.token' 2>/dev/null || true)
    if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
        echo "Error: token exchange failed"
        echo "Response: $RESPONSE"
        exit 1
    fi
    echo "$TOKEN"
    exit 0
fi

CONTAINER=$(ssh "$REMOTE" "docker ps --format '{{.Names}}'" 2>/dev/null | grep '^togather-server-' | head -1)

if [ -z "$CONTAINER" ]; then
    echo "Error: no running togather-server container found on $SSH_HOST"
    exit 1
fi

SERVER_ARGS="$*"

exec ssh "$REMOTE" "docker exec '$CONTAINER' /app/server $SERVER_ARGS"
