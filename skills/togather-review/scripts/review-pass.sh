#!/usr/bin/env bash
# Togather Review Pass — Standard sequence for clearing the review queue.
# Usage: cd ~/Documents/art/togather-server && source .env && ./scripts/review-pass.sh
#
# Runs a full review pass: stats → source-by-source approval → check remaining.
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
  $SERVER review queue --source "$LIMIT_SOURCE"
  echo ""
  echo "Approving all from source $LIMIT_SOURCE..."
  $SERVER review batch --source "$LIMIT_SOURCE" --action approve \
    --notes "$PASS_NOTES" $BATCH_ARGS
  echo ""
  echo "=== Final Stats ==="
  $SERVER review stats
  exit 0
fi

# Well-known clean sources — batch approve
KNOWN_SOURCES=(
  "11f98d04-f875-403f-92ff-d65eccd4dd7f:TSO"
  "99a16f47-e8f5-421a-98b0-5eeb85434def:National Ballet"
  "38289172-50f6-4e58-8ab6-dadf75f54d52:Jazz Venue"
  "9c76893a-3bd3-49de-8a3a-18a402e71b4d:National Geographic Live"
)

for entry in "${KNOWN_SOURCES[@]}"; do
  src="${entry%%:*}"
  name="${entry##*:}"
  echo ""
  echo "=== Processing: $name ($src) ==="
  $SERVER review batch --source "$src" --action approve \
    --notes "Known-clean source: $name — $PASS_NOTES" $BATCH_ARGS || true
done

# Check what's remaining
echo ""
echo "=== Remaining After Clean Sources ==="
$SERVER review stats

# Report recurring series groups
echo ""
echo "=== Recurring Series Groups ==="
$SERVER review queue --group-by name 2>/dev/null | head -30

echo ""
echo "=== Review pass complete ==="
echo "Run individual approvals for remaining items (if any)."
