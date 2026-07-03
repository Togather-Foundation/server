---
name: togather-events-search
description: Search events from a Togather SEL instance — query the public API with domain, keyword, city, and date filters. Use when looking for events in a specific city or when the user asks "what's happening" and has a Togather instance configured.
license: MIT
compatibility: Requires curl and Python 3.9+. Requires a Togather SEL instance with public API access. Set TOGATHER_API_KEY and TOGATHER_BASE_URL environment variables.
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
public REST API with domain, keyword, date, and city filters. Also
covers the MCP endpoint for simpler queries.

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
| City-specific | `GET /api/v1/events?city=Toronto&limit=200` |
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
- **City**: city name filter
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
| `city` | — | `Toronto` | City name |
| `state` | `lifecycle_state` | `published`, `draft`, `postponed` | Default: published |
| `limit` | — | `200` | Max results per page |
| `after` | — | pagination cursor | For fetching next page |

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
| Wrong-city events in results | API may not filter by city without explicit param | Add `city=YourCity` parameter |

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
