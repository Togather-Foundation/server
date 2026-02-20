#!/usr/bin/env bash
# gather-release-data.sh — Collect structured release data for changelog generation
#
# Usage: scripts/gather-release-data.sh <version>
#   version: semver without 'v' prefix, e.g. 0.1.0
#
# Output: .release-data.md (gitignored)
#   A structured markdown file the release agent reads to generate the changelog.

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <version>"
    echo "  version: semver without 'v' prefix (e.g. 0.1.0)"
    exit 1
fi

VERSION="$1"
TAG="v${VERSION#v}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
OUTPUT="${REPO_ROOT}/.release-data.md"

cd "$REPO_ROOT"

# Determine base: last tag, or the initial commit if no tags exist yet
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || git rev-list --max-parents=0 HEAD)
LAST_TAG_DISPLAY=$(git describe --tags --abbrev=0 2>/dev/null || echo "(initial release — no previous tag)")
TODAY=$(date -u +"%Y-%m-%d")
HEAD_SHA=$(git rev-parse HEAD)
HEAD_SHORT=$(git rev-parse --short HEAD)
COMMIT_COUNT=$(git log "${LAST_TAG}..HEAD" --no-merges --oneline | wc -l | tr -d ' ')

echo "Gathering release data for ${TAG} (base: ${LAST_TAG_DISPLAY})..."

# ---------------------------------------------------------------------------
# Build the output file
# ---------------------------------------------------------------------------

cat > "$OUTPUT" << EOF
# Release Data: ${TAG}

Generated: ${TODAY}
HEAD: ${HEAD_SHORT} (${HEAD_SHA})
Base: ${LAST_TAG_DISPLAY}
Commit range: ${LAST_TAG}..HEAD
Total commits (no merges): ${COMMIT_COUNT}

---

## Diff Stats

EOF

git diff "${LAST_TAG}..HEAD" --stat | tail -1 >> "$OUTPUT" || echo "(no diff stats)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

---

## All Commits (no merges)

EOF

git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges >> "$OUTPUT" || echo "(no commits)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

---

## Conventional Commit Breakdown

### Features

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- feat(\([^)]+\))?[!]?:" || echo "(none)" >> "$OUTPUT"
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- feat(\([^)]+\))?[!]?:" >> "$OUTPUT" || true

cat >> "$OUTPUT" << 'EOF'

### Bug Fixes

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- fix(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

### Performance

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- perf(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

### Breaking Changes

EOF
# Subject-line breaking changes (! suffix)
BREAKING_SUBJECTS=$(git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- [a-z]+(\([^)]+\))?!:" || true)
# Body-level BREAKING CHANGE: footers
BREAKING_BODIES=$(git log "${LAST_TAG}..HEAD" --format="### %s%n%n%b" --no-merges \
    | grep -A5 "^BREAKING CHANGE:" || true)

if [[ -z "$BREAKING_SUBJECTS" && -z "$BREAKING_BODIES" ]]; then
    echo "(none)" >> "$OUTPUT"
else
    [[ -n "$BREAKING_SUBJECTS" ]] && echo "$BREAKING_SUBJECTS" >> "$OUTPUT"
    [[ -n "$BREAKING_BODIES" ]]   && echo "$BREAKING_BODIES"   >> "$OUTPUT"
fi

cat >> "$OUTPUT" << 'EOF'

### Refactoring

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- refactor(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

### Docs

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- docs(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

### Tests

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- test(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

### CI / Build / Chores

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -E "^- (ci|build|chore)(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

### Uncategorized

EOF
git log "${LAST_TAG}..HEAD" --format="- %s  [%h]" --no-merges \
    | grep -vE "^- (feat|fix|perf|refactor|docs|test|ci|build|chore)(\([^)]+\))?[!]?:" >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

---

## Contributors

EOF
git shortlog -sn "${LAST_TAG}..HEAD" --no-merges >> "$OUTPUT" || echo "(none)" >> "$OUTPUT"

cat >> "$OUTPUT" << 'EOF'

---

## Existing [Unreleased] Section from CHANGELOG.md

EOF
awk '/^## \[Unreleased\]/{found=1; next} /^## \[/{if(found) exit} found{print}' \
    "${REPO_ROOT}/CHANGELOG.md" >> "$OUTPUT" || echo "(empty)" >> "$OUTPUT"

# Add a trailing newline and footer
cat >> "$OUTPUT" << EOF

---

## Metadata

- Version to release: ${TAG}
- Date: ${TODAY}
- GitHub compare URL: https://github.com/Togather-Foundation/server/compare/${LAST_TAG}...${TAG}
EOF

echo ""
echo "Release data written to: ${OUTPUT}"
echo ""
echo "Next step: the release agent reads this file to generate the changelog."
echo "  /release ${VERSION}"
