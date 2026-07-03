---
name: togather-server-ops
description: Operate a Togather SEL server — authentication, REST API, MCP connectivity, admin endpoints, and data quality patterns. Use when configuring server access, debugging API issues, or understanding the architecture.
license: MIT
compatibility: Requires a running Togather SEL server. Requires curl and jq. Admin operations require admin-role API key.
metadata:
  author: togather-foundation
  version: "2.0"
  hermes:
    tags: [togather, server, admin, api, mcp, data-quality]
    category: devops
    related_skills: [togather-review]
---

# Togather Server Operations

Operate and maintain a Togather Shared Events Library (SEL) instance —
authentication, REST API reference, MCP connectivity, admin endpoints,
and data quality patterns.

## Quick Context

Togather is a Go server (github.com/Togather-Foundation/server) that scrapes
event data from configured sources, normalizes it, and puts flagged events
into a review queue. The review queue holds events with quality warnings
(missing descriptions, date extraction failures, duplicate detection,
cross-week series companions). An admin (human or AI agent) reviews and
approves/rejects/fixes/merges them.

The server exposes:
- **Public REST API** (`/api/v1/events`) — query published events
- **Admin REST API** (`/api/v1/admin/*`) — manage review queue
- **MCP endpoint** (`/mcp`) — agent-native tool access
- **Change feed** (`/api/v1/feeds/changes`) — poll for new/modified events

## Auth: Two-Tier System

The Togather server uses TWO separate auth mechanisms. Know which one you need.

### Agent/Public Auth (API keys)

Used for: MCP endpoint (`/mcp`), public REST API (`/api/v1/events`, `/api/v1/feeds/changes`)

- Header: `Authorization: Bearer <api-key>`
- Created via: `./server api-key create <name>` (CLI) or `manage_api_key` (MCP tool)
- Store in: `~/.hermes/.env` as `TOGATHER_API_KEY`

### Admin Auth (JWT)

Used for: Admin REST endpoints (`/api/v1/admin/*`), review queue operations

- Header: `Authorization: Bearer <jwt-token>`
- Obtained via token exchange: `POST /api/v1/auth/token` with an admin-role API key
- Admin keys stored in: `~/.hermes/.env` as `TOGATHER_ADMIN_API_KEY`
- Created via: `./server api-key create <name> --role admin --expires-in-days 30`

**The admin key itself does NOT work on admin endpoints** — you MUST exchange
it for a JWT first.

### Token Exchange (API Key → JWT)

```bash
curl -s -X POST "$TOGATHER_BASE_URL/api/v1/auth/token" \
  -H "Authorization: Bearer $TOGATHER_ADMIN_API_KEY"
# → { "token": "<jwt>", "expires_at": "<ISO timestamp>" }
```

Or via CLI:
```bash
./server token-exchange --key "$TOGATHER_ADMIN_API_KEY" \
  --server https://your-instance.example.com
```

The `./server review` CLI does this automatically when given `--key`.

## REST API Reference

### Public Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/v1/events` | GET | Agent | List published events with filters |
| `/api/v1/events/{ulid}` | GET | Agent | Get single event (full JSON-LD) |
| `/api/v1/feeds/changes` | GET | Agent | Poll change feed (cursor-based, for sync) |

**Event list parameters:**

| Parameter | Aliases | Values | Notes |
|-----------|---------|--------|-------|
| `startDate` | `start_date` | `2026-07-02` | Defaults to today |
| `endDate` | `end_date` | `2026-07-05` | Inclusive |
| `q` | `search` | free text | Searches name + description |
| `domain` | `event_domain` | arts, music, culture, community, education, sports, general | Single value |
| `keywords` | — | comma-separated | Exact tag match |
| `city` | — | city name | City filter |
| `state` | `lifecycle_state` | published, draft, postponed | Default: published |
| `limit` | — | 200 max | Results per page |
| `after` | — | cursor | Pagination |

**Change feed parameters:**

| Parameter | Value | Notes |
|-----------|-------|-------|
| `since` | ISO timestamp | Cursor for incremental sync |
| `action` | create | Filter by action type |
| `include_snapshot` | true | Include full event data |
| `limit` | 200 max | Per-page limit |

