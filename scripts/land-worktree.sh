#!/usr/bin/env bash
# land-worktree.sh — Teardown after merging a git worktree back into main
#
# Removes the worktree, prunes stale references, and cleans agent output.
# Safe to re-run on an already-removed worktree.
#
# Usage:
#   scripts/land-worktree.sh <worktree-path>
#
# Example (matches Phase 9 of the orchestrate workflow):
#   scripts/land-worktree.sh .worktrees/togather-srv-8jk24

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <worktree-path>"
    exit 1
fi

WORKTREE="$1"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="$(cd "$WORKTREE" 2>/dev/null && pwd || echo "")"

if [ -n "$TARGET" ] && [ "$TARGET" = "$REPO_ROOT" ]; then
    echo "Error: worktree path resolves to the main repo root — refusing to remove."
    exit 1
fi

echo "[land-worktree] Repo root:  $REPO_ROOT"
if [ -n "$TARGET" ]; then
    echo "[land-worktree] Worktree:   $TARGET"
else
    echo "[land-worktree] Worktree:   $WORKTREE (not accessible, may already be removed)"
fi
echo ""

# ---------------------------------------------------------------------------
# 1. Remove the worktree
#    --force is required because worktrees may contain copied (not symlinked)
#    embed files like web/robots.txt and web/sitemap.xml. These show as
#    modified/untracked and prevent clean removal without --force.
# ---------------------------------------------------------------------------

echo "Removing worktree..."
if git worktree remove --force "$WORKTREE" 2>/dev/null; then
    echo "  removed $WORKTREE"
else
    echo "  skip   $WORKTREE (already removed or not a worktree)"
fi

# ---------------------------------------------------------------------------
# 2. Prune stale worktree references
# ---------------------------------------------------------------------------

echo ""
echo "Pruning stale worktree references..."
git worktree prune
echo "  pruned"

# ---------------------------------------------------------------------------
# 3. Clean agent output files
# ---------------------------------------------------------------------------

echo ""
echo "Cleaning agent output..."
if [ -x "$REPO_ROOT/scripts/agent-cleanup.sh" ]; then
    "$REPO_ROOT/scripts/agent-cleanup.sh"
else
    echo "  skip   agent-cleanup.sh not found"
fi

echo ""
echo "[land-worktree] Done."
