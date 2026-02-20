# Releasing the Togather SEL Server

This document describes the complete release process for the Togather SEL Server,
from preparing a release to deploying it to production.

## Quick Reference

```bash
# Option A: Agent-assisted (recommended) — use in OpenCode
/release 0.1.0

# Option B: Manual release
make release-check VERSION=0.1.0   # dry run
make release VERSION=0.1.0         # execute

# Deploy after release workflow completes on GitHub
make deploy-staging VERSION=v0.1.0
make test-staging
make deploy-production VERSION=v0.1.0
make test-production-smoke
```

---

## Prerequisites

- On the `main` branch with a clean working tree
- All intended changes are merged to `main`
- `gh` CLI installed and authenticated: `gh auth status`
- Access to the staging server via SSH

---

## Release Workflow Overview

```
1. Prepare       All changes merged to main, CI green
2. Changelog     Generate/review changelog + release notes
3. Tag & push    scripts/release.sh OR /release in OpenCode
                   ↓ triggers GitHub Actions
4. CI gate       Full test suite with race detector
5. Artifacts     Binary + Docker image (GHCR) published
6. GitHub Release Created with changelog + binary
7. Staging       Deploy, test, verify
8. Production    Deploy, smoke test, announce
```

---

## Step 1: Prepare for Release

Ensure all work intended for this release is merged to `main`:

```bash
git checkout main
git pull --rebase origin main

# Verify CI is clean
make release-check VERSION=<version>
```

The `release-check` target validates:
- On `main` branch
- Working tree clean
- In sync with `origin/main`
- Tag doesn't already exist
- GitHub Actions CI is green for HEAD

---

## How the Changelog Works

The changelog is built in two layers that combine at release time.

### Layer 1: Ongoing — the `[Unreleased]` section

`CHANGELOG.md` always has an `## [Unreleased]` section at the top. As significant
changes land on `main`, add a human-readable line there in
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format. This is the
curated, user-facing description of what changed and why — not a commit message.

```markdown
## [Unreleased]

### Changed
- Email sending now respects request cancellation via context propagation
```

This is optional but encouraged for notable changes. It gives you a head start
on the release notes and captures context that isn't always obvious from commit
messages alone.

### Layer 2: At release time — agent synthesis

When you run `/release 0.1.0`, the agent:

1. **Reads the existing `[Unreleased]` content** — your hand-written notes
2. **Gathers raw git data** — buckets all commits since the last tag by type
   (feat, fix, refactor, breaking changes, etc.) using Conventional Commit prefixes
3. **Synthesizes both** using the prompt in `agents/release.md` — merging manual
   notes with anything in the git log not yet captured, producing a polished
   Keep a Changelog entry
4. **Shows you the draft** for review before writing anything
5. **Writes and commits** — `scripts/release.sh` moves `[Unreleased]` →
   `[0.1.0] - YYYY-MM-DD` and updates the comparison URLs at the bottom

The GitHub Release body (richer, narrative format) is generated separately from
the same data and attached automatically by the release workflow.

### Why Conventional Commits matter

The git log bucketing in Step 2 relies on commit message prefixes:
`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, etc. Without them, the agent
has to guess what kind of change each commit is. See
`docs/contributing/commits.md` for the full conventions.

---

## Step 2: Generate Changelog

### Option A: Agent-Assisted (Recommended)

Use the `/release` command in OpenCode:

```
/release 0.1.0
```

The agent will:
1. Validate preconditions
2. Gather git history data
3. Generate a polished CHANGELOG.md entry and GitHub Release notes
4. Show you the draft for review
5. Execute the release after your approval

### Option B: Manual

Edit `CHANGELOG.md` directly:

1. Rename `## [Unreleased]` to `## [0.1.0] - YYYY-MM-DD`
2. Add a fresh `## [Unreleased]` section above it (for the next release)
3. Update the comparison URL at the bottom:
   ```
   [Unreleased]: https://github.com/Togather-Foundation/server/compare/v0.1.0...HEAD
   [0.1.0]: https://github.com/Togather-Foundation/server/releases/tag/v0.1.0
   ```

Then run:
```bash
make release VERSION=0.1.0
```

---

## Step 3: Tag and Push

The `scripts/release.sh` script (called by `make release` and the `/release` command) will:

1. Re-validate preconditions
2. Check GitHub Actions status for HEAD
3. (Optionally) run local `make ci`
4. Show a summary of what will be released
5. Commit the CHANGELOG.md update
6. Create annotated tag `v0.1.0`
7. Push commit and tag to origin

**Skip local CI** (when you know GitHub Actions already passed):
```bash
SKIP_LOCAL_CI=true make release VERSION=0.1.0
```

---

## Step 4: Monitor the GitHub Release Workflow

After pushing the tag, go to:
```
https://github.com/Togather-Foundation/server/actions
```

The `.github/workflows/release.yml` workflow will:

| Job | What it does |
|-----|-------------|
| CI Gate | Full test suite + linting + vuln check (race detector) |
| Build Binary | Compiles linux/amd64 binary with version embedded, creates `.tar.gz` + checksum |
| Build Docker | Builds and pushes to GHCR as `ghcr.io/togather-foundation/server:vX.Y.Z` |
| Create Release | GitHub Release with changelog body + binary attached |

**The release only proceeds if all CI tests pass.** If CI fails, the GitHub Release is not created.

---

## Step 5: Deploy to Staging

Once the release workflow succeeds:

```bash
# Deploy
make deploy-staging VERSION=v0.1.0

# Test
make test-staging
```

The deploy will:
1. Take a database snapshot (automatic rollback point)
2. Run any pending migrations
3. Blue-green deploy to the inactive slot
4. Health check the new slot
5. Switch Caddy traffic to the new slot
6. Verify the `/version` endpoint returns the new version

**If something goes wrong:**
```bash
make rollback-staging
```

---

## Step 6: Staging Verification

Manually verify the staging deployment:

```bash
NODE_DOMAIN=$(grep '^NODE_DOMAIN=' .deploy.conf.staging | cut -d= -f2)

# Health check
curl "https://${NODE_DOMAIN}/health" | jq .

# Version check
curl "https://${NODE_DOMAIN}/version" | jq .

# API spot check
curl "https://${NODE_DOMAIN}/events" | jq .
```

Check:
- [ ] `/health` returns `{"status":"healthy"}` with the correct version
- [ ] `/version` returns `{"version":"vX.Y.Z",...}`
- [ ] Admin UI loads and is functional
- [ ] Event API endpoints return expected responses
- [ ] Grafana metrics dashboard shows no anomalies
- [ ] Application logs show no new errors

Soak period: at your discretion. There is no minimum required wait time.

---

## Step 7: Deploy to Production

After staging verification:

```bash
# Deploy (will prompt for confirmation)
make deploy-production VERSION=v0.1.0

# Smoke tests (read-only)
make test-production-smoke
```

**If something goes wrong:**
```bash
make rollback-production
```

For detailed rollback procedures, see `docs/deploy/rollback.md`.

---

## Step 8: Post-Release

```bash
# Verify the production deployment
NODE_DOMAIN=$(grep '^NODE_DOMAIN=' .deploy.conf.production | cut -d= -f2)
curl "https://${NODE_DOMAIN}/version" | jq .

# Close any release-related beads
bd list --status=open --json | jq -r '.[] | "\(.id) \(.title)"'
# bd close <id> --reason "Released in vX.Y.Z"

# Sync beads
bd sync

# Push any final state changes
git push
```

---

## Version Numbering

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

| Change | Example | Bump |
|--------|---------|------|
| Bug fixes only | Fix cursor pagination off-by-one | PATCH: `0.1.0 → 0.1.1` |
| New backward-compatible features | Add event search by radius | MINOR: `0.1.0 → 0.2.0` |
| Breaking API changes | Remove deprecated v1 endpoints | MAJOR: `0.1.0 → 1.0.0` |
| Dependency updates only | Bump Go 1.24 → 1.25 | PATCH: `0.1.1 → 0.1.2` |

**Pre-1.0 convention:** While in `v0.x`, MINOR bumps may contain breaking changes.

**Pre-release tags:** Use `-rc.1`, `-beta.1` suffix for pre-release versions:
```bash
make release VERSION=0.2.0-rc.1
```
GitHub Actions marks these as pre-releases automatically.

---

## Rollback

### Immediate rollback (within minutes)

```bash
make rollback-staging     # or
make rollback-production
```

This restarts the old Docker slot and switches Caddy traffic back. The database
snapshot taken during deployment can be restored if needed.

### Full restore with database rollback

See `docs/deploy/rollback.md` for step-by-step instructions including database
snapshot restoration.

---

## Troubleshooting

### Release script fails: "Not on main branch"
```bash
git checkout main && git pull --rebase
```

### Release script fails: "Working tree not clean"
```bash
git status  # see what's changed
git stash   # stash uncommitted work
```

### Release script fails: "Not in sync with origin/main"
```bash
git pull --rebase origin main
```

### GitHub Actions release workflow fails
- Check the failed job for details at `https://github.com/Togather-Foundation/server/actions`
- The tag will still exist, but no GitHub Release is created
- Fix the issue, delete the tag, and re-release:
  ```bash
  git tag -d v0.1.0
  git push origin :refs/tags/v0.1.0
  # fix the issue, then:
  make release VERSION=0.1.0
  ```

### Deployment fails on staging
- The deploy script performs automatic rollback on health check failure
- Check the output of `make deploy-staging` for details
- Check server logs: `ssh togather "docker logs togather-green"` (or `-blue`)
- Use `make rollback-staging` if you need to manually revert

---

## Related Documentation

- `agents/release.md` — Full step-by-step agent workflow
- `docs/contributing/commits.md` — Commit message conventions
- `docs/deploy/rollback.md` — Detailed rollback procedures
- `docs/deploy/deployment-testing.md` — Deployment testing checklist
- `.github/workflows/release.yml` — GitHub Actions release workflow

---

*Last updated: see git log for this file*
