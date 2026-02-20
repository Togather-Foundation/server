#!/usr/bin/env bash
# release.sh — Togather SEL Server release execution script
#
# Usage: scripts/release.sh <version>
#   version: semver without 'v' prefix, e.g. 0.1.0
#
# This script:
#   1. Validates preconditions (branch, clean tree, CI status)
#   2. Updates CHANGELOG.md ([Unreleased] → [version] with today's date)
#   3. Commits the changelog update
#   4. Creates an annotated git tag
#   5. Pushes commit and tag to origin
#
# The GitHub Actions release workflow takes over from there.

set -euo pipefail

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info()    { echo -e "${BLUE}==>${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; }
warn()    { echo -e "${YELLOW}⚠${NC}  $*"; }
fail()    { echo -e "${RED}✗${NC} $*" >&2; exit 1; }
header()  { echo -e "\n${BOLD}${BLUE}$*${NC}"; echo -e "${BLUE}$(printf '%.0s─' {1..50})${NC}"; }

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <version>"
    echo "  version: semver without 'v' prefix (e.g. 0.1.0)"
    exit 1
fi

VERSION="$1"
TAG="v${VERSION}"

# Validate semver format (MAJOR.MINOR.PATCH, optionally with pre-release)
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
    fail "Invalid version format: '$VERSION'. Expected semver like '0.1.0' or '1.2.3-rc.1'"
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
CHANGELOG="${REPO_ROOT}/CHANGELOG.md"
TODAY=$(date -u +"%Y-%m-%d")

# ---------------------------------------------------------------------------
# Step 1: Precondition checks
# ---------------------------------------------------------------------------

header "Pre-Release Validation"

# Must be in repo root
cd "$REPO_ROOT"

# Must be on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [[ "$CURRENT_BRANCH" != "main" ]]; then
    fail "Must be on 'main' branch. Currently on: '$CURRENT_BRANCH'\nRun: git checkout main && git pull --rebase"
fi
success "On main branch"

# Working tree must be clean (except for CHANGELOG.md if already edited)
DIRTY_FILES=$(git status --porcelain | grep -v "^.. CHANGELOG.md$" || true)
if [[ -n "$DIRTY_FILES" ]]; then
    fail "Working tree is not clean. Commit or stash changes first:\n$DIRTY_FILES"
fi
success "Working tree clean"

# Must be in sync with origin/main
git fetch origin main --quiet
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)
if [[ "$LOCAL" != "$REMOTE" ]]; then
    fail "Local main is not in sync with origin/main.\nLocal:  ${LOCAL:0:7}\nRemote: ${REMOTE:0:7}\nRun: git pull --rebase origin main"
fi
success "In sync with origin/main ($(git rev-parse --short HEAD))"

# Tag must not already exist
if git tag -l | grep -q "^${TAG}$"; then
    fail "Tag '${TAG}' already exists. Use a different version."
fi
success "Tag '${TAG}' is available"

# CHANGELOG.md must exist
if [[ ! -f "$CHANGELOG" ]]; then
    fail "CHANGELOG.md not found at: $CHANGELOG"
fi
success "CHANGELOG.md found"

# CHANGELOG must have an [Unreleased] section
if ! grep -q "^## \[Unreleased\]" "$CHANGELOG"; then
    warn "No [Unreleased] section found in CHANGELOG.md"
    warn "The release tag will be created without changelog update"
    SKIP_CHANGELOG=true
else
    SKIP_CHANGELOG=false
    success "[Unreleased] section found in CHANGELOG.md"
fi

# ---------------------------------------------------------------------------
# Step 2: GitHub Actions status check
# ---------------------------------------------------------------------------

header "GitHub Actions Status"

if ! command -v gh &> /dev/null; then
    warn "gh CLI not found — skipping GitHub Actions status check"
    warn "Install gh CLI for automated CI status verification: https://cli.github.com"
