# Unified Duplicate Detection & Review Workflow

**Bead:** srv-mka  
**Status:** Design  
**Date:** 2026-02-11

## Problem

Duplicate detection and the review queue are two separate systems that should be unified:

1. **Dedup hash is broken**: The `events.dedup_hash` column is `GENERATED ALWAYS` (MD5 of `name|venue_id|series_id`), but the application computes SHA256 of `name|venue_name|start_date`. `FindByDedupHash()` searches for the app hash, but the DB overwrites it with its own MD5. They never match. Cross-source duplicates slip through.

2. **Duplicates are silently swallowed**: When the dedup hash matches (if it worked), the duplicate is dropped with no record. No data merging occurs, so better data from a second source is lost.

3. **No near-duplicate detection**: Events with slightly different names for the same real-world event (different scrapers, different formatting) produce different hashes and are treated as separate events.

4. **No place/org dedup**: Places and organizations use name-based upsert but have no fuzzy matching. "The Rex Jazz Bar" and "Rex Hotel Jazz & Blues Bar" create separate place records.

5. **Duplicates admin page is a stub**: The `ListDuplicates` API returns `[]`. The frontend exists but shows nothing.

## Design Principles

- **Err on the side of review** — flag uncertain cases for human/LLM review rather than auto-deciding
- **Minimize human labor** — auto-merge when confidence is high; nodes are volunteer-run
- **Trust-based data merging** — sources have trust levels (1-10); higher-trust data wins, but any source can fill gaps
- **Configurable thresholds** — similarity thresholds are config values, not hardcoded
- **LLM-ready API** — review queue API should be consumable by an automated LLM reviewer

## Architecture

### Ingestion Flow (revised)

```
Event arrives →
  1. Source external ID check → update existing (not a duplicate)
  2. Exact dedup hash match →
     a. Auto-merge: fill gaps, overwrite if new source has higher trust
     b. Record provenance (new event_source)
     c. Return IsMerged: true (no review needed)
  3. Near-duplicate check (same venue + overlapping date + fuzzy name) →
     a. Add "potential_duplicate" warning with candidate event ULID(s)
     b. Route to review queue
  4. Place/Org similarity check →
     a. If fuzzy match found, add "place_possible_duplicate" or "org_possible_duplicate" warning
     b. Route to review queue (for the event)
  5. Quality checks → accumulate warnings
  6. If ANY warnings → review queue
  7. If clean → publish
```

### Auto-Merge Strategy (exact dedup hash match)

When two events have identical dedup hashes (same normalized name, same venue, same start date) from different sources:

```
For each field on the existing event:
  1. Existing has value AND new source trust <= existing trust → keep existing
  2. Existing is empty → fill from new source (regardless of trust)
  3. Both have value AND new source trust > existing trust → overwrite with new
```

Fields subject to merge: `description`, `image_url`, `public_url`, `keywords`, `in_language`, `is_accessible_for_free`, `event_status`, `attendance_mode`.

Occurrences are NOT duplicated — the existing occurrence set is kept.

The new source is recorded via a new `event_source` provenance entry linking the source to the existing event.

### Near-Duplicate Detection

After the exact hash check fails, query for potential duplicates:

```sql
SELECT e.ulid, e.name, e.dedup_hash
FROM events e
JOIN event_occurrences o ON o.event_id = e.id
JOIN places p ON p.id = e.primary_venue_id
WHERE e.lifecycle_state NOT IN ('deleted')
  AND e.primary_venue_id = $venue_id       -- same venue
  AND o.start_time::date = $start_date     -- same date
  AND similarity(e.name, $name) > $threshold  -- fuzzy name match
ORDER BY similarity(e.name, $name) DESC
LIMIT 5;
```

This uses `pg_trgm`'s `similarity()` function. The threshold is configurable (default: 0.4).

When candidates are found, the event enters the review queue with a `potential_duplicate` warning containing:
- `duplicate_of_ulid` — the candidate event's ULID
- `similarity_score` — the trigram similarity score
- `candidate_name` — the candidate event's name (for display)

### Place & Organization Dedup

During ingestion, before upserting a place/org, check for fuzzy matches:

