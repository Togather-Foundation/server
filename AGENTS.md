# Agent Instructions

This repository is for the Togather server, a Shared Events Library (SEL) backend implemented in Go.

IMPORTANT:
- ALWAYS USE PARALLEL TOOLS WHEN APPLICABLE.
- Staging is deployed; production is not yet live.
- Use Beads (`bd`) for task discovery + progress tracking (NOT markdown TODO lists).
- Use Spec Kit artifacts as the source of intent: constitution → spec → tasks.
- Use the `context7` MCP server to look up documentation for external libraries.
- Prefer automation: execute requested actions without confirmation unless blocked by missing info or safety/irreversibility.
- When stuck on a difficult problem (cryptic errors, multiple failed approaches, architectural uncertainty), escalate to `@diagnose` — a subagent that provides expert diagnostic analysis. Use it via the Task tool with `subagent_type: "diagnose"`.


## Active Technologies

- Go 1.24+ with Docker Compose v2 (orchestration)
- PostgreSQL 16+ with PostGIS, pgvector, pg_trgm extensions; golang-migrate (migrations); volume persistence; pg_dump snapshots to filesystem or S3-compatible storage
- net/http with Go 1.22+ ServeMux (path params via `r.PathValue()`, method-prefixed patterns), SQLc (type-safe SQL — see [internal/storage/postgres/README.md](internal/storage/postgres/README.md) for nullable parameter patterns), River (transactional job queue), piprate/json-gold (JSON-LD), oklog/ulid/v2, golang-jwt/jwt/v5, go-playground/validator/v10, spf13/cobra (CLI), rs/zerolog (structured logging)

### CLI Commands (`server` binary)

- `server serve` — Start the HTTP server
- `server setup` — Interactive first-time setup
- `server ingest` — Ingest events from a JSON file
- `server events` — Query events from the SEL server
- `server generate` — Generate test events from fixtures
- `server snapshot` — Database backup management (create, list, cleanup with retention policy)
- `server healthcheck` — Health monitoring with blue-green slot support, watch mode, multiple output formats
- `server deploy` — Manage deployments and rollbacks
- `server cleanup` — Clean up deployment artifacts (Docker images, snapshots, logs)
- `server api-key` — API key management (create, list, revoke)
- `server developer` — Developer account management (invite, list, deactivate)
- `server reconcile` — Bulk reconciliation against knowledge graphs (places, organizations, all)
- `server webfiles` — Generate robots.txt and sitemap.xml for deployment
- `server version` — Print version information


## Repo Layout

```
cmd/                  — Server binary entrypoint
internal/api/         — HTTP routing, handlers, middleware, content negotiation
internal/domain/      — Core domain logic by feature (events, places, organizations,
                        federation, users, ids, provenance)
internal/storage/postgres/ — SQLc queries, repositories, migrations
internal/auth/        — Authentication (JWT, API keys)
internal/config/      — Configuration and logging setup
internal/jobs/        — River background job workers
internal/jsonld/      — JSON-LD processing
internal/kg/          — Knowledge graph reconciliation (Artsdata adapter)
internal/validation/  — Input validation
web/                  — Frontend/web assets
tests/                — Integration, contract, e2e, performance tests
deploy/               — Deployment scripts, Docker, Caddy config, monitoring
docs/                 — SEL specification, deployment, and architecture documentation
specs/                — Spec Kit artifacts (source of intent)
contexts/ and shapes/ — JSON-LD contexts and SHACL shapes
Makefile              — Common build/test/lint targets
```


## Issue Tracking

Use **bd (beads)** for issue tracking.
Run `bd prime` for workflow context, or install hooks (`bd hooks install`) for auto-injection.

**Quick reference:**
- `bd ready` — Find unblocked work
- `bd create "Title" --type task --priority 2` — Create issue
- `bd update <id> --status in_progress` — Claim work
- `bd close <id>` — Complete work
- `bd sync` — Sync with git (run at session end)

