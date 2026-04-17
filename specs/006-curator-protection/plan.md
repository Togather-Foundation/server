# Plan: Curator Protection — Prevent Scraper from Overwriting Human-Curated Event Data

**Spec**: 006-curator-protection | **Date**: 2026-04-16 | **Status**: Planning
**Goal**: When a human admin edits, consolidates, or enriches an event, subsequent scraper runs must not silently overwrite those changes.

## Vision

SEL's value proposition is a curated, high-quality events commons. Scrapers fill the pipeline, but human curators make the data trustworthy. Today, the auto-merge path overwrites admin edits whenever a scraper resubmits data with equal or higher trust. This erodes curator effort and makes the system untrustworthy for human operators.

Curator protection makes the system respect human intent: curated events route source updates to review instead of auto-merging, tombstoned events stay dead, and identical re-submissions are short-circuited cheaply.

## Current State

| Capability | Status | Code Location |
|---|---|---|
| Source external ID dedup (Layer 1) | ✅ Works | `create_event_core.go:139-173` |
| Dedup hash auto-merge (Layer 2) | ✅ Works | `create_event_core.go:175-234` |
| Trust-level merge (higher trust wins) | ✅ Works | `merge.go:15-78` |
| Payload hash on event_sources | ✅ Indexed | `000002_provenance.up.sql:55,64` |
| Tombstones created on consolidate/delete | ✅ Created | `admin_service.go:1906+` |
| Tombstones consulted during ingest | ❌ Not checked | — |
| "Human edited" signal on events | ❌ Missing | — |
| FindByDedupHash returns deleted events | ✅ No lifecycle_state filter | `events_repository.go:897-903` |
| FindBySourceExternalID returns deleted events | ✅ No lifecycle_state filter | `events_repository.go:830-837` |
| event_changes.user_id populated | ❌ Never populated | `000013_event_changes_triggers.up.sql` |
| ErrPreviouslyRejected | ✅ Exists | `errors.go:8-19` |
| Soft-delete preserves event row | ✅ `lifecycle_state = 'deleted'` | — |

### How Auto-Merge Works Today

```
Scraped event arrives →
  1. source_event_id match →
     a. EXISTS → auto-merge fields (description, image_url, public_url, keywords)
        using trust comparison (higher trust wins)
     b. NOT FOUND → continue to Step 2
  2. dedup_hash match →
     a. EXISTS → auto-merge fields using trust comparison
     b. NOT FOUND → continue to Step 3
  3. Near-duplicate (pg_trgm) → route to review queue
  4. Quality warnings → route to review queue
  5. Clean → publish
```

**Problem**: Steps 1a and 2a have no check for "was this event human-curated?" and no check for "was this event soft-deleted/tombstoned?" A human can fix an event name, and the next scraper run will overwrite it. A human can delete a bad event, and the next scrape will resurrect it.

### Key Architectural Observations

1. **Both `FindBySourceExternalID` and `FindByDedupHash` return soft-deleted events** — no lifecycle_state filter. This means tombstone checking is a sub-check within the existing match layers, not a separate pre-step.

2. **`payload_hash` is already indexed** on `event_sources` (`idx_event_sources_hash`). Checking "have we seen this exact payload before?" is a single index lookup — the cheapest possible short-circuit.

3. **The review queue already handles `potential_duplicate`** with admin diff/merge UI. A curated event receiving source updates is functionally identical to a potential duplicate — admin sees two versions, picks which data to keep. No new entry type needed.

4. **Auto-merge logic is identical in both layers** — same `AutoMergeFields` call, same trust comparison. The curator guard and tombstone check can be a shared function called from both layers.

## Architecture

### Design Principles

1. **Cheapest check first** — payload hash short-circuit before any DB joins
2. **Reuse existing review queue** — curated updates are potential duplicates, not a new concept
3. **Single guard function** — both dedup layers call the same curator/tombstone check
4. **Event-level curation flag** — `last_curated_at` timestamp, not per-field tracking
5. **No new entry types** — `potential_duplicate` covers curated update routing

### Data Flow — New Ingest Path

