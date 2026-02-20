# Implementation Plan: Integrated Event Scraper

**Branch**: `003-scraper` | **Date**: 2026-02-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-scraper/spec.md`

## Summary

Add an integrated event scraper to the SEL server that extracts events from Toronto-area arts/culture websites and ingests them via the existing batch API. Two-tier extraction: Tier 0 (JSON-LD, zero-config) and Tier 1 (Colly + CSS selectors, per-site YAML config). Community contributes source URLs as YAML files; the SEL handles dedup, reconciliation, and provenance automatically.

## Technical Context

**New Dependencies**:
- `github.com/PuerkitoBio/goquery` v1.x — HTML parsing and JSON-LD extraction
- `github.com/gocolly/colly/v2` v2.x — Web scraping framework with rate limiting, robots.txt, caching

**Existing Infrastructure Leveraged**:
- Batch ingest API (`POST /api/v1/events:batch`) — scraper is a consumer
- Source registry with trust levels and provenance tracking
- 3-layer deduplication (source external ID, content hash, manual merge)
- Artsdata reconciliation for places and organizations
- Cobra CLI framework for `server scrape` command
- PostgreSQL for `scraper_runs` tracking table
- zerolog for structured logging

**Not Used (Yet)**:
- River job queue (Phase 3 — scheduling)
- Rod/Chromedp (Tier 2 — JS-heavy sites, future)

## Architecture

```
                   ┌─────────────────────────────────────┐
                   │         CLI: server scrape           │
                   │   url │ source │ all │ list          │
                   └──────────────┬──────────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │         Scraper Service              │
                   │  Load config → Pick tier → Extract   │
                   └──┬───────────────────────────────┬──┘
                      │                               │
           ┌──────────▼──────────┐       ┌───────────▼──────────┐
           │  Tier 0: JSON-LD    │       │  Tier 1: Colly       │
           │  net/http + goquery │       │  CSS selectors       │
           │  Zero-config        │       │  Per-site YAML       │
           └──────────┬──────────┘       └───────────┬──────────┘
                      │                               │
                      └───────────┬───────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │         Normalizer                   │
                   │  schema.org JSON-LD → EventInput     │
                   └──────────────┬──────────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │         Ingest Client                │
                   │  POST /api/v1/events:batch           │
                   │  (HTTP, with API key auth)           │
                   └──────────────┬──────────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │       Existing SEL Pipeline          │
                   │  Validate → Dedup → Ingest →         │
                   │  Reconcile → Enrich                  │
                   └─────────────────────────────────────┘
```

### Storage: Hybrid YAML + DB

- **YAML files** (`configs/sources/*.yaml`): Source definitions — what to scrape, how, and with what trust level. Version controlled, community-contributable via PRs.
- **Database** (`scraper_runs` table): Runtime state — when each source was last scraped, what happened, metrics. Queryable for monitoring and debugging.

### JSON-LD Normalization Challenges

Schema.org Event JSON-LD in the wild comes in many shapes:

```jsonld
// Shape 1: Single event at top level
{"@type": "Event", "name": "Concert", ...}

// Shape 2: Array of events
[{"@type": "Event", ...}, {"@type": "Event", ...}]

// Shape 3: @graph container
{"@graph": [{"@type": "Event", ...}, ...]}

// Shape 4: ItemList wrapping events
{"@type": "ItemList", "itemListElement": [{"@type": "ListItem", "item": {"@type": "Event", ...}}]}

// Shape 5: Nested location as string vs object
"location": "Massey Hall"  vs  "location": {"@type": "Place", "name": "Massey Hall", ...}

// Shape 6: Date as string vs structured
"startDate": "2026-03-15T20:00:00-05:00"  vs  "startDate": {"@type": "Date", ...}

// Shape 7: offers as object vs array
"offers": {"@type": "Offer", ...}  vs  "offers": [{"@type": "Offer", ...}]
```

The normalizer must handle all of these gracefully.

## Phases

### Phase 1: Foundation + Tier 0 (JSON-LD Extraction)

**Goal**: `server scrape url <URL>` works end-to-end with JSON-LD sites.

1. Add Goquery + Colly dependencies
2. Create `scraper_runs` migration
3. Implement `internal/scraper/config.go` — types and YAML loader
4. Implement `internal/scraper/jsonld.go` — Tier 0 extraction
5. Implement `internal/scraper/normalize.go` — schema.org → EventInput mapping
6. Implement `internal/scraper/ingest.go` — HTTP client for batch API
7. Implement `internal/scraper/scraper.go` — orchestrator
8. Implement `cmd/server/cmd/scrape.go` — CLI with `url` subcommand
9. Unit tests for JSON-LD extraction, normalization
10. Integration test: scrape a real URL, verify events appear in API

### Phase 2: Tier 1 (Colly) + Source Configs

**Goal**: Config-driven scraping with CSS selectors for sites without JSON-LD.

1. Implement `internal/scraper/colly.go` — Colly-based extraction
2. Create `configs/sources/_example.yaml` — documented template
3. Create 5-10 real GTA source configs
4. Implement `server scrape source`, `server scrape all`, `server scrape list`
5. Add `scraper_runs` DB tracking
6. Unit tests for Colly extraction, config validation
7. End-to-end test: scrape all sources on staging

### Phase 3: Scheduling + Production (Future)

1. River job worker for periodic scraping
2. Config-driven schedules (daily, weekly)
3. Prometheus metrics for scrape success/failure rates
4. Admin UI page for scrape status

### Phase 4: Agent Feedback + Quality (Future)

1. Event completeness scoring
2. Source quality metrics over time
3. MCP tool for agent to report data quality issues
4. Agent-assisted source config generation

## Constitution Check

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Schema.org alignment | PASS | Extracts native schema.org/Event JSON-LD; normalizes to SEL's schema.org-aligned EventInput |
| Provenance tracking | PASS | Each scraped event carries source URL, source event ID, scraper trust level |
| License compliance | PASS | Source config declares license; scraper passes to ingest API; SEL rejects non-CC0 |
| Robots.txt compliance | PASS | Colly respects by default; Tier 0 checks manually |
| Rate limiting | PASS | 1 req/sec default; configurable per source; transparent User-Agent |
| Deduplication | PASS | Uses existing 3-layer dedup via batch ingest API |

## Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Few Toronto sites have JSON-LD | Medium | Tier 1 CSS selectors cover non-JSON-LD sites; advocacy for schema.org adoption |
| Schema.org variants break parser | Medium | Extensive normalization with fallbacks; test against real sites |
| Sites block scraper | Low | Transparent User-Agent; respect robots.txt; manual outreach |
| Scraper generates low-quality data | Medium | --dry-run for testing; trust_level controls merge priority; admin review queue |

## Test Strategy

- **Unit tests**: JSON-LD extraction from sample HTML, normalization of all schema.org variants, config loading/validation
- **Integration tests**: Scrape a test server (httptest) with known JSON-LD, verify EventInput output
- **E2E tests**: Scrape real staging sources, verify events appear in API responses
- **Contract tests**: Verify scraped JSON-LD produces valid SHACL shapes