For full workflow details: `bd prime`


## Workflow (do this every coding or documentation task)

1) Pick work:
   - `bd list --status ready` (or equivalent) and choose ONE task.
2) Bind to the spec:
   - Ensure the bead description links to the relevant spec section.
3) Claim bead:
   - `bd update <id> --status in_progress` at start.
4) Implement:
   - Small commits, tests where appropriate, keep diffs reviewable.
   - Run `make ci` before pushing to catch CI failures locally.
5) Update bead:
   - `bd update <id> --status closed --close-reason "<what changed + why>"` when done.
6) Sync Beads state:
   - `bd sync` (safe to run often).
7) Never merge `beads-sync` into main.


## Session Completion

**CRITICAL: Work is NOT complete until `git push` succeeds.** Never stop before pushing — that leaves work stranded locally. Never say "ready to push when you are" — YOU must push.

**Mandatory steps:**

1. **File issues for remaining work** — Create beads for anything needing follow-up
2. **Run quality gates** (if code changed) — `make ci`
3. **Update issue status** — Close finished work, update in-progress items
4. **Push to remote:**
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** — Clear stashes, prune remote branches, clean agent output:
   ```bash
   scripts/agent-cleanup.sh          # Remove agent output files
   ```
6. **Verify** — All changes committed AND pushed
7. **Hand off** — Provide context for next session


## Local Development

### First-Time Setup

```bash
make setup           # Interactive setup (RECOMMENDED)
# Or non-interactive:
make setup-docker    # Docker-based setup
```

This creates `.env` from `.env.example` and initializes the database. You can also run `make db-init` to set up the database independently.

### Running the Server

```bash
make run             # Build and run (kills existing first)
make dev             # Live reload with air (requires air from install-tools)
make stop            # Stop running server processes
```

### Docker Development

```bash
make docker-db       # Start database only (port 5433)
make docker-up       # Start database and server in Docker
make docker-down     # Stop all Docker containers
make docker-logs     # View Docker container logs
make docker-rebuild  # Rebuild and restart containers
make docker-clean    # Stop containers and remove volumes
```

### Database Access

```bash
source .env && psql "$DATABASE_URL"
```

### Code Generation

After modifying SQL queries in `internal/storage/postgres/queries/`:
```bash
make sqlc            # Regenerate SQLc code
```


## Build, Lint, Test Commands

Use the Makefile for common build tasks. Run `make help` for all available targets.

### Agent Output Management

When running build, test, or lint commands, use the **agent-run wrapper** to preserve
context window. It captures verbose output to `.agent-output/` and shows only a concise
summary (errors, warnings, pass/fail status).

**Two ways to use it:**

```bash
# Option 1: Explicit wrapper (works with any command)
scripts/agent-run.sh make test
scripts/agent-run.sh go build ./...
scripts/agent-run.sh make ci

# Option 2: AGENT=1 env var (works with supported Makefile targets)
AGENT=1 make test
AGENT=1 make build
AGENT=1 make ci
```

Supported Makefile targets with `AGENT=1`: `build`, `test`, `test-ci`, `test-ci-race`, `test-v`,
`test-race`, `coverage`, `test-contracts`, `validate-shapes`.

For compound targets (e.g., `lint`, `lint-ci`) that have multi-line shell blocks,
use the wrapper script directly: `scripts/agent-run.sh make lint`.

**Parallel sessions:** Set `AGENT_SESSION` to isolate output from concurrent sessions:
```bash
AGENT_SESSION=session1 scripts/agent-run.sh make test
AGENT_SESSION=session2 scripts/agent-run.sh make lint
```

Output files live in `.agent-output/<session-id>/` and can be searched:
- Use `Grep` to search for specific errors in the log files
- Use `Read` to view sections of the full output
- File paths are reported in the summary output

