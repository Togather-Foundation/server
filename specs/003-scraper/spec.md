# Feature Specification: Integrated Event Scraper

**Feature Branch**: `003-scraper`
**Created**: 2026-02-20
**Status**: Implemented (Tier 0–3, DB-backed configs, scheduling, metrics)
**Input**: Need to populate the SEL with real event data from Toronto arts/culture websites for dogfooding and agent curation testing via SEL MCP.

## Context

The SEL server has a mature ingestion pipeline (batch API, deduplication, reconciliation, provenance tracking) but currently relies on manual JSON file imports or external agents to submit events. To dogfood the system on staging and enable MCP-based curation agents to work with real data, we need an integrated scraper that can automatically extract events from Toronto-area arts/culture websites.

### Design Principles

- **Tiered extraction**: Start with the easiest, most reliable method (existing JSON-LD) and fall back to more complex approaches (CSS selectors, eventually headless browsers)
- **Community-friendly**: Source configurations are YAML files that can be contributed via PRs
- **Respectful**: Obey robots.txt, rate limit, identify ourselves transparently
- **SEL-native**: Uses the existing batch ingest API, so dedup/reconciliation/provenance all work automatically
- **Observable**: Track scrape runs with metrics and logging

### Non-Goals (current)

- Email/newsletter parsing (Tier 3 — future)
- Email/newsletter parsing (Tier 4 — future)
- API-first ingestion outside `server scrape` workflows (e.g., bespoke REST harvesters)
- Prometheus dashboards beyond scraper metrics (tracked separately)

### Design Additions (post-v0.1)

- **Agent-led selector generation**: `/configure-source` orchestrates parallel
  subagents that run `server scrape inspect`, propose Tier 1 selectors, validate via
  `server scrape test`, and commit vetted YAML configs. This workflow is defined in
  `agents/commands/configure-source.md` and is now the default Tier 1 authoring path.
- **DB-backed source configs**: `scraper_sources` table stores configs alongside
  YAML files as the canonical seed format. `server scrape sync` imports YAML→DB;
  `server scrape export` dumps DB→YAML. Scraper runtime reads from DB with YAML
  fallback. Org/place linkage via join tables exposes sources in JSON-LD API responses.
  Beads: srv-65kvw, srv-iorfa, srv-2nu7e, srv-l71q1, srv-17zth.
- **Scraper global config + scheduling**: `scraper_config` table holds operator
  tunables (`auto_scrape`, concurrency, timeouts, retry caps, batch sizes). `GET/PATCH
  /api/admin/scraper/config` powers the admin UI toggle, and `ScrapeSourceWorker`
  + `NewPeriodicJobsFromSources` enqueue River jobs automatically when schedules are
  set to `daily`/`weekly`. Delivered in srv-pfeud and deployed to staging.
- **Tier 2 headless + Tier 3 GraphQL**: JS-rendered sources use Rod-based headless
  scraping (`tier: 2` with a `headless` config block). GraphQL-backed sources use
  API extraction (`tier: 3` with a `graphql` config block) with optional URL
  templating for canonical event URLs. `server scrape capture` renders JS pages for
  selector discovery.

- **Public URL submission endpoint**: Rate-limited `POST /api/v1/scraper/submissions`
  allows community members to suggest URLs for scraping without admin access. URLs are
  validated asynchronously (HEAD + robots.txt) by a self-throttling River worker, then
  held in an admin-reviewable queue. Admins download pending URLs and trigger scraping
  manually via CLI. Per-IP rate limit (5/24h), 30-day dedup window, max 10 URLs per
  request. Full spec in `specs/003-scraper/url-submissions-spec.md`.
  Epic: srv-1cxmi.

## User Scenarios & Testing

### User Story 1 — Operator Scrapes a URL for JSON-LD Events (Priority: P1)

An operator wants to quickly test whether a website has extractable event data by pointing the scraper at a URL. This is the primary discovery and testing workflow.

**Independent Test**: Running `server scrape url https://example.com/events` fetches the page, extracts any `<script type="application/ld+json">` Event data, normalizes it to SEL format, and either displays it (--dry-run) or submits it to the batch ingest API.

**Acceptance Scenarios**:

