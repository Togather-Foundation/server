# Contributors Guide

Welcome to the SEL backend contributor documentation!

## Who This Is For

You're building features, fixing bugs, or improving the SEL codebase. These docs will help you:
- Understand the system architecture
- Follow development workflows
- Write effective tests
- Maintain security standards

## Quick Start

1. **[Development Guide](DEVELOPMENT.md)** - Start here for coding standards, logging, validation
2. **[Architecture Overview](ARCHITECTURE.md)** - System design and component structure
3. **[Database Guide](DATABASE.md)** - Schema design, migrations, query patterns
4. **[Testing Guide](TESTING.md)** - TDD workflow, test types, coverage requirements
5. **[Security Guide](SECURITY.md)** - Threat model, security implementations, best practices

## Essential Resources

- **[Glossary](../glossary.md)** - Canonical terminology reference
- **[Spec Kit](../../specs/001-sel-backend/)** - Feature specifications and tasks
- **[AGENTS.md](../../AGENTS.md)** - Agent collaboration guidelines

## Development Workflow

```bash
# 1. Pick work
bd ready

# 2. Claim a task
bd update <id> --status in_progress

# 3. Implement with tests
make test

# 4. Run full CI locally
make ci

# 5. Update bead and push
bd close <id> --reason "description"
bd sync
git push
```

## Build & Test Commands

```bash
# Full CI pipeline (ALWAYS run before pushing)
make ci

# Run tests
make test
make test-v        # verbose
make test-race     # with race detector
make test-ci       # all test suites (no race detector)
make test-ci-race  # all test suites with race detector (~10min)

# Linting
make lint
make lint-ci       # CI configuration

# Build
make build
make run

# Development mode (with live reload)
make dev
```

## Architecture at a Glance

```
internal/
├── api/              # HTTP routing, handlers, middleware
├── domain/           # Business logic by feature (events, places, orgs, federation)
├── storage/postgres/ # SQLc repositories, migrations
└── jsonld/           # JSON-LD serialization, SHACL validation

tests/
└── integration/      # End-to-end tests
```

## Key Principles

- **Specification-Driven**: Constitution → Spec → Plan → Tasks
- **Test-First**: Write tests before implementation (TDD)
- **Observability**: Everything must be inspectable through CLI interfaces
- **Simplicity**: Start simple, add complexity only when proven necessary

## Meta Documentation

Interested in how this project uses AI agents for development?

- **[Agent Workflows](meta/agent_workflows.md)** - Collaboration patterns and tooling

## Questions?

- **GitHub Issues**: [togather/server/issues](https://github.com/Togather-Foundation/server/issues)
- **Discussions**: [togather/discussions](https://github.com/Togather-Foundation/discussions)

---

**Back to:** [Documentation Index](../README.md)
