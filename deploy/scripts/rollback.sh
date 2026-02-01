#!/usr/bin/env bash
#
# DEPRECATED: This script has been replaced by CLI commands
#
# This script is deprecated and will be removed in a future release.
# Please use the server CLI commands instead:
#
#   Check deployment status:
#     server deploy status
#     server deploy status --format json
#
#   Rollback deployment:
#     server deploy rollback <environment>
#     server deploy rollback <environment> --force
#     server deploy rollback <environment> --dry-run
#
# See CLI documentation:
#   ./server deploy --help
#
# Migration guide: deploy/docs/rollback.md

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "⚠️  WARNING: This script is DEPRECATED"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Please use the server CLI instead:"
echo ""
echo "  server deploy rollback <environment>"
echo ""
echo "For more information:"
echo "  ./server deploy --help"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Uncomment to force migration (will exit with error)
# exit 1

# For backward compatibility during migration period, forward to deprecated script
exec "$(dirname "$0")/rollback.sh.deprecated" "$@"
