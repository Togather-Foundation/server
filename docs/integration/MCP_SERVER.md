# MCP Server Integration

This document describes how to run and integrate the Togather MCP server with AI clients.

## Overview

The MCP server exposes the Togather SEL API via tools, resources, and prompts. It supports three transports:

- **stdio**: Claude Desktop or local agent processes
- **SSE**: simple web deployments
- **Streamable HTTP**: production web services

The MCP server is available as:

- Standalone binary: `cmd/mcp-server`
- Embeddable package: `internal/mcp`
- Optional HTTP endpoint: `/mcp` on the main API server (disabled by default)

## Configuration

Environment variables are shared with the main server configuration, plus MCP-specific settings:

```
MCP_SERVER_NAME="Togather SEL MCP Server"
MCP_SERVER_VERSION="1.0.0"
MCP_TRANSPORT=stdio|sse|http
PORT=8080
HOST=0.0.0.0
MCP_HTTP_ENABLED=false
```

## Running the MCP Server

### stdio (default)

```
./mcp-server
```

### SSE

```
MCP_TRANSPORT=sse PORT=8080 ./mcp-server
```

### Streamable HTTP

```
MCP_TRANSPORT=http PORT=8080 ./mcp-server
```

## Embedding MCP in the Main Server

Set `MCP_HTTP_ENABLED=true` to expose `/mcp` on the main API server. The endpoint is protected by API key auth and agent tier rate limiting.

```
MCP_HTTP_ENABLED=true
```

## Authentication and Rate Limiting

All MCP HTTP and SSE requests require an API key in the `Authorization` header:

```
Authorization: Bearer <api-key>
```

Rate limiting uses the agent tier limits from `RateLimitConfig`.

## Tools

| Tool | Description |
|------|-------------|
| `list_events` | List events with filters and pagination |
| `get_event` | Get a single event by ULID |
| `create_event` | Create an event from JSON-LD |
| `list_places` | List places with filters and pagination |
| `get_place` | Get a single place by ULID |
| `create_place` | Create a place from JSON-LD |
| `list_organizations` | List organizations with filters |
| `get_organization` | Get a single organization by ULID |
| `create_organization` | Create an organization from JSON-LD |
| `search` | Cross-entity search across events/places/orgs |

## Resources

| Resource | Description |
|----------|-------------|
| `context://sel-event` | JSON-LD context for events |
| `context://sel-place` | JSON-LD context for places |
| `context://sel-organization` | JSON-LD context for organizations |
| `schema://openapi` | OpenAPI spec in JSON |
| `info://server` | MCP server metadata |

## Prompts

| Prompt | Description |
|--------|-------------|
| `create_event_from_text` | Parse natural language into JSON-LD event payload |
| `find_venue` | Identify a venue based on requirements |
| `duplicate_check` | Check for potential duplicate events |

## Claude Desktop Configuration (macOS)

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "togather": {
      "command": "/path/to/mcp-server",
      "args": []
    }
  }
}
```

## Troubleshooting

- If stdio transport hangs, ensure nothing writes to stdout.
- If HTTP/SSE returns 401, confirm the API key and Bearer header format.
- If `/mcp` endpoint does not appear, verify `MCP_HTTP_ENABLED=true`.