**Cleanup:**
```bash
scripts/agent-cleanup.sh                  # Remove all sessions
scripts/agent-cleanup.sh <session-id>     # Remove one session
scripts/agent-cleanup.sh --list           # List sessions with stats
scripts/agent-cleanup.sh --older-than 1h  # Remove stale sessions
make agent-clean                          # Remove all sessions
```

### Command Reference

```bash
# Full CI pipeline locally (run before pushing)
make ci

# Build
make build

# Tests
make test            # Run all tests
make test-ci         # Run all test suites (fast, no race detector)
make test-ci-race    # Tests exactly as CI does (with race detector, ~10min)
make test-race       # Tests with race detector
make test-v          # Tests with verbose output
make coverage        # Tests with coverage report (enforces 35% min threshold)

# E2E / Playwright Tests (requires running server + uvx)
make e2e             # Run all Python E2E tests (pytest + standalone)
make e2e-pytest      # Run only pytest-based E2E tests

# Linting
make lint            # Run golangci-lint
make lint-ci         # Lint exactly as CI does (5m timeout)
make lint-js         # Validate JavaScript syntax
make lint-openapi    # Validate OpenAPI specification
make lint-yaml       # Validate YAML files (GitHub workflows, configs)
make vulncheck       # Run govulncheck vulnerability scan

# Formatting
make fmt             # Format all Go files

# Contract and shape validation
make test-contracts  # Run contract tests (requires pyshacl)
make validate-shapes # Validate SHACL shapes against sample data

# Code generation
make sqlc            # Generate SQLc code from SQL queries

# Migrations
make migrate-up      # Run database migrations
make migrate-down    # Roll back one migration
make migrate-river   # Run River job queue migrations

# Remote testing
make test-local           # Run all tests on local server
make test-staging         # Run all tests on staging server
make test-staging-smoke   # Run smoke tests on staging
make test-production-smoke # Run smoke tests on production (read-only)

# Utilities
make install-tools   # Install development tools (golangci-lint, esbuild, air)
make clean           # Remove build artifacts
```

Standard Go commands also work:
```bash
go build ./...
go test ./...
go test -v ./path/to/package -run TestName
gofmt -w path/to/file.go
```


## Code Style and Architecture Guidelines

Use idiomatic Go, consistent with SEL docs in `docs/`.

This project uses Specification Driven Development:

- **Observability Over Opacity**: Everything must be inspectable through CLI interfaces
- **Simplicity Over Cleverness**: Start simple, add complexity only when proven necessary
- **Integration Over Isolation**: Test with real dependencies in real environments
- **Modularity Over Monoliths**: Every feature is a reusable library with clear boundaries


### Packages and Structure

- Organize by feature/domain instead of layer-only folders when possible (e.g., `events`, `places`, `organizations`).
- Keep HTTP handlers thin; push business logic into services.
- Prefer explicit constructors over global state.
- Avoid circular imports; create shared packages for common domain concepts.

### Naming Conventions

- Package names: lowercase, short, no underscores (e.g., `events`, `jsonld`).
- Types: PascalCase, exported only when needed externally.
- Methods/functions: verbs for actions (`ValidateEvent`, `Publish`), nouns for getters (`EventByID`).
- File names: snake_case where helpful (`event_service.go`, `jsonld_encoder.go`).

### Imports

- Group imports in standard Go format: stdlib, third-party, local.
- Avoid dot imports; avoid aliasing unless required for conflicts.
- Prefer context-aware packages (e.g., `net/http`, `database/sql`, `context`).

### Formatting

- Use `gofmt` and keep line length reasonable (no hard cap).
- Prefer one responsibility per function; keep functions short.
- Use early returns to reduce nesting.

### Types and Validation

- Use strong types for domain IDs (e.g., ULID wrappers) rather than raw `string` when possible.
- Validate inputs at the boundary (HTTP handlers, ingestion jobs).
- Follow SEL requirements: CC0 license defaults, JSON-LD export support, and SHACL validation paths.
- Preserve source provenance and avoid lossy conversions.

