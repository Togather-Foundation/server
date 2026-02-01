#!/usr/bin/env bash
#
# DEPRECATED: This script has been replaced by CLI commands
#
# This script is deprecated and will be removed in a future release.
# Please use the server CLI commands instead:
#
#   Create snapshot:
#     server snapshot create --reason "pre-deploy backup"
#
#   List snapshots:
#     server snapshot list
#     server snapshot list --format json
#
#   Cleanup old snapshots:
#     server snapshot cleanup --dry-run
#     server snapshot cleanup --retention-days 7
#
# See CLI documentation:
#   ./server snapshot --help
#
# Migration guide: docs/deploy/quickstart.md

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "⚠️  WARNING: This script is DEPRECATED"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Please use the server CLI instead:"
echo ""
echo "  server snapshot create --reason \"pre-deploy backup\""
echo ""
echo "For more information:"
echo "  ./server snapshot --help"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Uncomment to force migration (will exit with error)
# exit 1

# For backward compatibility during migration period, forward to deprecated script
exec "$(dirname "$0")/snapshot-db.sh.deprecated" "$@"