Response includes `changes[]` with `snapshot` (full JSON-LD event) and
`next_cursor` for pagination.

### Admin Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/v1/admin/stats` | GET | JWT | Queue statistics |
| `/api/v1/admin/queue` | GET | JWT | List pending items |
| `/api/v1/admin/queue/{id}` | GET | JWT | Inspect one item |
| `/api/v1/admin/queue/{id}/approve` | POST | JWT | Approve item |
| `/api/v1/admin/queue/{id}/reject` | POST | JWT | Reject item |
| `/api/v1/admin/queue/batch` | POST | JWT | Batch approve/reject |
| `/api/v1/admin/events/{ulid}/merge` | POST | JWT | Merge into primary |
| `/api/v1/auth/token` | POST | Admin API key | Exchange key for JWT |

## MCP vs REST — Capability Gap

The MCP `events` tool is intentionally sparse (public-facing). It returns
only: `@context`, `@id`, `@type`, `location`, `name`, `startDate`.

It does NOT return: `lifecycle_state`, `eventStatus`, `description`, `url`,
`source`, or `quality_warnings`.

**For quality review, use the REST admin API or the `./server review` CLI.**
The MCP is for querying published events, not for admin review work.

MCP is useful for:
- `events` — listing published events (basic fields only)
- `add_event` — submitting new events
- `search` — full-text search across events/places/orgs
- `api_keys` / `manage_api_key` — managing your own API keys
- Geocoding tools

### Connecting MCP to Hermes

```bash
hermes mcp add togather --url https://your-instance.example.com/mcp
hermes mcp test togather
```

The MCP server auto-discovers tools. API key is sent as a bearer token.

## Data Quality Patterns

Common warning types and their prevalence on a typical instance:

| Warning | Typical Cause | Action |
|---------|--------------|--------|
| `missing_description` | Source has no description field | Usually approve if name/location good |
| `cross_week_series_companion` | Weekly recurring event, each occurrence separate | merge-into-primary or approve as individual |
| `multi_session_likely` | Event spans multiple dates | Check if intentional, approve or fix |
| `zero_duration_occurrence` | Start date = end date | Check if date extraction failed |
| `near_duplicate_of_new_event` | Same event scraped from two sources | Check, consolidate or mark not-duplicate |
| `potential_duplicate` | Similar name/location to existing | Check, merge or mark not-duplicate |

**Address field patterns:** The `streetAddress` field availability varies by
source. Some sources populate it directly; others only have the full address
in `location.name`. When extracting venue info for display, check both:

```python3
loc = event.get('location', {})
venue = loc.get('name', '')
street = loc.get('address', {}).get('streetAddress', '')
# street is often empty — fall back to venue name
display_location = street if street else venue
```

## Env Vars Reference

| Variable | Purpose | Where |
|----------|---------|-------|
| `TOGATHER_API_KEY` | Agent/public API key (MCP, public REST) | `~/.hermes/.env` |
| `TOGATHER_ADMIN_API_KEY` | Admin API key (token exchange → JWT) | `~/.hermes/.env` |
| `TOGATHER_BASE_URL` | Server base URL for CLI tools | optional, or use `--server` flag |

## Pitfalls

| Symptom | Cause | Fix |
|---------|-------|-----|
| Admin endpoint returns 401 | Using API key directly instead of JWT | Exchange admin key for JWT via `/api/v1/auth/token` |
| MCP `events` missing description | MCP returns sparse fields by design | Use REST API for rich queries |
| 422 on merge-into-primary | Companion ULID points to already-deleted event | Approve as individual instead |
| `curl` with env vars fails | API key contains special characters | Use double-quotes: `"Bearer $KEY"` |
| `streetAddress` always empty | Source doesn't populate structured address | Fall back to `location.name` |

## Verification

1. Public API works: `curl -s "$TOGATHER_BASE_URL/api/v1/events?limit=1" -H "Authorization: Bearer $TOGATHER_API_KEY"`
2. Token exchange works: `curl -s -X POST "$TOGATHER_BASE_URL/api/v1/auth/token" -H "Authorization: Bearer $TOGATHER_ADMIN_API_KEY"`
3. MCP connects: `hermes mcp test togather`