### Error Handling

- Wrap errors with context: `fmt.Errorf("...: %w", err)`.
- Return sentinel errors for domain-specific conditions.
- Use RFC 7807 style error envelopes for API responses (see SEL Interoperability Profile).
- Avoid panics except for truly unrecoverable startup failures.

### Logging and Observability

- Use zerolog (`github.com/rs/zerolog`) for structured logging.
- Log request IDs, correlation IDs, and core identifiers (event ULID, source URI).
- Avoid logging sensitive material (tokens, credentials, full payloads).

### Concurrency and Context

- Use `context.Context` for all external calls and DB operations.
- Propagate cancellation and timeouts from request boundaries.
- Avoid goroutine leaks; ensure each goroutine has a clear lifecycle.
- Mutexes are used for protecting shared state, otherwise use channels.


### Database and Migrations

**IMPORTANT: Always use `make` targets for migrations, NOT direct `migrate` commands!**

The `migrate` binary is in `$GOPATH/bin` which may not be in your PATH. The Makefile handles path resolution automatically.

```bash
make migrate-up      # Run all pending migrations
make migrate-down    # Roll back one migration (use carefully!)
make migrate-river   # Run River job queue migrations
```

`DATABASE_URL` must be in your environment. Run `source .env` first if needed.

**If migrations fail with "migrate: command not found":**
```bash
make install-tools
# Or manually:
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

**Creating new migrations:**
```bash
migrate create -ext sql -dir internal/storage/postgres/migrations -seq my_migration_name
```

**Schema best practices:**
- Use migrations for schema changes; keep them backwards compatible.
- Prefer parameterized queries; avoid string concatenation for SQL.
- Keep JSONB payloads intact for provenance; store normalized fields for queries.

### API and Serialization

- Support `Accept: application/ld+json` content negotiation.
- Use stable response envelopes with `items` and `next_cursor`.
- For errors, use `type`, `title`, `status`, `detail`, `instance` fields.

### Testing

- Table-driven tests for validators and transformers.
- Use test factories for event/place/org fixtures.
- Include JSON-LD validation tests against `shapes/*.ttl` where relevant.
- Keep unit tests deterministic; isolate external dependencies with fakes.
- **Browser E2E tests**: See [`tests/e2e/AGENTS.md`](tests/e2e/AGENTS.md) for writing and running Playwright tests against the admin UI. Use `make e2e` to run all browser tests (requires running server + `uvx`).

### Documentation

- Update `docs/` when behavior changes to profiles/schemas.
- Keep schema and context artifacts (`contexts/`, `shapes/`) in sync with code changes.


## Deployment and Testing

When the user asks to deploy (to local/staging/production), follow the comprehensive deployment testing process.
Ask which environment to deploy to if not specified, and create a subagent to complete the task.

DO NOT create a bead to track deployment tasks!

### Deployment Configuration

**IMPORTANT:** Each environment has a `.deploy.conf.{environment}` file (gitignored) that contains:
- `NODE_DOMAIN` — The FQDN for this environment (e.g., `staging.toronto.togather.foundation`)
- `SSH_HOST` — SSH hostname/alias for deployment (e.g., `togather`)
- `SSH_USER` — SSH username (e.g., `deploy`)
- `CITY`, `REGION` — Geographic identifiers
- Other deployment settings

**Before deploying, check if `.deploy.conf.{environment}` exists:**
```bash
if [ -f .deploy.conf.staging ]; then
    source .deploy.conf.staging
    echo "Deploying to: $NODE_DOMAIN via $SSH_HOST"
fi
```

**DO NOT guess or hallucinate domain names or SSH hosts** — always read from `.deploy.conf.*` files.

See `docs/deploy/deploy-conf.md` for complete documentation.

### Deployment Workflow

**IMPORTANT: Always wrap deployment commands with `scripts/agent-run.sh`** to capture verbose output and preserve context window. Deployment scripts produce hundreds of lines of colored logs that flood agent context.

**For full deployment + testing:**
1. Read `docs/deploy/deployment-testing.md` for complete instructions
2. Load deployment config: `source .deploy.conf.{environment}` (if it exists)
3. **IMPORTANT:** Determine what to deploy:
   - **Current branch/commit:** Use `--version HEAD` or `--version $(git rev-parse HEAD)`
   - **Specific commit:** Use `--version <commit-hash>`
   - **Default (not recommended):** Omitting `--version` deploys whatever is checked out on the remote server (usually main)
4. Execute deployment (config auto-loads if available):
   ```bash
   # Deploy current HEAD commit to staging (RECOMMENDED for feature branches):
   scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD

   # Deploy specific commit:
   scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version abc123

   # Deploy to remote server with current commit:
   scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --remote deploy@server --version HEAD
   ```
5. Run automated tests (health wait is handled automatically by `wait-for-health.sh`):
   ```bash
   scripts/agent-run.sh ./deploy/scripts/test-remote.sh staging all
   ```
6. If automated tests pass, report success summary
7. If issues found, run specific checks from deployment-testing.md checklist
8. Report comprehensive results to user

**CRITICAL:** When deploying feature branches, ALWAYS use `--version HEAD` or `--version $(git rev-parse HEAD)` to ensure you're deploying the current branch's code, not main.

### Deployment Documentation

- **Deployment Config:** `docs/deploy/deploy-conf.md` — Per-environment .deploy.conf files
- **Complete Testing Checklist:** `docs/deploy/deployment-testing.md`
- **Quick Start Guide:** `docs/deploy/quickstart.md`
- **Remote Deployment:** `docs/deploy/remote-deployment.md`
- **Troubleshooting:** `docs/deploy/troubleshooting.md`
- **Caddy Architecture:** `deploy/CADDY-ARCHITECTURE.md`


<skills_system priority="1">

## Available Skills

<!-- SKILLS_TABLE_START -->
<usage>
When users ask you to perform tasks, check if any of the available skills below can help complete the task more effectively. Skills provide specialized capabilities and domain knowledge.

How to use skills:
- Invoke: `npx openskills read <skill-name>` (run in your shell)
  - For multiple: `npx openskills read skill-one,skill-two`
- The skill content will load with detailed instructions on how to complete the task
- Base directory provided in output for resolving bundled resources (references/, scripts/, assets/)

Usage notes:
- Only use skills listed in <available_skills> below
- Do not invoke a skill that is already loaded in your context
- Each skill invocation is stateless
</usage>

<available_skills>

<skill>
<name>frontend-design</name>
<description>Create distinctive, production-grade frontend interfaces with high design quality. Use this skill when the user asks to build web components, pages, artifacts, posters, or applications (examples include websites, landing pages, dashboards, React components, HTML/CSS layouts, or when styling/beautifying any web UI). Generates creative, polished code and UI design that avoids generic AI aesthetics.</description>
<location>project</location>
</skill>

<skill>
<name>skill-creator</name>
<description>Guide for creating effective skills. This skill should be used when users want to create a new skill (or update an existing skill) that extends assistants capabilities with specialized knowledge, workflows, or tool integrations.</description>
<location>project</location>
</skill>

<skill>
<name>speckit-to-beads</name>
<description>Convert Spec Kit tasks.md into beads with proper epics, priorities, and dependencies. Use when the user wants to import tasks from a spec kit tasks.md file into the bd issue tracker, or when they ask to sync tasks, create beads from spec kit, or convert spec kit to beads.</description>
<location>project</location>
</skill>

<skill>
<name>webapp-testing</name>
<description>Toolkit for interacting with and testing local web applications using Playwright. Supports verifying frontend functionality, debugging UI behavior, capturing browser screenshots, and viewing browser logs.</description>
<location>project</location>
</skill>

</available_skills>
<!-- SKILLS_TABLE_END -->

</skills_system>
