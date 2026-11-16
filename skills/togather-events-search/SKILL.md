---
name: togather-events-search
description: Search events from a Togather SEL instance — query the public API with domain, keyword, node locality, date, or coordinate-radius filters. Use when looking for events in a specific area or when the user asks "what's happening" and has a Togather instance configured.
license: MIT
metadata:
  author: togather-foundation
  version: "2.0"
  hermes:
    tags: [togather, events, search, discovery, api]
    category: productivity
    related_skills: [togather-events-install]
---

# Togather Events Search

Query events from a Togather Shared Events Library instance. Use the
public REST API with domain, keyword, date, node-boundary locality, and
coordinate-radius filters. Each node serves only its configured geographic
scope; proximity narrows that node's inventory and does not discover events
from other nodes. Also
covers the MCP endpoint for simpler queries.

Requires `curl`, Python 3.9+, public API access, and `TOGATHER_BASE_URL` plus
`TOGATHER_API_KEY` (the key is needed only for authenticated operations).

## When to Use

- User asks "what's happening this weekend" or "find events in [city]"
- User wants to search by domain (arts, music, community, education)
- User wants ad-hoc queries that don't need the full curation pipeline
- User needs to find specific events by name, venue, or date range

For automated daily curation, use `togather-events-install` instead.

## Quick Reference

| Task | Approach |
|------|----------|
| List today's events | `GET /api/v1/events?startDate=YYYY-MM-DD&limit=50` |
| Search by keyword | `GET /api/v1/events?q=dance+workshop&limit=50` |
| Filter by domain | `GET /api/v1/events?domain=arts&startDate=...` |
| Node locality | `GET /api/v1/events?city=Toronto&limit=200` (checks configured node boundary; not an event-address filter) |
| Nearby events | `GET /api/v1/events?near_lat=43.6532&near_lon=-79.3832&radius=5&limit=50` (within this node only) |
| Get single event | `GET /api/v1/events/{ulid}` |
| MCP (sparse fields) | Connect to `/mcp` endpoint with API key |

## Setup

Set environment variables before use. Add to `~/.hermes/.env`:

```bash
TOGATHER_BASE_URL=https://your-instance.example.com
TOGATHER_API_KEY=your-api-key-here
```

Reload env in your Hermes session: `/reload`

If these are not set, ask the user for their server URL and API key.

## Procedure

### Phase 1: Determine the Query

Ask the user what they're looking for, or infer from the conversation:

- **Date**: today, tomorrow, this weekend, a specific date
- **Domain**: arts, music, culture, community, education, sports, general
- **Keywords**: free-text search across name and description
- **City/region**: locality request checked against this node's configured geographic boundary; it does not filter each event's address, and an unconfigured dimension is not filtered
- **Node scope**: each node serves its configured city/region boundary, which may span multiple municipalities; the query cannot search across nodes
- **Nearby**: when coordinates are available, finite latitude and longitude plus an optional radius in kilometres
- **Limit**: how many results (max 200 per request)

If the user doesn't specify, default to today, all domains, limit 50.

### Phase 2: Query the API

Use `execute_code` or `terminal` with curl. Authorization uses the
`TOGATHER_API_KEY` env var:

**Direct curl:**
```bash
curl -s "$TOGATHER_BASE_URL/api/v1/events?startDate=$(date +%Y-%m-%d)&limit=50" \
  -H "Authorization: Bearer $TOGATHER_API_KEY" | python3 -m json.tool
```

**Python (execute_code):**
```python3
from hermes_tools import terminal
import json

result = terminal(
    command=f'curl -s "$TOGATHER_BASE_URL/api/v1/events?startDate=2026-07-02&domain=arts&limit=50" '
            f'-H "Authorization: Bearer $TOGATHER_API_KEY"'
)
data = json.loads(result['output'])
for event in data.get('items', []):
    name = event.get('name', '?')
    start = event.get('startDate', '?')
    venue = (event.get('location') or {}).get('name', '?')
    print(f"{start[:10]} | {name} @ {venue}")
```

