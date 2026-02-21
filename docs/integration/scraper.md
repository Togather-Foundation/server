# Integrated Event Scraper

**Version:** 0.1.0
**Date:** 2026-02-20
**Status:** Implemented

The Togather SEL server includes a built-in two-tier event scraper for automatically
extracting events from Toronto-area arts and culture websites. This document covers
usage, configuration, and how to contribute new source configs.

For external scraper best practices (third-party agents submitting to the API),
see [scrapers.md](scrapers.md).

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [CLI Reference](#cli-reference)
3. [Tiered Extraction](#tiered-extraction)
4. [Source Configuration](#source-configuration)
5. [Database Run Tracking](#database-run-tracking)
6. [Environment Variables](#environment-variables)
7. [Adding New Sources](#adding-new-sources)
8. [Security Design](#security-design)

---

## Quick Start

```bash
# Test whether a URL has extractable JSON-LD events (dry-run)
server scrape url https://example.com/events --dry-run

# Scrape a configured source and ingest into the running server
server scrape source toronto-symphony-orch

# Scrape all enabled sources
server scrape all

# List configured sources
server scrape list
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
| `--limit N` | — | `0` (no limit) | Max events per source |
| `--sources` | — | `configs/sources` | Path to sources directory |

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

### `server scrape all`

Scrape all enabled sources sequentially. Per-source errors are reported in the
output table but do not abort the run. Exits non-zero if any source failed.

```bash
server scrape all
server scrape all --dry-run --limit 10
```

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

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique identifier (used in `server scrape source <name>`) |
| `url` | string | Entry-point URL to scrape |
| `tier` | int | `0` = JSON-LD, `1` = CSS selectors |

### Optional Fields

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `true` | Set `false` to disable without deleting |
| `schedule` | `"manual"` | Hint for future scheduling (`daily`, `weekly`, `manual`) |
| `trust_level` | `5` | SEL source trust score (1–10) |
| `license` | `""` | License applied to ingested events |
| `event_url_pattern` | `""` | Colly URL allow-list pattern |
| `max_pages` | `10` | Tier 1 pagination limit |
| `selectors` | — | Required when `tier: 1` |

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

## Adding New Sources

To contribute a new GTA arts/culture source:

1. Check whether the site has JSON-LD:
   ```bash
   curl -sA "Togather-SEL-Scraper/0.1" https://example.com/events | grep -i 'application/ld+json'
   ```
2. If JSON-LD exists, create a Tier 0 config (minimal, zero selectors needed).
3. If no JSON-LD, inspect the event listing page in browser dev tools and build Tier 1 selectors.
4. Test with `--dry-run` first:
   ```bash
   server scrape source my-new-source --dry-run
   ```
5. Verify event counts and data quality in the output.
6. Submit a PR with the new `configs/sources/<slug>.yaml` file.

### Candidate Sites

See `configs/sources/README.md` for a curated list of high-priority GTA venues to
investigate (confirmed and unconfirmed JSON-LD).

### Currently Configured Sources

| Name | URL | Tier |
|------|-----|------|
| Toronto Symphony Orchestra | https://www.tso.ca/concerts-events | 0 |
| Roy Thomson / Massey Hall | https://www.mfrh.org/events | 0 |
| Hot Docs Cinema | https://hotdocs.ca/whats-on | 0 |
| Glad Day Bookshop | https://gladdaybookshop.com/pages/events | 0 |
| Toronto Public Library | https://www.torontopubliclibrary.ca/events/ | 0 |
| Harbourfront Centre | https://harbourfrontcentre.com/events/ | 0 |

---

## Security Design

- **Body size limits**: HTML responses capped at 10 MiB; ingest API responses at 1 MiB
- **No-redirect HTTP client**: Prevents SSRF via open redirect chains
- **robots.txt compliance**: Tier 0 checks manually; Colly checks natively
- **Signal-aware context**: CLI commands respect `SIGINT`/`SIGTERM` for clean shutdown
- **No credentials in configs**: API key passed via flag or environment variable only
