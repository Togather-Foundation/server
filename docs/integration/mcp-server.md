# MCP Server Integration

This document describes how to connect AI clients to the Togather MCP server.

## Overview

The MCP server exposes the Togather SEL API via tools, resources, and prompts. It supports three transports:

- **stdio**: Claude Desktop or local agent processes
- **SSE**: simple web deployments
- **Streamable HTTP**: production web services (this is what `/mcp` uses on the main server)

The MCP server is available as:

- **Embedded endpoint**: `/mcp` on the main API server (this is the recommended and deployed path — see [Authentication](../interop/core-profile-v0.1.md) for node URLs)
- Standalone binary: `cmd/mcp-server` (for local/stdio setups)

> **Authentication model (read this first):** the server has **no OAuth**. Auth is a
> static API key in an `Authorization: Bearer <key>` header. **The read-only tools are
> public — no key needed.** The six read tools (`events`, `places`, `organizations`,
> `search`, `geocode_address`, `reverse_geocode`) work anonymously, exactly like the
> public REST read endpoints. The key is only required for the write/account tools
> (`add_event`, `api_keys`, `manage_api_key`) and it enables per-agent rate limits and
> usage stats.
>
> **Register `/mcp` as a *local* MCP server** with a custom header. Do **not** use a
> claude.ai *remote connector* — remote connectors speak only OAuth and cannot send a
> static header, so they will fail with "still unauthorized" (that is expected, not a
> login problem). Get a free key at `/dev/login` (GitHub OAuth, instant) for the write
> tools.

## Configuration

Environment variables are shared with the main server configuration, plus MCP-specific settings:

```
MCP_TRANSPORT=stdio|sse|http
PORT=8080
HOST=0.0.0.0
MCP_HTTP_ENABLED=false
```

## Running the MCP Server

### Embedded in the main server (recommended)

Set `MCP_HTTP_ENABLED=true` to expose `/mcp` on the main API server. Read-only tools
are public; write/account tools require an API key and are rate-limited at the agent tier:

```
MCP_HTTP_ENABLED=true
```

### Standalone binary

```
./mcp-server
```

For stdio, run it directly. For SSE/HTTP, set the transport and port:

```
MCP_TRANSPORT=sse PORT=8080 ./mcp-server
MCP_TRANSPORT=http PORT=8080 ./mcp-server
```

## Authentication and Rate Limiting

The **read-only tools are public** — anonymous sessions may call `events`, `places`,
`organizations`, `search`, `geocode_address`, and `reverse_geocode` without any header.
Anonymous requests are rate-limited at the public tier (IP-keyed).

The **write/account tools** (`add_event`, `api_keys`, `manage_api_key`) require an API
key in the `Authorization` header:

```
Authorization: Bearer <api-key>
```

They are rate-limited at the agent tier from `RateLimitConfig`. Get a key at `/dev/login`
(instant, GitHub OAuth) or by email invitation.

Behavior on the wire:

- **Anonymous + read tool** → 200, results returned.
- **Anonymous + write tool** → the request is accepted (HTTP 200) but returns a JSON-RPC
  error: `tool "add_event" requires an API key (...)`. This is a per-tool denial, not an
  HTTP 401 — the server intentionally keeps one endpoint for both surfaces.
- **Invalid key** (present but wrong) → HTTP `401` with `WWW-Authenticate: Bearer`.
- **`tools/list`** shows 6 tools anonymously and all 9 with a valid key.

Every tool also carries MCP tool annotations (`title`, `readOnlyHint`/`destructiveHint`)
so conformant clients know which tools are safe to call without a confirmation prompt.

## Tools

The MCP server provides 9 tools:

| Tool | Auth | Description |
|------|------|-------------|
| `events` | public | List events with filters and pagination, OR get a single event by ULID (if `id` parameter provided) |
| `search` | public | Cross-entity search across events/places/orgs |
| `places` | public | List places with filters and pagination, OR get a single place by ULID (if `id` parameter provided) |
| `organizations` | public | List organizations with filters, OR get a single organization by ULID (if `id` parameter provided) |
| `geocode_address` | public | Geocode an address or place name to coordinates |
| `reverse_geocode` | public | Reverse geocode coordinates to a human-readable address |
| `add_event` | **API key** | Create an event from JSON-LD (requires API key) |
| `api_keys` | **API key** | List API keys and usage statistics for the authenticated developer |
| `manage_api_key` | **API key** | Create or revoke API keys (requires API key) |

### Unified List/Get Operations

The `events`, `places`, and `organizations` tools accept an optional `id` parameter:
- **With `id` parameter**: Returns a single entity (e.g., `{"id": "01KGSV7H8ZDHTYTV6QKFGMFFMZ"}`)
- **Without `id` parameter**: Returns a list of entities matching filter criteria

Examples:

```javascript
// List events
{"query": "tech conference", "city": "Toronto", "limit": 10}

// Get specific event by ULID
{"id": "01KGSV7H8ZDHTYTV6QKFGMFFMZ"}
```