### Phase 3: API Reference

**Base endpoint:** `{TOGATHER_BASE_URL}/api/v1/events`

| Parameter | Aliases | Values | Notes |
|-----------|---------|--------|-------|
| `startDate` | `start_date` | `2026-07-02` | Defaults to today if unset |
| `endDate` | `end_date` | `2026-07-05` | Inclusive |
| `q` | `search` | `free text` | Searches name + description |
| `domain` | `event_domain` | `arts`, `music`, `culture`, `community`, `education`, `sports`, `general` | Single value |
| `keywords` | — | `kids,family,workshop` | Comma-separated |
| `city` | — | `Toronto` | Checks against the node's configured locality boundary; matching city includes the node inventory, not just events whose address says Toronto |
| `region` | — | `Ontario` | Checks against the node's configured region boundary; matching region includes the node inventory; an unconfigured region dimension is not filtered |
| `near_lat` | — | `43.6532` | WGS84 latitude; requires `near_lon`, finite and within [-90, 90] |
| `near_lon` | — | `-79.3832` | WGS84 longitude; requires `near_lat`, finite and within [-180, 180] |
| `radius` | — | `5` | Positive finite kilometres; default `EVENTS_DEFAULT_RADIUS_KM` (10), maximum `EVENTS_MAX_RADIUS_KM` (100) |
| `state` | `lifecycle_state` | public lifecycle state | Public endpoints return only their public-state allowlist; `draft` gives no results and `pending_review` is rejected |
| `limit` | — | `200` | Max results per page |
| `after` | — | pagination cursor | For fetching next page |

Event proximity applies to the effective venue of an occurrence. Occurrences
without coordinates on that effective venue are excluded. An occurrence venue
overrides the primary venue; when no occurrence venue is set, the primary venue
is used. A virtual URL does not prevent a match when that effective venue has
coordinates. Venue coordinate coverage is incomplete. If an event has
multiple occurrences, the same occurrence must satisfy both the date and radius
filters. Matching events remain in chronological order by their earliest
matching occurrence and use the existing cursor. Responses include no distance
field and are not sorted by distance. `near_place` and a combined `near=`
parameter are not supported for event searches. Supply both coordinates; a
radius without both coordinates is invalid.

Check the REST response `warnings` after each filtered request; a filter named
there was not applied. Against an older node, verify support in that node's
OpenAPI document before relying on proximity. If the node does not advertise
these filters, do not claim its results are proximity-filtered; use a newer
node or filter locally only when returned venue coordinates are available.

**Event detail:**
```
GET /api/v1/events/{ulid}
```
Returns full JSON-LD with description, offers, location details, organizer,
image, and URL.

### Phase 4: MCP (Alternative)

If your Togather instance exposes an MCP endpoint at `/mcp`, you can use
Hermes' MCP integration. Connect once:

```bash
hermes mcp add togather --url https://your-instance.example.com/mcp
```

The MCP `events` tool returns a subset of fields: `@context`, `@id`,
`@type`, `location`, `name`, `startDate`. It does NOT return `description`,
`url`, `lifecycle_state`, or `source`. Use the REST API for richer queries.

### Phase 5: Present Results

Present findings clearly:

- How many results found, from which source
- For each pick: name, date (abbreviated: Fri, Sat), venue, one-line why
- 3-7 picks max, ordered by relevance
- If filtering for a specific audience (e.g., families), note which events
  are age-appropriate and why

## Filtering Patterns

### By Domain

Use the `domain` parameter for broad category filters. Combine with
`startDate` for future events:

```
GET /api/v1/events?domain=arts&startDate=2026-07-02&limit=200
```

### By Keywords

The `keywords` parameter matches exact tags. For broader search, use `q`:

```
GET /api/v1/events?q=kid+family+workshop&limit=200
```