```
Scraped event arrives →
  0. ALREADY-SEEN CHECK (payload_hash on event_sources)
     → exact match: skip, done (identical payload previously ingested)
     → no match: continue (new or changed payload)

  1. SOURCE_EVENT_ID MATCH (existing Layer 1)
     a. EXISTS, lifecycle_state = 'deleted' →
        check tombstone:
          → admin-deleted (superseded_by IS NULL): hard reject (ErrPreviouslyDeleted)
          → consolidated (superseded_by IS NOT NULL): follow redirect to canonical
            event, re-run match logic against canonical
     b. EXISTS, not deleted, last_curated_at IS NOT NULL →
        route to review queue as potential_duplicate (admin sees diff)
     c. EXISTS, not deleted, last_curated_at IS NULL →
        auto-merge as today (trust comparison)
     d. NOT FOUND → continue to Step 2

  2. DEDUP_HASH MATCH (existing Layer 2)
     a. EXISTS, lifecycle_state = 'deleted' → same tombstone logic as 1a
     b. EXISTS, not deleted, last_curated_at IS NOT NULL →
        route to review queue as potential_duplicate
     c. EXISTS, not deleted, last_curated_at IS NULL →
        auto-merge as today
     d. NOT FOUND → continue to Step 3

  3. Near-duplicate (pg_trgm) → route to review queue (existing behavior)
  4. Quality warnings → route to review queue (existing behavior)
  5. Clean → publish (existing behavior)
```

### Shared Guard Function

Both Layer 1 and Layer 2 perform the same post-match logic. This is extracted into a single function to keep things DRY:

```go
// handleExistingMatch decides what to do when an ingest match is found.
// Returns:
//   - (*CreateEventCoreResult, nil) if the match was handled (merged, skipped, or rejected)
//   - (nil, nil) if the match should be treated as a review candidate (caller creates review entry)
//   - (nil, error) on failure
func (s *IngestService) handleExistingMatch(
    ctx context.Context,
    existing *Event,
    validated EventInput,
    sourceID string,
    warnings []ValidationWarning,
) (*CreateEventCoreResult, error)
```

Decision tree inside `handleExistingMatch`:

1. `existing.LifecycleState == "deleted"` → look up tombstone
   - Admin-deleted → return `ErrPreviouslyDeleted`
   - Consolidated → follow `superseded_by_uri` to canonical, recurse
2. `existing.LastCuratedAt != nil` → return `(nil, nil)` signaling "route to review"
3. Otherwise → run `AutoMergeFields` as today

### Component Diagram

```
                          ┌──────────────────────┐
  Scraped event ─────────►│  IngestService       │
                          │  (create_event_core)  │
                          └──────┬───────────────┘
                                 │
                    ┌────────────┤
                    ▼            ▼
              payload_hash    Layer 1 or 2 match
              exists?         (source_event_id / dedup_hash)
                │                    │
                ▼                    ▼
              SKIP           handleExistingMatch()
              (already seen)   │         │          │
                           deleted?   curated?   uncurated?
                               │         │          │
                               ▼         ▼          ▼
                          tombstone   route to    auto-merge
                          logic       review      (existing)
                          (reject/    queue
                           redirect)
```

## Design Constraints

1. **No auto-update for curated events** — source updates always route to review
2. **Event-level curation flag** — `last_curated_at` timestamp, not per-field. Accepts that any admin edit protects the entire event. Simple to evolve to per-field later if needed.
3. **Payload hash is the cheapest gate** — index lookup, no joins, short-circuits before any complex logic
4. **Reuse `potential_duplicate` review type** — no new entry type, no new admin UI needed for Phase 1
5. **`last_curated_at` set only by meaningful curation** — not by routine workflow actions (see table below)
6. **No backfill required** — system is pre-launch; existing events don't need curation timestamps
7. **Guard function shared across layers** — identical logic in one place, called from both Layer 1 and Layer 2

### What Sets `last_curated_at`

