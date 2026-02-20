# Release Workflow

Tool-agnostic instructions for cutting a Togather SEL Server release.
Works with any AI coding agent (OpenCode, Claude Code, Cursor, etc.).

## Overview

Releases follow semantic versioning. The release process:
1. Validates all quality gates pass
2. Gathers structured data about changes since last release
3. Uses that data + a consistent prompt to generate a polished changelog
4. Tags the commit and pushes to trigger the automated GitHub release workflow
5. Deploys to staging and then production

## Prerequisites

- You are on the `main` branch with a clean working tree
- `gh` CLI is authenticated (`gh auth status`)
- All intended changes are merged to `main`

---

## Step 1: Pre-Release Validation

Run these checks in order. All must pass before proceeding.

```bash
# 1. Verify branch and clean state
git status
git branch --show-current  # must be: main

# 2. Pull latest main
git pull --rebase origin main

# 3. Run full local CI pipeline
AGENT=1 make ci

# 4. Check GitHub Actions status for current HEAD
CURRENT_SHA=$(git rev-parse HEAD)
echo "Checking CI for commit: $CURRENT_SHA"
gh run list --branch main --limit 5
# All runs for HEAD should be green (✓) or in progress
# Get the latest run result:
gh run list --branch main --limit 1 --json status,conclusion,headSha \
  | jq '.[0] | {status, conclusion, sha: .headSha[:7]}'
```

**Stop if:** local CI fails OR the most recent GitHub Actions run for HEAD is failing.

---

## Step 2: Gather Release Data

Run the data-gathering script. It collects all structured git history and writes
it to `.release-data.md` for the agent to read in the next step.

```bash
scripts/gather-release-data.sh <version>
# e.g. scripts/gather-release-data.sh 0.1.0
```

The script collects:
- All commits since the last tag, bucketed by Conventional Commit type
- Diff stats (files changed, insertions, deletions)
- Contributors (from `git shortlog`)
- The existing `[Unreleased]` section from `CHANGELOG.md`
- Metadata: version, date, GitHub compare URL

Output: `.release-data.md` (gitignored — only used during the release session).

---

## Step 3: Generate Changelog

Read `.release-data.md` and use the prompt below to generate the changelog content.

---

**Changelog Generation Prompt:**

```
You are writing the changelog for Togather SEL Server version $VERSION.

Read the file .release-data.md. It contains:
- All commits since the last release, bucketed by type
- Diff stats and contributors
- The existing [Unreleased] section from CHANGELOG.md (may be empty)

Generate two things:

## 1. CHANGELOG.md Entry

A new section to replace [Unreleased]:

## [$VERSION] - $DATE

Requirements:
- Use Keep a Changelog format (https://keepachangelog.com/en/1.0.0/)
- Sections: Added, Changed, Deprecated, Removed, Fixed, Security
- Only include sections that have content
- Each entry is a concise, user-focused description (not the raw commit message)
- Group related commits together under a single entry where appropriate
- For breaking changes: add a prominent "### Breaking Changes" subsection at the top
- Incorporate any content from the existing [Unreleased] section that adds context
  not already captured by the commit log

## 2. GitHub Release Notes

A richer document for the GitHub Release body:

## What's Changed

[2-4 sentence narrative overview of the most significant changes and why they matter]

### Highlights
- [Most important change with brief context]
- [Second most important]
- [Third most important]

### All Changes

#### Features
- ...

#### Bug Fixes
- ...

#### Performance
- ...

#### Other
- ...

### Breaking Changes
[Only if any — with detailed migration instructions]

### Contributors
[From the contributors section of .release-data.md]

**Full Changelog**: [use the GitHub compare URL from .release-data.md]

Keep tone: technical, precise, informative. Not marketing language.
```

---

## Step 4: Review and Edit

Before proceeding:
- [ ] Review the generated CHANGELOG.md entry for accuracy
- [ ] Verify breaking changes are documented with migration paths
- [ ] Check the GitHub Release notes narrative makes sense
- [ ] Confirm the version number follows semver (MAJOR.MINOR.PATCH)
  - PATCH: bug fixes only
  - MINOR: new features, backward compatible
  - MAJOR: breaking changes

---

## Step 5: Execute the Release

```bash
# Set the version (without 'v' prefix in variable, script adds it)
VERSION=0.1.0  # ← change this each release

# Run the release script (handles commit, tag, push)
scripts/release.sh $VERSION
```

The script will:
1. Validate you are on main with a clean tree
2. Update the CHANGELOG.md (move [Unreleased] → [$VERSION] with today's date)
3. Update the comparison URL at the bottom of CHANGELOG.md
4. Commit: `release: v$VERSION`
5. Create annotated tag: `v$VERSION`
6. Push commit and tag to origin
7. Report the GitHub Actions URL to watch

---

## Step 6: Watch the GitHub Release

After pushing the tag, GitHub Actions runs `.github/workflows/release.yml`:

1. **CI Gate** — full test suite with race detector (all tests must pass)
2. **Build** — binary compiled with version from the tag
3. **Docker** — image built and pushed to GHCR as `ghcr.io/togather-foundation/server:v$VERSION`
4. **GitHub Release** — created automatically with the changelog as the body, binary attached

Monitor at:
```
https://github.com/Togather-Foundation/server/actions
```

---

## Step 7: Deploy to Staging

```bash
# Deploy the tagged version to staging
make deploy-staging VERSION=v$VERSION

# Run full test suite against staging
make test-staging
```

Watch the deploy output. It will:
- Take a database snapshot
- Run migrations
- Blue-green deploy to the inactive slot
- Health check
- Switch traffic
- Verify the version endpoint returns the new version

---

## Step 8: Staging Verification

Verify manually at `https://staging.toronto.togather.foundation`:

- [ ] `/health` returns `{"status":"healthy"}` with correct version
- [ ] `/version` returns `{"version":"v$VERSION",...}`
- [ ] Admin UI loads and functions correctly
- [ ] API endpoints respond correctly
- [ ] Check Grafana dashboard for any anomalies

---

## Step 9: Deploy to Production

After you are satisfied with staging:

```bash
# Deploy the same tagged version to production
make deploy-production VERSION=v$VERSION

# Run production smoke tests (read-only)
make test-production-smoke
```

---

## Step 10: Post-Release

```bash
# Update beads: close any release-related issues
bd list --status=open --json | jq -r '.[] | select(.title | test("release|v$VERSION"; "i")) | .id'
# bd close <id> --reason "Released in v$VERSION"

# Sync beads state
bd sync

# Announce to stakeholders
```

---

## Rollback

If a deployment fails or issues are found after release:

```bash
# Rollback staging
make rollback-staging

# Rollback production (only if absolutely necessary)
make rollback-production
```

See `docs/deploy/rollback.md` for detailed rollback procedures.

---

## Version Numbering Guide

| Change Type | Example | Version Bump |
|-------------|---------|--------------|
| Bug fix | Fix pagination cursor off-by-one | PATCH: 0.1.0 → 0.1.1 |
| New feature | Add event search by location | MINOR: 0.1.0 → 0.2.0 |
| Breaking API change | Remove deprecated v1 endpoints | MAJOR: 0.1.0 → 1.0.0 |
| Dependency update only | Bump Go 1.24 → 1.25 | PATCH: 0.1.0 → 0.1.1 |
| Performance improvement | Add database index | PATCH or MINOR depending on impact |

While in v0.x, MINOR bumps may contain breaking changes (pre-1.0 semver convention).
