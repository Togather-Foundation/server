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

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKTREE_REL="$1"

# Resolve relative to REPO_ROOT so CWD doesn't affect path resolution
case "$WORKTREE_REL" in
    /*) WORKTREE="$WORKTREE_REL" ;;
    *)  WORKTREE="$REPO_ROOT/$WORKTREE_REL" ;;
esac

if [ "$WORKTREE" = "$REPO_ROOT" ]; then
    echo "Error: worktree path resolves to the main repo root — refusing to remove."
    exit 1
fi

echo "[land-worktree] Repo root:  $REPO_ROOT"
echo "[land-worktree] Worktree:   $WORKTREE"

if [ ! -d "$WORKTREE" ]; then
    echo "[land-worktree] Worktree directory not found — may already be removed."
else
    echo "[land-worktree] Worktree directory exists"
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
    echo "  skip   $WORKTREE (not a registered worktree or already removed)"
fi

# ---------------------------------------------------------------------------
# 2. Prune stale worktree references
# ---------------------------------------------------------------------------

echo ""
echo "Pruning stale worktree references..."
git worktree prune || echo "  warn   git worktree prune failed (non-fatal)"
echo "  pruned"

# ---------------------------------------------------------------------------
# 3. Clean agent output files
# ---------------------------------------------------------------------------

echo ""
echo "Cleaning agent output..."
if [ -x "$REPO_ROOT/scripts/agent-cleanup.sh" ]; then
    "$REPO_ROOT/scripts/agent-cleanup.sh" || echo "  warn   agent-cleanup.sh failed (non-fatal)"
else
    echo "  skip   agent-cleanup.sh not found"
fi

echo ""
echo "[land-worktree] Done."
