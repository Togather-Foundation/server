# ICS Rollout Outcome Taxonomy

**Spec**: 005-ics-integration / Phase 5 | **Date**: 2026-04-14 | **Status**: Draft
**Parent**: `specs/005-ics-integration/spec-phase5.md`

## Outcome Definitions

Every source in the rollout manifest must end in exactly one of these four outcomes.
No other labels are valid. If a source doesn't fit, create a new outcome or document
why in the source's rollout notes and flag for review.

| Outcome | Definition | Capture Criteria |
|---------|-----------|-----------------|
| `onboarded` | Source fully working: config synced, events appearing in API, `eventSchedule` on recurring events | Config exists in `configs/sources/`, synced via `server scrape sync`, ≥1 event in DB within one scrape cycle. For recurring sources: `event_series` row has `series_start_date`, `schedule_timezone`, and `rrule` populated; `/api/v1/events/{id}` JSON-LD includes `eventSchedule`. |
| `deferred` | Works technically but deprioritized: low event count, manual tweaks needed, or low audience fit | Config exists or was partially validated. Reason for deferral documented in manifest `notes`. Revisit date set (e.g., `deferred until 2026-07` for seasonal sources). Source remains in manifest with `rollout_status: deferred`. |
| `blocked` | Cannot complete onboarding: ICS feed broken, requires auth, rate-limited, or URL changed | Block reason documented: `feed_url`, error observed, date of attempt. Next action specified (retry with different approach, escalate, revisit). Bead left open for follow-up. Manifest `rollout_status: blocked`. |
| `non_starter` | No usable ICS feed exists and no alternative ingest path is viable | Confirmed via direct inspection (HTTP 404, 403, malformed feed, no feed discovered). Documented in manifest. Bead closed with `non_starter` outcome. `rollout_status: non_starter`. |

## Decision Guide

### When to use `deferred` vs `blocked` vs `non_starter`

**Use `deferred` when:**
- The ICS feed works and produces events, but the event count is too low to justify
  ongoing monitoring right now (e.g., <5 events/year).
- The feed has minor issues that require manual intervention per cycle (e.g., date
  format quirks that need per-scrape normalization tweaks).
- The source's audience overlap with Toronto SEL is uncertain or low.
- The feed is seasonal (e.g., summer festivals) and onboarding should wait for the
  active season.

**Use `blocked` when:**
- The ICS feed URL returns an error (403, 404, 503, timeout, SSL failure) that
  might be transient or fixable with configuration changes.
- The feed requires authentication that hasn't been set up yet.
- The feed is behind a rate limit or abuse protection that needs a different
  scraping strategy (e.g., Rod headless, proxy rotation).
- The feed structure is valid but produces malformed RRULE or timezone data that
  might be fixable with a parser improvement upstream.

**Use `non_starter` when:**
- Repeated attempts (≥2) to discover an ICS feed have failed.
- The venue confirmed no ICS feed exists (via direct inquiry or reliable secondary
  source).
- The feed was once available but has been permanently removed (e.g., site migrated
  to a platform with no ICS support).
- The platform is fundamentally incompatible (e.g., requires interactive JavaScript
  with no server-rendered events, Cloudflare-protected with no API path).

### Key distinction

- **`blocked`** → there is a plausible path forward; work is paused, not abandoned.
- **`deferred`** → the source works but is not a priority; may revisit later.
- **`non_starter`** → no plausible path; close and move on.

## Anti-Patterns to Avoid

These labels are **not** valid outcomes. If you find yourself using any of these,
re-evaluate against the decision guide above:

| Anti-Pattern | Why It's Bad | Correct Outcome |
|---|---|---|
| "maybe later" | Vague; no revisit date or reason | `deferred` with revisit date |
| "needs work" | No specificity; what work? whose? | `blocked` with specific blocker and next action |
| "TBD" | Not a decision; defers accountability | `blocked` or `deferred` with documented reason |
| "in progress" | Process state, not outcome | Not a terminal outcome; use `planned` → `onboarded`/`blocked`/`deferred` |
| "partially working" | Ambiguous; how partial? | `onboarded` (if ≥1 event in API) or `blocked` (if fundamental issue remains) |
| "skip" | No documented reason | `non_starter` (no viable ingest path) or `deferred` (low priority) |
| "waiting on X" | Tracking blocker without outcome | `blocked` with next action pointing to X |

## Escalation Path

If a source is blocked by a bug or limitation in the ICS pipeline itself (not the
feed), do not attempt to fix the pipeline during rollout. Instead:

1. **Create a follow-up bead** against the relevant Phase 1–3 component:
   - Parser bug → bead tagged `ics-parser`, referencing `internal/ical/parse.go`
   - Mapper bug → bead tagged `ics-mapper`, referencing `internal/ical/mapper.go`
   - RRULE expansion bug → bead tagged `rrule-expansion`, referencing `internal/ical/rrule.go`
   - Scraper integration bug → bead tagged `ics-extractor`, referencing `internal/scraper/ics.go`
   - Series dates pipeline bug → bead tagged `series-dates`, referencing `internal/domain/events/create_event_core.go`

2. **Set the manifest `rollout_status` to `blocked`** with notes referencing the
   bead ID and the specific pipeline component.

3. **Include the blocked source in the cohort report** with:
   - Feed URL attempted
   - Error observed (parse error, unexpected RRULE shape, timezone issue, etc.)
   - Expected fix (parser tolerance, timezone mapping, new RRULE pattern support)
   - Bead ID for the follow-up

4. **Do not patch the pipeline mid-cohort.** The rollout tests the pipeline as-is;
   pipeline fixes belong in separate beads merged outside the cohort workflow.

### Resolution after pipeline fix

Once the follow-up bead is resolved:
1. Re-attempt the blocked source in the current or next cohort.
2. Update manifest `rollout_status` from `blocked` to `onboarded`/`deferred`/`non_starter`
   based on the re-attempt result.
3. Add a note to the manifest with the resolution date and the PR that fixed the
   pipeline issue.