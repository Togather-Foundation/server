---
description: "Task list for integrated event scraper"
---

# Tasks: Integrated Event Scraper

**Input**: Design documents from /specs/003-scraper/
**Prerequisites**: plan.md, spec.md, existing batch ingest API, source registry
**Tests**: REQUIRED (TDD where practical). Target 80%+ coverage for normalizer, config loader, extraction logic.

---

## Completed Work

| Phase | Scope | Status |
|-------|-------|--------|
| Phase 1 — Tier 0 foundation | `internal/scraper/{jsonld,normalize,ingest,scraper}.go`, CLI `server scrape url`, `scraper_runs` migration/tests | ✅ Delivered (beads srv-p6vbo → srv-xyp0r) |
| Phase 2 — Tier 1 + configs | `internal/scraper/colly.go`, selector normalization/tests, initial GTA configs in `configs/sources/`, `server scrape {source,all,list}` | ✅ Delivered (srv-rnb2s, srv-ztij5, srv-0mnq0) |
| Phase 3 — DB-backed configs | `scraper_sources` migration + repository, YAML↔DB sync/export, DB-first loading, API exposure on org/place handlers | ✅ Delivered (srv-65kvw, srv-iorfa, srv-2nu7e, srv-l71q1, srv-17zth) |
| Phase 4 — Scheduling & admin UI | River `ScrapeSourceWorker`, `scraper_config` tunables, admin UI surfacing scrape status/history | ✅ Delivered (srv-pfeud, srv-5127b) |

Supporting assets:
- `/agents/generate-selectors.md` orchestrates Tier 1 selector authoring via `server scrape inspect` + `server scrape test` and writes vetted configs.
- `configs/sources/_example.yaml` documents the full schema; real configs live beside it and mirror DB rows via `server scrape sync/export`.
- `docs/integration/scraper.md` and `docs/integration/api-guide.md` describe `sel:scraperSource` exposure.

---

## Remaining / Upcoming Tasks

### S025 — Scraper Prometheus Metrics *(srv-sf4vs, P3, closed)*
- ✅ Implemented metrics counters/histograms and registration.
- ✅ Updated `docs/deploy/monitoring.md`.
- ✅ Added unit tests covering metric increments.

### S026 — Tier 2 Headless Scraping *(srv-h264z, P4, open)*
- ✅ Rod-based Tier 2 extraction (`internal/scraper/rod.go`) with `headless` config block.
- ✅ `scraper_sources` schema extended via `000035_scraper_sources_headless`.
- ✅ CLI support (`--headless`, `server scrape capture`).
- ✅ Tests for headless extractor and round-trip DB config.
- ⏳ Remaining: advanced headless enhancements (browser pool, additional selectors, staging-only smoke).

### S027 — Tier 3 GraphQL Scraping *(implemented)*
- ✅ GraphQL extractor and config block (`internal/scraper/graphql.go`).
- ✅ DB JSONB column (`graphql_config`) via migration `000036_scraper_sources_graphql_config`.
- ✅ Tests for GraphQL fetch/extract mapping.

### S028 — Data Quality & Agent Feedback *(future backlog)*
- Event completeness scoring per scrape (percentage of populated fields) persisted to `scraper_runs.metadata`.
- Source quality trend metrics surfaced in admin UI and MCP tooling.
- MCP workflow for curators to flag/resolve scraper regressions directly from SEL.

### Operational Hygiene
- Keep `/agents/generate-selectors.md` workflow up-to-date with new CLI options (e.g., tier 2 flags) and ensure new configs round-trip via `server scrape sync/export`.
- Re-run `server scrape all --dry-run` whenever configs change materially and document findings in `configs/sources/README.md`.

---

## Archived Task Lists

Historical detailed checklists for Phases 1–3 are preserved in git history (see commit range `003-scraper` feature branch). Refer to earlier revisions of this file if granular task context is required for audits.
