# Agent Instructions

This repository is for the Togather server, a Shared Events Library (SEL) backend implemented in Go.

IMPORTANT:
- Use Beads (`bd`) for task discovery + progress tracking (NOT markdown TODO lists).
- Use Spec Kit artifacts as the source of intent: constitution → spec → plan → tasks.
- Use context7 to look up docs for external libs

## Issue Tracking

This project uses **bd (beads)** for issue tracking.
Run `bd prime` for workflow context, or install hooks (`bd hooks install`) for auto-injection.

**Quick reference:**
- `bd ready` - Find unblocked work
- `bd create "Title" --type task --priority 2` - Create issue
- `bd update <id> --status in_progress` - Claim work
- `bd close <id>` - Complete work
- `bd sync` - Sync with git (run at session end)

For full workflow details: `bd prime`


## Workflow (do this every task)
1) Pick work:
   - `bd list --status ready` (or equivalent) and choose ONE task.
2) Bind to the spec:
   - Ensure the bead description links to the relevant spec/plan section.
3) Claim bead:
   - `bd update <id> --status in_progress` at start.
4) Implement:
   - Small commits, tests where appropriate, keep diffs reviewable.
5) Update bead:
   - `bd update <id> --status closed --close-reason "<what changed + why>"` when done.
6) Sync Beads state:
   - `bd sync` (safe to run often).
7) Never merge `beads-sync` into main.

## Build, Lint, Test Commands

The project uses a `Makefile` for common build tasks. Use `make help` to see all available targets.

```bash
# Build the server
make build

# Run all tests
make test

# Run tests with verbose output
make test-v

# Run tests with race detector
make test-race

# Generate coverage report
make coverage

# Run linter (requires golangci-lint)
make lint

# Format all Go files
make fmt

# Clean build artifacts
make clean

# Build and run the server
make run

# Run in development mode (with live reload if air is available)
make dev

# Install development tools (golangci-lint, air)
make install-tools
```

Standard Go commands also work:
```bash
go build ./...
go test ./...
go test -v ./path/to/package -run TestName
gofmt -w path/to/file.go
```

## Code Style and Architecture Guidelines

Use idiomatic Go, consistent with SEL docs in `docs/` and `plan/`.

This project uses Specification Driven Development:

**Observability Over Opacity**: Everything must be inspectable through CLI interfaces
**Simplicity Over Cleverness**: Start simple, add complexity only when proven necessary
**Integration Over Isolation**: Test with real dependencies in real environments
**Modularity Over Monoliths**: Every feature is a reusable library with clear boundaries


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

- Prefer structured logging (zap/logrus/zerolog) once selected.
- Log request IDs, correlation IDs, and core identifiers (event ULID, source URI).
- Avoid logging sensitive material (tokens, credentials, full payloads).

### Concurrency and Context

- Use `context.Context` for all external calls and DB operations.
- Propagate cancellation and timeouts from request boundaries.
- Avoid goroutine leaks; ensure each goroutine has a clear lifecycle.
- Mutexes are used for protecting shared state, otherwise use channels.

### Database and Migrations

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

### Documentation

- Update `docs/` when behavior changes to profiles/schemas.
- Keep schema and context artifacts (`contexts/`, `shapes/`) in sync with code changes.


## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

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

## Active Technologies
- Go 1.22+ + Huma (HTTP/OpenAPI 3.1), SQLc (type-safe SQL), River (transactional job queue), piprate/json-gold (JSON-LD), oklog/ulid/v2, golang-jwt/jwt/v5, go-playground/validator/v10, spf13/cobra (CLI) (001-sel-backend)
- PostgreSQL 16+ with PostGIS, pgvector, pg_trgm extensions (001-sel-backend)

## Recent Changes
- 001-sel-backend: Added Go 1.22+ + Huma (HTTP/OpenAPI 3.1), SQLc (type-safe SQL), River (transactional job queue), piprate/json-gold (JSON-LD), oklog/ulid/v2, golang-jwt/jwt/v5, go-playground/validator/v10
