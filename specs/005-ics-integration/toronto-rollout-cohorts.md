# Toronto ICS Rollout Cohorts

**Spec**: 005-ics-integration / Phase 5 | **Date**: 2026-04-14 | **Status**: Draft
**Parent**: `specs/005-ics-integration/spec-phase5.md`

## Overview

Two-phase staged rollout of Toronto ICS sources. Each cohort has explicit entry/exit
criteria, acceptance targets, and a published outcome report. Cohorts run sequentially:
Cohort 1 must complete before Cohort 2 begins.

## Cohort 1: High-Value Net-New Sources

### Entry Criteria

- [x] ICS pipeline operational (Phase 1–4 delivered)
- [x] `server scrape sync` available for config-to-DB sync
- [x] `eventSchedule` e2e test passing (recurring ICS → series dates → JSON-LD)
- [ ] Cohort 1 manifest subset identified and config files drafted

### Sources (high priority first)

| # | source_key | display_name | feed_type | est_events |
|---|-----------|-------------|-----------|------------|
| 1 | torevent | torevent (Toronto Events) | tockify | ~2,900 |
| 2 | york-university | York University | wordpress_mec | ~6,558 |
| 3 | culture-link | CultureLink | wordpress_events_manager | ~494 |
| 4 | now-toronto | NOW Toronto | wordpress_tribe | ~30 |
| 5 | cita-local-events | CITA Local Events | google_calendar | ~533 |
| 6 | cita-seminars | CITA Seminars | google_calendar | ~533 |
| 7 | cita-special-events | CITA Special Events | google_calendar | ~534 |
| 8 | show-up-toronto | Show Up Toronto | static_ics | unknown |
| 9 | repair-cafe-toronto | Repair Cafe Toronto | wordpress_events_manager | ~82 |
| 10 | another-story-bookshop | Another Story Bookshop | eventbrite_bridge | unknown |
| 11–19 | meetup-tech-* | Tech/Dev Meetup groups (8) | meetup_ics | varies |
| 20–25 | meetup-outdoors-* | Hiking/Outdoors Meetup groups (6) | meetup_ics | varies |
| 26–30 | meetup-social-* | Social/Activities Meetup groups (5) | meetup_ics | varies |

**Total Cohort 1 high-priority**: 30 sources

Medium-priority Cohort 1 sources (remaining net-new) follow after high-priority
onboarding succeeds for ≥80% of attempted sources.

### Acceptance Targets

1. **Onboarding rate**: ≥80% of attempted sources onboarded within one scrape cycle.
2. **Recurring sources**: `eventSchedule` present in `/api/v1/events/{id}` JSON-LD
   within one scrape cycle for sources with RRULE-bearing ICS feeds.
3. **Idempotent re-scrape**: re-scraping same source produces upsert (not duplicates)
   via `external_key` in `event_series`.
4. **Blocked sources**: each blocked source has documented reason + next action.
5. **No data loss**: ICS-sourced events retain source provenance (`X-SOURCE` /
   `SOURCE` properties preserved in payload).

### Expected Event Count Range

- Floor: ~200 events (if only high-value sources with small feeds succeed)
- Ceiling: ~4,000 events (torevent alone ~2,900; York U needs date filtering to
  avoid ingesting ~6,558 past events)

### Exit Criteria

- Rollout report published at `specs/005-ics-integration/toronto-rollout-report-cohort1.md`
- Per-source outcomes recorded with taxonomy (`onboarded`/`deferred`/`blocked`/`non_starter`)
- Taxonomy outcomes feed back into `docs/integration/event-platforms.md` and
  `docs/integration/ics-feeds.md`

---

## Cohort 2: Overlap Validation

### Entry Criteria

- Cohort 1 complete (rollout report published)
- Overlap source configs identified (existing YAML configs in `configs/sources/`)
- ICS ingest pipeline proven on ≥80% of Cohort 1 sources

### Sources

| # | source_key | display_name | sel_config_name | ICS type |
|---|-----------|-------------|----------------|----------|
| 1 | bloor-west-village-bia | Bloor West Village BIA | bloor-west-village-bia | WordPress Tribe |
| 2 | buddies-in-bad-times | Buddies in Bad Times | buddies-in-bad-times | WordPress Tribe |
| 3 | factory-theatre | Factory Theatre | factory-theatre | WordPress Tribe |
| 4 | gardiner-museum | Gardiner Museum | gardiner-museum | WordPress Tribe |
| 5 | glad-day-bookshop | Glad Day Bookshop | glad-day-bookshop | Eventbrite bridge |
| 6 | high-park-nature-centre | High Park Nature Centre | high-park-nature-centre | WordPress Tribe |
| 7 | jazz-bistro | Jazz Bistro | jazz-bistro | WordPress Tribe |
| 8 | textile-museum | Textile Museum | textile-museum | WordPress Tribe |
| 9 | toronto-botanical-garden | Toronto Botanical Garden | toronto-botanical-garden | WordPress Tribe |
| 10 | toronto-knitters-guild | Toronto Knitters Guild | toronto-knitters-guild | WordPress Tribe |
| 11 | toronto-union | Union Station | toronto-union | WordPress Tribe |

**Total Cohort 2**: 11 sources

### Goal

Compare ICS ingest against existing HTML/JSON-LD scraper for each overlap source.
Determine per source whether ICS or HTML should be the primary ingest path.

### Acceptance Targets

1. **ICS coverage**: ICS ingest produces ≥90% of events already captured by
   the HTML scraper (measured by event name + startDate + venue match).
2. **No duplicates**: dedup by content hash prevents duplicate events when both
   ICS and HTML scrapers run for the same source.
3. **Per-source decision**: for each overlap source, document one of:
   - **Keep ICS as primary** — disable HTML scraper, promote ICS config
   - **Keep HTML as primary** — ICS config remains as fallback/monitoring only
   - **Run both** — both paths active; justify with data

### Exit Criteria

- Per-source decision documented in rollout report
- ICS configs for sources switching to ICS-primary synced to DB via `server scrape sync`
- HTML configs for sources remaining HTML-primary left unchanged

---

## Out-of-Scope Guardrails

1. **No new dashboard** — rollout tracking uses beads and the manifest; no UI
   application is built in Phase 5.
2. **No bulk onboarding** — sources are onboarded one-at-a-time or in small batches
   within a cohort; no mass-import tooling.
3. **No architecture changes** — if Cohort 1/2 reveals core pipeline bugs, create
   follow-up beads against Phase 1–3 components; do not patch the ICS pipeline
   during rollout.
4. **No new feed types** — the nine feed types in the manifest are known quantities;
   discovering a new platform pattern creates a follow-up investigation bead, not
   an in-phase implementation.
5. **No SEL-only source work** — the ~96 arts institutions without ICS feeds stay
   on HTML/JSON-LD scrapers; Phase 5 does not attempt to find or create ICS feeds
   for them.