else
    CURRENT_SHA=$(git rev-parse HEAD)
    info "Checking CI status for HEAD commit: ${CURRENT_SHA:0:7}"

    # Get the most recent run for this commit on main
    RUN_INFO=$(gh run list --branch main --limit 10 --json status,conclusion,headSha,name,event 2>/dev/null || echo "[]")

    # Find a completed run for the current SHA
    MATCHING_RUN=$(echo "$RUN_INFO" | jq -r --arg sha "$CURRENT_SHA" \
        '.[] | select(.headSha == $sha) | select(.status == "completed") | .conclusion' | head -1 || true)

    IN_PROGRESS_RUN=$(echo "$RUN_INFO" | jq -r --arg sha "$CURRENT_SHA" \
        '.[] | select(.headSha == $sha) | select(.status != "completed") | .name' | head -1 || true)

    if [[ -n "$IN_PROGRESS_RUN" ]]; then
        warn "A CI run is still in progress for HEAD: '$IN_PROGRESS_RUN'"
        warn "Consider waiting for it to complete before releasing."
        echo ""
        read -p "Continue anyway? [y/N] " -n 1 -r REPLY
        echo ""
        if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
            fail "Aborted. Wait for CI to complete and re-run."
        fi
    elif [[ "$MATCHING_RUN" == "success" ]]; then
        success "GitHub Actions CI passed for HEAD commit"
    elif [[ "$MATCHING_RUN" == "failure" ]]; then
        fail "GitHub Actions CI FAILED for HEAD commit.\nFix CI failures before releasing.\nhttps://github.com/Togather-Foundation/server/actions"
    elif [[ "$MATCHING_RUN" == "cancelled" ]]; then
        warn "Most recent CI run was cancelled for HEAD."
        read -p "Continue anyway? [y/N] " -n 1 -r REPLY
        echo ""
        if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
            fail "Aborted."
        fi
    else
        warn "No completed CI run found for HEAD commit (${CURRENT_SHA:0:7})"
        warn "This may be the first run, or CI hasn't run yet."
        echo ""
        read -p "Continue without CI verification? [y/N] " -n 1 -r REPLY
        echo ""
        if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
            fail "Aborted. Push a commit to trigger CI, wait for it to pass, then re-run."
        fi
    fi
fi

# ---------------------------------------------------------------------------
# Step 3: Local CI gate
# ---------------------------------------------------------------------------

header "Local CI Gate"

SKIP_LOCAL_CI="${SKIP_LOCAL_CI:-false}"
if [[ "$SKIP_LOCAL_CI" == "true" ]]; then
    warn "Skipping local CI (SKIP_LOCAL_CI=true)"
else
    info "Running make ci... (this will take a few minutes)"
    info "Set SKIP_LOCAL_CI=true to skip (not recommended)"
    echo ""
    if ! make ci; then
        fail "Local CI failed. Fix the issues above before releasing."
    fi
    echo ""
    success "Local CI passed"
fi

# ---------------------------------------------------------------------------
# Step 4: Show release summary
# ---------------------------------------------------------------------------

header "Release Summary"

LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "(initial release)")
COMMIT_COUNT=$(git log "${LAST_TAG}..HEAD" --no-merges --oneline 2>/dev/null | wc -l | tr -d ' ' || echo "?")

echo ""
echo -e "  ${BOLD}Version:${NC}         ${TAG}"
echo -e "  ${BOLD}Date:${NC}            ${TODAY}"
echo -e "  ${BOLD}Branch:${NC}          main @ $(git rev-parse --short HEAD)"
echo -e "  ${BOLD}Previous tag:${NC}    ${LAST_TAG}"
echo -e "  ${BOLD}Commits:${NC}         ${COMMIT_COUNT} since ${LAST_TAG}"
echo ""
echo -e "  ${BOLD}Actions after tag:${NC}"
echo "    1. GitHub Actions: full CI + build binary + push Docker image to GHCR"
echo "    2. GitHub Release created with changelog + binary artifact"
echo "    3. You deploy to staging: make deploy-staging VERSION=${TAG}"
echo "    4. You deploy to production: make deploy-production VERSION=${TAG}"
echo ""

read -p "Proceed with release ${TAG}? [y/N] " -n 1 -r REPLY
echo ""
if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
    fail "Aborted by user."
fi

# ---------------------------------------------------------------------------
# Step 5: Update CHANGELOG.md
# ---------------------------------------------------------------------------

