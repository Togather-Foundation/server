# Commit Message Conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
for structured, machine-readable commit history.

This convention powers the automated changelog generation during releases — consistent
commit messages make changelog creation much easier and more accurate.

---

## Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Example:**
```
feat(kg): add Wikidata reconciliation adapter for organizations

Implements W3C Reconciliation API client for Wikidata, allowing
organizations to be matched against Wikidata entities by name and
identifiers.

Closes srv-abc12
```

---

## Types

| Type | When to use |
|------|-------------|
| `feat` | A new feature or capability |
| `fix` | A bug fix |
| `perf` | A performance improvement |
| `refactor` | Code restructuring without behavior change |
| `test` | Adding or improving tests |
| `docs` | Documentation only changes |
| `ci` | Changes to CI/CD configuration |
| `build` | Changes to the build system or dependencies |
| `chore` | Maintenance work (minor cleanup, tooling, etc.) |

---

## Scopes

Scopes are optional but recommended for clarity. Use the feature or subsystem area:

| Scope | Area |
|-------|------|
| `api` | HTTP handlers, routing, middleware |
| `auth` | Authentication (JWT, API keys) |
| `admin` | Admin UI |
| `events` | Event domain logic |
| `places` | Place domain logic |
| `orgs` | Organization domain logic |
| `kg` | Knowledge graph reconciliation (Artsdata, Wikidata, etc.) |
| `federation` | Federation and change feed |
| `jobs` | Background job workers (River) |
| `db` | Database, migrations, queries |
| `deploy` | Deployment scripts, Docker, Caddy |
| `docs` | Documentation files |
| `ci` | GitHub Actions workflows |

---

## Breaking Changes

For breaking changes, add `!` after the type/scope:

```
feat(api)!: remove v1 event endpoint

The /v1/events endpoint has been removed. Use /events instead.

BREAKING CHANGE: Callers must update to the new endpoint URL.
Migration: Replace all /v1/events with /events in client code.
```

You can also use the `BREAKING CHANGE:` footer (without `!`), but `!` is preferred
as it is visible in `git log --oneline`.

---

## Description Guidelines

- Use the **imperative mood**: "add feature" not "added feature" or "adds feature"
- Start with a **lowercase letter**
- **No period** at the end
- Keep it **under 72 characters**
- Be specific: "fix pagination cursor for events by date" not "fix pagination bug"

**Good:**
```
feat(kg): add Artsdata batch reconciliation support
fix(auth): prevent JWT token reuse after revocation
perf(db): add composite index on events(start_date, place_id)
```

**Not as good:**
```
Fixed bug
Updated stuff
WIP
feat: things
```

---

## Body and Footer

The body is optional but valuable for non-obvious changes:

```
fix(federation): correct change feed cursor advancement

The change feed cursor was advancing by 1 instead of the actual
batch size, causing duplicate events to be returned on reconnect.

Closes srv-xyz99
```

Useful footer tokens:

| Token | Purpose |
|-------|---------|
| `Closes srv-XXXX` | Links to and closes a beads issue |
| `Refs srv-XXXX` | Links to a beads issue without closing it |
| `BREAKING CHANGE: ...` | Documents a breaking change |
| `Co-authored-by: Name <email>` | Credit co-authors |

---

## Multi-Commit PRs

For pull requests with multiple commits, each commit should follow these conventions.
The PR merge commit (if squashing) should also follow the convention, summarizing
all changes with the most significant type.

---

## Quick Reference Card

```
feat(scope): add something new
fix(scope): fix something broken
perf(scope): make something faster
refactor(scope): restructure without behavior change
test(scope): add or fix tests
docs(scope): update documentation
ci(scope): update CI configuration
build(scope): update build or dependencies
chore: misc maintenance

feat(scope)!: breaking change — add ! for breaking
```

---

## Why This Matters

During the release process, commit messages are parsed to:

1. **Categorize changes** into `Added`, `Fixed`, `Changed`, etc. in the changelog
2. **Detect breaking changes** that require MAJOR version bumps
3. **Generate release summaries** that are readable and useful
4. **Link changes** back to beads issues for traceability

The more consistently commit messages follow this format, the higher quality the
auto-generated changelog entries will be.

---

*See also: [Conventional Commits specification](https://www.conventionalcommits.org/en/v1.0.0/)*
