# Agent Instructions

Go 1.24+ SEL backend (Togather server). PostgreSQL 16+/PostGIS, SQLc, River, net/http ServeMux.

## Defaults

- Use parallel tools whenever applicable.
- Use `bd` (beads) for task tracking — not markdown todo lists.
- Use `context7` MCP server for external library docs.
- Act without confirmation unless blocked by missing info or irreversibility.
- When stuck (cryptic errors, multiple failed approaches): escalate via Task tool with `subagent_type: "diagnose"`.

## Documentation First

Before planning or writing code, search the project docs:

- `docs/` — architecture, API design, interop profile, operations, deployment
- `specs/` — Spec Kit artifacts (constitution → spec → tasks); source of intent for every feature
- Subdirectory `AGENTS.md` files — storage, web, e2e each have their own constraints

Use the Grep/Glob/Read tools to find relevant docs. Do not assume — the project is well-documented and the docs often contain decisions that must be preserved.

## Fast Path

```bash
# CI before pushing (required)
scripts/agent-run.sh make ci

# Targeted tests
scripts/agent-run.sh make test-ci

# Lint
scripts/agent-run.sh make lint

# Format
make fmt

# Rebuild after code changes
make build
```

**Always wrap build/test/lint with `scripts/agent-run.sh`** — captures verbose output to `.agent-output/`, shows only summary. Alternatively: `AGENT=1 make test`.

## Repo-Specific Constraints

**Generated files — never edit directly:**
- `internal/storage/postgres/*.sql.go` and `querier.go` — run `make sqlc` after changing `.sql` files
- `web/` static assets are Go-embedded; changes require rebuild

**Migrations — use `make` targets only:**
```bash
make migrate-up        # never: direct `migrate` binary (not in PATH)
make migrate-down      # rolls back ONE migration
make migrate-river     # River job queue schema
```
Create new migration: `migrate create -ext sql -dir internal/storage/postgres/migrations -seq <name>`

**Architecture boundaries:**
- HTTP handlers stay thin — business logic in `internal/domain/`
- Feature packages: `internal/domain/{events,places,organizations,...}`
- Storage: `internal/storage/postgres/` (SQLc queries + repositories + migrations)
- Auth: `internal/auth/` — JWT + API keys
- Scraper: `internal/scraper/` — three-tier (JSON-LD → Colly CSS → Rod headless)

**SEL requirements (non-negotiable):**
- CC0 license defaults on ingested events
- RFC 7807 error envelopes: `type`, `title`, `status`, `detail`, `instance`
- `Accept: application/ld+json` content negotiation on all event endpoints
- Preserve source provenance (JSONB payloads); avoid lossy conversions
- Error wrapping: `fmt.Errorf("...: %w", err)`

**Testing:**
- `go test ./internal/storage/postgres` — needs live DB (see `.env`)
- `make e2e` — browser E2E tests; requires running server + `uvx`; see `tests/e2e/AGENTS.md`
- Run targeted package tests first; expand to `make ci` only when needed

## Beads Workflow

```bash
bd ready                              # find unblocked work
bd update <id> --status in_progress  # claim before starting
bd close <id> --reason "..."         # close when done
bd dolt push                         # sync beads state to remote
```

Never merge `beads-sync` into main. For full workflow: `bd prime`.

## Session Close Protocol

Work is NOT complete until `git push` succeeds.

```bash
scripts/agent-run.sh make ci          # quality gate (if code changed)
bd close <id> --reason "..."          # close finished beads
git pull --rebase
bd dolt push
git push
git status                            # must show "up to date with origin"
scripts/agent-cleanup.sh              # remove agent output files
```

## Deployment

**Never guess domain names or SSH hosts** — always read from `.deploy.conf.{environment}` (gitignored).

```bash
source .deploy.conf.staging           # load env (if exists)
scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD
scripts/agent-run.sh ./deploy/scripts/test-remote.sh staging all
```

- Feature branches: always use `--version HEAD`, never omit it
- New env vars: run `./deploy/scripts/env-audit.sh staging` before deploying
- Do NOT create beads for deployment tasks
- Full docs: `docs/deploy/deployment-testing.md`

## Entry Points

- `cmd/` — binary entrypoint
- `internal/api/` — HTTP routing + handlers
- `internal/domain/` — business logic by feature
- `internal/storage/postgres/` — SQLc queries, repos, migrations
- `tests/` — integration, contract, e2e, performance
- `specs/` — Spec Kit artifacts (source of intent: constitution → spec → tasks)

