#!/usr/bin/env bash
#
# DEPRECATED: This script has been replaced by CLI commands
#
# This script is deprecated and will be removed in a future release.
# Please use the server CLI commands instead:
#
#   Basic health check:
#     server healthcheck
#     server healthcheck --url http://localhost:8080/health
#
#   Check specific slot:
#     server healthcheck --slot blue
#     server healthcheck --slot green
#
#   Check deployment:
#     server healthcheck --deployment production
#
#   Watch mode:
#     server healthcheck --watch --interval 5s
#
#   Different output formats:
#     server healthcheck --format json
#     server healthcheck --format table
#
# See CLI documentation:
#   ./server healthcheck --help
#
# Migration guide: docs/deploy/quickstart.md

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "⚠️  WARNING: This script is DEPRECATED"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Please use the server CLI instead:"
echo ""
echo "  server healthcheck --slot blue"
echo ""
echo "For more information:"
echo "  ./server healthcheck --help"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Uncomment to force migration (will exit with error)
# exit 1

# For backward compatibility during migration period, forward to deprecated script
exec "$(dirname "$0")/health-check.sh.deprecated" "$@"
