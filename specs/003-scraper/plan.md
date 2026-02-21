# Implementation Plan: Integrated Event Scraper

**Branch**: `003-scraper` | **Date**: 2026-02-21 | **Spec**: [spec.md](./spec.md)
**Status**: Phases 1 & 2 complete. Phase 3 (DB-backed source configs) in progress — migration, domain/storage layer, sync/export CLI, and DB-first runtime done. API exposure (`srv-17zth`) pending.
**Input**: Feature specification from `/specs/003-scraper/spec.md`

## Summary

Add an integrated event scraper to the SEL server that extracts events from Toronto-area arts/culture websites and ingests them via the existing batch API. Two-tier extraction: Tier 0 (JSON-LD, zero-config) and Tier 1 (Colly + CSS selectors, per-site YAML config). Community contributes source URLs as YAML files; the SEL handles dedup, reconciliation, and provenance automatically.

## Technical Context

**New Dependencies** (added):
- `github.com/PuerkitoBio/goquery` v1.11.0 — HTML parsing and JSON-LD extraction
- `github.com/gocolly/colly/v2` v2.3.0 — Web scraping framework with rate limiting, robots.txt, caching
- `gopkg.in/yaml.v3` — promoted from indirect to direct for source config loading

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
- **Database** (`scraper_sources` table, Phase 3): Runtime store for source configs. `server scrape sync` upserts YAML→DB; `server scrape export` dumps DB→YAML. Runtime scraping reads from DB first, falls back to YAML if DB unavailable or empty.
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

### Phase 1: Foundation + Tier 0 (JSON-LD Extraction) ✅ COMPLETE

**Goal**: `server scrape url <URL>` works end-to-end with JSON-LD sites.

1. ✅ Add Goquery + Colly dependencies (`srv-p6vbo`)
2. ✅ Create `scraper_runs` migration + SQLc queries (`srv-0hk2l`)
3. ✅ Implement `internal/scraper/config.go` — types and YAML loader (`srv-rje5r`)
4. ✅ Implement `internal/scraper/jsonld.go` — Tier 0 extraction (`srv-5atjx`)
5. ✅ Implement `internal/scraper/normalize.go` — schema.org → EventInput mapping (`srv-2gby0`)
6. ✅ Implement `internal/scraper/ingest.go` — HTTP client for batch API (`srv-qiii1`)
7. ✅ Implement `internal/scraper/scraper.go` — orchestrator (`srv-4xupn`)
8. ✅ Implement `cmd/server/cmd/scrape.go` — CLI with `url` subcommand (`srv-xyp0r`)
9. ✅ Unit tests: JSON-LD extraction (22 tests), normalization (35 tests), 7 HTML fixtures
10. ✅ Integration tests: httptest-based scrape tests

### Phase 2: Tier 1 (Colly) + Source Configs ✅ COMPLETE

**Goal**: Config-driven scraping with CSS selectors for sites without JSON-LD.

1. ✅ Implement `internal/scraper/colly.go` — Colly-based extraction (`srv-rnb2s`)
2. ✅ Create `configs/sources/_example.yaml` — documented template (`srv-ztij5`)
3. ✅ Create 6 GTA source configs: TSO, Roy Thomson, Hot Docs, Glad Day, TPL, Harbourfront (`srv-ztij5`)
4. ✅ Implement `server scrape source`, `server scrape all`, `server scrape list` (`srv-xyp0r`)
5. ✅ Add `scraper_runs` DB tracking wired into all tiers (`srv-0mnq0`)
6. ✅ Code review + security fixes: body limits, no-redirect clients, signal contexts, EventID dedup, @type preservation, CHECK constraint
7. ✅ Deployed to staging — all smoke tests passing

**Known issue**: The 6 GTA source configs contain placeholder URLs that have not been validated against live sites. See `srv-aany8`.

### Phase 3: DB-Backed Source Configs (In Progress)

**Goal**: `scraper_sources` table is the runtime store; YAML files remain version-controlled canonical source. Sync and export CLI commands; runtime reads DB-first.

1. ✅ Migration `000032_scraper_sources.up.sql` + SQLc queries (`srv-65kvw`)
2. ✅ `internal/domain/scraper/source.go` — `Source` type, `Repository` interface, `Service` (`srv-iorfa`)
3. ✅ `internal/storage/postgres/scraper_sources_repository.go` — postgres impl (`srv-iorfa`)
4. ✅ `server scrape sync` (YAML→DB upsert) + `server scrape export` (DB→YAML) CLI commands (`srv-2nu7e`)
5. ✅ `internal/scraper/db_source.go` — `domain/scraper.Source` → `SourceConfig` converter (`srv-l71q1`)
6. ✅ `internal/scraper/scraper.go` — added `sourceRepo` field, `NewScraperWithSourceRepo`, `loadSourceConfigs` (DB-first + YAML fallback) (`srv-l71q1`)
7. ✅ `server scrape list` — DB-first listing with YAML fallback (`srv-l71q1`)
8. ✅ `newScraperWithDB` wires `ScraperSourceRepository` into all scrape subcommands (`srv-l71q1`)
9. [ ] `srv-17zth` — expose `sel:scraperSource` in org/place JSON-LD API responses (blocked on srv-l71q1 ✅)

