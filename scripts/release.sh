#!/usr/bin/env bash
# release.sh — Togather SEL Server release execution script
#
# Usage: scripts/release.sh [--dry-run] <version>
#   version: semver without 'v' prefix, e.g. 0.1.0
#
# Options:
#   --dry-run   Validate, preview the CHANGELOG.md update, but make no
#               permanent changes (no commit, no tag, no push).
#               CHANGELOG.md is written and diffed, then restored.
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
dryinfo() { echo -e "${YELLOW}[DRY RUN]${NC} $*"; }

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

DRY_RUN=false

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 [--dry-run] <version>"
    echo "  version: semver without 'v' prefix (e.g. 0.1.0)"
    echo "  --dry-run: validate and preview changelog, no commit/tag/push"
    exit 1
fi

# Parse --dry-run flag (accept in any position)
ARGS=()
for arg in "$@"; do
    if [[ "$arg" == "--dry-run" ]]; then
        DRY_RUN=true
    else
        ARGS+=("$arg")
    fi
done

if [[ ${#ARGS[@]} -lt 1 ]]; then
    echo "Usage: $0 [--dry-run] <version>"
    echo "  version: semver without 'v' prefix (e.g. 0.1.0)"
    exit 1
fi

VERSION="${ARGS[0]}"
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

if [[ "$DRY_RUN" == "true" ]]; then
    header "Pre-Release Validation (DRY RUN)"
    dryinfo "No permanent changes will be made."
else
    header "Pre-Release Validation"
fi

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
        if [[ "$DRY_RUN" == "true" ]]; then
            dryinfo "Continuing in dry-run mode."
        else
            read -p "Continue anyway? [y/N] " -n 1 -r REPLY
            echo ""
            if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
                fail "Aborted. Wait for CI to complete and re-run."
            fi
        fi
    elif [[ "$MATCHING_RUN" == "success" ]]; then
        success "GitHub Actions CI passed for HEAD commit"
    elif [[ "$MATCHING_RUN" == "failure" ]]; then
        fail "GitHub Actions CI FAILED for HEAD commit.\nFix CI failures before releasing.\nhttps://github.com/Togather-Foundation/server/actions"
    elif [[ "$MATCHING_RUN" == "cancelled" ]]; then
        warn "Most recent CI run was cancelled for HEAD."
        if [[ "$DRY_RUN" == "true" ]]; then
            dryinfo "Continuing in dry-run mode."
        else
            read -p "Continue anyway? [y/N] " -n 1 -r REPLY
            echo ""
            if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
                fail "Aborted."
            fi
        fi
    else
        warn "No completed CI run found for HEAD commit (${CURRENT_SHA:0:7})"
        warn "This may be the first run, or CI hasn't run yet."
        echo ""
        if [[ "$DRY_RUN" == "true" ]]; then
            dryinfo "Continuing in dry-run mode."
        else
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
if [[ "$DRY_RUN" == "true" ]]; then
    dryinfo "Skipping local CI in dry-run mode"
elif [[ "$SKIP_LOCAL_CI" == "true" ]]; then
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

if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "  ${YELLOW}${BOLD}DRY RUN — nothing will be committed, tagged, or pushed.${NC}"
    echo ""
else
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
fi

# ---------------------------------------------------------------------------
# Step 5: Update CHANGELOG.md
# ---------------------------------------------------------------------------

if [[ "$SKIP_CHANGELOG" == "false" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
        header "CHANGELOG.md Preview (DRY RUN)"
    else
        header "Updating CHANGELOG.md"
    fi

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
        GITHUB_URL="https://github.com/Togather-Foundation/server"
        if grep -q "^\[Unreleased\]:" "$TEMP_FILE"; then
            sed -i "s|^\[Unreleased\]:.*|[Unreleased]: ${GITHUB_URL}/compare/v${VERSION}...HEAD|" "$TEMP_FILE"
        else
            echo "" >> "$TEMP_FILE"
            echo "[Unreleased]: ${GITHUB_URL}/compare/v${VERSION}...HEAD" >> "$TEMP_FILE"
        fi

        # Add the new version link
        if [[ "$LAST_TAG" != "(initial release)" ]]; then
            sed -i "/^\[Unreleased\]:/a [${VERSION}]: ${GITHUB_URL}/compare/${LAST_TAG}...v${VERSION}" "$TEMP_FILE"
        else
            if grep -q "^\[Unreleased\]:" "$TEMP_FILE"; then
                sed -i "/^\[Unreleased\]:/a [${VERSION}]: ${GITHUB_URL}/releases/tag/v${VERSION}" "$TEMP_FILE"
            else
                echo "[${VERSION}]: ${GITHUB_URL}/releases/tag/v${VERSION}" >> "$TEMP_FILE"
            fi
        fi

        if [[ "$DRY_RUN" == "true" ]]; then
            # Show what would change, then restore
            echo ""
            dryinfo "CHANGELOG.md diff (what would be written):"
            echo ""
            diff --color=always "$CHANGELOG" "$TEMP_FILE" || true
            rm -f "$TEMP_FILE"
            echo ""
            dryinfo "CHANGELOG.md not modified."
        else
            mv "$TEMP_FILE" "$CHANGELOG"
            success "CHANGELOG.md updated: [Unreleased] → [${VERSION}] - ${TODAY}"
        fi
    fi
fi

# ---------------------------------------------------------------------------
# Step 6: Commit and tag
# ---------------------------------------------------------------------------

if [[ "$DRY_RUN" == "true" ]]; then
    header "Commit and Tag (DRY RUN — skipped)"
    dryinfo "Would commit: release: v${VERSION} [skip ci]"
    dryinfo "Would create annotated tag: ${TAG}"
else
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
fi

# ---------------------------------------------------------------------------
# Step 7: Push
# ---------------------------------------------------------------------------

if [[ "$DRY_RUN" == "true" ]]; then
    header "Push (DRY RUN — skipped)"
    dryinfo "Would push: git push origin main"
    dryinfo "Would push: git push origin ${TAG}"
else
    header "Pushing to Origin"

    info "Pushing commit and tag to origin/main..."
    git push origin main
    git push origin "${TAG}"

    success "Pushed main and tag ${TAG} to origin"
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo ""
if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${BOLD}${YELLOW}Dry run complete for ${TAG}. No changes made.${NC}"
    echo ""
    echo "To execute the real release:"
    echo "  scripts/release.sh ${VERSION}"
    echo "  make release VERSION=${VERSION}"
else
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
fi