### REST-style parameter aliases

MCP tools use snake_case parameter names, while the REST API uses camelCase.
To avoid silent no-op filters when carrying names across interfaces, the MCP
tools **accept both conventions** — REST-style aliases are normalized to their
snake_case equivalents automatically:

| MCP (snake_case) | REST alias |
|---|---|
| `query` | `q` |
| `start_date` | `startDate` |
| `end_date` | `endDate` |
| `cursor` | `after` |
| `near_lat` | `nearLat` |
| `near_lon` | `nearLon` |
| `radius` | `radiusKm` |

When both spellings are supplied, the explicit snake_case value wins.

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

### Common Issues and Solutions

#### stdio transport hangs or no response
- **Cause**: Application writes to stdout, interfering with JSON-RPC protocol
- **Solution**: Set `LOG_OUTPUT=stderr` or redirect logs: `./mcp-server 2>mcp.log`
- **Verification**: Check Claude Desktop logs at `~/Library/Logs/Claude/mcp-server-togather.log`

#### Write tool denied with "requires an API key"
- **Cause**: Anonymous session tried to call a write/account tool (`add_event`,
  `api_keys`, `manage_api_key`). Read-only tools work without a key.
- **Solution**: Add the Bearer token to the Authorization header:
  ```bash
  curl -H "Authorization: Bearer YOUR_API_KEY" https://localhost:8080/mcp
  ```
- **Verification**: Check if API key exists: `psql $DATABASE_URL -c "SELECT key_prefix FROM api_keys LIMIT 1;"`

#### HTTP/SSE returns 401 Unauthorized
- **Cause**: An Authorization header was present but the key is missing or invalid.
  (Anonymous requests are allowed — a 401 here means you sent a bad credential, which
  is never silently downgraded.)
- **Solution**: Provide a valid key, or omit the header entirely to use the public
  read-only surface.

#### `/mcp` endpoint returns 404
- **Cause**: MCP HTTP endpoint not enabled
- **Solution**: Set `MCP_HTTP_ENABLED=true` in environment or `.env`
- **Verification**: Check startup logs for "MCP HTTP endpoint enabled at /mcp"

#### Rate limit exceeded (429 Too Many Requests)
- **Cause**: Exceeded agent tier rate limit
- **Solution**: Review `RateLimitConfig` settings or upgrade API key tier
- **Log example**:
  ```
  2026-02-06T10:30:15Z ERROR rate_limit_exceeded key_prefix=abc123 limit=100 window=60s
  ```

#### Tools return empty results
- **Cause**: Database contains no events/places/organizations, or filters are too restrictive
- **Solution**: 
  - Check database: `psql $DATABASE_URL -c "SELECT COUNT(*) FROM events;"`
  - Remove filters and retry: `{"limit": 10}` instead of complex filters
  - Verify data ingestion is working

#### Connection refused or timeout
- **Cause**: Server not running or wrong port
- **Solution**: 
  - Check process: `ps aux | grep mcp-server`
  - Verify port: `lsof -i :8080` or `netstat -an | grep 8080`
  - Check firewall rules

## Testing with curl

### HTTP Transport

#### Initialize MCP session
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "curl-test",
        "version": "1.0.0"
      }
    }
  }'
```

**Expected response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {},
      "resources": {},
      "prompts": {}
    },
    "serverInfo": {
      "name": "Togather SEL MCP Server",
      "version": "1.0.0"
    }
  }
}
```

#### List available tools
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

#### Call the events tool
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "events",
      "arguments": {
        "limit": 5,
        "include_past": false
      }
    }
  }'
```

**Expected response:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"items\":[{\"@context\":\"...\",\"@type\":\"Event\",\"@id\":\"01HZXY...\",\"name\":\"Community Meetup\",\"startDate\":\"2026-02-15T19:00:00Z\"}],\"next_cursor\":\"abc123\"}"
      }
    ]
  }
}
```

> **Output-budget tip:** the events tool can be passed `context: "document"`.
> That emits a single top-level `@context` scoping every item instead of a full
> `@context` block on each item, roughly halving the response — handy when a
> large page would otherwise exceed the MCP tool-output limit. Individual items
> can be re-standalized by merging the document-level `@context` back in.

#### Get a specific event
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "events",
      "arguments": {
        "id": "01HZXY..."
      }
    }
  }'
```

#### Read context resource
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "resources/read",
    "params": {
      "uri": "context://sel-event"
    }
  }'
```

### SSE Transport

SSE uses GET requests with query parameters:

```bash
# Initialize and list events
curl -N -H "Authorization: Bearer YOUR_API_KEY" \
  'http://localhost:8080/mcp?method=tools/list'
```

Note: SSE responses are streamed as `data:` lines. Use `-N` to disable buffering.

### stdio Transport

