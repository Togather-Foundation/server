# Duplicate Detection & Review Workflow: Comprehensive Scenarios

**Bead:** srv-gepsf  
**Status:** Complete  
**Date:** 2026-02-11  
**Purpose:** Specification for comprehensive test suite covering all branches of the duplicate detection and review system.

## Table of Contents

1. [Ingestion Flow Overview](#1-ingestion-flow-overview)
2. [Idempotency Key Scenarios](#2-idempotency-key-scenarios)
3. [Source External ID Scenarios](#3-source-external-id-scenarios)
4. [Exact Dedup Hash Scenarios](#4-exact-dedup-hash-scenarios)
5. [Review Queue Resubmission Scenarios](#5-review-queue-resubmission-scenarios)
6. [Near-Duplicate Detection Scenarios](#6-near-duplicate-detection-scenarios)
7. [Place Fuzzy Dedup Scenarios](#7-place-fuzzy-dedup-scenarios)
8. [Organization Fuzzy Dedup Scenarios](#8-organization-fuzzy-dedup-scenarios)
9. [Review Queue Admin Actions](#9-review-queue-admin-actions)
10. [Trust-Based Field Merge Scenarios](#10-trust-based-field-merge-scenarios)
11. [Chain Merge & Transitive Resolution](#11-chain-merge--transitive-resolution)
12. [Warning Code Combinations](#12-warning-code-combinations)
13. [Concurrent Submission Scenarios](#13-concurrent-submission-scenarios)
14. [Config Threshold Edge Cases](#14-config-threshold-edge-cases)
15. [Source Provenance Through Merges](#15-source-provenance-through-merges)
16. [Resolved Gaps](#16-resolved-gaps)

---

## 1. Ingestion Flow Overview

The `IngestWithIdempotency` function (`internal/domain/events/ingest.go`) processes events through these stages in exact order:

```
1. Idempotency key check
2. Normalize → Validate → Quality warnings
3. Source lookup (GetOrCreateSource, default trust 5)
4. Source external ID check (FindBySourceExternalID)
5. Exact dedup hash check (FindByDedupHash) → auto-merge if found
6. Review queue dedup check (FindReviewByDedup) — only if needsReview
7. Event creation (in transaction):
   a. Place upsert → place fuzzy dedup (Layer 3)
   b. Near-duplicate detection (Layer 2)
   c. Org upsert → org fuzzy dedup (Layer 3)
   d. Create event + occurrences + source + review queue entry
8. Commit transaction
```

**Key ordering invariants:**
- Source external ID check now calls `AutoMergeFields` with actual trust levels from both sources, persisting field updates via `UpdateEvent`. It also records the source contribution. Returns `IsMerged: true` if any fields changed.
- Dedup hash auto-merge (step 5) runs BEFORE review queue check (step 6) — an exact match never enters the review queue.
- Place/org fuzzy dedup (step 7a, 7c) runs AFTER event creation begins — the place is created first, then checked for similarity. Auto-merge gap-fills primary place/org fields from duplicate before reassigning events.
- Near-duplicate check (step 7b) runs AFTER place reconciliation — needs the venue ID. Known not-duplicate pairs are filtered out before creating warnings.
- Review queue dedup check (step 6) only runs if `needsReview` is true (warnings exist or quality checks fail).

---

## 2. Idempotency Key Scenarios

**Code path:** `ingest.go:59-94`

### S2.1: First submission with idempotency key

- **Given:** Key "abc123" has never been used.
- **Action:** Event submitted with `idempotencyKey="abc123"`.
- **Expected:** Key is inserted with empty EventID. Event proceeds through normal flow. After creation, key is updated with the event's ID and ULID.
- **Result:** `IngestResult{Event: <new>, IsDuplicate: false}`

### S2.2: Replay with matching payload hash

- **Given:** Key "abc123" already exists with EventULID="01J..." and RequestHash="sha256_A".
- **Action:** Same event resubmitted with same `idempotencyKey="abc123"`. Payload hash matches.
- **Expected:** Returns the existing event immediately. No merge, no re-processing.
- **Result:** `IngestResult{Event: <existing>, IsDuplicate: true}`

### S2.3: Replay with different payload hash (conflict)

- **Given:** Key "abc123" already exists with EventULID="01J..." and RequestHash="sha256_A".
- **Action:** Different event submitted with same `idempotencyKey="abc123"`. Payload hash differs.
- **Expected:** Returns `ErrConflict`. No new event created.

### S2.4: Key exists but EventULID is nil (in-flight)

- **Given:** Key "abc123" was inserted but event creation hasn't completed (EventULID is nil/empty).
- **Action:** Same key submitted again.
- **Expected:** Returns `ErrConflict`. Prevents double-creation race.

### S2.5: Empty idempotency key

- **Given:** Any state.
- **Action:** Event submitted with empty `idempotencyKey=""`.
- **Expected:** Idempotency check is skipped entirely. Proceeds to normalize/validate.

---

## 3. Source External ID Scenarios

**Code path:** `ingest.go:125-145`

### S3.1: Same source, same external ID — duplicate detected with field merge

- **Given:** Source "scraper-A" previously submitted event with externalID "evt-001", creating event E1.
- **Action:** Source "scraper-A" submits again with externalID "evt-001".
- **Expected:** `FindBySourceExternalID` returns E1. `AutoMergeFields` is called using actual trust levels from both the existing event's source and the submitting source (looked up via `GetSourceTrustLevel` and `GetSourceTrustLevelBySourceID`). If fields changed, `UpdateEvent` persists the updates. Source contribution is recorded via `recordSourceForExisting`.
- **Result:** `IngestResult{Event: E1_updated, IsDuplicate: true, IsMerged: <true if fields changed>, Warnings: <current>}`
- **Key detail:** This IS an auto-merge with real trust levels. Since both submissions share the same source, trust levels typically match, so the merge primarily gap-fills empty fields. Higher-trust resubmissions can also overwrite existing data.

### S3.2: Same source, different external ID — no match

- **Given:** Source "scraper-A" previously submitted event with externalID "evt-001".
- **Action:** Source "scraper-A" submits with externalID "evt-002".
- **Expected:** `FindBySourceExternalID` returns ErrNotFound. Proceeds to dedup hash check (step 5).

### S3.3: Different source, same external ID — no match

- **Given:** Source "scraper-A" submitted event with externalID "evt-001".
- **Action:** Source "scraper-B" submits with externalID "evt-001".
- **Expected:** `FindBySourceExternalID` searches by (sourceID="scraper-B", externalID="evt-001"), finds nothing. Proceeds to dedup hash check.
- **Key detail:** Source external IDs are scoped per-source.

### S3.4: No source provided — skip check

- **Given:** Any state.
- **Action:** Event submitted without `Source` field.
- **Expected:** Source lookup and external ID check are both skipped. `sourceID` remains empty.

### S3.5: Source provided but no external ID — skip check

- **Given:** Source "scraper-A" exists but event has no `Source.EventID`.
- **Action:** Event submitted with source URL but no event ID.
- **Expected:** `FindBySourceExternalID` called with empty externalID. Behavior depends on DB query (likely returns nothing). Source is still created via `GetOrCreateSource`.

---

## 4. Exact Dedup Hash Scenarios

**Code path:** `ingest.go:147-186`  
**Dedup hash:** SHA256 of `normalized_name|venue_key|start_date`

### S4.1: Exact hash match — auto-merge with field updates

- **Given:** Event E1 exists with hash "abc123". E1 has description "Old desc", no image, trust=5.
- **Action:** New event submitted producing same hash "abc123". Has description "New desc", image URL, trust=7.
- **Expected:**
  - `AutoMergeFields` called: Description overwritten (higher trust), ImageURL gap-filled.
  - `UpdateEvent` called with merged fields.
  - New source recorded via `recordSourceForExisting`.
  - Returns `IngestResult{Event: E1_updated, IsDuplicate: true, IsMerged: true}`.

### S4.2: Exact hash match — no field changes needed

- **Given:** Event E1 exists with full data, trust=8.
- **Action:** Same hash, less complete data, trust=3.
- **Expected:** `AutoMergeFields` returns `changed=false`. No `UpdateEvent` call. Source still recorded.
- **Result:** `IngestResult{Event: E1_unchanged, IsDuplicate: true, IsMerged: false}`
- **Key detail:** `IsMerged` is false even though hash matched, because no fields actually changed.

### S4.3: Hash match — gap fill regardless of trust

- **Given:** Event E1 has description but no image, trust=8.
- **Action:** Same hash, event has image URL, trust=2.
- **Expected:** ImageURL is filled (gap fill ignores trust level). Description unchanged (existing has value, new trust lower).
- **Result:** `IsMerged: true` (at least one field changed).

### S4.4: No dedup hash — skip check

- **Given:** Event has no name, no venue, or no start date.
- **Action:** `BuildDedupHash` returns "". 
- **Expected:** Hash check is skipped entirely. Event proceeds to review queue check or creation.

### S4.5: Hash match — trust lookup fails

- **Given:** Event E1 exists with hash match.
- **Action:** `GetSourceTrustLevel` returns error.
- **Expected:** Entire ingest fails with wrapped error. No partial update.

### S4.6: Dedup hash — venue key normalization

- **Given:** Event submitted with `Location.Name = "  The  Rex  "` (extra whitespace, no ID).
- **Expected:** `NormalizeVenueKey` normalizes the name: lowercase, trim leading/trailing whitespace, collapse internal whitespace runs. Hash = SHA256("event name|the rex|2026-03-01T...").
- **Given:** Same event submitted with `Location.Name = "the rex"`.
- **Expected:** Produces the same normalized venue key `"the rex"`. Hash matches.
- **Given:** Event submitted with `Location.ID = "  place-uuid-123  "`.
- **Expected:** `NormalizeVenueKey` returns the trimmed ID `"place-uuid-123"`. IDs are used as-is (not lowercased) since they are already canonical identifiers.
- **Key detail:** `NormalizeVenueKey` in `dedup.go` applies consistent canonicalization. When only a name is available, lowercasing and whitespace normalization prevent trivial formatting differences from producing different hashes. When an ID is available, it takes precedence. Note that a submission with a place ID and one with only a place name will still produce different hashes — this is inherent to having two different key types, but the normalization prevents false negatives within each key type. `BuildDedupHash` also collapses whitespace in the name field for additional safety.

---

## 5. Review Queue Resubmission Scenarios

**Code path:** `ingest.go:188-269`  
**FindReviewByDedup query:** Matches on `(source_id + source_external_id) OR dedup_hash`, only looks at `pending` or `rejected` statuses.

### S5.1: Pending review — resubmission with no warnings (auto-approve)

- **Given:** Event E1 in review queue as `pending`, had warning `missing_description`.
- **Action:** Same event resubmitted, now has description. No warnings.
- **Expected:**
  - `ApproveReview` called for the existing entry with "Auto-approved: resubmission with no warnings".
  - Event updated to `lifecycle_state = "published"`.
  - Returns `IngestResult{Event: E1_published, NeedsReview: false, Warnings: nil}`.
- **Key detail:** No new event is created. The existing event is published.

### S5.2: Pending review — resubmission still has warnings

- **Given:** Event E1 in review queue as `pending` with warnings `[missing_description, missing_image]`.
- **Action:** Same event resubmitted, still missing description but now has image. Warnings: `[missing_description]`.
- **Expected:**
  - `UpdateReviewQueueEntry` called with new payloads (original, normalized, warnings).
  - Returns the existing event with `NeedsReview: true, Warnings: [missing_description]`.
- **Key detail:** The review queue entry's payloads are UPDATED in place, not a new entry.

### S5.3: Rejected review — resubmission with same issues, event not past

- **Given:** Event E1 rejected with reason "Low quality", warnings `[missing_description]`. Event end time is in the future.
- **Action:** Same event resubmitted, still has `missing_description` warning.
- **Expected:** Returns `ErrPreviouslyRejected` with the rejection reason, timestamp, and reviewer.
- **Key detail:** `stillHasSameIssues` compares warning CODE sets, not messages. `{missing_description}` == `{missing_description}`.

### S5.4: Rejected review — resubmission with different issues, event not past

- **Given:** Event E1 rejected with warnings `[missing_description]`. Event end time in future.
- **Action:** Same event resubmitted, now has description but missing image. Warnings: `[missing_image]`.
- **Expected:** `stillHasSameIssues` returns false (code sets differ). Resubmission allowed — falls through to create a new event.
- **Key detail:** This creates a SECOND event for what may be the same real-world event.

### S5.5: Rejected review — resubmission after event has passed

- **Given:** Event E1 rejected, event end time is in the past.
- **Action:** Same event resubmitted with same issues.
- **Expected:** `isEventPast` returns true. Resubmission allowed — falls through to create a new event (since the original was for a past event, a new future event is acceptable).

### S5.6: Rejected review — event has no end time

- **Given:** Event E1 rejected, `EventEndTime` is nil.
- **Action:** Same event resubmitted.
- **Expected:** `isEventPast(nil)` returns false. Proceeds to same-issues check. If same issues, returns `ErrPreviouslyRejected`.

### S5.7: No existing review found — proceed to create

- **Given:** No matching review entry for this source+externalID or dedupHash.
- **Action:** Event needs review but no existing review.
- **Expected:** Falls through to event creation (step 7) with a new review queue entry.

### S5.8: Event doesn't need review — skip FindReviewByDedup entirely

- **Given:** Event has no warnings, passes all quality checks.
- **Action:** Clean event submitted.
- **Expected:** `needsReview` is false. `FindReviewByDedup` is never called. Proceeds directly to creation with `lifecycle_state = "published"`.

### S5.9: Approved/merged/superseded review exists — not found by query

- **Given:** Event was previously reviewed and approved (status = "approved").
- **Action:** Same event resubmitted with warnings.
- **Expected:** `FindReviewByDedup` only searches `pending` or `rejected` statuses. Returns ErrNotFound. Falls through to create new event.
- **Key detail:** Approved reviews are invisible to the resubmission check.

### S5.10: Review matched by dedup hash vs source+externalID

- **Given:** Review entry has dedup_hash="hash_A", source_id="src-1", source_external_id="ext-1".
- **Action A:** New submission with same source_id + source_external_id but different dedup_hash.
- **Expected A:** Matched by source+externalID. Review found.
- **Action B:** New submission from different source but same dedup_hash.
- **Expected B:** Matched by dedup_hash. Review found.
- **Key detail:** These are OR conditions. A review entry could be matched by either path, potentially returning different reviews for the same logical query.

---

## 6. Near-Duplicate Detection Scenarios

**Code path:** `ingest.go:398-431`  
**SQL:** `FindNearDuplicates` — same venue + same date + `similarity(name) > threshold`  
**Default threshold:** 0.4

### S6.1: Near-duplicate found — flagged for review (with not-duplicate filtering)

- **Given:** Event "Jazz at the Rex" exists at venue V1 on 2026-03-01. This pair has NOT been previously marked as not-duplicates.
- **Action:** New event "Rex Jazz Night" submitted for venue V1 on 2026-03-01. Name similarity = 0.55.
- **Expected:** `FindNearDuplicates` returns the candidate. Each candidate is checked against the `event_not_duplicates` table via `IsNotDuplicate` — known not-duplicate pairs are filtered out. Remaining candidates produce a warning with code `potential_duplicate`, including candidate ULID, name, and similarity score. `needsReview` set to true.

### S6.2: Same venue, same date, different name — no match

- **Given:** Event "Jazz at the Rex" at V1 on 2026-03-01.
- **Action:** Event "Poetry Slam" at V1 on 2026-03-01. Similarity = 0.1.
- **Expected:** Similarity below threshold. No near-duplicate warning.

### S6.3: Similar name, different venue — no match

- **Given:** Event "Jazz Night" at venue V1.
- **Action:** Event "Jazz Night" at venue V2.
- **Expected:** `FindNearDuplicates` queries by venue ID. Different venue = no match.

### S6.4: Similar name, same venue, different date — no match

- **Given:** Event "Jazz Night" at V1 on 2026-03-01.
- **Action:** Event "Jazz Night" at V1 on 2026-03-08.
- **Expected:** Query filters by `start_time::date`. Different date = no match.

### S6.5: No venue (virtual event) — skip check

- **Given:** Event is online-only, no `PrimaryVenueID`.
- **Action:** Event ingested.
- **Expected:** Check guarded by `params.PrimaryVenueID != nil`. Skipped for virtual events.

### S6.6: Start date parse failure — skip check

- **Given:** Event with malformed StartDate.
- **Action:** `time.Parse(time.RFC3339, ...)` returns error.
- **Expected:** Near-duplicate check skipped silently (the `if err == nil` guard). Event continues through ingestion.

### S6.7: DB error during check — log and continue

- **Given:** `FindNearDuplicates` returns a database error.
- **Expected:** Warning logged, but ingestion continues. Event is NOT blocked by a failed similarity check.

### S6.8: Multiple near-duplicate candidates

- **Given:** Three similar events at the same venue on the same date.
- **Action:** New event submitted.
- **Expected:** All candidates returned in `matches` array within the warning. Only one `potential_duplicate` warning, but with multiple matches in Details.

### S6.9: Threshold = 0 — check disabled

- **Given:** `DedupConfig.NearDuplicateThreshold = 0`.
- **Expected:** Guard `s.dedupConfig.NearDuplicateThreshold > 0` prevents the check.

### S6.10: Near-duplicate combined with exact dedup hash

- **Given:** Event with hash match.
- **Action:** Exact dedup hash matches an existing event.
- **Expected:** Auto-merge at step 5 returns early. Near-duplicate check at step 7b is never reached.

---

## 7. Place Fuzzy Dedup Scenarios

**Code path:** `ingest.go:330-396`  
**Default thresholds:** Review=0.6, AutoMerge=0.95

### S7.1: New place, high similarity — auto-merge with gap-fill

- **Given:** Place "Rex Hotel Jazz Bar" exists with no description or email.
- **Action:** Event submitted with venue "The Rex Jazz & Blues Bar" that includes description and email. Similarity = 0.96.
- **Expected:**
  - `UpsertPlace` creates a new place (ULID matches generated one).
  - `FindSimilarPlaces` returns the existing place with similarity >= 0.95.
  - `MergePlaces` called:
    - Gap-fills primary place fields from duplicate via SQL `UPDATE ... FROM` join (14 fields: description, street_address, address_locality, address_region, postal_code, address_country, latitude, longitude, telephone, email, url, maximum_attendee_capacity, venue_type, accessibility_features). Only fills NULL/empty fields on the primary.
    - Reassigns events/occurrences from new to existing.
    - Soft-deletes new place with `merged_into_id` set.
  - `params.PrimaryVenueID` updated to `mergeResult.CanonicalID`.
  - No warning added. No review needed (from this check alone).

### S7.2: New place, moderate similarity — flagged for review

- **Given:** Place "The Rex Jazz Bar" exists.
- **Action:** Event with venue "Rex Pub". Similarity = 0.72.
- **Expected:**
  - New place created.
  - Similarity >= 0.6 but < 0.95.
  - Warning `place_possible_duplicate` added with match details.
  - `needsReview` set to true.
  - New place is NOT merged (admin must decide).

### S7.3: New place, low similarity — no action

- **Given:** Place "The Rex Jazz Bar" exists.
- **Action:** Event with venue "Massey Hall". Similarity = 0.15.
- **Expected:** Below review threshold. No warning, no merge. New place created normally.

### S7.4: Existing place returned by UpsertPlace — skip check

- **Given:** Place "The Rex" already exists with exact name match.
- **Action:** Event submitted with venue "The Rex".
- **Expected:** `UpsertPlace` returns the existing record (ULID differs from generated one). Guard `place.ULID == placeULID` is false. Similarity check skipped.
- **Key detail:** Exact name matches are handled by upsert, not fuzzy dedup.

### S7.5: Self-match filtered out

- **Given:** No similar places exist.
- **Action:** New place created, similarity search returns only the newly created place itself.
- **Expected:** Self-match filtered by `c.ID != place.ID`. `filtered` is empty. No action.

### S7.6: Auto-merge fails or duplicate already merged — continue gracefully

- **Given:** Similar place found with similarity 0.97.
- **Action:** `MergePlaces` returns an error, or returns `MergeResult{AlreadyMerged: true}` if another goroutine already merged the duplicate.
- **Expected:** If the merge result indicates `AlreadyMerged`, the `CanonicalID` from the result is used as the venue ID (the chain was followed to the final canonical place). If an error occurs, warning logged and event continues using the new place. No warning added to review queue.

### S7.7: FindSimilarPlaces DB error — continue

- **Given:** Database error during similarity search.
- **Expected:** Warning logged. Event continues with new place. No error returned.

### S7.8: Multiple similar places

- **Given:** Three places with similarities 0.92, 0.85, 0.72.
- **Action:** New place created.
- **Expected:** Best candidate (0.92) checked first. Since 0.92 < 0.95, all three are added to the `place_possible_duplicate` warning matches array. `needsReview = true`.

### S7.9: Place threshold = 0 — check disabled

- **Given:** `DedupConfig.PlaceReviewThreshold = 0`.
- **Expected:** Guard `s.dedupConfig.PlaceReviewThreshold > 0` prevents the check.

### S7.10: No location provided — skip

- **Given:** Event has no `Location` field (or empty name).
- **Expected:** Entire place block at `ingest.go:307-396` is skipped.

### S7.11: Place auto-merge with locality/region filtering

- **Given:** "Rex Bar" exists in Toronto, ON. "Rex Bar" also exists in Montreal, QC.
- **Action:** Event with venue "Rex Bar" in Toronto.
- **Expected:** `FindSimilarPlaces` accepts locality and region parameters. If the query filters by locality/region, only the Toronto match is returned.

---

## 8. Organization Fuzzy Dedup Scenarios

**Code path:** `ingest.go:433-525`  
**Default thresholds:** Review=0.6, AutoMerge=0.95

### S8.1: New org, high similarity — auto-merge with gap-fill

- **Given:** Org "Toronto Arts Council" exists with no description or email.
- **Action:** Event with organizer "Toronto Arts Council Inc." that includes description and email. Similarity = 0.96.
- **Expected:** Same pattern as place auto-merge. `MergeOrganizations` gap-fills primary org fields from duplicate via SQL `UPDATE ... FROM` join (13 fields: legal_name, alternate_name, description, email, telephone, url, street_address, address_locality, address_region, postal_code, address_country, organization_type, founding_date). Only fills NULL/empty fields. Then reassigns events, soft-deletes duplicate. `params.OrganizerID` updated to `mergeResult.CanonicalID`.

### S8.2: New org, moderate similarity — flagged for review

- **Given:** Org "Toronto Arts Council" exists.
- **Action:** Event with organizer "TO Arts Council". Similarity = 0.75.
- **Expected:** Warning `org_possible_duplicate` added. `needsReview = true`.

### S8.3: Existing org returned by UpsertOrganization — skip check

- **Given:** Org "Toronto Arts" exists with exact name.
- **Action:** Event with organizer "Toronto Arts".
- **Expected:** `UpsertOrganization` returns existing. Guard `org.ULID == orgULID` false. Skip.

### S8.4: Org address derived from event location

- **Code:** `ingest.go:438-445` — if `validated.Location != nil`, org gets `addressLocality`, `addressRegion`, `addressCountry` from the event's location.
- **Given:** Event has `Location.AddressLocality = "Toronto"`.
- **Expected:** Org created/looked up with locality="Toronto". Similarity search also uses these for filtering.

### S8.5: No organizer provided — skip

- **Given:** Event has no `Organizer` field (or empty name).
- **Expected:** Entire org block at `ingest.go:433-525` is skipped.

---

## 9. Review Queue Admin Actions

### S9.1: Approve — publish event

**Code path:** `admin_review_queue.go:198-303`

- **Given:** Review entry #42, status "pending", event ULID "01J...".
- **Action:** `POST /api/v1/admin/review-queue/42/approve` with `{"notes":"Looks good"}`.
- **Expected:**
  - `AdminService.PublishEvent` called → event `lifecycle_state` changed to "published".
  - `ApproveReview` called → review status changed to "approved", `reviewed_by` and `reviewed_at` set.
  - Audit log entry created.

### S9.2: Reject — soft-delete event

**Code path:** `admin_review_queue.go:308-414`

- **Given:** Review entry #42, status "pending", event ULID "01J...".
- **Action:** `POST /api/v1/admin/review-queue/42/reject` with `{"reason":"Spam event"}`.
- **Expected:**
  - `AdminService.DeleteEvent` called → event soft-deleted.
  - `RejectReview` called → review status "rejected", `rejection_reason` set.
  - **Important:** Rejection reason is required (400 if empty).

### S9.3: Fix — apply date corrections

**Code path:** `admin_review_queue.go:421-565`

- **Given:** Review entry #42 with `date_order_reversed` warning.
- **Action:** `POST /api/v1/admin/review-queue/42/fix` with corrected startDate/endDate.
- **Expected:**
  - `AdminService.FixEventOccurrenceDates` called first: updates occurrence-level start_time and end_time via `UpdateOccurrenceDates` SQL query. Validates that end is not before start. If only one date is provided, the other is preserved from the existing occurrence.
  - `AdminService.PublishEvent` called to change lifecycle state to "published".
  - `ApproveReview` called with notes describing the corrections applied.
  - At least one correction is required (400 if both startDate and endDate are null).
  - If the event has no occurrences, returns an error.

### S9.4: Merge — merge duplicate into primary (atomic)

**Code path:** `admin_review_queue.go:567-668`

- **Given:** Review entry #42 for event E_dup with `potential_duplicate` warning pointing to E_primary.
- **Action:** `POST /api/v1/admin/review-queue/42/merge` with `{"primary_event_ulid":"01J_PRIMARY"}`.
- **Expected:**
  - Guard: Cannot merge event into itself (400).
  - `AdminService.MergeEventsWithReview` called — performs ALL of the following in a single database transaction:
    - Both events verified to exist and neither may have `LifecycleState == "deleted"` (returns `ErrEventDeleted` if so).
    - Primary enriched from duplicate via `AutoMergeFields` with trust (0,0) — gap-fills empty fields on primary without overwriting existing data. Uses `EventInputFromEvent` helper to convert the duplicate Event to an EventInput.
    - `repo.MergeEvents` sets `merged_into_id` on duplicate, soft-deletes it. Resolves transitive chains via `ResolveCanonicalEventULID` and flattens stale pointers.
    - Tombstone created for duplicate with `superseded_by` pointing to primary.
    - Review status set to "merged" via `MergeReview`, `duplicate_of_event_id` set.
    - Transaction committed — all operations succeed or none do.
  - **Key detail:** `MergeReview` only updates reviews with status "pending". Already-reviewed entries cannot be merged.

### S9.5: Merge — transaction guarantees atomicity

- **Given:** Events and review status update are performed together.
- **Expected:** `MergeEventsWithReview` wraps the event merge, tombstone creation, AND review status update in a single database transaction. If any step fails, the entire operation rolls back. No inconsistency is possible — either everything succeeds or nothing does.
- **Key detail:** The handler calls `MergeEventsWithReview` (single atomic call) instead of separate `MergeEvents` + `MergeReview` calls.

### S9.6: Approve already-reviewed entry

- **Given:** Review #42 already has status "approved".
- **Action:** Attempt to approve again.
- **Expected:** `ApproveReview` SQL has `WHERE status = 'pending'` — returns `pgx.ErrNoRows`. Handler returns 404 "Review entry not found or already reviewed".

### S9.7: Event not found during approve/reject

- **Given:** Review #42 exists but the referenced event was deleted outside the review workflow.
- **Action:** Attempt to approve.
- **Expected:** `AdminService.PublishEvent` returns `ErrNotFound`. Handler returns 404.

---

## 10. Trust-Based Field Merge Scenarios

**Code path:** `internal/domain/events/merge.go`  
**Trust levels:** 1-10, HIGHER = MORE trusted.

### Fields subject to merge:
- Description, ImageURL, PublicURL, EventDomain, Keywords

### Fields NEVER merged:
- Name (changing it would change the dedup hash)
- LifecycleState
- Occurrences (existing set kept)

### S10.1: Gap fill — existing empty, new has data (any trust)

| Existing | New | ExistingTrust | NewTrust | Result |
|----------|-----|---------------|----------|--------|
| `""` | `"desc"` | 8 | 2 | `"desc"` (filled) |
| `""` | `"desc"` | 2 | 8 | `"desc"` (filled) |
| `""` | `"desc"` | 5 | 5 | `"desc"` (filled) |
| `""` | `""` | 5 | 5 | `""` (no change) |

- **Key rule:** Gap fill ignores trust. Any source can fill empty fields.

### S10.2: Overwrite — both have data, new trust strictly higher

| Existing | New | ExistingTrust | NewTrust | Result |
|----------|-----|---------------|----------|--------|
| `"old"` | `"new"` | 5 | 8 | `"new"` (overwritten) |
| `"old"` | `"new"` | 5 | 6 | `"new"` (overwritten) |

### S10.3: Keep existing — both have data, same or lower trust

| Existing | New | ExistingTrust | NewTrust | Result |
|----------|-----|---------------|----------|--------|
| `"old"` | `"new"` | 5 | 5 | `"old"` (kept, tie goes to existing) |
| `"old"` | `"new"` | 8 | 3 | `"old"` (kept) |
| `"old"` | `"new"` | 5 | 4 | `"old"` (kept) |

### S10.4: New has empty value — no change regardless of trust

| Existing | New | ExistingTrust | NewTrust | Result |
|----------|-----|---------------|----------|--------|
| `"old"` | `""` | 3 | 10 | `"old"` (kept) |
| `""` | `""` | 3 | 10 | `""` (no change) |

- **Key rule:** Empty new data never triggers any change.

### S10.5: Keywords merge strategy

- Same rules as strings but for `[]string`:
  - Existing empty + new has keywords → fill.
  - Both have keywords + new trust higher → overwrite (replace entire slice, not append).
  - Both have keywords + same/lower trust → keep existing.
  - New empty → no change.

### S10.6: Whitespace-only values treated as empty

- `mergeStringField` calls `strings.TrimSpace()` on both values.
- `"  "` is treated as empty for both existing and new values.

### S10.7: Mixed field outcomes

- **Given:** Existing has description (trust 5) but no image. New has description and image (trust 3).
- **Expected:** Description unchanged (both have value, new trust lower). Image filled (gap fill). `changed = true`.

### S10.8: Trust level 1 vs trust level 10

- The full range of trust levels (1-10) works. Trust 10 always overwrites trust 1-9 when both have data.

---

## 11. Chain Merge & Transitive Resolution

### S11.1: Event chain merge — A merged into B, then B merged into C

- **Given:** Event A merged into B (`A.merged_into_id = B`). Then B merged into C (`B.merged_into_id = C`).
- **Behavior:** At the time B is merged into C, `repo.MergeEvents` calls `ResolveCanonicalEventULID` (recursive CTE, max depth 10) to find the final canonical target. It then calls `UpdateMergedIntoChain` to flatten all pointers: A's `merged_into_id` is updated from B to C. After the merge, there are no transitive chains — all events point directly to the canonical target C.
- **Key detail:** Chain flattening is bi-directional: both events previously pointing to the primary AND events previously pointing to the duplicate are re-pointed to the resolved canonical. This ensures no stale references remain.

### S11.2: Place chain merge

- **Given:** Place P1 merged into P2, then P2 merged into P3.
- **Behavior:** `MergePlaces` calls `resolveCanonicalPlace` (iterative chain follower, max 10 hops) to resolve the primary to its canonical target. If P2 was merged into P3, the merge of a new duplicate into P2 is redirected to P3. Events reassigned from P1 to P2 during the first merge are reassigned to P3 during the second merge (since all events pointing to P2 are reassigned as part of the merge).

### S11.3: Org chain merge

- Same pattern as places. `MergeOrganizations` uses `resolveCanonicalOrg` to follow chains.

### S11.4: Merge into already-merged event

- **Given:** Event B is already merged into C (B has `merged_into_id = C`, `deleted_at` set).
- **Action:** Admin attempts to merge A into B via review queue.
- **Expected:** `AdminService.MergeEventsWithReview` checks `LifecycleState` of both events. If B has `LifecycleState == "deleted"`, the merge is rejected with `ErrEventDeleted`. If the lifecycle state check passes but B has `merged_into_id` set, `repo.MergeEvents` resolves the chain via `ResolveCanonicalEventULID` — A would be merged into C (the canonical target), not B.

---

## 12. Warning Code Combinations

### Warning codes produced by ingestion:

| Code | Source | Description |
|------|--------|-------------|
| `missing_description` | `appendQualityWarnings` | No description |
| `missing_image` | `appendQualityWarnings` | No image (if RequireImage=true) |
| `too_far_future` | `appendQualityWarnings` | Start date >2 years ahead |
| `low_confidence` | `appendQualityWarnings` | Quality score <60% |
| `link_check_failed` | `appendQualityWarnings` | HTTP 400+ on URL check |
| `potential_duplicate` | Near-duplicate (L2) | Similar event at same venue+date |
| `place_possible_duplicate` | Place dedup (L3) | Similar place name |
| `org_possible_duplicate` | Org dedup (L3) | Similar org name |
| `date_order_reversed` | Validation | endDate before startDate |
| `timezone_corrected` | Validation | Timezone auto-corrected |

### S12.1: Duplicate + quality warnings combined

- **Given:** New event has no description AND is a near-duplicate.
- **Expected:** Both `missing_description` and `potential_duplicate` warnings present. Event goes to review queue.

### S12.2: Place + org + quality warnings

- **Given:** New event creates a fuzzy-matching place AND fuzzy-matching org AND has no image (RequireImage=true).
- **Expected:** Three warnings: `place_possible_duplicate`, `org_possible_duplicate`, `missing_image`. All appear in the review queue entry.

### S12.3: Duplicate warning doesn't trigger if exact hash matched

- **Given:** Event has no description (would trigger `missing_description`) but exact dedup hash matches.
- **Expected:** Auto-merge at step 5 returns early. Quality warnings were computed at step 2 but never result in a review queue entry. Warnings are still in the `IngestResult` but the event is merged, not queued.

### S12.4: stillHasSameIssues — code comparison

- `{missing_description, potential_duplicate}` vs `{missing_description, potential_duplicate}` → same (true).
- `{missing_description, potential_duplicate}` vs `{missing_description}` → different (false).
- `{missing_description}` vs `{missing_image}` → different (false).
- **Key detail:** The function compares CODE sets only. Different messages with the same code are still "same issues". Order doesn't matter.

---

## 13. Concurrent Submission Scenarios

### S13.1: Race between two identical submissions

- **Given:** Two goroutines submit the same event simultaneously, both pass the dedup hash check (no existing event yet).
- **Expected:** Both attempt to create the event. The first to commit wins. The second may fail with a unique constraint violation on dedup_hash (if indexed) or may create a duplicate.
- **Mitigation:** Idempotency keys prevent this for API clients. Without idempotency keys, duplicates are possible.

### S13.2: Race between submission and review action

- **Given:** Admin approves review entry #42. Simultaneously, same event is resubmitted and `FindReviewByDedup` finds entry #42 still "pending".
- **Expected:** Resubmission might try to update the entry's payloads, while approval changes its status. Depends on DB transaction isolation. Possible outcomes:
  - Resubmission's `UpdateReviewQueueEntry` succeeds but the event was already published.
  - Resubmission's `ApproveReview` (auto-approve case) conflicts with admin's `ApproveReview`.

### S13.3: Race in place auto-merge — handled with row locking

- **Given:** Two events submitted simultaneously, both creating a new place "The Rex Jazz Bar" that fuzzy-matches an existing "Rex Jazz Bar".
- **Expected:** Both may call `MergePlaces` targeting the same duplicate. `MergePlaces` uses `SELECT ... FOR UPDATE SKIP LOCKED` on the duplicate row:
  - The first goroutine acquires the lock and performs the merge normally.
  - The second goroutine's `SELECT FOR UPDATE SKIP LOCKED` returns no rows (the row is locked). It then calls `resolveCanonicalPlace` to follow the merge chain, finding where the duplicate ended up.
  - The second goroutine returns `MergeResult{CanonicalID: <resolved>, AlreadyMerged: true}` — no error, no duplicate merge.
  - Ingestion code uses `mergeResult.CanonicalID` as the venue ID regardless of whether it performed the merge or found an already-merged result.
- **Key detail:** The same pattern applies to `MergeOrganizations`. This eliminates race-condition failures during concurrent ingestion.

### S13.4: Idempotency key prevents double-creation

- **Given:** Same event submitted twice with same idempotency key.
- **Expected:** Second submission returns existing event from key lookup. No duplicate.

---

## 14. Config Threshold Edge Cases

### S14.1: Similarity exactly at auto-merge threshold

- **Given:** `PlaceAutoMergeThreshold = 0.95`. Place similarity = 0.95 exactly.
- **Expected:** Condition is `best.Similarity >= s.dedupConfig.PlaceAutoMergeThreshold`. Similarity 0.95 >= 0.95 is true → auto-merge.

### S14.2: Similarity exactly at review threshold

- **Given:** `PlaceReviewThreshold = 0.6`. Place similarity = 0.6 exactly.
- **Expected:** `FindSimilarPlaces` uses threshold as a lower bound in the SQL query (`similarity(name, $1) > $threshold`). If the SQL uses `>` (strict), 0.6 is excluded. If `>=`, included.
- **Key detail:** Check the actual SQL query operator. The Go code passes the threshold to the SQL query, which filters candidates.

### S14.3: Similarity between review and auto-merge thresholds

- **Given:** Review=0.6, AutoMerge=0.95. Best candidate similarity = 0.80.
- **Expected:** Not >= 0.95, so no auto-merge. Falls to the `else` branch — warning added with `place_possible_duplicate`.

### S14.4: All thresholds set to 0 — all checks disabled

- **Given:** `NearDuplicateThreshold=0, PlaceReviewThreshold=0, OrgReviewThreshold=0`.
- **Expected:** All three fuzzy checks are guarded by `> 0` conditions. All disabled.

### S14.5: Auto-merge threshold lower than review threshold (misconfiguration)

- **Given:** `PlaceAutoMergeThreshold=0.5, PlaceReviewThreshold=0.6`.
- **Expected:** `FindSimilarPlaces` returns candidates with similarity >= 0.6 (review threshold is the lower bound). Then:
  - Candidates with similarity >= 0.5 (auto-merge) would all qualify since they're all >= 0.6.
  - All fuzzy matches above the review threshold would be auto-merged.
  - This is likely unintended but not guarded against.

### S14.6: Near-duplicate threshold = 1.0

- **Given:** `NearDuplicateThreshold = 1.0`.
- **Expected:** Only exact name matches (similarity = 1.0) would be flagged. In practice, pg_trgm rarely gives exactly 1.0 even for identical strings (depends on padding). This effectively disables near-duplicate detection.

---

## 15. Source Provenance Through Merges

### S15.1: Auto-merge records new source

- **Given:** Event E1 exists, contributed by source S1. Source S2 submits same event (dedup hash matches).
- **Expected:** `recordSourceForExisting` creates a new `event_source` entry linking S2 to E1. Both S1 and S2 are now recorded as sources for E1.

### S15.2: Source recording failure during auto-merge — non-fatal

- **Code path:** `ingest.go:177-179` — `_ = s.recordSourceForExisting(...)`.
- **Expected:** Error from `recordSourceForExisting` is silently discarded. The merge still succeeds. The `IngestResult` is returned normally.
- **Key detail:** This is intentional — the merge is the primary operation, source recording is secondary.

### S15.3: Source recording in transaction during creation

- **Code path:** `ingest.go:550-553` — `s.recordSourceWithRepo(ctx, txRepo, ...)`.
- **Expected:** Source recording during NEW event creation is inside the transaction. If it fails, the entire transaction rolls back. Unlike auto-merge, this IS fatal.

### S15.4: No source provided — skip source recording

- **Given:** Event submitted without `Source` field.
- **Expected:** `sourceID` is empty. `recordSourceForExisting` guard `input.Source == nil || input.Source.URL == ""` returns early. No source recorded.

### S15.5: Admin merge (MergeEvents) — enriches primary via AutoMergeFields

- **Given:** Admin merges event E_dup (has description, image) into E_primary (missing description).
- **Expected:** `AdminService.MergeEvents` (and `MergeEventsWithReview`) calls `AutoMergeFields` with trust levels (0, 0) before soft-deleting the duplicate. The `EventInputFromEvent` helper converts the duplicate Event into an EventInput suitable for the merge function. With equal trust (0, 0), only gap-filling occurs: the duplicate's description fills the primary's empty description field, but no existing fields on the primary are overwritten. Changes are persisted via `UpdateEvent` within the same transaction.
- **Key detail:** The primary event is enriched before the duplicate is lost. The tombstone preserves the duplicate's full data as a secondary reference.

---

## 16. Resolved Gaps

All gaps identified during the initial design review have been resolved. This section documents what each gap was and how it was fixed.

---

**Last Updated:** 2026-02-20
