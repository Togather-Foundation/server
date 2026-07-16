#!/usr/bin/env bash
# Togather Review Pass — Batch-first queue clearing.
# Usage: cd ~/Documents/art/togather-server && source .env && ./scripts/review-pass.sh
#
# Strategy: survey → batch approve by source → check remaining → report.
# Flags: pass --dry-run to preview without executing
#        pass --source <uuid> to only process one source

set -euo pipefail

SERVER="./server"
DRY_RUN=""
LIMIT_SOURCE=""
PASS_NOTES="review-pass $(date +%Y-%m-%d)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN="--dry-run"; shift ;;
    --source) LIMIT_SOURCE="$2"; shift 2 ;;
    *) echo "Unknown flag: $1"; exit 1 ;;
  esac
done

BATCH_ARGS=""
if [ -n "$DRY_RUN" ]; then
  BATCH_ARGS="$DRY_RUN"
  echo "=== DRY RUN MODE ==="
fi

echo "=== Queue Stats ==="
$SERVER review stats

echo ""
echo "=== Source Breakdown ==="
$SERVER review queue --group-by source

if [ -n "$LIMIT_SOURCE" ]; then
  echo ""
  echo "=== Processing single source: $LIMIT_SOURCE ==="
  $SERVER review batch --source "$LIMIT_SOURCE" --action approve \
    --notes "$PASS_NOTES" $BATCH_ARGS --limit 200
  echo ""
  echo "=== Final Stats ==="
  $SERVER review stats
  exit 0
fi

echo ""
echo "=== Remaining Items ==="
$SERVER review queue --group-by source

echo ""
echo "=== Review pass complete ==="
echo "Run batch-by-source for each remaining source group."
