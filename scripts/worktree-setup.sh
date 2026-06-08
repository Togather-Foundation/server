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

# ---------------------------------------------------------------------------
# 5. Convenience symlink: ./server → worktree binary
#    So you can always use './server' regardless of which worktree is active.
# ---------------------------------------------------------------------------

echo ""
echo "Linking ./server to worktree binary..."
SERVER_LINK="$REPO_ROOT/server"
WT_BINARY="$TARGET/bin/togather-server"

if [ -L "$SERVER_LINK" ]; then
    rm -f "$SERVER_LINK"
    echo "  removed old ./server symlink"
elif [ -e "$SERVER_LINK" ]; then
    echo "  warn   ./server exists as a real file — not replacing"
fi

if [ ! -e "$SERVER_LINK" ] && [ ! -L "$SERVER_LINK" ]; then
    ln -s "$WT_BINARY" "$SERVER_LINK"
    echo "  linked ./server -> $WT_BINARY"
    echo "  Run './server --help' to verify."
fi

# ---------------------------------------------------------------------------
# 6. Check PostgreSQL connectivity
#    Postgres integration tests (internal/storage/postgres/) need a running DB.
#    The worktree inherits DATABASE_URL from .env. Warn if unreachable so agents
#    know ci-fast will time out on DB tests and can skip to unit tests instead.
# ---------------------------------------------------------------------------

echo ""
echo "Checking PostgreSQL connectivity..."
DB_CHECK_URL=""
if [ -f "$TARGET/.env" ]; then
    DB_CHECK_URL="$(grep -E '^DATABASE_URL=' "$TARGET/.env" | head -1 | cut -d= -f2-)"
elif [ -f "$REPO_ROOT/.env" ]; then
    DB_CHECK_URL="$(grep -E '^DATABASE_URL=' "$REPO_ROOT/.env" | head -1 | cut -d= -f2-)"
fi

if [ -z "$DB_CHECK_URL" ]; then
    echo "  skip   DATABASE_URL not found in .env — postgres tests will fail"
else
    if command -v pg_isready >/dev/null 2>&1; then
        DB_HOST="$(echo "$DB_CHECK_URL" | sed -n 's|.*@\([^:/]*\).*|\1|p')"
        DB_PORT="$(echo "$DB_CHECK_URL" | sed -n 's|.*:\([0-9]*\)/.*|\1|p')"
        if [ -z "$DB_PORT" ]; then DB_PORT=5432; fi
        if [ -n "$DB_HOST" ] && pg_isready -h "$DB_HOST" -p "$DB_PORT" -t 3 >/dev/null 2>&1; then
            echo "  ok     PostgreSQL reachable at $DB_HOST:$DB_PORT"
        else
            echo "  warn   PostgreSQL NOT reachable at ${DB_HOST:-unknown}:${DB_PORT} — postgres tests will fail (unit tests still work)"
        fi
    else
        echo "  skip   pg_isready not found — cannot verify DB connectivity"
    fi
fi

echo ""
echo "[worktree-setup] Done. Run 'make build' in the worktree to verify."