1. **Given** a URL with embedded schema.org/Event JSON-LD, **When** the operator runs `server scrape url <URL>`, **Then** the system extracts events, normalizes them, and submits to the batch ingest API, reporting counts (found, new, duplicate, failed)
2. **Given** a URL with no JSON-LD, **When** the operator runs `server scrape url <URL>`, **Then** the system reports "no structured data found" and suggests creating a selector config
3. **Given** the `--dry-run` flag, **When** scraping any URL, **Then** the system displays extracted events as JSON without submitting them
4. **Given** a URL that returns an error (404, timeout, connection refused), **Then** the system reports the error clearly with the HTTP status and URL
5. **Given** a URL blocked by robots.txt, **Then** the system reports it was blocked and does not fetch the page

---

### User Story 2 — Operator Manages Source Configurations (Priority: P1)

An operator maintains a set of known event sources as YAML configuration files, enabling repeatable scraping. Sources can use Tier 0 (JSON-LD) or Tier 1 (CSS selectors) extraction.

**Independent Test**: Source configs in `configs/sources/*.yaml` are loaded, validated, and listed via `server scrape list`. Individual sources can be scraped with `server scrape source <name>`.

**Acceptance Scenarios**:

1. **Given** YAML or DB-backed source configs exist, **When** the operator runs `server scrape list`, **Then** the system lists all sources with name, URL, tier, schedule, enabled flag, and last scrape status (preferring DB entries when available)
2. **Given** a valid source config, **When** the operator runs `server scrape source <name>`, **Then** the system scrapes that source using the configured tier and settings
3. **Given** an invalid source config (missing required fields), **When** the system loads configs, **Then** it reports validation errors with the file path and field name
4. **Given** a Tier 1 source config with CSS selectors, **When** scraping, **Then** the system uses Colly with the configured selectors to extract event data
5. **Given** a Tier 2 source config with `headless` settings, **When** scraping, **Then** the system renders the page via Rod/Chromium and extracts events from the rendered DOM
6. **Given** a Tier 3 source config with `graphql` settings, **When** scraping, **Then** the system queries the GraphQL endpoint and maps response records into events

---

### User Story 3 — Operator Scrapes All Configured Sources or Schedules Them (Priority: P2)

An operator wants to run all configured sources in one command for bulk data collection.

**Independent Test**: Running `server scrape all` iterates through all enabled source configs, scrapes each one, and reports aggregate results.

**Acceptance Scenarios**:

1. **Given** multiple source configs exist, **When** the operator runs `server scrape all`, **Then** the system scrapes each enabled source sequentially and reports per-source and aggregate results
2. **Given** one source fails during `scrape all`, **Then** the system continues with remaining sources and includes the failure in the summary
3. **Given** `--dry-run`, **Then** no events are submitted for any source
4. **Given** `--limit N`, **Then** at most N events are extracted per source
5. **Given** sources with `schedule` set to `daily` or `weekly`, **Then** the River `ScrapeSourceWorker` enqueues and executes periodic jobs when `auto_scrape` is enabled in `scraper_config`, honoring concurrency/timeouts from that table.

---

### User Story 4 — Scrape Runs & Metrics Are Tracked (Priority: P2)

Each scrape execution is recorded in the database for monitoring, debugging, and preventing excessive re-scraping.

**Independent Test**: After a scrape completes, a `scraper_runs` record exists with source, timing, event counts, and status.

**Acceptance Scenarios**:

1. **Given** a scrape starts, **Then** a `scraper_runs` row is created with status "running"
2. **Given** a scrape completes successfully, **Then** the row is updated with status "completed", event counts, and duration
3. **Given** a scrape fails, **Then** the row is updated with status "failed" and the error message
4. **Given** Prometheus metrics work (future bead `srv-sf4vs`), **Then** the scraper emits success/failure counters, duration histograms, and event totals for both CLI-triggered and River-triggered runs.

---

## Technical Design

### Dependencies

| Package | Purpose | Notes |
|---------|---------|-------|
| `github.com/PuerkitoBio/goquery` | HTML parsing, JSON-LD extraction | jQuery-like CSS selectors |
| `github.com/gocolly/colly/v2` | Web crawling framework | Rate limiting, robots.txt, caching |
| `github.com/go-rod/rod` | Headless Chromium scraping | Tier 2 JS-rendered sources |

### Package Structure