stdio uses JSON-RPC over stdin/stdout. Test with echo and pipes:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}' | ./mcp-server
```

## Log Samples

### Normal Operation (HTTP transport)

```
2026-02-06T10:15:30Z INFO  server_starting transport=http port=8080 mcp_version=2024-11-05
2026-02-06T10:15:30Z INFO  database_connected pool_size=10 max_conns=25
2026-02-06T10:15:30Z INFO  mcp_initialized tools=9 resources=5 prompts=3
2026-02-06T10:15:45Z INFO  request method=initialize client=curl-test duration_ms=12
2026-02-06T10:15:50Z INFO  request method=tools/call tool=events duration_ms=45 results=5
```

### Error Cases

#### Invalid JSON-RPC format
```
2026-02-06T10:20:15Z ERROR invalid_request error="missing jsonrpc field" body_preview="{\"method\":..."
```

#### Missing API key
```
2026-02-06T10:21:00Z WARN  auth_failed reason="missing Authorization header" ip=192.168.1.100
```

#### Tool execution error
```
2026-02-06T10:22:30Z ERROR tool_call_failed tool=events id=01HZXY... error="event not found" duration_ms=8
```

#### Database connection error
```
2026-02-06T10:23:00Z ERROR db_connection_failed error="connection refused" host=localhost:5432 retry_in=5s
```

#### Rate limit exceeded
```
2026-02-06T10:24:15Z WARN  rate_limit_exceeded key_prefix=abc123 tier=agent limit=100 window=60s
```

## Error Codes

The MCP server returns JSON-RPC 2.0 error responses following the specification:

### Standard JSON-RPC Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32700 | Parse error | Invalid JSON received |
| -32600 | Invalid Request | Missing required JSON-RPC fields |
| -32601 | Method not found | Unknown method (e.g., typo in `tools/call`) |
| -32602 | Invalid params | Missing or malformed parameters |
| -32603 | Internal error | Server-side error during execution |

### MCP-Specific Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32001 | Unauthorized | Missing or invalid API key |
| -32002 | Rate limit exceeded | Too many requests in time window |
| -32003 | Resource not found | Requested event/place/org does not exist |
| -32004 | Validation error | Input failed schema/SHACL validation |
| -32005 | Database error | Failed to query or write to database |

### Error Response Format

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "error": {
    "code": -32003,
    "message": "Resource not found",
    "data": {
      "type": "https://togather.foundation/errors/not-found",
      "title": "Event Not Found",
      "status": 404,
      "detail": "No event with ID 01HZXY... exists",
      "instance": "/mcp/tools/call"
    }
  }
}
```

The `data` field follows RFC 7807 (Problem Details) for structured error information.

## Testing Tips

### Verify MCP Server is Running

1. **Check process**: `ps aux | grep mcp-server`
2. **Check port binding**: `lsof -i :8080` or `netstat -tuln | grep 8080`
3. **Test health endpoint** (if available): `curl http://localhost:8080/health`

### Validate Authentication

```bash
# Read-only tools work without a key (public)
curl -X POST http://localhost:8080/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'

# Write tools need a key (JSON-RPC error, not 401, when anonymous)
curl -X POST http://localhost:8080/mcp \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add_event","arguments":{}}}'

# Test with invalid key (present but wrong → HTTP 401)
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer invalid" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

### Test Each Tool

Use the `tools/list` method first to get available tools and their schemas, then call each tool with minimal valid arguments:

```bash
# events with no filters
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"events","arguments":{}}}

# get a single event (replace with valid ULID)
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"events","arguments":{"id":"01HZXY..."}}}

# search across all entities
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"meetup","limit":5}}}
```

### Test Resources

```bash
# List all resources
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}'

# Read each resource
for uri in "context://sel-event" "context://sel-place" "schema://openapi"; do
  curl -X POST http://localhost:8080/mcp \
    -H "Authorization: Bearer YOUR_API_KEY" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"resources/read\",\"params\":{\"uri\":\"$uri\"}}"
done
```

### Monitor Logs

Run the server with verbose logging to see detailed request/response flow:

```bash
LOG_LEVEL=debug MCP_TRANSPORT=http PORT=8080 ./mcp-server 2>&1 | tee mcp-debug.log
```

### Test Rate Limiting

```bash
# Send 150 requests rapidly (if limit is 100/minute)
for i in {1..150}; do
  curl -X POST http://localhost:8080/mcp \
    -H "Authorization: Bearer YOUR_API_KEY" \
    -d '{"jsonrpc":"2.0","id":'$i',"method":"tools/list","params":{}}' &
done
wait
# Should see 429 responses after threshold
```

### Debugging stdio Transport

If Claude Desktop integration isn't working:

1. **Check Claude Desktop logs**:
   ```bash
   tail -f ~/Library/Logs/Claude/mcp-server-togather.log
   ```

2. **Test stdio manually**:
   ```bash
   # Send initialize, then list tools
   (echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'; \
    echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}') | \
   ./mcp-server 2>mcp-stderr.log
   ```

3. **Verify no stdout pollution**:
   ```bash
   # Should only see JSON-RPC responses, no log messages
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | \
   ./mcp-server 2>/dev/null | jq .
   ```

---

**Last Updated:** 2026-08-09
