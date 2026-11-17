<!-- AGENT:NAV
purpose:~test;agents;development;tests;agent;guide;race;architecture
lines:122
nav[10]{s,n,name,about}:
17,106,#Contributors Guide,~test;agents;development;tests;agent
21,8,##Who This Is For,~architecture;bugs;building;codebase;development
29,9,##Quick Start,~guide;security;agent;architecture;database
38,6,##Essential Resources,~agents;glossary;001;agent;backend
44,16,##Development Workflow,~agents;branch;push;required;review
60,25,##Build & Test Commands,~test;race;build;detector;lint
85,13,##Architecture at a Glance,~end;tests;api;architecture;business
98,7,##Key Principles,~add;cli;complexity;constitution;driven
105,6,##Meta Documentation,~agent;meta;workflows;agents;collaboration
111,12,##Questions?,~togather;discussions;github;issues;com
-->

# Contributors Guide

Welcome to the SEL backend contributor documentation!

## Who This Is For

You're building features, fixing bugs, or improving the SEL codebase. These docs will help you:
- Understand the system architecture
- Follow development workflows
- Write effective tests
- Maintain security standards

## Quick Start

1. **[Development Guide](development.md)** - Start here for coding standards, logging, validation
2. **[Architecture Overview](architecture.md)** - System design and component structure
3. **[Database Guide](database.md)** - Schema design, migrations, query patterns
4. **[Testing Guide](testing.md)** - TDD workflow, test types, coverage requirements
5. **[Security Guide](security.md)** - Threat model, security implementations, best practices
6. **[Agent Tooling](agent-tooling.md)** - Codex/OpenCode setup, shared skills, custom agents, and MCP connections

## Essential Resources

- **[Glossary](../glossary.md)** - Canonical terminology reference
- **[Spec Kit](../../specs/001-sel-backend/)** - Feature specifications and tasks
- **[AGENTS.md](../../AGENTS.md)** - Agent collaboration guidelines

## Development Workflow

Use your configured task tracker and review process; no particular board or MCP
service is required. See [AGENTS.md](../../AGENTS.md) for repository conventions.

```bash
# Coordinate ownership, create a feature branch, and implement with tests.
make test

# Run required checks, then push the reviewable revision.
make ci
git push
# Request review with repo/branch/SHA and check evidence.
# Resolve feedback, then merge when approved and authorized.
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
make lint-debug-cold  # Debug GH Actions lint failures (cold cache, slow)

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

---

**Last Updated:** 2026-08-07
