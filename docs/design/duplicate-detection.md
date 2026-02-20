# Unified Duplicate Detection & Review Workflow

## Design Principles

- **Err on the side of review** — flag uncertain cases for human/LLM review rather than auto-deciding
- **Minimize human labor** — auto-merge when confidence is high; nodes are volunteer-run
- **Trust-based data merging** — sources have trust levels (1-10); higher-trust data wins, but any source can fill gaps
- **Configurable thresholds** — similarity thresholds are config values, not hardcoded
- **LLM-ready API** — review queue API is consumable by an automated LLM reviewer

## Ingestion Flow

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

## Auto-Merge Strategy (exact dedup hash match)

When two events have identical dedup hashes (same normalized name, same venue, same start date) from different sources:

```
For each field on the existing event:
  1. Existing has value AND new source trust <= existing trust → keep existing
  2. Existing is empty → fill from new source (regardless of trust)
  3. Both have value AND new source trust > existing trust → overwrite with new
```

Fields subject to merge: `description`, `image_url`, `public_url`, `keywords`, `in_language`, `is_accessible_for_free`, `event_status`, `attendance_mode`.

Occurrences are not duplicated — the existing occurrence set is kept. The new source is recorded via a new `event_source` provenance entry linking the source to the existing event.

## Near-Duplicate Detection

After the exact hash check fails, query for potential duplicates:

```sql
SELECT e.ulid, e.name, e.dedup_hash
FROM events e
JOIN event_occurrences o ON o.event_id = e.id
WHERE e.lifecycle_state NOT IN ('deleted')
  AND e.primary_venue_id = $venue_id
  AND o.start_time::date = $start_date
  AND similarity(e.name, $name) > $threshold
ORDER BY similarity(e.name, $name) DESC
LIMIT 5;
```

Uses `pg_trgm`'s `similarity()` function. Threshold is configurable (default: 0.4).

When candidates are found, the event enters the review queue with a `potential_duplicate` warning containing:
- `duplicate_of_ulid` — the candidate event's ULID
- `similarity_score` — the trigram similarity score
- `candidate_name` — the candidate event's name (for display)

## Place & Organization Dedup

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
- `>= review_threshold` (default 0.6): Flag event for review with warning
- `< review_threshold`: No match — create new place/org

**Merge operations:**
- `MergePlaces(duplicate_id, primary_id)`: Reassign all events referencing the duplicate to the primary. Fill gaps on primary from duplicate fields. Soft-delete duplicate.
- `MergeOrganizations(duplicate_id, primary_id)`: Same pattern.

Both operations use `SELECT ... FOR UPDATE SKIP LOCKED` to handle concurrent ingestion races. `MergeEvents` includes transitive chain resolution via a recursive CTE (max depth 10) to flatten merge chains.

## Review Queue Integration

### New warning codes

- `potential_duplicate` — event may be a duplicate of another event
- `place_possible_duplicate` — event's venue may be a duplicate of an existing place
- `org_possible_duplicate` — event's organizer may be a duplicate of an existing org

### Merge action

```
POST /api/v1/admin/review-queue/{id}/merge
Body: {"merge_into": "<target_event_ulid>"}
```

Merges the review event's data into the target using the trust-based merge strategy. Soft-deletes the review event, creates tombstone. Marks review entry as resolved with status `merged`. The entire operation (field merge, tombstone creation, review status update) is wrapped in a single database transaction.

### Keep Separate action

`POST /api/v1/admin/review-queue/{id}/approve` — approves the event as non-duplicate and records the pair in `event_not_duplicates` to prevent re-flagging on future ingestion.

### New review queue fields

- `duplicate_of_event_id UUID` — references the candidate event for duplicate reviews (nullable)
- `merged` status — added to allowed review statuses

### Duplicates admin page

`/admin/duplicates` redirects to `/admin/review-queue?warning=potential_duplicate`.

### Review Queue API (LLM-ready)

`GET /api/v1/admin/review-queue/{id}` includes both original and normalized payloads, all warnings with details (including duplicate candidate info), and the candidate event's full data when `potential_duplicate` is present. An LLM agent can: list pending reviews → pick one → read details → call approve/reject/merge.

## Configuration

```go
type DedupConfig struct {
    // Trigram similarity threshold for flagging potential duplicate events.
    // Range: 0.0-1.0. Default: 0.4
    NearDuplicateThreshold float64

    // Similarity threshold for flagging potential place duplicate for review. Default: 0.6
    PlaceReviewThreshold float64

    // Similarity threshold above which places are auto-merged. Default: 0.95
    PlaceAutoMergeThreshold float64

    // Similarity threshold for flagging potential org duplicate for review. Default: 0.6
    OrgReviewThreshold float64

    // Similarity threshold above which orgs are auto-merged. Default: 0.95
    OrgAutoMergeThreshold float64
}
```

Environment variables:
- `DEDUP_NEAR_DUPLICATE_THRESHOLD` (default: 0.4)
- `DEDUP_PLACE_REVIEW_THRESHOLD` (default: 0.6)
- `DEDUP_PLACE_AUTO_MERGE_THRESHOLD` (default: 0.95)
- `DEDUP_ORG_REVIEW_THRESHOLD` (default: 0.6)
- `DEDUP_ORG_AUTO_MERGE_THRESHOLD` (default: 0.95)

## Database Schema

### Dedup hash

`events.dedup_hash` is a regular stored column (not a `GENERATED ALWAYS` expression). The application computes SHA256 of the normalized `name|venue_key|start_date`. `NormalizeVenueKey()` in `dedup.go` canonicalizes venue keys (uses place ID if available, otherwise normalizes name: lowercase, trim, collapse whitespace).

### Review queue additions

```sql
ALTER TABLE event_review_queue
  ADD COLUMN duplicate_of_event_id UUID REFERENCES events(id);
```

### pg_trgm indexes

```sql
CREATE INDEX idx_places_name_trgm ON places USING gin (name gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX idx_organizations_name_trgm ON organizations USING gin (name gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX idx_events_name_trgm ON events USING gin (name gin_trgm_ops)
  WHERE lifecycle_state NOT IN ('deleted');
```

### Merge tracking

```sql
ALTER TABLE places ADD COLUMN merged_into_id UUID REFERENCES places(id);
ALTER TABLE organizations ADD COLUMN merged_into_id UUID REFERENCES organizations(id);
```

### Not-duplicate tracking

```sql
-- event_not_duplicates (migration 000027)
-- Composite PK (event_id_a, event_id_b) with canonical ULID ordering
-- Prevents re-flagging known non-duplicate pairs on future ingestion
```