| Admin Action | Sets `last_curated_at`? | Reasoning |
|---|---|---|
| `UpdateEvent` (edit fields) | **Yes** | Core case — admin curated content |
| `Consolidate` (on canonical) | **Yes** | Admin enriched the canonical event |
| `FixAndApproveEventWithReview` | **Yes** | Admin corrected data during approval |
| `ApproveEventWithReview` (no edits) | **No** | Admin unblocked, didn't curate |
| `AddOccurrenceFromReview` | **No** | Schedule data, not content curation |
| `CreateOccurrenceOnEvent` | **No** | Schedule data, not content curation |
| `PublishEvent` / `UnpublishEvent` | **No** | Workflow state, not content |
| `RejectEventWithReview` | **No** | Event gets tombstoned, not curated |
| `DeleteEvent` | **No** | Event gets tombstoned, not curated |
| `MergeEventsWithReview` | **Yes** (on surviving event) | Admin chose which data to keep |
| `FixEventOccurrenceDates` | **No** | Schedule correction, not content curation |

**Design for extensibility**: If we later need per-field tracking, we add a `curated_fields TEXT[]` column alongside `last_curated_at`. The guard function checks `curated_fields` if non-null, falls back to `last_curated_at` if null. The Phase 1 migration doesn't preclude this.

## Implementation Phases

### Phase 1: Curator Protection Core (this spec)

Delivers payload-hash short-circuit, `last_curated_at` guard, tombstone blocking, and curated-event routing to review.

**Tasks:**
1. Migration: add `last_curated_at` to events table
2. Domain model: add `LastCuratedAt` to Event struct, new error types
3. Payload hash "already-seen" check (new Step 0 in ingest)
4. `handleExistingMatch` guard function (tombstone + curation check)
5. Wire guard into Layer 1 and Layer 2
6. Set `last_curated_at` on qualifying admin actions
7. TDD tests (table-driven: already-seen, curated, tombstoned, uncurated)

### Phase 2: Review Queue Diff View (future)

Delivers admin-facing UI/API for viewing field-level diffs when a curated event receives an update via review queue.

- Diff computation endpoint
- Structured field-by-field comparison
- Accept/reject individual field changes

### Phase 3: Per-Field Curation Tracking (future, deferred)

If source updates become frequent enough that whole-event protection is too coarse:

- `curated_fields TEXT[]` column
- Guard function checks per-field
- Auto-merge non-curated fields, route curated fields to review

### Deferred (not in any phase yet)

- **Permanent rejection flag** on review queue (solves spam sources, separate concern)
- **`event_changes.user_id` tracking** (useful but orthogonal to curator protection)
- **Batch payload hash check** (optimization if ingest throughput becomes a concern)

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Review queue volume from curated events | Low | Low | Source updates on curated events should be rare; identical payloads are short-circuited by hash |
| `last_curated_at` not set on all relevant admin paths | Low | High — curated events could be overwritten | Audit all admin write paths; table above is exhaustive; integration tests verify |
| Consolidated tombstone redirect loop | Very Low | Medium | Cap redirect depth at 1 (consolidated into consolidated is an error) |
| `FindByDedupHash` returns wrong deleted event | Low | Low | LIMIT 1 is existing behavior; dedup hash collisions are astronomically unlikely |

## Security

### Trust Boundaries

- **Scraped data** (untrusted) → enters via ingest pipeline, subject to all protection checks
- **Admin actions** (trusted) → set `last_curated_at`, bypass protection
- **Review queue** (semi-trusted) → admin-reviewed, may set `last_curated_at` on fix/merge

### Threat Model

| Threat | Defense |
|---|---|
| Source resubmits stale data to overwrite curation | Payload hash short-circuit (identical = skip); curation guard (different = review) |
| Source resubmits previously deleted event | Tombstone check in Layer 1/2 → hard reject |
| Tombstone redirect used to pollute canonical event | Redirect routes to review, not auto-merge; admin decides |

## Open Questions

1. **Consolidated tombstone redirect depth**: Should we follow one level (consolidated → canonical) or support chains? Recommendation: one level, error if canonical is also tombstoned.
2. **Clear `last_curated_at`**: Should admins be able to un-protect an event? Not in Phase 1 scope, but `SET last_curated_at = NULL` is the mechanism if needed.
3. **Payload hash across sources**: The already-seen check matches any `event_sources` row regardless of source. If Source A and Source B submit identical payloads, the second is skipped. This seems correct (same data is same data) but worth noting.
