# Implementation Plan: MCP Server for Togather API

**Branch**: `002-mcp-server` | **Date**: 2026-02-05 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from user request + MCP protocol documentation

## Summary

Build a Model Context Protocol (MCP) server that exposes the Togather SEL API to AI agents (Claude, ChatGPT, custom agents) through standardized tools, resources, and prompts. The server will:

- Expose read and create operations for events, places, and organizations as MCP tools
- Support stdio (Claude Desktop), SSE, and Streamable HTTP transports for maximum flexibility
- Provide JSON-LD contexts and OpenAPI schema as MCP resources
- Include helpful prompts for common agent tasks (event creation, venue finding, duplicate checking)
- Run as both a standalone binary (`cmd/mcp-server`) and an embeddable package (`internal/mcp`)
- Reuse existing authentication, rate limiting, and business logic from the Togather API
- Follow Go MCP best practices using the `mark3labs/mcp-go` library

## Technical Context

**Language/Version**: Go 1.25+  
**Primary Dependencies**: 
- `github.com/mark3labs/mcp-go` (MCP server SDK)
- Existing Togather dependencies (pgx, SQLc, River, json-gold, etc.)

**Transport Options**:
- **stdio**: For Claude Desktop and spawned agent processes
- **SSE**: For simple web deployments with unidirectional streaming
- **Streamable HTTP**: For production web services with bidirectional communication

**Storage**: Shared PostgreSQL connection pool with main API  
**Authentication**: API key authentication (reusing existing `middleware.AgentAuth`)  
**Rate Limiting**: Tier-based rate limiting (reusing existing middleware)  
**Testing**: go test, integration tests with MCP client  
**Target Platform**: Linux server (Docker), single binary deployment  
**Project Type**: Dual-mode: standalone binary + importable package  
**Performance Goals**: Sub-100ms tool execution latency, support concurrent agent requests  
**Constraints**: 
- Must not write to stdout when using stdio transport (breaks JSON-RPC)
- Must reuse existing services to avoid code duplication
- Must respect SEL semantic web requirements

## Constitution Check

*GATE: Must pass before implementation starts.*

### I. MCP Protocol Compliance (NON-NEGOTIABLE)

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Tools properly defined | 🟡 PENDING | Tool schema with JSON Schema validation |
| Resources accessible | 🟡 PENDING | Resource URIs follow `protocol://path` convention |
| Prompts include arguments | 🟡 PENDING | Prompt templates with argument definitions |
| Stdio transport safe | 🟡 PENDING | No stdout writes, only stderr logging |
| JSON-RPC 2.0 compliance | 🟡 PENDING | Using `mark3labs/mcp-go` which handles protocol |

### II. Integration with Existing System

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Reuses existing services | 🟡 PENDING | `eventsService`, `placesService`, `orgService` |
| Respects authentication | 🟡 PENDING | API key auth middleware |
| Follows rate limits | 🟡 PENDING | Tier-based rate limiting |
| Maintains audit trail | 🟡 PENDING | Audit logging for create operations |
| Preserves provenance | 🟡 PENDING | Source tracking for agent-created entities |

### III. Semantic Web Alignment

| Requirement | Status | Evidence |
|-------------|--------|----------|
| JSON-LD input/output | 🟡 PENDING | Tools accept/return JSON-LD payloads |
| Schema.org compliance | 🟡 PENDING | Validation through existing services |
| License enforcement | 🟡 PENDING | CC0 rejection at tool boundary |
| URI preservation | 🟡 PENDING | Never re-mint URIs from other nodes |

## Architecture Overview

```
cmd/mcp-server/               # Standalone binary
├── main.go                   # Entry point, transport selection
└── config.go                 # MCP-specific configuration

internal/mcp/                 # MCP implementation (importable)
├── server.go                 # MCP server setup and capabilities
├── tools/
│   ├── events.go             # Event tools (list, get, create)
│   ├── places.go             # Place tools (list, get, create)
│   ├── organizations.go      # Org tools (list, get, create)
│   └── search.go             # Cross-entity search
├── resources/
│   ├── context.go            # JSON-LD contexts as resources
│   └── schema.go             # OpenAPI schema as resource
├── prompts/
│   └── templates.go          # Common agent task prompts
└── transport.go              # Transport configuration (stdio/SSE/HTTP)

internal/api/router.go        # OPTIONAL: Mount MCP at /mcp endpoint
```

## Tools to Implement