```
internal/scraper/
  scraper.go           — Scraper service: orchestrates tiers, manages runs
  jsonld.go            — Tier 0: fetch URL, extract JSON-LD Event blocks
  colly.go             — Tier 1: Colly-based CSS selector extraction
  rod.go               — Tier 2: Headless Chromium + CSS selector extraction
  graphql.go           — Tier 3: GraphQL API extraction
  normalize.go         — Map schema.org JSON-LD variants → EventInput
  config.go            — Source config types, YAML loader, validation (DB or YAML)
  db_source.go         — Domain → scraper SourceConfig translation helpers
  ingest.go            — HTTP client for SEL batch ingest API

cmd/server/cmd/
  scrape_cmd.go         — CLI: server scrape {url,source,all,list,inspect,test, sync,export}
  scrape_capture.go     — CLI: server scrape capture (headless render)

configs/sources/
  _example.yaml        — Documented example source config
  *.yaml               — Per-source configs (community-contributed)

internal/storage/postgres/migrations/
  000031_add_scraper_runs.{up,down}.sql
  000032_scraper_sources.{up,down}.sql
  000033_drop_extra_scraper_sources_index.sql
  000034_scraper_config.{up,down}.sql
```

### Source Config Schema

```yaml
# Required fields
name: "Toronto Symphony Orchestra"     # Unique identifier
url: "https://www.tso.ca/concerts-events"

# Optional fields
schedule: "daily"                      # daily, weekly, manual
trust_level: 5                         # 1-10, maps to SEL source trust
license: "CC-BY-4.0"                   # Default license attribution
enabled: true                          # Can disable without deleting
event_url_pattern: "/events/*"         # Only follow links matching pattern
max_pages: 10                          # Safety limit for pagination
rate_limit_ms: 1000                    # Overrides global rate limiting when set
headers:
  User-Agent: "Mozilla/5.0..."         # Optional per-source headers

# Tier 1 selectors (required when tier=1 or tier=2)
selectors:
  event_list: "div.event-card"         # Container for each event
  name: "h2.event-title"
  start_date: "time[datetime]"         # Prefers datetime attribute
  end_date: "time.end-time[datetime]"
  location: "span.venue-name"
  description: "p.event-description"
  url: "a.event-link"                  # Link to detail page
  image: "img.event-image"
  pagination: "a.next-page"

# Tier 2 headless options (required when tier=2)
headless:
  wait_selector: "div.event-card"       # Wait for selector before extract
  wait_timeout_ms: 15000                # Navigation timeout
  pagination_button: "button.next"      # Optional JS pagination click
  rate_limit_ms: 1000                   # Delay between pages
  headers:
    Accept-Language: "en-CA"

# Tier 3 GraphQL options (required when tier=3)
graphql:
  endpoint: "https://graphql.example.com/"
  query: |
    query AllEvents { events { title startDate } }
  event_field: "events"
  token: "public-token"
  url_template: "https://example.com/events/{{.slug}}"
  timeout_ms: 30000
```

Tier assignments map to both YAML configs and `scraper_sources.tier`: Tier 0 (JSON-LD), Tier 1 (CSS selectors), Tier 2 (headless Chromium), Tier 3 (GraphQL API).

### Database Schema

#### `scraper_sources` (implemented)

```sql
CREATE TABLE scraper_sources (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT UNIQUE NOT NULL,
  url             TEXT NOT NULL,
  tier            INT NOT NULL DEFAULT 0,
  schedule        TEXT NOT NULL DEFAULT 'manual'
                    CHECK (schedule IN ('daily', 'weekly', 'manual')),
  trust_level     INT NOT NULL DEFAULT 5,
  license         TEXT NOT NULL DEFAULT 'CC0-1.0',
  enabled         BOOL NOT NULL DEFAULT true,
  max_pages       INT NOT NULL DEFAULT 10,
  selectors       JSONB,           -- null for tier 0
  headless_wait_selector   TEXT,
  headless_wait_timeout_ms INT NOT NULL DEFAULT 0,
  headless_pagination_btn  TEXT,
  headless_headers         JSONB,
  headless_rate_limit_ms   INT NOT NULL DEFAULT 0,
  graphql_config JSONB,    -- null for non-GraphQL sources
  notes           TEXT,            -- curator freetext, exposed in API
  last_scraped_at TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE org_scraper_sources (
  organization_id UUID   NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  scraper_source_id BIGINT NOT NULL REFERENCES scraper_sources(id) ON DELETE CASCADE,
  PRIMARY KEY (organization_id, scraper_source_id)
);

CREATE TABLE place_scraper_sources (
  place_id          UUID   NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  scraper_source_id BIGINT NOT NULL REFERENCES scraper_sources(id) ON DELETE CASCADE,
  PRIMARY KEY (place_id, scraper_source_id)
);
```

