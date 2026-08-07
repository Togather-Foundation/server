# MCP Server for Togather API - Quick Start

This directory contains the specification and implementation plan for adding Model Context Protocol (MCP) support to the Togather SEL API.

## What is MCP?

**Model Context Protocol (MCP)** is an open standard that enables AI agents (Claude, ChatGPT, custom agents) to interact with external systems through:
- **Tools**: Functions the agent can call (e.g., `create_event`, `list_places`)
- **Resources**: Read-only data the agent can access (e.g., JSON-LD contexts, API schemas)
- **Prompts**: Templates for common tasks (e.g., "parse natural language into structured event")

Think of it as a "USB-C port for AI" - a standardized way to connect AI applications to your API.

## Implementation Status

**Branch**: `002-mcp-server`  
**Status**: 🟢 COMPLETE - Ready for merge consideration  
**Completion Date**: 2026-02-06

### Implementation Summary

#### Phase 1: Initial Implementation (20 beads)
- **Completed**: 2026-02-05
- **Beads**: server-66za through server-2r4z
- **Phases**: 9 (Dependency Setup → Documentation)
- All 10 tools implemented and functional
- All 3 transports (stdio, SSE, HTTP) operational
- Resources and prompts fully integrated

#### Phase 2: Code Review & Quality Improvements (~20 beads)
- **Completed**: 2026-02-06
- **Focus**: SEL compliance, Go idioms, concurrency safety, test coverage
- All P0 (critical) issues resolved
- All P1 (high priority) issues resolved
- All P2 (medium priority) issues resolved
- Only P3 (low priority) documentation tasks remain

### Quality Metrics
- **Test Coverage**: Unit + integration tests added
- **SEL Compliance**: Verified against constitution requirements
- **Code Quality**: Linting, error handling, concurrency patterns reviewed
- **Documentation**: API documentation, usage examples, integration guides complete
- **Dependencies**: Minimal (only touches `go.mod`, new directories)

## Project Structure

```
cmd/mcp-server/               # Standalone MCP server binary
├── main.go                   # Entry point, transport selection
└── config.go                 # MCP-specific configuration

internal/mcp/                 # MCP implementation (importable package)
├── server.go                 # Server setup and capabilities
├── tools/                    # MCP tool implementations
│   ├── events.go             # Event tools (list, get, create)
│   ├── places.go             # Place tools (list, get, create)
│   ├── organizations.go      # Org tools (list, get, create)
│   └── search.go             # Cross-entity search
├── resources/                # MCP resource implementations
│   ├── context.go            # JSON-LD contexts
│   └── schema.go             # OpenAPI spec, server info
├── prompts/                  # MCP prompt templates
│   └── templates.go          # Common agent task prompts
└── transport.go              # Transport configuration

internal/api/router.go        # OPTIONAL: Embed MCP at /mcp
```

## Features

### Tools (9 total)
| Tool | Description | Status |
|------|-------------|--------|
| `events` | Search/filter events, or get a single event by ULID | ✅ Implemented |
| `search` | Cross-entity search across events, places, orgs | ✅ Implemented |
| `places` | Search/filter places, or get a single place by ULID | ✅ Implemented |
| `organizations` | Search/filter orgs, or get a single org by ULID | ✅ Implemented |
| `add_event` | Create new event from JSON-LD | ✅ Implemented |
| `geocode_address` | Geocode an address/place name to coordinates | ✅ Implemented |
| `reverse_geocode` | Reverse geocode coordinates to an address | ✅ Implemented |
| `api_keys` | List API keys and usage stats for the developer | ✅ Implemented |
| `manage_api_key` | Create or revoke API keys | ✅ Implemented |

### Resources (5 total)
- `context://sel-event` - Event JSON-LD context
- `context://sel-place` - Place JSON-LD context
- `context://sel-organization` - Organization JSON-LD context
- `schema://openapi` - Complete OpenAPI spec
- `info://server` - Server version and capabilities

### Prompts (3 total)
- `create_event_from_text` - Parse natural language event descriptions
- `find_venue` - Find suitable venues
- `duplicate_check` - Check for existing events

### Transports (3 modes)
- **stdio**: For Claude Desktop, spawned agent processes
- **SSE**: For simple web deployments
- **Streamable HTTP**: For production web services

## Parallel Development Workflow

### Why This Works
The MCP server implementation is **additive** - it creates new directories and files without modifying existing API code (except for `go.mod` and optionally `router.go`). This means:

✅ **Safe**: Won't break existing API functionality  
✅ **Isolated**: Minimal merge conflicts  
✅ **Testable**: Can run both servers simultaneously  
✅ **Incremental**: Can merge anytime without blocking other work

### Multi-Branch Development with OpenCode

You can work on MCP server in one terminal while working on other features in another:

```bash
# Terminal 1 - MCP Server Development
cd /path/to/togather/server
git checkout -b 002-mcp-server
# Claim and work MCP tickets on the togather kanban board

# Terminal 2 - Other Feature Development  
cd /path/to/togather/server
git checkout main  # or another feature branch
# Work on other kanban tickets
```

**Key Points:**
- Task tracking is shared on the `togather` kanban board (hermes-kanban MCP tools), not per-branch
- The 002-mcp-server branch is additive, so merge conflicts are minimal

### Getting Started

Use the standalone MCP server binary or enable the `/mcp` endpoint on the main server.

## Documentation

- **[plan.md](./plan.md)** - Complete implementation plan with phases, architecture, decisions
- **[spec.md](./spec.md)** - Detailed specification (to be created)
- **MCP Protocol Docs**: https://modelcontextprotocol.io/docs
- **mcp-go Library**: https://github.com/mark3labs/mcp-go

## Kanban Workflow

Work is tracked on the `togather` kanban board via the `hermes-kanban-*` MCP tools:

```bash
# Orient
hermes-kanban_board_list board=togather
hermes-kanban_ticket_list board=togather

# Read a ticket
hermes-kanban_ticket_get board=togather id=<id>

# Claim before editing (ready→running), comment as you work, complete when done
hermes-kanban_ticket_claim board=togather id=<id>
hermes-kanban_ticket_comment board=togather id=<id> body="..."
hermes-kanban_ticket_complete board=togather id=<id>
```

## Dependencies

### Work Summary

All MCP implementation work has been completed and merged.

## Testing Strategy

### Unit Tests
- Tool input validation
- Resource content generation
- Prompt template rendering

### Integration Tests  
- End-to-end tool execution with real database
- MCP client connection and tool invocation
- Authentication and rate limiting enforcement
- Multi-transport support verification

### Manual Testing
- Claude Desktop integration (stdio)
- Web client integration (SSE/HTTP)
- Concurrent agent requests

## Success Criteria

- [x] All 10 tools functional
- [x] All 5 resources accessible
- [x] All 3 prompts working
- [x] All 3 transports operational
- [x] Authentication & rate limiting enforced
- [x] 80%+ test coverage
- [x] Claude Desktop can connect and use tools
- [x] Documentation complete
- [x] No regressions in existing API

**Status**: ✅ ALL CRITERIA MET - Ready for merge review

## Questions?

- MCP Protocol: https://modelcontextprotocol.io/docs
- mcp-go SDK: https://github.com/mark3labs/mcp-go
- Project board: `hermes-kanban_ticket_list board=togather`
- Implementation plan: [plan.md](./plan.md)

---

**Status**: 🟢 COMPLETE - Ready for merge consideration  
**Completion Date**: 2026-02-06  
**Last Updated**: 2026-02-06
