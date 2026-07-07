# Orchestrate Project Extension — Togather Server

This is loaded by the global `orchestrate` command when orchestrating work in
this project. It provides project-specific commands for build, test, deploy,
and cleanup that the core workflow plugs into phases 3–9.

## Build & Test Commands

| Purpose | Command |
|---|---|
| Full CI (fast gate) | `scripts/agent-run.sh make ci-fast` |
| Unit tests only | `scripts/agent-run.sh make test` |
| Full CI (main branch) | `scripts/agent-run.sh make ci` |
| Race detector | `make test-race` |
| Coverage | `make coverage` |
| Generate SQLc | `make sqlc` |
| Lint OpenAPI | `make lint-openapi` |
| Dev server | `make run` |
| Dev server (live reload) | `make dev` |

## Technology Stack

- **Language:** Go
- **Database:** PostgreSQL with PostGIS (via pgx driver)
- **Query builder:** SQLc — all queries in `.sql` files under `internal/storage/postgres/`
- **Migrations:** `migrate create -ext sql -dir internal/storage/postgres/migrations -seq <name>`
- **CI runner:** `scripts/agent-run.sh` wraps all build/test/deploy commands
- **Code review:** delegate to `@beads-code-reviewer`
- **Problem diagnosis:** delegate to `@diagnose`

## Phase 3 — Worktree Setup

```
WORKTREE=".worktrees/togather-<primary-bead-id>"
git worktree add -b <type>/<primary-bead-id>-<short-description> "$WORKTREE" main
scripts/worktree-setup.sh "$WORKTREE"
```

**Postgres tests in worktrees:** `internal/storage/postgres/` tests need a running
PostgreSQL instance. If `pg_isready` fails during worktree setup, skip `make ci-fast`
(the DB tests will hang) and run targeted unit tests instead:
```
go test -count=1 $(go list ./... | grep -v '/storage/postgres$')
```

## Phase 4 — Post-Implementation Verification

After each subagent returns:
1. Run `scripts/agent-run.sh make test` to verify tests pass
2. Run `make sqlc` if SQL queries changed
3. After all steps: run `scripts/agent-run.sh make ci-fast`
4. Start dev server: `make run` (or `make dev` for live reload)

## Phase 5 — CI Gate

Run `scripts/agent-run.sh make ci-fast` before and after code review fixes.

## Phase 8 — Final Verification (Deploy to Staging)

This is a web service — verification means deploying to staging and running
integration tests.

**Step 1 — Push branch (REQUIRED before deploy):**
```
git push -u origin HEAD
```

**Step 2 — Deploy and test (use timeout: 600000 for Docker builds):**
```
source .deploy.conf.staging 2>/dev/null
scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD
scripts/agent-run.sh ./deploy/scripts/test-remote.sh staging all
```

**Staging data reset (if needed):**
```
scripts/staging-reset.sh --yes          # wipe events, keep users/keys/sources
scripts/staging-reset.sh --wipe-all     # full wipe (keeps only users)
```

## Phase 9 — Land & Cleanup

**Default (in-tree):**
```
scripts/agent-run.sh make ci    # re-verify after rebase
```

**Worktree mode:**
```
scripts/land-worktree.sh "$WORKTREE"
```

**General cleanup after land:**
```
scripts/agent-cleanup.sh
```
