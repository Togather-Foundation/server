# Agent Instructions

Go 1.24+ SEL backend (Togather server). PostgreSQL 16+/PostGIS, SQLc, River, net/http ServeMux.

## Defaults

- Spec driven red/green TDD.
- Use parallel tools whenever applicable.
- Use `bd` (beads) for task tracking — not markdown todo lists.
- Use `context7` MCP server for external library docs.
- Act without confirmation unless blocked by missing info or irreversibility.
- When stuck (cryptic errors, multiple failed approaches): escalate via Task tool with `subagent_type: "diagnose"`.

## Documentation First

Before planning or writing code, search the project docs:

- `docs/` — architecture, API design, interop profile, operations, deployment
- `specs/` — Spec artifacts (constitution → spec → tasks); source of intent for every feature
- `@internal/storage/postgres/AGENTS.md`, `@web/AGENTS.md`, `@tests/e2e/AGENTS.md` — read before touching files in those directories

Use Grep/Glob/Read to find relevant docs. Do not assume — the project is well-documented and docs often contain decisions that must be preserved.
You MUST update docs as needed.

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

**Always wrap build/test/lint/long output commands with `scripts/agent-run.sh`** — captures verbose output to `.agent-output/`, shows only summary. Alternatively: `AGENT=1 make test`.

## Repo-Specific Constraints

**API changes — update the spec:**
- Any new or modified HTTP endpoint **must** be reflected in `docs/api/openapi.yaml` before the work is closed.
- `make lint-openapi` enforces this — CI will fail if the spec is invalid.
- OAS 3.1 note: use `type: [string, 'null']` instead of `nullable: true`.

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

**Configuration — keep values DRY:**
- Runtime tunables (limits, thresholds, timeouts) belong in `internal/config/config.go` — add a typed field to the relevant struct (e.g. `RateLimitConfig`) and wire it via `getEnvInt`/`getEnvFloat`/`getEnv` with a sensible default.
- Pass the value into constructors (`NewFooService(repo, cfg.X)`) rather than hardcoding constants inside domain packages.
- If you find a magic number in domain code that an operator might reasonably want to change, move it to config in the same PR.
- **OpenAPI spec must reflect config-tunable defaults:** if an endpoint behaviour (rate limit, batch size, threshold) or schema constraint (minLength, maxLength) is controlled by an env var, the relevant `description` in `docs/api/openapi.yaml` must mention the env var name and its default (e.g. `configurable via RATE_LIMIT_SUBMISSIONS_PER_IP_PER_24H, default 20`). This applies to both endpoint descriptions and individual schema property descriptions. The lint test in `internal/config/openapi_lint_test.go` enforces a manifest of known tunables — add new entries there whenever you add a new guard.


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
- **Fault injection pattern:** for packages that call `os.*` directly (like `internal/fileutil`), introduce an unexported interface (e.g. `atomicFS`) with a `defaultFS` production impl and a `failFS` test impl. Use same-package tests (`package foo`, not `package foo_test`) to access the seam. See `internal/fileutil/atomicwrite.go` + `atomicwrite_fault_test.go` as a reference.

## Beads Workflow

```bash
bd ready                              # find unblocked work
bd update <id> --status in_progress  # claim before starting
bd close <id> --reason "..."         # close when done
```

Beads state is persisted in a local Dolt SQL server and auto-flushed to `.beads/issues.jsonl`, which is committed to git. There is no Dolt remote — `bd dolt push` will fail. Beads travel with the repo via normal `git push`.

Never merge `beads-sync` into main. For full workflow: `bd prime`.

## Session Close Protocol

Work is NOT complete until docs are updated and `git push` succeeds.

```bash
scripts/agent-run.sh make ci          # quality gate (if code changed)
bd close <id> --reason "..."          # close finished beads
git pull --rebase
git push
git status                            # must show "up to date with origin"
scripts/agent-cleanup.sh              # remove agent output files
```

**Commit messages** use Conventional Commits and must include a `Generated-by` trailer:

```
feat(scope): short description

Generated-by: <your-model-name>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`. Scope is optional (e.g. `scraper`, `events`, `deploy`).


## Deployment

Environments: **local** → **staging** → **production**. Iterate locally until solid, then confirm on staging before closing work. Production is live.

**Never guess domain names or SSH hosts** — always read from `.deploy.conf.{environment}` (gitignored). `deploy.sh` auto-sources it.

```bash
scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD
scripts/agent-run.sh ./deploy/scripts/test-remote.sh staging all
```

- Feature branches: always use `--version HEAD`, never omit it
- New env vars: run `./deploy/scripts/env-audit.sh staging` before deploying
- Do NOT create beads for deployment tasks

Docs:
- `docs/deploy/remote-deployment.md` — how `deploy.sh` works, options, first-time setup
- `docs/deploy/deployment-testing.md` — post-deploy verification checklist
- `docs/deploy/env-management.md` — adding/changing env vars
- `docs/deploy/rollback.md` — when health checks fail
- `docs/deploy/troubleshooting.md` — diagnosing failures

## Entry Points

- `cmd/` — binary entrypoint
- `internal/api/` — HTTP routing + handlers
- `internal/domain/` — business logic by feature
- `internal/storage/postgres/` — SQLc queries, repos, migrations
- `tests/` — integration, contract, e2e, performance
- `specs/` — Spec Kit artifacts (source of intent: constitution → spec → tasks)