#### `scraper_runs` (existing)

```sql
CREATE TABLE scraper_runs (
  id            BIGSERIAL PRIMARY KEY,
  source_name   TEXT NOT NULL,
  source_url    TEXT NOT NULL,
  tier          INT NOT NULL DEFAULT 0,
  started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at  TIMESTAMPTZ,
  status        TEXT NOT NULL DEFAULT 'running',
  events_found  INT DEFAULT 0,
  events_new    INT DEFAULT 0,
  events_dup    INT DEFAULT 0,
  events_failed INT DEFAULT 0,
  error_message TEXT,
  metadata      JSONB
);

CREATE INDEX idx_scraper_runs_source ON scraper_runs(source_name);
CREATE INDEX idx_scraper_runs_started ON scraper_runs(started_at DESC);
```

### Ingestion Path

The scraper submits events via the existing HTTP batch ingest API (`POST /api/v1/events:batch`), not via direct service calls. This:
- Validates the API itself during dogfooding
- Ensures all middleware (auth, rate limiting, validation) is exercised
- Keeps the scraper loosely coupled from server internals
- Means the scraper needs an API key (configured via env or flag)

### Tiered Extraction Strategy

**Tier 0 — JSON-LD Extraction** (preferred, zero-config per site):
1. Fetch page with `net/http`
2. Parse HTML with Goquery
3. Find all `<script type="application/ld+json">` blocks
4. Parse each as JSON, filter for `@type: "Event"` (or arrays containing Events)
5. Handle nested structures: `@graph`, `ItemList`, `EventSeries`
6. Normalize to `EventInput` and submit

**Tier 1 — Colly CSS Selector Scraping** (requires per-site config):
1. Create Colly collector with rate limiting and robots.txt compliance
2. Visit source URL
3. For each element matching `selectors.event_list`:
   - Extract fields using configured CSS selectors
   - Follow detail page links if URL selector is configured
4. Follow pagination links if configured
5. Normalize extracted data to `EventInput` and submit

**Tier 2 — Headless Browser Scraping** (requires per-site config):
1. Launch Rod-controlled Chromium with rate limiting
2. Navigate to source URL and wait for `headless.wait_selector`
3. Extract rendered DOM using CSS selectors
4. Normalize extracted data to `EventInput` and submit

**Tier 3 — GraphQL API Scraping** (requires per-site config):
1. POST GraphQL query to `graphql.endpoint`
2. Map response records to `RawEvent` fields
3. Render optional `graphql.url_template` for canonical URLs
4. Normalize extracted data to `EventInput` and submit

### User-Agent and Identification

```
Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)
```

### Rate Limiting

- Default: 1 request/second per domain
- Configurable per source in YAML
- Colly handles this natively
- For Tier 0 single-page fetches, enforced manually

### Robots.txt Compliance

- Colly respects robots.txt by default
- For Tier 0, check robots.txt manually before fetching
- Log when a URL is blocked by robots.txt

### Error Handling

- HTTP errors: log and skip, continue with next source
- Parse errors: log with source URL and raw data snippet, continue
- Normalization errors: log which fields failed, submit what we can
- API errors: respect 429 with backoff, fail on persistent 4xx/5xx

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Sites change HTML structure | Tier 1 selectors break | Monitor scrape success rates; Tier 0 is resilient to layout changes |
| Sites block our scraper | No events from that source | Transparent User-Agent; respect robots.txt; contact site operators |
| Schema.org JSON-LD is inconsistent across sites | Parse failures | Robust normalization with fallbacks for common variants |
| Too many events overwhelm staging | System overload | --limit flag; per-source max_pages; trust_level controls |
| License ambiguity for scraped data | Legal risk | Default to source config license; flag events without clear license for admin review |

## Success Metrics

- Can scrape 5+ real Toronto arts/culture sites and ingest events into staging
- Tier 0 extraction works with zero per-site config on sites with JSON-LD
- Tier 1 extraction works for 3+ sites with custom selectors
- MCP curation agent can discover and work with scraped events
- Scrape runs are tracked and observable