```sql
SELECT id, name, similarity(name, $1) AS sim
FROM places
WHERE similarity(name, $1) > $threshold
  AND name != $1
  AND deleted_at IS NULL
ORDER BY sim DESC
LIMIT 3;
```

**Behavior by similarity score (configurable):**
- `>= auto_merge_threshold` (default 0.95): Auto-merge — use the existing place/org, fill gaps
- `>= review_threshold` (default 0.6): Flag event for review with `place_possible_duplicate` warning
- `< review_threshold`: No match — create new place/org as usual

Place/Org merge:
- `MergePlaces(duplicate_id, primary_id)`: Reassign all events referencing the duplicate to the primary. Fill gaps on primary from duplicate fields. Soft-delete duplicate.
- `MergeOrganizations(duplicate_id, primary_id)`: Same pattern.

### Review Queue Changes

**New review warning codes:**
- `potential_duplicate` — event may be a duplicate of another event
- `place_possible_duplicate` — event's venue may be a duplicate of an existing place
- `org_possible_duplicate` — event's organizer may be a duplicate of an existing org

**New review action: Merge**
- `POST /api/v1/admin/review-queue/{id}/merge` 
- Body: `{ "merge_into": "<target_event_ulid>" }`
- Merges the review event's data into the target using trust-based strategy
- Soft-deletes the review event, creates tombstone
- Marks review entry as resolved with status `merged`

**New review action: Keep Separate**
- `POST /api/v1/admin/review-queue/{id}/approve` (existing, reused)
- Approves the event as non-duplicate
- Future: could record a `not_duplicate_of` entry to prevent re-flagging (deferred)

**New review queue field:**
- `duplicate_of_event_id UUID` — references the candidate event for duplicate reviews
- Nullable; only set when the review reason includes `potential_duplicate`

**Duplicates admin page:**
- Redirect `/admin/duplicates` to `/admin/review-queue?warning=potential_duplicate`
- Or: make the duplicates page a filtered view of the review queue
- Remove the stub `ListDuplicates` API (or make it query the review queue with duplicate filter)

### Review Queue API (LLM-ready)

The existing API is already well-structured for programmatic use. Ensure:

1. `GET /api/v1/admin/review-queue/{id}` response includes:
   - Both original and normalized payloads
   - All warnings with details (including duplicate candidate info)
   - The candidate event's full data (when `potential_duplicate`)
   
2. All actions return the updated review entry state

3. Document the API contract so an LLM agent can call:
   - List pending reviews → pick one → read details → decide → call approve/reject/merge

## Configuration

New fields in `ValidationConfig`:

```go
type DedupConfig struct {
    // NearDuplicateThreshold is the pg_trgm similarity threshold for
    // flagging potential duplicate events. Range: 0.0-1.0.
    // Lower values catch more duplicates but increase false positives.
    // Default: 0.4
    NearDuplicateThreshold float64

    // PlaceReviewThreshold is the similarity threshold for flagging
    // a potential place duplicate for review. Default: 0.6
    PlaceReviewThreshold float64

    // PlaceAutoMergeThreshold is the similarity threshold above which
    // places are auto-merged without review. Default: 0.95
    PlaceAutoMergeThreshold float64

    // OrgReviewThreshold is the similarity threshold for flagging
    // a potential organization duplicate for review. Default: 0.6
    OrgReviewThreshold float64

    // OrgAutoMergeThreshold is the similarity threshold above which
    // organizations are auto-merged without review. Default: 0.95
    OrgAutoMergeThreshold float64
}
```

Environment variables:
- `DEDUP_NEAR_DUPLICATE_THRESHOLD` (default: 0.4)
- `DEDUP_PLACE_REVIEW_THRESHOLD` (default: 0.6)
- `DEDUP_PLACE_AUTO_MERGE_THRESHOLD` (default: 0.95)
- `DEDUP_ORG_REVIEW_THRESHOLD` (default: 0.6)
- `DEDUP_ORG_AUTO_MERGE_THRESHOLD` (default: 0.95)

## Database Changes

### Migration: Fix dedup hash

```sql
-- Drop the generated expression, make it a regular stored column
ALTER TABLE events ALTER COLUMN dedup_hash DROP EXPRESSION;
-- The existing idx_events_dedup index remains valid
```

