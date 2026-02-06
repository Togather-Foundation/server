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

### Common Issues and Solutions

#### stdio transport hangs or no response
- **Cause**: Application writes to stdout, interfering with JSON-RPC protocol
- **Solution**: Set `LOG_OUTPUT=stderr` or redirect logs: `./mcp-server 2>mcp.log`
- **Verification**: Check Claude Desktop logs at `~/Library/Logs/Claude/mcp-server-togather.log`

#### HTTP/SSE returns 401 Unauthorized
- **Cause**: Missing or invalid API key
- **Solution**: Include Bearer token in Authorization header:
  ```bash
  curl -H "Authorization: Bearer YOUR_API_KEY" https://localhost:8080/mcp
  ```
- **Verification**: Check if API key exists: `psql $DATABASE_URL -c "SELECT key_prefix FROM api_keys LIMIT 1;"`

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

#### Call list_events tool
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "list_events",
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
      "name": "get_event",
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
2026-02-06T10:15:50Z INFO  request method=tools/call tool=list_events duration_ms=45 results=5
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
2026-02-06T10:22:30Z ERROR tool_call_failed tool=get_event id=01HZXY... error="event not found" duration_ms=8
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
# Test with invalid key (should return 401)
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer invalid" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'

# Test without auth header (should return 401)
curl -X POST http://localhost:8080/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

### Test Each Tool

Use the `tools/list` method first to get available tools and their schemas, then call each tool with minimal valid arguments:

```bash
# list_events with no filters
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_events","arguments":{}}}

# get_event (replace with valid ULID)
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_event","arguments":{"id":"01HZXY..."}}}

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