| Tool Name | Description | Input Parameters | Output |
|-----------|-------------|------------------|--------|
| `list_events` | Search/list events | `query` (string, optional), `start_date` (ISO8601, optional), `end_date` (ISO8601, optional), `location` (string, optional), `limit` (int, default 20), `cursor` (string, optional) | JSON array of events |
| `get_event` | Get single event by ID | `id` (string, required) | JSON-LD event object |
| `create_event` | Create new event | `event` (JSON-LD object, required) | Created event with ID |
| `list_places` | Search/list places | `query` (string, optional), `near_lat` (float, optional), `near_lon` (float, optional), `radius` (float, optional), `limit` (int, default 20), `cursor` (string, optional) | JSON array of places |
| `get_place` | Get single place by ID | `id` (string, required) | JSON-LD place object |
| `create_place` | Create new place | `place` (JSON-LD object, required) | Created place with ID |
| `list_organizations` | Search/list organizations | `query` (string, optional), `limit` (int, default 20), `cursor` (string, optional) | JSON array of orgs |
| `get_organization` | Get single org by ID | `id` (string, required) | JSON-LD org object |
| `create_organization` | Create new organization | `organization` (JSON-LD object, required) | Created org with ID |
| `search` | Cross-entity semantic search | `query` (string, required), `types` (array of strings, optional), `limit` (int, default 20) | Mixed array of results |

## Resources to Expose

| Resource URI | MIME Type | Description |
|--------------|-----------|-------------|
| `context://sel-event` | application/ld+json | SEL Event JSON-LD context |
| `context://sel-place` | application/ld+json | SEL Place JSON-LD context |
| `context://sel-organization` | application/ld+json | SEL Organization JSON-LD context |
| `schema://openapi` | application/json | Complete OpenAPI 3.1 spec |
| `info://server` | application/json | Server version, capabilities, node info |

## Prompts to Include

| Prompt Name | Description | Arguments |
|-------------|-------------|-----------|
| `create_event_from_text` | Parse natural language event description and create structured event | `description` (string), `default_timezone` (string, optional) |
| `find_venue` | Find suitable venue for event requirements | `requirements` (string), `location` (string), `capacity` (int, optional) |
| `duplicate_check` | Check if event already exists before creating | `event_description` (string), `date` (string), `location` (string) |

## Implementation Phases

### Phase 0: Dependency Setup (1 bead)
- Add `github.com/mark3labs/mcp-go` to go.mod
- Verify compatibility with existing dependencies
- Update documentation with MCP overview

### Phase 1: Core Infrastructure (3 beads)
- Create `internal/mcp/server.go` with server initialization
- Implement transport selection logic (stdio/SSE/HTTP)
- Create `cmd/mcp-server/main.go` standalone binary
- Add configuration loading (environment variables + flags)

### Phase 2: Tools - Events (3 beads)
- Implement `list_events` tool with filtering
- Implement `get_event` tool
- Implement `create_event` tool with validation
- Add error handling and JSON-LD serialization

### Phase 3: Tools - Places (2 beads)
- Implement `list_places` and `get_place` tools
- Implement `create_place` tool
- Add geospatial query support

### Phase 4: Tools - Organizations (2 beads)
- Implement `list_organizations` and `get_organization` tools
- Implement `create_organization` tool

### Phase 5: Search Tool (1 bead)
- Implement cross-entity `search` tool
- Add type filtering and ranking

### Phase 6: Resources (2 beads)
- Expose JSON-LD contexts as MCP resources
- Expose OpenAPI spec and server info as resources

### Phase 7: Prompts (2 beads)
- Implement `create_event_from_text` prompt
- Implement `find_venue` and `duplicate_check` prompts

### Phase 8: Integration & Testing (3 beads)
- Add MCP endpoint to existing router (optional embed mode)
- Write integration tests with MCP client
- Add authentication and rate limiting integration
- Performance testing

### Phase 9: Documentation (1 bead)
- Add usage guide for Claude Desktop configuration
- Document tool schemas and examples
- Add troubleshooting guide

## Key Design Decisions

### Authentication
- **Decision**: Require API key authentication for all tool calls
- **Rationale**: Maintains consistency with existing agent auth, enables usage tracking
- **Implementation**: Wrap tools with existing `middleware.AgentAuth`

### Rate Limiting
- **Decision**: Apply tier-based rate limiting (same as existing API)
- **Rationale**: Prevents abuse, agent tier allows higher limits than public
- **Implementation**: Reuse existing rate limiting middleware

### Logging
- **Decision**: Use stderr for stdio transport, structured logging for HTTP/SSE
- **Rationale**: stdout corruption breaks JSON-RPC in stdio mode
- **Implementation**: Configure logger based on transport type

