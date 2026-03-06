# Integrated Event Scraper

**Version:** 0.5.0
**Date:** 2026-03-06
**Status:** Implemented (Tier 0–3, DB-backed source configs, periodic River scheduling, network capture + intercept)

The Togather SEL server includes a built-in four-tier event scraper for automatically
extracting events from Toronto-area arts and culture websites. This document covers
usage, configuration, and how to contribute new source configs.

For guidance on building your own scraper that submits events to the SEL API,
see [building-scrapers.md](building-scrapers.md).

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [CLI Reference](#cli-reference)
3. [Tiered Extraction](#tiered-extraction)
4. [Source Configuration](#source-configuration)
5. [Periodic Scheduling](#periodic-scheduling)
6. [Scraper Global Config](#scraper-global-config)
7. [Database Run Tracking](#database-run-tracking)
8. [Environment Variables](#environment-variables)
9. [Staging Scrape Workflow](#staging-scrape-workflow)
10. [Adding New Sources](#adding-new-sources)
11. [Security Design](#security-design)

---

## Quick Start

```bash
# Test whether a URL has extractable JSON-LD events (dry-run)
server scrape url https://example.com/events --dry-run

# Scrape a configured source and ingest into the running server
server scrape source harbourfront-centre

# Scrape all enabled sources
server scrape all

# List configured sources
server scrape list

# Inspect a new URL to discover CSS class structure for Tier 1 selectors
server scrape inspect https://example.com/events

# Test selectors against a live URL without writing a config
server scrape test https://example.com/events --event-list ".event-card" --name "h2"

# AI-assisted: generate a source config for a URL (requires OpenCode)
# /generate-selectors https://example.com/events
```

The scraper submits events to the SEL batch ingest API (`POST /api/v1/events:batch`),
so all middleware (auth, rate limiting, deduplication, reconciliation) runs automatically.
An API key is required unless `--dry-run` is used.

---

## CLI Reference

All subcommands are under `server scrape`. Persistent flags apply to all subcommands.

### Persistent Flags

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--server` | `SEL_SERVER_URL` | `http://localhost:8080` | SEL server base URL |
| `--key` | `SEL_API_KEY` or `SEL_INGEST_KEY` | — | API key for ingest |
| `--dry-run` | — | `false` | Display extracted events without submitting |
| `--verbose` | — | `false` | Show individual event details and quality warnings in dry-run output |
| `--limit N` | — | `0` (no limit) | Max events per source |
| `--sources` | — | `configs/sources` | Path to sources directory |

### `server scrape inspect <URL>`

Fetch a URL and print a structured summary of the DOM: top CSS class frequencies,
`data-*` attributes, candidate event container elements with sample HTML, and event
link patterns. Used for Tier 1 selector research.

```bash
server scrape inspect https://example.com/events
```

### `server scrape test <URL>`

Run a live selector test against a URL without writing a config or ingesting events.
Pass selectors as flags or via a `--config` YAML file.

```bash
server scrape test https://example.com/events \
  --event-list ".event-card" \
  --name "h2.title" \
  --start-date "time[datetime]" \
  --url "a.event-link"

# Or load from a config file
server scrape test https://example.com/events --config my-source.yaml
```

### `server scrape capture <URL>`

Render a page via headless browser and save the final HTML. Used for debugging
wait selectors — the captured HTML is exactly what `extractEventsFromHTML` would
receive, so you can inspect the DOM offline.

```bash
# Capture HTML from a bare URL
server scrape capture https://example.com/events

# Capture using full source config (respects headless settings, iframe, etc.)
server scrape capture --source-file configs/sources/my-venue.yaml

# Capture with network activity log (diagnostic mode)
server scrape capture https://example.com/events --network

# Network capture with JSON output (machine-readable for agents)
server scrape capture https://example.com/events --network --format json

# Save a screenshot for visual debugging (useful for agents)
server scrape capture https://example.com/events --screenshot /tmp/page.png
```

**`--network` flag:** Enables CDP network activity capture during rendering.
Shows all HTTP requests/responses the page made (XHR, Fetch, Script, Document,
etc.) with status codes, content types, body sizes, and timing. API-like
requests (XHR/Fetch returning JSON) are flagged with `[API]`. Use this to
diagnose why JS-rendered pages render empty — it reveals what API endpoints the
page calls and whether the data lives in DOM or in API responses.

When `--json` is combined with `--network`, output is a JSON object with `html`
and `network_requests` fields.

**`--screenshot` flag:** Saves a PNG screenshot of the page after rendering and
wait-selector resolution (or timeout). The screenshot captures exactly what the
headless browser sees — use it to diagnose empty containers, loading spinners,
or content that renders in a different tab/section than expected.

```bash
# Save a screenshot after rendering
server scrape capture https://example.com/events --screenshot /tmp/page.png

# Combine with wait selector and network capture
server scrape capture https://example.com/events \
  --wait-selector ".event-list" --wait-timeout 15000 \
  --screenshot /tmp/events.png --network

# Screenshot with source config
server scrape capture --source-file configs/sources/my-venue.yaml \
  --screenshot /tmp/venue.png
```

The screenshot is taken after `--wait-selector` resolves (or times out) and after
network-idle (if configured), but before HTML extraction. If the wait times out,
the screenshot still captures whatever the page looks like at that point — this is
the key diagnostic value for understanding what the headless browser actually sees.

### `server scrape url <URL>`

Fetch a single URL, extract JSON-LD events (Tier 0), normalize, and ingest.

```bash
server scrape url https://tso.ca/concerts
server scrape url https://example.com/events --dry-run --limit 5
```

Output columns: `Source`, `Found`, `New`, `Duplicate`, `Failed`.

Dry-run mode outputs a compact JSON summary with counts only.

### `server scrape list`

List all source configs found in the sources directory.

```bash
server scrape list
server scrape list --sources /custom/path
```

Output columns: `NAME`, `URL`, `TIER`, `ENABLED`, `SCHEDULE`.

### `server scrape source <name>`

Load and scrape a named source from the sources directory.

```bash
server scrape source toronto-symphony-orch
server scrape source glad-day-bookshop --dry-run
```

Source name matching is case-insensitive. Returns an error if the source is not
found or is disabled.

**`--source-file <path>`** — load a YAML config file directly, bypassing the DB
and sources directory lookup. The source runs regardless of its `enabled` flag,
making it easy to test draft or disabled configs without a DB sync. The `<name>`
argument is optional when `--source-file` is set (the name is read from the file).

```bash
# Test a draft config without adding it to the sources directory
server scrape source --source-file /tmp/my-draft.yaml --dry-run

# Test a disabled source with headless flags
SCRAPER_HEADLESS_ENABLED=true server scrape source \
  --source-file configs/sources/burdock-brewery.yaml --dry-run
```

### `server scrape all`

Scrape all enabled sources sequentially. Per-source errors are reported in the
output table but do not abort the run. Exits non-zero if any source failed.

```bash
server scrape all
server scrape all --dry-run --limit 10
server scrape all --tier 0          # JSON-LD sources only
server scrape all --tier 1          # CSS-selector sources only
```

**`--tier` flag** (applies to `scrape all` only):

| Value | Meaning |
|-------|---------|
| `-1` | All tiers (default) |
| `0` | Tier 0 — JSON-LD sources only |
| `1` | Tier 1 — CSS-selector sources only |
| `2` | Tier 2 — headless browser sources only |
| `3` | Tier 3 — API (GraphQL or REST JSON) sources only |

Output: per-source table with totals row.

---

## Tiered Extraction

The scraper uses a tiered strategy, starting with the easiest and most reliable
extraction method.

### Tier 0 — JSON-LD (zero per-site config)

1. Fetch the page via `net/http` (10 MiB body limit, no-redirect client)
2. Check `robots.txt` compliance
3. Parse HTML with `goquery`
4. Find all `<script type="application/ld+json">` blocks
5. Filter for `@type: "Event"` including nested structures:
   - Top-level `Event` objects
   - `@graph` arrays containing Events
   - `ItemList` with `Event` list items
   - `EventSeries` parent containers
6. Normalize each event to `EventInput` and submit via batch API

Most modern CMS platforms (WordPress, Drupal, Squarespace) inject schema.org JSON-LD
via SEO plugins. Tier 0 handles these automatically without per-site configuration.

### Tier 1 — Colly CSS Selectors (requires per-site config)

Used when a site lacks JSON-LD or has unreliable JSON-LD quality.

1. Create a Colly collector with rate limiting (1 req/s default) and robots.txt compliance
2. Visit the source URL
3. For each element matching `selectors.event_list`:
   - Extract fields with configured CSS selectors
   - Follow detail page links if `selectors.url` is configured
4. Follow pagination if `selectors.pagination` is configured
5. Normalize extracted text to `EventInput` and submit

**User-Agent:** `Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)`

### Tier 2 — Headless Browser (requires per-site config)

Used when JavaScript rendering is required and CSS selectors are insufficient or
when the site requires browser-level interaction (e.g. lazy loading, shadow DOM).

1. Launch a Rod-controlled Chromium instance
2. Navigate to the source URL and wait for a configurable selector or fixed delay
3. Extract the rendered DOM with configured CSS selectors (same `selectors` block as Tier 1)
4. Normalize and submit

Headless config block (`headless:`) controls timeouts, wait selectors, and navigation
options. See [Full Config with Tier 2 Headless](#full-config-with-tier-2-headless) below.

### Tier 3 — API: GraphQL or REST JSON (requires per-site config)

Used when a site exposes a structured API. Two variants are supported — exactly one
must be configured per source:

#### GraphQL variant

Used when a site exposes a GraphQL API (e.g. DatoCMS-powered venues).

1. POST a configured GraphQL query to the endpoint (with optional Bearer token)
2. Decode the response envelope: `{"data": {"<event_field>": [...]}}`
3. Map each record to a `RawEvent` using known field names
4. If `url_template` is set, render the Go `text/template` with the raw record to
   produce each event's canonical URL
5. Normalize and submit

Response body is capped at 10 MiB. `User-Agent` header is set to the standard
scraper agent string. Timeout behaviour: `graphql.timeout_ms` applies only when it
exceeds the global request timeout; the larger of the two wins.

#### REST JSON variant

Used when a site exposes a paginated JSON REST API (e.g. Showpass).

1. GET the configured endpoint
2. Decode the `results_field` array (default: `"results"`) from the JSON response.
   When `results_field` is `"."`, the entire response body is treated as a bare JSON
   array (`[{...}, {...}]`) — used for APIs that return arrays without an envelope object
   (e.g. Showclix S3 buckets). Bare array mode has no pagination support.
3. Map each item to a `RawEvent` via `field_map` (or identity mapping if none)
4. If `url_template` is set, render the Go `text/template` with the raw item map to
   produce each event's canonical URL
5. Follow `next_field` (default: `"next"`) for pagination until null or `max_pages` reached
6. Normalize and submit

Response body is capped at 10 MiB per page. Timeout behaviour: `rest.timeout_ms`
applies only when it exceeds the global request timeout; the larger of the two wins.

---

## Source Configuration

Source configs live in `configs/sources/*.yaml`. The `_example.yaml` file is a
fully-documented template. Sources starting with `_` are ignored by the loader.

### Minimal Tier 0 Config

```yaml
name: "My Arts Venue"
url: "https://example.com/events"
tier: 0
enabled: true
```

### Multiple Entry-Point URLs (any tier)

Use `urls` instead of `url` when a source's events are spread across multiple pages:

```yaml
name: "Multi-page Venue"
urls:
  - "https://example.com/events/music"
  - "https://example.com/events/theatre"
tier: 0
enabled: true
```

`url` and `urls` are mutually exclusive. All listed URLs are scraped in sequence
during a single run.

### Full Config with Tier 1 Selectors

```yaml
name: "My Arts Venue"
url: "https://example.com/events"
tier: 1
schedule: "daily"         # daily | weekly | manual
trust_level: 5            # 1–10, maps to SEL source trust
license: "CC-BY-4.0"
enabled: true
event_url_pattern: "/events/*"   # Colly URL filter
max_pages: 10                    # Pagination safety limit

selectors:
  event_list: "div.event-card"
  name: "h2.event-title"
  start_date: "time[datetime]"   # Prefers datetime attribute
  end_date: "time.end-time[datetime]"
  location: "span.venue-name"
  description: "p.event-description"
  url: "a.event-link"
  image: "img.event-image"
  pagination: "a.next-page"
```

#### Composite Date Selectors (`date_selectors`)

When a site doesn't use `<time>` elements (common with CSS Modules frameworks
like Ticket Spot), use `date_selectors` to extract date and time text from
multiple DOM elements. The smart date assembler combines the fragments into
RFC 3339 start/end datetimes.

```yaml
selectors:
  event_list: "[class^='list-'] > div"
  name: "[class^='title-']"
  url: "a[href*='eventbrite']"
  # Extract date and time from separate elements:
  date_selectors:
    - ".first [class^='time-container-']"                    # e.g. "Thu 5th March"
    - "[style*='display: flex'] [class^='time-container-']"  # e.g. "9:30 PM"
```

**How it works:**

`date_selectors` uses a **grab-bag** model: you can list any number of date and
time selectors, and the smart date assembler figures out the correct data from
whatever matches. This is especially useful when a site has multiple possible
locations for date/time information.

1. Each selector in `date_selectors` extracts text from within the event card
   (selectors that don't match are recorded as empty — index `i` always
   corresponds to `date_selectors[i]`)
2. Text fragments are classified as date-only, time-only, or combined
3. The assembler strips ordinal suffixes (`1st`, `2nd`, `3rd`, `th`), removes
   day-of-week prefixes, recognises month names, and handles 12h/24h time formats
4. First date + first time = `startDate`; second time (if present) = `endDate`
5. Missing year is inferred (current year, or next year if >30 days in the past)
6. Timezone comes from `DEFAULT_TIMEZONE` env var (default: `America/Toronto`)
7. When selectors fail, quality warnings include first-container diagnostics
   showing exactly what each selector found (see [Quality Warnings](#quality-warnings))

When `date_selectors` is set, it takes priority over `start_date`/`end_date`.
If `date_selectors` produces no result, the scraper falls back to `start_date`/`end_date`.

Existing configs using `start_date` with ISO 8601 dates (e.g. `"2026-05-10T19:00:00"`)
or `<time datetime="...">` attributes continue to work unchanged — the fuzzy parser
detects partial ISO 8601 and passes through without modification.

### Quality Warnings

The scraper performs automatic quality checks during extraction and reports warnings
in the dry-run output. Use `--verbose` to see individual event details alongside
quality warnings.

```bash
# Verbose dry-run — the primary validation workflow
SCRAPER_HEADLESS_ENABLED=true ./server scrape source lula-lounge \
  --source-file configs/sources/lula-lounge.yaml --dry-run --verbose
```

#### Warning Types

| Warning Code | Trigger | Meaning |
|-------------|---------|---------|
| `date_selector_never_matched` | A `date_selectors` entry matched 0 events | The CSS selector is wrong — inspect the DOM and fix it |
| `date_selector_partial_match` | A `date_selectors` entry matched some but not all events | The selector may need to be more general, or some events genuinely lack that element |
| `all_midnight` | All extracted events have `T00:00:00` start times | Time extraction failed — the time selector is broken or missing |

When a `date_selector` fails, the warning includes **first-container diagnostic
detail** showing what the selector found (or didn't) in the first event container:

```
date_selector_never_matched: selector #2 (".time") matched 0/57 events — first container: no element found for this selector
date_selector_never_matched: selector #3 ("[data-time]") matched 0/57 events — first container: element found but text was empty
date_selector_partial_match: selector #1 (".date") matched 42/57 events — first container: "Thu 5th March"
```

This tells you whether the selector failed to match any DOM element, matched an
element with empty text content, or what text it extracted. This is especially
useful for debugging grab-bag `date_selectors` configs where multiple selectors
target different possible date/time elements.

Warnings are logged to stderr and also attached to the `ScrapeResult` as
`QualityWarnings []string` for programmatic consumption. They do **not** prevent
event ingestion — they are advisory signals for selector debugging.

### Full Config with Tier 2 Headless

```yaml
name: "JS-Rendered Venue"
url: "https://example.com/events"
tier: 2
enabled: true

headless:
  wait_selector: "div.event-card"   # Wait for this element before extracting
  wait_timeout_ms: 15000            # Max ms to wait for wait_selector (default: 10000)
  wait_network_idle: true           # Also wait for XHR/fetch requests to settle
  undetected: false                 # Enable stealth evasions for bot-detection bypass
  # iframe:                         # optional — extract content from a cross-origin iframe
  #   selector: "iframe[title='Ticket Spot']"  # CSS selector for the target iframe element
  #   wait_selector: ".events-container"         # wait for content inside iframe
  #   wait_timeout_ms: 10000                      # timeout for iframe content (default: 10000)
  # intercept:                        # optional — capture API responses instead of DOM scraping
  #   url_pattern: "api\\.example\\.com/events"  # Go regex matching intercepted URLs
  #   results_path: "data.events"                # dot-notation path to events array in JSON
  #   field_map:                                 # map RawEvent fields to source JSON keys
  #     name: "title"
  #     start_date: "dates.start"
  #     location: "venue.name"

selectors:
  event_list: "div.event-card"
  name: "h2.event-title"
  start_date: "time[datetime]"
  url: "a.event-link"
```

When `iframe:` is set, the scraper uses Chrome DevTools Protocol (CDP) frame navigation
to enter the iframe's execution context and extracts HTML from the iframe. CSS selectors
in `selectors:` then apply to the iframe DOM, not the parent page. This enables
extraction from cross-origin iframes such as Ticket Spot (Wix embed) and Elevent.

### Full Config with Tier 3 GraphQL

```yaml
name: "DatoCMS Venue"
url: "https://example.com/events"   # Canonical source URL (informational)
tier: 3
enabled: true

graphql:
  endpoint: "https://graphql.datocms.com/"
  token: "abc123publictoken"          # Read-only public token — safe to commit
  event_field: "allEvents"            # Top-level key in the GraphQL data response
  timeout_ms: 30000                   # Optional; uses global timeout if not set or smaller
  url_template: "https://example.com/events/{{.slug}}"  # Go text/template; fields from event record
  query: |
    query AllEvents {
      allEvents(orderBy: startDate_ASC, first: 100) {
        title
        slug
        startDate
        endDate
        location
        description
        image { url }
      }
    }
```

`url_template` is a Go `text/template` string rendered with the raw GraphQL record as
data (field names are the query response keys). The template runs with
`missingkey=error`, so a missing key causes a template execution error; the error is
logged at debug level and the URL is left empty.

### Full Config with Tier 3 REST

```yaml
name: "showpass-venue"
url: "https://example.showpass.com"  # Canonical source URL (informational)
tier: 3
enabled: true

rest:
  endpoint: "https://www.showpass.com/api/public/events/?venue=12345"
  results_field: "results"           # JSON key containing the events array (default: "results")
  next_field: "next"                 # JSON key for the next-page URL (default: "next")
  url_template: "https://www.showpass.com/{{.slug}}"  # Go text/template; fields from raw item
  timeout_ms: 30000                  # Optional; uses global timeout if not set or smaller
  field_map:
    name: "name"                     # RawEvent field: source JSON key (dot-notation for nested: "title.text")
    start_date: "starts_on"
    end_date: "ends_on"
    image: "logo.url"                # Dot-notation traverses nested JSON: {"logo": {"url": "..."}}
    # url: omitted — populated by url_template above
```

**Bare array variant** (for APIs that return `[{...}, {...}]` without an envelope):

```yaml
- name: "Venue Name (Showclix)"
  url: "https://venuename.com/events"
  tier: 3
  rest:
    endpoint: "https://venuenameeventsbucket.s3.amazonaws.com/events.json"
    results_field: "."                 # Bare array: entire response is the results array
    field_map:
      name: "name"
      start_date: "starts_on"
      end_date: "ends_on"
      image: "image"
    url_template: "https://www.showclix.com/event/{{.slug}}"
```

`field_map` maps RawEvent field names (`name`, `start_date`, `end_date`, `url`, `image`,
`location`, `description`) to source JSON keys. Values support dot-separated paths for
nested JSON traversal (e.g. `"logo.url"` extracts `response.logo.url`). Dots are always
treated as path separators — there is no escape mechanism. Omit `field_map` entirely for
identity mapping (source keys must match RawEvent Go field names: `Name`, `StartDate`, etc.).

`url_template` is a Go `text/template` rendered with the raw item map as data. A
missing key causes a template execution error (`missingkey=error`); the error is logged
at debug level and the URL is left empty.

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique identifier (used in `server scrape source <name>`) |
| `url` or `urls` | string or []string | Entry-point URL(s) to scrape (mutually exclusive) |
| `tier` | int | `0` = JSON-LD, `1` = CSS selectors, `2` = headless browser, `3` = API (GraphQL or REST) |

### Optional Fields

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `true` | Set `false` to disable without deleting |
| `schedule` | `"manual"` | Hint for future scheduling (`daily`, `weekly`, `manual`) |
| `trust_level` | `5` | SEL source trust score (1–10) |
| `license` | `""` | License applied to ingested events |
| `event_url_pattern` | `""` | Colly URL allow-list pattern |
| `max_pages` | `10` | Tier 1 pagination limit |
| `skip_multi_session_check` | `false` | Skip multi-session detection for this source. Use for sources that legitimately publish long-duration events (e.g. exhibitions, residencies, summer institutes). |
| `timezone` | `""` | IANA timezone for date parsing (e.g. `"America/Toronto"`). Overrides `DEFAULT_TIMEZONE` env var. Falls back to `America/Toronto` if neither is set. |
| `selectors` | — | Required when `tier: 1` or `tier: 2` |
| `headless` | — | Required fields for `tier: 2` (`wait_selector` or `selectors.event_list`) |
| `graphql` | — | Required for `tier: 3` GraphQL variant (mutually exclusive with `rest`) |
| `rest` | — | Required for `tier: 3` REST variant (mutually exclusive with `graphql`) |

### Headless Config Fields (`headless:`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `wait_selector` | string | `body` | CSS selector to wait for before extracting. Use the most specific stable element. |
| `wait_timeout_ms` | int | `10000` | Max ms to wait for `wait_selector`. Increase for slow SPAs. |
| `wait_network_idle` | bool | `false` | After `wait_selector` resolves, additionally wait for in-flight XHR/fetch requests to settle (500 ms idle window). Use for pages that load content via async requests after the DOM is ready (e.g. third-party event widget embeds). |
| `undetected` | bool | `false` | Launch page with stealth evasions (patches `navigator.webdriver`, fake plugins, etc.) to reduce bot-detection by sites that check for headless Chrome fingerprints. |
| `pagination_button` | string | — | CSS selector for a JS "next page" / "load more" button. For URL-based pagination use `selectors.pagination` instead. |
| `rate_limit_ms` | int | `1000` | Delay between page loads in ms. |
| `headers` | map[string]string | — | Extra HTTP headers to inject (e.g. `Accept-Language`). |
| `iframe.selector` | string | — | CSS selector for the target cross-origin iframe element. When set, the scraper enters the iframe's execution context via CDP frame navigation and extracts HTML from inside the iframe. |
| `iframe.wait_selector` | string | — | CSS selector to wait for inside the iframe DOM before extracting. |
| `iframe.wait_timeout_ms` | int | `10000` | Timeout (ms) for `iframe.wait_selector`. |

### Intercept Config Fields (`headless.intercept:`)

Network request interception for Tier 2 headless sources. When configured, the
scraper intercepts matching API requests during page rendering, parses the JSON
response bodies, and maps them to events using `field_map`. Intercepted events
are merged with DOM-extracted events. This is a fallback for sites where the DOM
never populates with event data but the underlying API response contains it.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url_pattern` | string | yes | — | Go regex pattern to match intercepted request URLs. Rod uses a `"*"` glob to intercept all requests; the regex filters inside the callback. |
| `response_format` | string | no | `"json"` | Response body format. Only `"json"` is currently supported. |
| `results_path` | string | yes | — | Dot-notation path to the events array in the JSON response (e.g. `"hits.hit"`, `"data.events"`). |
| `cache_endpoint` | bool | no | `false` | Log captured endpoint URLs at info level (for diagnostic/caching purposes). |
| `field_map` | map | no | — | Map from RawEvent field names to source JSON keys; identical to `rest.field_map` (supports dot-notation). |

**`field_map` keys** (same as REST): `name`, `start_date`, `end_date`, `url`, `image`, `location`, `description`.

### GraphQL Config Fields (`graphql:`)

| Field | Required | Description |
|-------|----------|-------------|
| `endpoint` | yes | GraphQL API URL |
| `query` | yes | Full GraphQL query string |
| `event_field` | yes | Key in `data` response containing the events array |
| `token` | no | Bearer token for Authorization header |
| `url_template` | no | Go `text/template` string to construct each event's URL |
| `timeout_ms` | no | Request timeout; the larger of this and the global timeout applies |

### REST Config Fields (`rest:`)

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `endpoint` | yes | — | REST API URL (initial page URL) |
| `results_field` | no | `"results"` | JSON key containing the events array on each page. Use `"."` for bare JSON array responses (no envelope object; no pagination) |
| `next_field` | no | `"next"` | JSON key containing the next-page URL (string or null) |
| `url_template` | no | — | Go `text/template` string to construct each event's URL |
| `timeout_ms` | no | — | Request timeout; the larger of this and the global timeout applies |
| `headers` | no | — | Extra HTTP headers to inject (map[string]string) |
| `field_map` | no | — | Map from RawEvent field names to source JSON keys; values support dot-notation for nested fields (see below) |

**`field_map` keys** (all optional; omit for identity mapping):
`name`, `start_date`, `end_date`, `url`, `image`, `location`, `description`.
Values are source JSON keys; use dot-separated paths to traverse nested objects (e.g. `"logo.url"`, `"title.text"`).

> **Redirect behaviour:** The REST HTTP client allows up to 10 redirects. This is intentionally explicit — it matches the Go default but is configurable for auditability. JSON-LD (Tier 0) blocks all redirects for SSRF hardening.

---

## Periodic Scheduling

Sources with `schedule: "daily"` or `schedule: "weekly"` are automatically scraped
by a River background worker (`ScrapeSourceWorker`) registered at server startup.

| `schedule` value | Behaviour |
|-----------------|-----------|
| `daily` | Runs once per day (midnight UTC) |
| `weekly` | Runs once per week (Sunday midnight UTC) |
| `manual` | Never run automatically; CLI-only |

Periodic jobs are registered via `NewPeriodicJobsFromSources(sources)` during
`server serve` startup. Only sources where `enabled: true` are registered.
Job runs are recorded in `scraper_runs` (same as manual scrapes).

Automatic scheduling is gated by the `auto_scrape` flag in the global scraper
config (see [Scraper Global Config](#scraper-global-config)). When `auto_scrape`
is `false`, no periodic jobs fire even if sources have a `schedule` set.

---

## Scraper Global Config

A single `scraper_config` row (migration `000034`) stores operator-level settings
that apply to all scrape runs.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto_scrape` | bool | `false` | Enable/disable periodic River job scheduling |
| `max_concurrent_sources` | int | `3` | Max sources scraped in parallel during `scrape all` |
| `request_timeout_seconds` | int | `30` | HTTP request timeout per page fetch |
| `retry_max_attempts` | int | `3` | Retries on transient network errors |
| `max_batch_size` | int | `100` | Max events per ingest batch POST |
| `rate_limit_ms` | int | `1000` | Minimum ms between requests to the same domain |

### Admin API

```
GET  /api/admin/scraper/config   — Read current config
PATCH /api/admin/scraper/config  — Update one or more fields (partial JSON body)
```

Both endpoints require an admin API key (`Authorization: Bearer <key>`).

### Admin UI Toggle

The `/admin/scraper` page includes an **Auto-scrape** toggle that sets `auto_scrape`
via `PATCH /api/admin/scraper/config`. Enabling it activates periodic job scheduling
for all sources with a `daily` or `weekly` schedule.

---

## Database Run Tracking

Each scrape execution is recorded in the `scraper_runs` table when `DATABASE_URL`
is set. If the database is unavailable, tracking is skipped silently and the
scrape proceeds.

### Schema

```sql
CREATE TABLE scraper_runs (
  id            BIGSERIAL PRIMARY KEY,
  source_name   TEXT NOT NULL,
  source_url    TEXT NOT NULL,
  tier          INT NOT NULL DEFAULT 0,
  started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at  TIMESTAMPTZ,
  status        TEXT NOT NULL DEFAULT 'running'
                  CHECK (status IN ('running', 'completed', 'failed')),
  events_found  INT DEFAULT 0,
  events_new    INT DEFAULT 0,
  events_dup    INT DEFAULT 0,
  events_failed INT DEFAULT 0,
  error_message TEXT,
  metadata      JSONB
);
```

### Lifecycle

| Status | When set |
|--------|----------|
| `running` | Row inserted when scrape starts |
| `completed` | Updated with event counts when scrape finishes |
| `failed` | Updated with `error_message` when scrape errors |

Query recent runs:

```sql
SELECT source_name, status, events_new, events_dup, started_at, completed_at
FROM scraper_runs
ORDER BY started_at DESC
LIMIT 20;
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `SEL_SERVER_URL` | Base URL of the SEL server (default: `http://localhost:8080`) |
| `SEL_API_KEY` | API key for ingest submissions |
| `SEL_INGEST_KEY` | Alternative API key env var (checked after `SEL_API_KEY`) |
| `DATABASE_URL` | PostgreSQL connection string for scraper run tracking |

---

## Staging Scrape Workflow

Use these targets to populate the staging server with real event data. All
targets read connection details from `.deploy.conf.staging` — no manual
copy-pasting of URLs or keys is needed.

### Configuration: `.deploy.conf.staging`

`.deploy.conf.staging` (gitignored) stores per-environment deployment metadata:

```ini
NODE_DOMAIN=staging.toronto.togather.foundation
SSH_HOST=togather
SSH_USER=deploy
PERF_AGENT_API_KEY=<ulid-key-for-ingest>
PERF_ADMIN_API_KEY=<ulid-key-for-admin>
```

**API key roles:**

| Variable | Role |
|----------|------|
| `PERF_AGENT_API_KEY` | Agent / ingest operations — used for `scrape all` |
| `PERF_ADMIN_API_KEY` | Admin operations — used for managing users, keys, sources |

The scrape targets use `PERF_AGENT_API_KEY`. Use `PERF_ADMIN_API_KEY` only
when calling admin API endpoints directly.

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make scrape-staging` | Scrape all enabled sources (all tiers) to staging |
| `make scrape-staging-t0` | Scrape only Tier 0 (JSON-LD) sources to staging |
| `make staging-reset-scrape` | Wipe staging event data then scrape T0 sources |

```bash
# Typical usage: reset staging and populate with T0 sources
make staging-reset-scrape

# Add more events from T1 sources after T0 is complete
make scrape-staging

# Scrape only T0 sources without resetting first
make scrape-staging-t0
```

### Manual Commands

Equivalent manual commands (useful when you need extra flags like `--dry-run`
or `--limit`):

```bash
# Source the config
source .deploy.conf.staging

# Dry-run all T0 sources against staging
go run ./cmd/server scrape all \
  --tier 0 \
  --server "https://$NODE_DOMAIN" \
  --key "$PERF_AGENT_API_KEY" \
  --dry-run

# Scrape a single source against staging
go run ./cmd/server scrape source harbourfront-centre \
  --server "https://$NODE_DOMAIN" \
  --key "$PERF_AGENT_API_KEY"

# Scrape all sources with a per-source event limit
go run ./cmd/server scrape all \
  --server "https://$NODE_DOMAIN" \
  --key "$PERF_AGENT_API_KEY" \
  --limit 20
```

### Verifying Results

After a scrape run, verify results via the staging API:

```bash
source .deploy.conf.staging

# Check total event count
curl -s "https://$NODE_DOMAIN/api/v1/events?limit=1" \
  | jq '.total_count // (.items | length)'

# Check the review queue (events needing human review)
curl -s "https://$NODE_DOMAIN/api/v1/events?lifecycle_state=review&limit=10" \
  -H "Authorization: Bearer $PERF_ADMIN_API_KEY" \
  | jq '[.items[] | {name: .name, source: .source_name}]'

# List recent scraper runs (requires DB access)
source .env
psql "$DATABASE_URL" -c "
  SELECT source_name, status, events_new, events_dup, started_at
  FROM scraper_runs
  ORDER BY started_at DESC
  LIMIT 20;
"
```

---

## Adding New Sources

### Quickest path: AI-assisted generation (recommended)

Use the `/generate-selectors` OpenCode slash command (see `agents/commands/generate-selectors.md`):

```bash
# Single URL
/generate-selectors https://example.com/events

# File of URLs (one per line)
/generate-selectors urls.txt
```

The command inspects the URL, proposes CSS selectors, validates live, checks the
org database for a match, and writes `configs/sources/<name>.yaml`. It runs up to
5 URLs in parallel via subagents.

### Manual path

1. Check whether the site has JSON-LD:
   ```bash
   server scrape inspect https://example.com/events
   # Look for "Event hrefs" and top CSS classes in the output
   ```
2. If JSON-LD exists, create a minimal Tier 0 config.
3. If the site exposes a GraphQL API (e.g. DatoCMS), create a Tier 3 config with a `graphql:` block.
4. If no JSON-LD and no GraphQL, use `server scrape test` to iterate on selectors before writing the config. Use `tier: 2` if the site requires JavaScript rendering.
5. Test with `--dry-run`:
   ```bash
   server scrape source my-new-source --dry-run
   ```
6. Submit a PR with the new `configs/sources/<slug>.yaml` file.

### Currently Configured Sources (Tier 1)

| Name | URL | Events/page | Notes |
|------|-----|-------------|-------|
| harbourfront-centre | harbourfrontcentre.com/whats-on/ | 106 | |
| toronto-reference-library | tpl.bibliocommons.com/v2/events | 21 | Screen-reader span duplication |
| gardiner-museum | gardinermuseum.on.ca/whats-on/ | 7 | |
| soulpepper | soulpepper.ca/performances | 9 | |
| moca | moca.ca/events | 20 | Elementor template ID hook — fragile |
| factory-theatre | factorytheatre.ca/whats-on/ | 5 | |
| tarragon-theatre | tarragontheatre.com/whats-on/ | 13 | No dates on listing page |
| coc | coc.ca/tickets/2526-season | 7 | Season URL — annual update needed |
| national-ballet | national.ballet.ca/performances/202627-season/ | 9 | Season URL — annual update needed |
| rom | rom.on.ca/whats-on/events | 120 | 3 pages; Drupal span duplication |

### Currently Configured Sources (Tier 3)

| Name | Variant | Endpoint | Notes |
|------|---------|----------|-------|
| tranzac | GraphQL | graphql.datocms.com | DatoCMS public token; url_template constructs `/events/<slug>` |
| burdock-brewery | REST | showpass.com/api/public/events/?venue=17330 | Showpass venue API; paginated; 34 events across 2 pages |

See `configs/sources/README.md` for full status including disabled sources and unverified candidates.  
See `docs/integration/disabled-sources.md` for a detailed breakdown of every disabled source and its recommended fix path.

---

## Observability

### Prometheus Metrics

The scraper emits three metrics, all in the `togather_scraper_*` namespace:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `togather_scraper_runs_total` | Counter | `source`, `tier`, `result`, `slot` | Completed scrape runs (result: `success` \| `error` \| `dry_run`) |
| `togather_scraper_run_duration_seconds` | Histogram | `source`, `tier`, `slot` | Wall-clock time per scrape run |
| `togather_scraper_events_total` | Counter | `source`, `tier`, `outcome`, `slot` | Events processed (outcome: `found` \| `submitted` \| `created` \| `duplicate` \| `failed`) |

The `source` label is set to the source config `name` for named sources, and to
`parsedURL.Hostname()` for ad-hoc `ScrapeURL` calls (i.e. `server scrape url <URL>`).
The `tier` label reflects the extraction tier used: `"0"` (JSON-LD), `"1"` (Colly CSS),
`"2"` (headless browser), or `"3"` (GraphQL API).

**Cardinality note:** The hostname-derived label is safe today because `ScrapeURL`
is only called from the CLI with a bounded set of operator-supplied URLs. Do NOT
call `ScrapeURL` from a user-facing HTTP endpoint — the label would become unbounded
and cause Prometheus memory growth. If that call-site is ever added, normalise the
label (e.g. strip subdomains, cap length) or use a fixed `"ad_hoc"` value.

For full metric documentation and dashboard guidance, see
[docs/deploy/monitoring.md](../deploy/monitoring.md).

---

## Public URL Submission

External contributors can suggest event source URLs without an API key via the
public URL submission endpoint. Submitted URLs are stored as `pending_validation`
and asynchronously checked (HEAD request + robots.txt) by a River background worker
before an admin reviews them.

### Endpoint

```
POST /api/v1/scraper/submissions
```

No authentication required. Rate-limited to **5 URLs per IP per 24 hours**.
Maximum 10 URLs per request. URLs submitted within 30 days are deduplicated.

### Request

```http
POST /api/v1/scraper/submissions
Content-Type: application/json

{
  "urls": [
    "https://example.com/events",
    "https://another-venue.ca/calendar"
  ]
}
```

### Response

```json
{
  "results": [
    {
      "url": "https://example.com/events",
      "status": "accepted",
      "message": "URL queued for review"
    },
    {
      "url": "https://another-venue.ca/calendar",
      "status": "duplicate",
      "message": "Already submitted within 30 days"
    }
  ]
}
```

Per-URL `status` values:

| Status | Meaning |
|--------|---------|
| `accepted` | URL queued for async validation |
| `duplicate` | Same normalized URL already submitted within 30 days |
| `rejected` | Invalid URL (bad scheme, no host, etc.) |
| `rate_limited` | IP quota exhausted; remaining URLs in batch not accepted |

### Admin Endpoints

Admins can list and review submissions with a JWT:

```
GET  /api/v1/admin/scraper/submissions?status=pending_validation&limit=50&offset=0
PATCH /api/v1/admin/scraper/submissions/{id}
```

**List query params:** `status` (optional filter), `limit` (1–200, default 50),
`offset` (default 0).

**PATCH body:**
```json
{
  "status": "processed",
  "notes": "Good source, added to configs/sources/example.yaml"
}
```

Valid `status` values for admin PATCH: `processed` | `rejected`.

---

## Security Design

- **Body size limits**: HTML responses capped at 10 MiB; ingest API responses at 1 MiB
- **No-redirect HTTP client**: Prevents SSRF via open redirect chains
- **robots.txt compliance**: Tier 0 checks manually; Colly checks natively
- **Signal-aware context**: CLI commands respect `SIGINT`/`SIGTERM` for clean shutdown
- **No credentials in configs**: API key passed via flag or environment variable only