**Architecture decisions (Phase 3)**:
- `scraper_sources` table is separate from `organizations`/`places`; linked via `org_scraper_sources` and `place_scraper_sources` join tables (many-to-many).
- `internal/scraper` imports `internal/domain/scraper` with alias `domainScraper` — no circular dependency.
- `loadSourceConfigs` only passes `enabled: true` to the DB query; the YAML fallback runs `LoadSourceConfigs` which returns all valid configs (enabled + disabled). Both `ScrapeSource` and `ScrapeAll` check `cfg.Enabled` before scraping.
- `ScrapeAll` now dispatches tier directly (avoids double `loadSourceConfigs` call that would occur if it called `ScrapeSource`).
- `scrape list` uses `repo.List(ctx, nil)` (all sources, not just enabled) so operators can see disabled sources too.
- `server scrape sync` smoke-tested: loaded all 14 YAML sources into DB. `server scrape export` smoke-tested: wrote them back to YAML correctly.

### Phase 4: Scheduling + Production (Future)

1. River job worker for periodic scraping — `srv-pfeud`
2. Config-driven schedules (daily, weekly)
3. Prometheus metrics for scrape success/failure rates
4. Admin UI page for scrape status — `srv-5127b`

### Phase 5: Agent Feedback + Quality (Future)

1. Event completeness scoring
2. Source quality metrics over time
3. MCP tool for agent to report data quality issues
4. Agent-assisted source config generation

## Reflect (Phase 6)

### Design Decisions That Worked Well

- **Two-tier split was clean** — JSON-LD zero-config path vs Colly selector path mapped naturally to separate files with no coupling.
- **Submitting via batch API** — exercises the full auth/dedup/reconciliation pipeline during dogfooding; scraper stays loosely coupled.
- **Optional DB tracking** (`queries *postgres.Queries` may be nil) — unit tests stay simple; CLI gracefully degrades when `DATABASE_URL` is absent.
- **7 HTML fixture files** covering all 7 schema.org event shapes — gave high confidence in the normalizer before hitting live sites.
- **DB-first source config loading with YAML fallback** — `loadSourceConfigs` in `Scraper` tries the repository first; falls back to YAML transparently. The scraper works identically before and after `server scrape sync` is run.
- **`ScrapeAll` dispatches tier directly** (not via `ScrapeSource`) — avoids a double `loadSourceConfigs` call on every run while keeping the iteration logic in one place.

### Issues Found in Code Review (all fixed in `aa839d4`)

| Issue | Fix |
|-------|-----|
| No HTTP body size limits | `io.LimitReader`: 10 MiB HTML, 1 MiB ingest response |
| SSRF via open redirect | `CheckRedirect` returns `http.ErrUseLastResponse` on all clients |
| CLI ignored SIGINT/SIGTERM | `signal.NotifyContext` replaces `context.Background()` |
| Tier 1 EventID not deterministic | SHA-256 hash of `(source.Name + raw.Name + raw.StartDate)` as fallback |
| `@type` overwritten with "Event" | Preserve original type (e.g. `MusicEvent`, `TheaterEvent`) |
| Abort on first JSON-LD parse error | Skip-and-continue instead of early return |
| No DB CHECK on `status` column | `CHECK (status IN ('running','completed','failed'))` added to migration |

### Spec Divergences (cosmetic, intentional)

- SQLc field names differ slightly from spec: `SourceUrl` (not `SourceURL`), `EventsNew` (not `EventsCreated`), `EventsDup` (not `EventsDuplicate`). Column semantics are correct.
- Batch ingest response is async (202 Accepted); event created/dup counts come from the async status result, not the initial submit response. `IngestResult` handles both shapes.

### Follow-up Beads Filed

| Bead | Title | Priority |
|------|-------|----------|
| `srv-aany8` | Validate and fix GTA source config URLs against live sites | P3 |
| `srv-pfeud` | River job scheduling for periodic automated scrapes | P4 |
| `srv-5127b` | Admin UI for source management and run history | P4 |
| `srv-h264z` | Tier 2 headless browser support via Rod | P4 |

---

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