### Service Reuse
- **Decision**: Inject existing services (`eventsService`, `placesService`, etc.)
- **Rationale**: Single source of truth, no code duplication, maintains business rules
- **Implementation**: Accept services as constructor parameters

### Provenance Tracking
- **Decision**: Tag agent-created entities with source="mcp-agent"
- **Rationale**: Enables audit trail, distinguishes agent vs human submissions
- **Implementation**: Add source metadata in tool implementations

### Error Handling
- **Decision**: Return MCP-compliant error responses with RFC 7807 details
- **Rationale**: Maintains consistency with existing API error format
- **Implementation**: Convert internal errors to `mcp.NewToolResultError()`

## Testing Strategy

### Unit Tests
- Tool input validation
- Resource content generation
- Prompt template rendering
- Transport configuration

### Integration Tests
- End-to-end tool execution with real database
- MCP client connection and tool invocation
- Authentication and rate limiting enforcement
- Multi-transport support verification

### Manual Testing
- Claude Desktop integration (stdio)
- Web client integration (SSE/HTTP)
- Concurrent agent requests
- Error condition handling

## Success Criteria

- [ ] All 10 tools implemented and functional
- [ ] All 5 resources accessible
- [ ] All 3 prompts working with argument substitution
- [ ] All 3 transports (stdio, SSE, HTTP) operational
- [ ] Authentication and rate limiting enforced
- [ ] Integration tests passing with 80%+ coverage
- [ ] Claude Desktop can connect and use tools
- [ ] Documentation complete with examples
- [ ] No regressions in existing API functionality

## Dependencies & Blockers

**Prerequisites**:
- Existing API must be functional (events, places, orgs CRUD)
- Database schema must be stable (no migrations during MCP dev)
- Authentication and rate limiting middleware must be working

**External Dependencies**:
- `mark3labs/mcp-go` library (stable, well-maintained)
- MCP protocol specification (stable v1.0)

**Potential Blockers**:
- Concurrent development on main API could cause merge conflicts
- Changes to service interfaces would require MCP tool updates
- Database schema changes would require tool parameter updates

## Parallel Development Strategy

### Branch Management
```bash
# Create feature branch from current main
git checkout main
git pull
git checkout -b 002-mcp-server

# Keep branch updated with main via rebase
git fetch origin
git rebase origin/main
```

### Development Workflow
1. **Isolation**: MCP server is additive (new cmd/, new internal/mcp/)
2. **Minimal Conflicts**: Only touches `go.mod` and optionally `internal/api/router.go`
3. **Safe Integration**: Can merge anytime without breaking existing functionality
4. **Testing**: Can run both servers simultaneously on different ports

### OpenCode Multi-Branch Workflow
To work on MCP server while building other features:

```bash
# Terminal 1 - MCP server development
cd /path/to/togather/server
git checkout 002-mcp-server
opencode  # or your OpenCode command

# Terminal 2 - Main feature development
cd /path/to/togather/server
git checkout main  # or another feature branch
opencode  # or your OpenCode command
```

**Key Points**:
- OpenCode instances work independently per terminal/directory
- Each can track its own beads for the respective branch
- Use `bd sync` regularly to push bead state to git
- Merge conflicts are minimal due to additive nature of MCP work

### Merge Strategy
```bash
# When ready to merge MCP server to main
git checkout main
git pull
git merge 002-mcp-server
# Resolve any conflicts (likely only go.mod and go.sum)
make ci  # Run full test suite
git push
```

## Timeline Estimate

**Total**: ~20 beads × ~2-4 hours per bead = **40-80 hours** (5-10 business days)

- Phase 0: 0.5 days
- Phase 1: 1.5 days
- Phase 2: 1.5 days
- Phase 3: 1 day
- Phase 4: 1 day
- Phase 5: 0.5 days
- Phase 6: 1 day
- Phase 7: 1 day
- Phase 8: 1.5 days
- Phase 9: 0.5 days

**Cadence**: Can be developed in parallel with other features due to additive nature.

## References

- [MCP Protocol Documentation](https://modelcontextprotocol.io/docs/getting-started/intro)
- [mark3labs/mcp-go GitHub](https://github.com/mark3labs/mcp-go)
- [SEL Backend Spec](../001-sel-backend/spec.md)
- [Togather API OpenAPI Spec](../../docs/api/openapi.yaml)

## Approvals

- [ ] Technical design review
- [ ] Security review (API key auth, rate limiting)
- [ ] Architecture review (service reuse, transport options)
- [ ] Ready to implement

---

**Status**: 🟡 DRAFT - Awaiting approval to begin implementation  
**Last Updated**: 2026-02-05  
**Next Action**: Create beads for all implementation tasks
