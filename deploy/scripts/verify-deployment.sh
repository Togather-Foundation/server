#!/usr/bin/env bash
# Print one JSON evidence report. The verifier never invokes a real rollback.
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
environment=${1:-}
case "$environment" in
    staging|production)
        # Keep diagnostics from private shell config out of the machine report.
        unset NODE_DOMAIN SSH_HOST SSH_USER
        # shellcheck disable=SC1090
        if ! source "$root/.deploy.conf.$environment" >/dev/null 2>&1; then
            unset NODE_DOMAIN SSH_HOST SSH_USER
        fi
        if [[ -z "${ADMIN_API_KEY:-${TOGATHER_ADMIN_API_KEY:-}}" && -r "$root/.agent-keys/$environment" ]]; then
            # shellcheck disable=SC1090
            source "$root/.agent-keys/$environment" >/dev/null 2>&1 || unset TOGATHER_ADMIN_API_KEY
        fi
        export NODE_DOMAIN SSH_HOST SSH_USER
        export ADMIN_API_KEY="${ADMIN_API_KEY:-${TOGATHER_ADMIN_API_KEY:-}}"
        ;;
esac
exec python3 "$root/deploy/scripts/verify_deployment.py" "$@"
