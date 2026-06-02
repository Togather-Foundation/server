#!/usr/bin/env bash
# worktree-setup.sh — Post-creation setup for a git worktree
#
# Creates or symlinks the gitignored files and directories that a new worktree
# needs to build, test, and run correctly. Safe to re-run on an existing worktree.
#
# Usage:
#   scripts/worktree-setup.sh <worktree-path>
#
# Example (matches Phase 3 of the orchestrate workflow):
#   WORKTREE=".worktrees/togather-beads-abc"
#   git worktree add -b feat/beads-abc-my-feature "$WORKTREE" main
#   scripts/worktree-setup.sh "$WORKTREE"

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <worktree-path>"
    exit 1
fi

WORKTREE="$1"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="$(cd "$WORKTREE" && pwd)"

if [ "$TARGET" = "$REPO_ROOT" ]; then
    echo "Error: worktree path resolves to the main repo root — nothing to set up."
    exit 1
fi

echo "[worktree-setup] Repo root:  $REPO_ROOT"
echo "[worktree-setup] Worktree:   $TARGET"
echo ""

# ---------------------------------------------------------------------------
# Symlink helpers
# ---------------------------------------------------------------------------

symlink() {
    local name="$1"
    local src="$REPO_ROOT/$name"
    local dst="$TARGET/$name"

    if [ ! -e "$src" ] && [ ! -L "$src" ]; then
        echo "  skip   $name  (not present in main repo)"
        return
    fi

    if [ -L "$dst" ]; then
        echo "  exists $name  (symlink already set)"
        return
    fi

    if [ -e "$dst" ]; then
        echo "  skip   $name  (exists as real file/dir — not replacing)"
        return
    fi

    ln -s "$src" "$dst"
    echo "  linked $name -> $src"
}

copy_if_missing() {
    local name="$1"
    local src="$REPO_ROOT/$name"
    local dst="$TARGET/$name"

    if [ ! -e "$src" ] && [ ! -L "$src" ]; then
        echo "  skip   $name  (source not present in main repo)"
        return
    fi

    if [ -e "$dst" ]; then
        echo "  exists $name"
        return
    fi

    # Resolve symlink in main repo before copying (embed doesn't support symlinks)
    cp -L "$src" "$dst"
    echo "  copied $name -> $src"
}

mkdir_if_missing() {
    local name="$1"
    local dst="$TARGET/$name"
    if [ -e "$dst" ] || [ -L "$dst" ]; then
        echo "  exists $name"
    else
        mkdir -p "$dst"
        echo "  mkdir  $name"
    fi
}

# ---------------------------------------------------------------------------
# 1. Symlink gitignored config and secret files
#    These are shared across all worktrees — they describe the local environment,
#    not the branch being worked on.
# ---------------------------------------------------------------------------

echo "Symlinking shared config files..."
symlink ".env"
symlink ".deploy.conf.staging"
symlink ".deploy.conf.production"
symlink ".agent-keys"
symlink "deploy/testing/environments"

# ---------------------------------------------------------------------------
# 2. Symlink generated web files (gitignored, needed for embed directives)
#    robots.txt and sitemap.xml are generated at deploy time by
#    cmd/server/cmd/webfiles.go. They are gitignored so git worktree add
#    won't create them. Symlink from main repo so go build works.
# ---------------------------------------------------------------------------

echo ""
echo "Copying generated web files (embed requires regular files, not symlinks)..."
copy_if_missing "web/robots.txt"
copy_if_missing "web/sitemap.xml"

# ---------------------------------------------------------------------------
# 3. Symlink the beads database
#    All worktrees share the same issue tracker so bead state isn't fragmented.
# ---------------------------------------------------------------------------

echo ""
echo "Symlinking beads database..."
symlink ".beads"

# ---------------------------------------------------------------------------
# 4. Create fresh local directories
#    These should NOT be shared — each worktree has its own logs and artifacts.
# ---------------------------------------------------------------------------

echo ""
echo "Creating local directories..."
mkdir_if_missing "tmp"
mkdir_if_missing ".agent-output"
mkdir_if_missing ".ci-logs"

echo ""
echo "[worktree-setup] Done. Run 'make build' in the worktree to verify."