Note: keyword search is narrower than `q` (free text). If you get zero
results with `keywords`, try `q` with the same terms.

### Kid-Friendly Filtering

When looking for family events, DON'T rely on `q=kid+family` alone — it's
too narrow. Instead:

1. Fetch a broad set: `domain=community&limit=200` or `domain=arts&limit=200`
2. Filter results by checking `name` + `description` against these signals:

**Strong signals** (definitely family-friendly): `kid`, `child`, `children`,
`family`, `youth`, `toddler`, `preschool`, `baby`, `parent`, `stroller`,
`all ages`, `family-friendly`, `storytime`, `teen`, `junior`

**Medium signals** (check timing + context): `workshop`, `craft`, `nature`,
`park`, `garden`, `museum`, `science`, `discovery`, `festival`, `outdoor`,
`camp`, `play`, `game`

**Exclusion signals** (not kid-friendly): `19+`, `21+`, `strip`, `burlesque`,
events starting after 9pm

**Time filter**: Morning = before 1pm, Afternoon = 1pm–5pm, Evening = after
5pm. For young kids, prefer morning/afternoon events.

Use `execute_code` to filter:

```python3
strong = ['kid', 'child', 'children', 'family', 'youth', 'toddler',
          'preschool', 'baby', 'parent', 'all ages', 'storytime', 'teen']
medium = ['workshop', 'craft', 'nature', 'park', 'museum', 'science',
          'festival', 'outdoor', 'camp', 'play', 'game']

for e in events:
    name = (e.get('name') or '').lower()
    desc = (e.get('description') or '').lower()
    text = name + ' ' + desc
    if any(w in text for w in strong):
        # strong match
        print(f"{e['name']} (strong family match)")
    elif any(w in text for w in medium):
        start = e.get('startDate', '')
        hour = int(start[11:13]) if len(start) > 13 else 0
        if hour < 17:  # daytime only for medium signals
            print(f"{e['name']} (possible family match, daytime)")
```

## Pitfalls

| Symptom | Cause | Fix |
|---------|-------|-----|
| Empty results from `keywords` search | Keywords are exact-tag match, not free text | Use `q` parameter instead |
| MCP `events` missing description/URL | MCP intentionally returns sparse fields | Use REST API for rich queries |
| `curl` command gets API key redacted | Hermes secret redaction in tool output | Use `execute_code` or reference env var indirectly |
| Too many results to process | Default limit is small, increase to 200 | Always set `limit=200` for broad queries |
| City name appears not to narrow event addresses | `city` checks the requested locality against the node's configured boundary; a matching locality includes the whole node | Use proximity filters for a coordinate radius, or inspect returned event addresses |
| Nearby results omit known venues | Venues without stored coordinates cannot match | Explain the coordinate coverage limit; check venue data |
| Proximity request returns ordinary unfiltered events | Node may not support the filter, or a request parameter may be unrecognized | Check `warnings` and that node's OpenAPI spec; do not trust the results as proximity-filtered |

### Env Var Access

On some Hermes configurations, `$TOGATHER_API_KEY` may be redacted in
terminal output. If this happens, use `execute_code` instead of `terminal`
for curl commands, or source the env file inline:

```bash
KEY=$(python3 -c "import os; print(os.getenv('TOGATHER_API_KEY', ''))")
curl -s "$TOGATHER_BASE_URL/api/v1/events" -H "Authorization: Bearer $KEY"
```

## Verification

1. Set `TOGATHER_BASE_URL` and `TOGATHER_API_KEY` in `~/.hermes/.env`
2. Run `/reload` in a Hermes session
3. Query today's events:
   ```bash
   curl -s "$TOGATHER_BASE_URL/api/v1/events?startDate=$(date +%Y-%m-%d)&limit=5" \
     -H "Authorization: Bearer $TOGATHER_API_KEY"
   ```
4. Should return a JSON object with an `items` array