if [[ "$SKIP_CHANGELOG" == "false" ]]; then
    header "Updating CHANGELOG.md"

    # Check if this version already has an entry
    if grep -q "^## \[${VERSION}\]" "$CHANGELOG"; then
        warn "Version [${VERSION}] already exists in CHANGELOG.md — skipping update"
    else
        # Replace [Unreleased] with [VERSION] - YYYY-MM-DD
        # Also add a new empty [Unreleased] section above it
        TEMP_FILE=$(mktemp)
        awk -v version="$VERSION" -v date="$TODAY" '
            /^## \[Unreleased\]/ {
                print "## [Unreleased]"
                print ""
                print ""
                print "## [" version "] - " date
                next
            }
            { print }
        ' "$CHANGELOG" > "$TEMP_FILE"

        # Update the comparison URL at the bottom
        # Pattern: [Unreleased]: https://...compare/vX.X.X...HEAD
        # Add the new version link
        GITHUB_URL="https://github.com/Togather-Foundation/server"
        if grep -q "^\[Unreleased\]:" "$TEMP_FILE"; then
            # Update existing [Unreleased] link
            sed -i "s|^\[Unreleased\]:.*|[Unreleased]: ${GITHUB_URL}/compare/v${VERSION}...HEAD|" "$TEMP_FILE"
        else
            # Add the links section if not present
            echo "" >> "$TEMP_FILE"
            echo "[Unreleased]: ${GITHUB_URL}/compare/v${VERSION}...HEAD" >> "$TEMP_FILE"
        fi

        # Add the new version link (before [Unreleased] link)
        if [[ "$LAST_TAG" != "(initial release)" ]]; then
            PREV_VERSION="${LAST_TAG#v}"
            # Insert after the [Unreleased] link line
            sed -i "/^\[Unreleased\]:/a [${VERSION}]: ${GITHUB_URL}/compare/${LAST_TAG}...v${VERSION}" "$TEMP_FILE"
        else
            # First release: add the version link
            if grep -q "^\[Unreleased\]:" "$TEMP_FILE"; then
                sed -i "/^\[Unreleased\]:/a [${VERSION}]: ${GITHUB_URL}/releases/tag/v${VERSION}" "$TEMP_FILE"
            else
                echo "[${VERSION}]: ${GITHUB_URL}/releases/tag/v${VERSION}" >> "$TEMP_FILE"
            fi
        fi

        mv "$TEMP_FILE" "$CHANGELOG"
        success "CHANGELOG.md updated: [Unreleased] → [${VERSION}] - ${TODAY}"
    fi
fi

# ---------------------------------------------------------------------------
# Step 6: Commit and tag
# ---------------------------------------------------------------------------

header "Creating Release Commit and Tag"

# Stage changelog
if [[ "$SKIP_CHANGELOG" == "false" ]]; then
    git add CHANGELOG.md
    info "Staged CHANGELOG.md"
fi

# Create commit if there's something staged
if ! git diff --cached --quiet; then
    git commit -m "release: v${VERSION}

Prepare release v${VERSION}.

[skip ci]"
    success "Created release commit: $(git rev-parse --short HEAD)"
else
    info "Nothing staged — skipping release commit"
fi

# Create annotated tag
git tag -a "${TAG}" -m "Release ${TAG}

Version ${VERSION} of the Togather SEL Server.
See CHANGELOG.md for details."

success "Created annotated tag: ${TAG}"

# ---------------------------------------------------------------------------
# Step 7: Push
# ---------------------------------------------------------------------------

header "Pushing to Origin"

info "Pushing commit and tag to origin/main..."
git push origin main
git push origin "${TAG}"

success "Pushed main and tag ${TAG} to origin"

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo ""
echo -e "${BOLD}${GREEN}Release ${TAG} initiated!${NC}"
echo ""
echo "Next steps:"
echo ""
echo "  1. Watch GitHub Actions:"
echo "     https://github.com/Togather-Foundation/server/actions"
echo ""
echo "  2. Once the release workflow completes, deploy to staging:"
echo "     make deploy-staging VERSION=${TAG}"
echo ""
echo "  3. Run staging tests:"
echo "     make test-staging"
echo ""
echo "  4. After staging verification, deploy to production:"
echo "     make deploy-production VERSION=${TAG}"
echo ""
echo "  5. Run production smoke tests:"
echo "     make test-production-smoke"
echo ""