### Migration: Add duplicate tracking to review queue

```sql
ALTER TABLE event_review_queue 
  ADD COLUMN duplicate_of_event_id UUID REFERENCES events(id);

-- Add 'merged' to allowed review statuses
-- (Currently: pending, approved, rejected, superseded)
```

### Migration: pg_trgm indexes for fuzzy matching

```sql
-- Extension already enabled in migration 000001
-- Add GIN indexes for trigram similarity
CREATE INDEX idx_places_name_trgm ON places USING gin (name gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX idx_organizations_name_trgm ON organizations USING gin (name gin_trgm_ops)
  WHERE deleted_at IS NULL;

-- Add GIN index on events for near-duplicate name matching
CREATE INDEX idx_events_name_trgm ON events USING gin (name gin_trgm_ops)
  WHERE lifecycle_state NOT IN ('deleted');
```

### Migration: Place and Org merge support

```sql
-- Track merged places
ALTER TABLE places ADD COLUMN merged_into_id UUID REFERENCES places(id);

-- Track merged organizations  
ALTER TABLE organizations ADD COLUMN merged_into_id UUID REFERENCES organizations(id);
```

## Implementation Layers

### Layer 0: Database + Dedup Hash Fix
- Migration: drop dedup_hash generated expression
- Migration: add `duplicate_of_event_id` to review queue, `merged_into_id` to places/orgs
- Migration: add pg_trgm GIN indexes
- No code changes needed beyond migrations

### Layer 1: Auto-Merge for Exact Duplicates
- New `AutoMergeEvent()` function in `internal/domain/events/merge.go`
- Modify `ingest.go`: replace silent drop with auto-merge call
- Add `IsMerged` field to `IngestResult`
- Add source trust level lookup during merge
- Tests for merge field selection logic

### Layer 2: Near-Duplicate Detection
- New SQL query: find events at same venue + date with fuzzy name match
- New repository method: `FindNearDuplicates(ctx, venueID, startDate, name, threshold)`
- Modify `ingest.go`: add near-duplicate check after exact hash check
- Add `potential_duplicate` warning type
- Tests for near-duplicate detection and review routing

### Layer 3: Place/Org Dedup
- New SQL queries: fuzzy place/org name search
- New repository methods: `FindSimilarPlaces()`, `FindSimilarOrganizations()`
- New merge methods: `MergePlaces()`, `MergeOrganizations()`
- Modify ingestion pipeline: check place/org similarity before upsert
- Add warning types: `place_possible_duplicate`, `org_possible_duplicate`
- Config: `DedupConfig` struct with thresholds

### Layer 4: Review Queue Merge Action + UI
- New handler: `MergeReviewEvent()` 
- New route: `POST /api/v1/admin/review-queue/{id}/merge`
- Review queue UI: show duplicate comparison when `potential_duplicate` warning present
- Duplicates page: redirect to filtered review queue
- Remove `ListDuplicates` stub

## Files Affected

### New files
- `internal/domain/events/merge.go` — auto-merge logic
- `internal/domain/events/merge_test.go` — merge tests
- Migration files (3-4 new migrations)

### Modified files
- `internal/config/config.go` — add `DedupConfig`
- `internal/domain/events/ingest.go` — revised duplicate handling flow
- `internal/domain/events/repository.go` — new interfaces for near-duplicate + merge
- `internal/storage/postgres/events_repository.go` — implement new queries
- `internal/storage/postgres/places_repository.go` — similarity search + merge
- `internal/storage/postgres/organizations_repository.go` — similarity search + merge
- `internal/storage/postgres/queries/events.sql` — near-duplicate query
- `internal/storage/postgres/queries/places.sql` — similarity query
- `internal/storage/postgres/queries/organizations.sql` — similarity query
- `internal/api/handlers/admin_review_queue.go` — merge action handler
- `internal/api/handlers/admin.go` — remove/redirect ListDuplicates stub
- `internal/api/router.go` — add merge route
- `web/admin/static/js/review-queue.js` — duplicate comparison UI in review
- `web/admin/templates/review_queue.html` — duplicate comparison template section
- `web/admin/static/js/duplicates.js` — redirect or reuse
- `web/admin/templates/duplicates.html` — redirect or remove
