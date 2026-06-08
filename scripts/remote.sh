#!/usr/bin/env bash
#
# remote.sh - Run a server CLI command on a remote environment (staging/production).
#
# Finds the active blue/green Docker container on the remote host and forwards
# the server subcommand into it via SSH + docker exec.  Reads SSH_HOST and
# SSH_USER from .deploy.conf.<env>.
#
# Usage:
#   scripts/remote.sh staging admin-token --duration 8h
#   scripts/remote.sh staging api-key create my-agent --role admin
#   scripts/remote.sh staging scrape failures
#   scripts/remote.sh staging scrape sync
#   scripts/remote.sh production events
#
# Shortcuts (optional Makefile targets or aliases):
#   make remote-staging CMD="admin-token --duration 8h"
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
  server args   Arguments to forward to the server binary (e.g. admin-token --duration 8h)

Examples:
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

CONTAINER=$(ssh "$REMOTE" "docker ps --format '{{.Names}}'" 2>/dev/null | grep '^togather-server-' | head -1)

if [ -z "$CONTAINER" ]; then
    echo "Error: no running togather-server container found on $SSH_HOST"
    exit 1
fi

SERVER_ARGS="$*"

exec ssh -t "$REMOTE" "docker exec -it '$CONTAINER' /app/server $SERVER_ARGS"
