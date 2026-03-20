# Event Review Workflow

## Overview

The Event Review Workflow handles events with data quality issues (e.g., reversed dates) by storing them in a review queue for admin approval rather than rejecting them outright. This document describes the complete workflow from ingestion through admin review to final resolution.

## Problem Statement

Event providers sometimes submit events with data quality issues, most commonly:
- **Reversed dates**: `endDate` appears chronologically before `startDate`
- **Timezone errors**: Overnight events incorrectly converted from local time to UTC

Example:
```json
{
  "name": "Late Night Jazz",
  "startDate": "2025-03-31T23:00:00Z",  // 11 PM
  "endDate": "2025-03-31T02:00:00Z"      // 2 AM same day (should be next day!)
}
```

Rather than rejecting these events (400 error), we want to:
1. Accept them with warnings (202 Accepted)
2. Auto-correct obvious errors
3. Queue ambiguous cases for admin review
4. Allow sources to fix and resubmit

## Design Rationale

### Why Always Auto-Fix Reversed Dates?

**Decision:** Normalization always corrects reversed dates by adding 24 hours to `endDate`, regardless of confidence level.

**Rationale:**
1. **Database Constraint**: The `event_occurrences` table has a CHECK constraint `end_time >= start_time`. Without auto-fixing, we cannot store the event at all.
2. **Accept Don't Reject**: The requirement is to *accept* events with issues, not reject them. Auto-fixing allows storage while preserving the signal via warnings.
3. **Most Likely Cause**: The vast majority of reversed dates are timezone errors on overnight events. Adding 24 hours is almost always correct.
4. **Admin Can Override**: If the auto-fix is wrong, admins can manually correct during review.

**Alternative Considered:** Remove the database CHECK constraint to allow storing invalid dates.
- **Rejected because:** Would allow truly invalid data into the system. Other queries/logic assume valid date ordering. Risk of data corruption.

---

### Why Store in Both `events` and `event_review_queue`?

**Decision:** Events needing review are stored in BOTH tables simultaneously with `lifecycle_state='pending_review'`.

**Rationale:**
1. **Deduplication Works**: Events get a real ID immediately, allowing deduplication to catch resubmissions (fixed or still broken).
2. **Idempotency Works**: Can track idempotency keys against real event IDs, not review queue IDs.
3. **Federation Ready**: Other nodes can reference the event ID even while under review.
4. **Simpler Rollback**: Approval is just updating `lifecycle_state`, not re-running full insert logic.
5. **Audit Trail**: Both tables form complete history (events table = what was stored, review queue = what was submitted).

**Alternative Considered:** Store ONLY in `event_review_queue` until approved, then move to `events`.
- **Rejected because:** Makes deduplication complex (need to check two tables). Idempotency becomes harder. Admin fixes require recreating full event creation logic.

---

### Why Track Rejection History?

**Decision:** Keep rejected reviews in the database (don't delete) and block resubmission of same bad data.

**Rationale:**
1. **Avoid Spam**: Prevents sources from repeatedly submitting known-bad data.
2. **Save Admin Time**: Don't re-review the exact same issue multiple times.
3. **Signal Data Quality**: Repeated rejections from a source indicate systemic issues.
4. **Feedback to Submitter**: Can return specific rejection reason from previous review.

**Expiry:** Rejections expire after the event passes (event can't happen anymore, so rejection is moot).

---

### Why Compare Original vs Normalized in Validation?

**Decision:** Pass both original and normalized input to validation to detect what changed.

**Rationale:**
1. **Transparency**: Admins need to see what was submitted vs what was auto-corrected.
2. **Confidence Levels**: Detection logic (early morning + duration check) runs on original dates to classify correction confidence.
3. **Warning Messages**: Can include specific details: "Changed from X to Y because Z".
4. **Learning**: Track which corrections admins approve/reject to improve heuristics over time.

**Alternative Considered:** Only pass normalized input, add metadata flags.
- **Rejected because:** Loses signal about *what* was wrong. Harder to audit. Can't recompute confidence on resubmission.

---

### Why Two Warning Codes (Not Three)?

**Decision:** Use two warning codes:
- `reversed_dates_timezone_likely` (high confidence: 0-4 AM, < 7h)
- `reversed_dates_corrected_needs_review` (low confidence: everything else)

**Rationale:**
1. **Simplicity**: Two clear categories: "probably right" vs "needs human judgment".
2. **Actionable**: Admins can filter and bulk-approve high-confidence cases.
3. **Conservative**: Early morning threshold (0-4 AM, not 0-8 AM) minimizes false positives.

**Earlier Design:** Three codes (timezone_likely, ambiguous, large_gap).
- **Rejected because:** "Ambiguous" was unclear. Large gap vs ambiguous distinction didn't help admin decision-making.

---

### Why HTTP 202 Accepted (Not 201 Created)?

**Decision:** Return HTTP 202 Accepted for events with warnings, not 201 Created.

**Rationale:**
1. **Semantic Accuracy**: 202 means "accepted for processing" which matches `pending_review` status.
2. **Client Signal**: Submitters know the event isn't fully published yet.
3. **Standards Compliant**: 202 is designed for asynchronous processing scenarios (admin review = async).

**Alternative Considered:** Return 201 Created with warnings.
- **Rejected because:** Misleading - suggests event is published when it's not. 201 implies synchronous creation success.

---

### Why No Auto-Merge for Consolidated Events?

**Decision:** When a consolidated canonical event is flagged as a near-duplicate or exact-hash match of an existing event, it is sent to the review queue rather than auto-merged.

**Rationale:**
1. **Admin intent preserved:** The admin explicitly chose this event as canonical — auto-merging would silently override that decision.
2. **Transparency:** The admin should be warned about the match but not have their choice changed without a second review action.
3. **Equal standing:** A consolidated event is not more valid than an ingested one — the same dedup rules apply.
4. **Self-correcting:** The admin can resolve the new review entry with another `POST /admin/events/consolidate` if needed.

**Alternative Considered:** Skip dedup checks entirely for consolidated events.
- **Rejected because:** Would allow the canonical to become a silent duplicate of an unrelated event. Dedup integrity must apply uniformly.

---

### Why Expire Reviews After Event Passes?

**Decision:** Delete rejected reviews 7 days after event ends. Delete pending reviews when event starts.

**Rationale:**
1. **No Value**: Can't fix a past event that already happened (or didn't happen).
2. **Database Hygiene**: Review queue is for *future* events only.
3. **Privacy**: No need to keep potentially incorrect personal data (organizer info, etc.) for past events.
4. **Rejection Expiry**: If a source repeatedly submits bad data for event A, after event A passes, let them try again (maybe their system is fixed).

**Grace Period (7 days):** Allows sources to resubmit corrected data even after event ends, in case they want to preserve historical record.

---

### Why Hybrid Status (pending_review + queue)?

**Decision:** Use `lifecycle_state='pending_review'` in events table, separate from `draft` and `published`.

**Rationale:**
1. **Clear Semantics**: 
   - `draft` = intentionally unpublished (user choice)
   - `pending_review` = system-flagged for quality issues
   - `published` = live and approved
2. **Query Simplicity**: `WHERE lifecycle_state = 'published'` excludes both drafts and pending reviews.
3. **Workflow Tracking**: Can distinguish "user saved draft" from "system caught data issue".

**Alternative Considered:** Use `draft` with `needs_review=true` flag.
- **Rejected because:** Conflates user intent (draft) with system flag (review). Harder to query.

---

## Architecture

### Data Flow

```
Submission → Normalization → Validation → Deduplication → Storage → Review (if needed)
```

### Components

1. **Normalization** (`internal/domain/events/normalize.go`)
   - Always corrects reversed dates by adding 24 hours to `endDate`
   - Required for database CHECK constraint: `end_time >= start_time`

2. **Validation** (`internal/domain/events/validation.go`)
   - Compares original vs normalized input
   - Generates warnings with confidence levels
   - Returns validation result with warnings array

3. **Ingestion** (`internal/domain/events/ingest.go`)
   - Coordinates normalization → validation → storage
   - Checks for existing reviews (pending or rejected)
   - Creates review queue entries when needed

4. **Review Queue** (new: `event_review_queue` table)
   - Stores original payload for admin inspection
   - Tracks review status and decisions
   - Expires after event passes

5. **Admin API** (new: admin review endpoints)
   - List pending reviews
   - View original vs corrected data
   - Approve/reject/manually fix events

## Database Schema

### New Table: `event_review_queue`

```sql
CREATE TABLE event_review_queue (
  id SERIAL PRIMARY KEY,
  event_id TEXT UNIQUE NOT NULL,  -- References events.id
  
  -- Original submission for admin comparison
  original_payload JSONB NOT NULL,
  normalized_payload JSONB NOT NULL,
  warnings JSONB NOT NULL,
  
  -- Deduplication keys (match events table)
  source_id TEXT,
  source_external_id TEXT,
  dedup_hash TEXT,
  
  -- Event timing (for expiry logic)
  event_start_time TIMESTAMPTZ NOT NULL,
  event_end_time TIMESTAMPTZ,
  
  -- Review workflow
  status TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, merged
  reviewed_by TEXT,
  reviewed_at TIMESTAMPTZ,
  review_notes TEXT,
  rejection_reason TEXT,
  
  -- Near-duplicate cross-linking (see Near-Duplicate Cross-Linking section)
  duplicate_of_event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
  
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  
  -- Foreign key to events table
  CONSTRAINT fk_event FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
);

-- Partial unique indexes: only one pending review per unique event
CREATE UNIQUE INDEX idx_review_queue_unique_pending_source 
  ON event_review_queue(source_id, source_external_id) 
  WHERE status = 'pending';

CREATE UNIQUE INDEX idx_review_queue_unique_pending_dedup 
  ON event_review_queue(dedup_hash) 
  WHERE status = 'pending' AND dedup_hash IS NOT NULL;

-- Other indexes
CREATE INDEX idx_review_queue_status ON event_review_queue(status);
CREATE INDEX idx_review_queue_expired_rejections ON event_review_queue(status, event_end_time) 
  WHERE status = 'rejected';
CREATE INDEX idx_review_queue_event_id ON event_review_queue(event_id);
```

### Modified: `events` table

Events requiring review are stored with:
```sql
lifecycle_state = 'pending_review'  -- (not 'published' or 'draft')
-- All other fields contain CORRECTED data (post-normalization)
```

**Note:** The `pending_review` state must be added to the existing `lifecycle_state` CHECK constraint:
```sql
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_lifecycle_state_check;
ALTER TABLE events ADD CONSTRAINT events_lifecycle_state_check 
  CHECK (lifecycle_state IN ('draft', 'published', 'deleted', 'pending_review'));
```

This distinguishes:
- `draft` = User-created, intentionally unpublished
- `pending_review` = System-flagged for data quality review
- `published` = Approved and live
- `deleted` = Tombstoned

## Warning Codes

Validation generates machine-readable warning codes:

| Code | Confidence | Description |
|------|-----------|-------------|
| `reversed_dates_timezone_likely` | High | End time 0-4 AM, duration < 7h after correction. Likely overnight event with timezone error. |
| `reversed_dates_corrected_needs_review` | Low | Reversed dates corrected but doesn't match high-confidence pattern. Needs admin review. |
| `multi_session_likely` | — | Event appears to be a multi-session course or recurring series. See [Multi-Session Detection](#multi-session-detection). |

## Multi-Session Detection

Events that appear to be multi-session courses or recurring series are automatically routed to the review queue instead of being published. Two heuristics are applied:

| Heuristic | Condition |
|-----------|-----------|
| **Duration** | Single occurrence spanning > duration threshold (default: 168 hours / 1 week) |
| **Title patterns** | Contains `(N sessions)`, `(N weeks)`, `(N classes)`, `(N workshops)`, `workshop series`, `course`, or `weekly` |

When either heuristic fires, the warning code `multi_session_likely` is added and the event is stored with `lifecycle_state = 'pending_review'`.

**Per-source opt-out:** Sources that legitimately publish long-duration events (e.g. exhibitions, residencies) can set `skip_multi_session_check: true` in their source config to disable all duration and title checks. See [scraper.md](../integration/scraper.md) for the config field reference.

**Per-source threshold override:** Sources that legitimately publish events spanning more than 1 week (e.g. multi-week festivals) can set `multi_session_duration_threshold` to a custom Go duration string (e.g. `"720h"` for 30 days) in their source config. This overrides the default 168h threshold for duration-based detection only; title pattern checks still apply. Example:

```yaml
multi_session_duration_threshold: "720h"  # 30 days
```

---

## Review Action State Machine

This section is the authoritative decision table for all admin review actions. "Companion review" refers to the cross-linked review row created on the other event during near-duplicate ingest (see Near-Duplicate Cross-Linking below).

### Action Decision Table

| Action | Endpoint | Review entry's event | Companion review row | Target lifecycle after action | Review status set to |
|--------|----------|---------------------|---------------------|-------------------------------|----------------------|
| **approve** | `POST .../approve` | Event's `lifecycle_state` → `published` (unless already published) | Not touched | Event = `published` | `approved` |
| **approve** (with `record_not_duplicates: true`) | same | Same as approve | Companion review is rechecked best-effort after duplicate warnings are removed; if no issues remain it is auto-approved, otherwise it stays pending with refreshed warnings | Event = `published` | `approved` |
| **reject** | `POST .../reject` | Soft-deleted (`lifecycle_state = 'deleted'`, tombstone inserted) | `potential_duplicate` warning pairs are recorded as not-duplicates; companion dismiss is best-effort | Event = `deleted` | `rejected` |
| **fix** | `POST .../fix` | Occurrence dates corrected; then published | Not touched | Event = `published` | `approved` |
| **merge** | `POST .../merge` | Soft-deleted (`lifecycle_state = 'deleted'`, tombstone with `superseded_by` → primary URI, reason `duplicate_merged`) | Not touched — the companion review row (if any) on the primary is **not** automatically cleared | Primary event unchanged | `merged` |
| **add-occurrence** (forward path — `potential_duplicate`) | `POST .../add-occurrence` | Soft-deleted (`lifecycle_state = 'deleted'`, tombstone reason `absorbed_as_occurrence`, `superseded_by` → target URI); occurrences explicitly deleted | Companion review on target event is located and dismissed atomically (status → `merged`) | Target lifecycle recomputed: if no remaining pending reviews → `published`; if other pending reviews remain → stays `pending_review` | `merged` |
| **add-occurrence** (near-dup path — `near_duplicate_of_new_event`) | `POST .../add-occurrence` | Event is the **target** (kept); `DuplicateOfEventULID` is the source (soft-deleted); occurrence added to this event | Companion review on the source event is located and dismissed atomically (status → `merged`) | Same recompute as forward path applied to this event | `merged` |
| **not-a-duplicate** (UI-only name) | `POST .../approve` with `record_not_duplicates: true` | Same as approve | `potential_duplicate` pairs recorded in `not_duplicates` table; companion pending review is rechecked best-effort and auto-approved only if no issues remain | Event = `published` | `approved` |

### Recompute Logic (add-occurrence only)

After `add-occurrence` completes — regardless of path — the surviving (target) event's lifecycle is recomputed **inside the same transaction**:

```
if target.lifecycle_state == "pending_review":
    remaining = GetPendingReviewByEventUlid(targetULID)
    if remaining == nil:
        target.lifecycle_state = "published"   // all reviews resolved
    else:
        # leave as pending_review — other unresolved reviews exist
```

**Approve, reject, fix, and merge do NOT run this recompute.** They directly set the event's lifecycle state unconditionally.

### Why merge does not recompute the primary

The `merge` action soft-deletes the duplicate event; the primary event's lifecycle state is **not modified**. If the primary was `pending_review` before the merge, it remains `pending_review` after. The primary's companion review row (if any) is also not automatically dismissed — the admin must handle the primary's review separately.

### Companion review dismissal — exact scope

Companion dismissal in `add-occurrence` targets only the **exact counterpart row**: `GetPendingReviewByEventUlidAndDuplicateUlid(companionEventULID, sourceEventULID)`. This is intentionally narrow — it avoids clearing reviews that reference a *different* near-dup pairing on the same event, which would be incorrect.

---

### POST /admin/events/consolidate (bead srv-won8q)

Atomically resolve N duplicate events into a single canonical event. This is the preferred resolution path for duplicate detection — it supersedes the per-review-entry `merge` and `add-occurrence` actions for that use case.

**Two dispatch paths:**

| Path | When to use | How to invoke |
|------|-------------|---------------|
| **Create** | The duplicates do not have a clear winner; build the canonical from scratch | Supply `event` (full event payload) in the request body; omit `event_ulid` |
| **Promote** | One of the duplicates is already the best version and should survive | Supply `event_ulid` (ULID of the event to promote); omit `event` |

**`retire` list (required on both paths):** An array of ULIDs to retire. Each retired event is:
1. Soft-deleted (`lifecycle_state = 'deleted'`, tombstone reason `"consolidated"`, `superseded_by` → canonical event URI).
2. All its pending review queue entries are dismissed atomically (status set to `"system"` by the dismissal record).

The canonical event is **never** in the retire list (enforced; returns `422` if violated).

**Create path — post-creation pipeline:**

The new canonical goes through the same normalization and validation pipeline as any ingested event:
- Quality warnings (reversed dates, multi-session, etc.) accumulate.
- Layer 1 exact dedup-hash check runs against existing non-retired events. A match that is not in the retire list causes `needsReview = true` and adds an `exact_duplicate` warning.
- Layer 2 near-duplicate check (pg_trgm) runs if the canonical has a venue and at least one occurrence. Matches against non-retired, non-self events add `potential_duplicate` warnings.

If any warnings are generated, the canonical is stored with `lifecycle_state = 'pending_review'` and a review queue entry is created. Otherwise it is `published`.

**Promote path — post-promotion pipeline:**

The promoted event is not re-normalized or re-validated (it already exists). Only the Layer 2 near-duplicate check runs post-promotion (same filtering: skip self, skip retired events). Matches produce `potential_duplicate` warnings and flip the canonical to `pending_review`.

**Transaction scope:**

All steps — locking retired events, resolving/creating the canonical, soft-deleting retired events, creating tombstones, dismissing review entries, and setting the canonical lifecycle state — run inside a single database transaction.

**Response fields (200 OK):**

```json
{
  "event": { /* canonical event object */ },
  "lifecycle_state": "published",   // or "pending_review" if warnings were generated
  "needs_review": false,
  "warnings": [],
  "retired": ["<ulid1>", "<ulid2>"],
  "review_entries_dismissed": ["<review-id-1>"]
}
```

**Error responses:**

- `400 Bad Request` — both or neither of `event`/`event_ulid` supplied; retire list empty; canonical ULID in the retire list
- `404 Not Found` — canonical or any retire-target not found
- `409 Conflict` — any retire-target is already deleted; canonical is deleted (promote path)
- `422 Unprocessable Entity` — payload validation failure on the create path

**When to use consolidate vs. merge / add-occurrence:**

| Scenario | Preferred action |
|----------|-----------------|
| Exact duplicates — one is clearly best | `POST /admin/events/consolidate` (promote path) |
| Exact duplicates — neither is best; want a clean canonical | `POST /admin/events/consolidate` (create path) |
| Near-duplicates from review queue — want to merge one into the other | `POST /admin/events/consolidate` (promote path) replaces per-entry `merge` |
| Two events that are legitimately separate occurrences of a series | `POST .../add-occurrence` (still the right tool) |
| Single review entry with a simple data quality issue (reversed dates) | `POST .../approve`, `POST .../fix`, or `POST .../reject` |

**Deprecation notice:** The per-review-entry `merge` and `add-occurrence` actions are kept for backward compatibility but are no longer the recommended path for duplicate resolution. Use `POST /api/v1/admin/events/consolidate` instead when resolving duplicates detected via the review queue. The `merge` and `add-occurrence` endpoints remain useful for edge cases (e.g. absorbing a single occurrence into a recurring series).

---

## Near-Duplicate Cross-Linking

When Layer 2 (pg_trgm) near-duplicate detection matches a new event against one or more existing published events, both sides of the match enter the review queue:

1. **New event** — stored with `lifecycle_state = 'pending_review'` (existing behaviour). Its review queue entry has `duplicate_of_event_id` pointing to the first matched existing event.
2. **Each matched existing event** — if currently `published`, its `lifecycle_state` is set to `'pending_review'` and a new review queue entry is created, with `duplicate_of_event_id` pointing to the new event.

This allows an admin to compare both candidates side-by-side and decide which to keep, merge, or discard.

**Scope:** Only Layer 2 (near-duplicate) matches trigger this behaviour. Layer 1 exact dedup-hash matches still auto-merge as before.

**Timing:** The existing-event updates happen after the transaction commits (for FK safety). Failures are non-critical — they are logged and skipped rather than failing the ingest request.

### `duplicate_of_event_id` field

The `event_review_queue` table has a `duplicate_of_event_id` column (nullable `TEXT`, references `events.id`). It is populated when a review entry is created as part of near-duplicate cross-linking:

- On the **new** event's entry: points to the first matched existing event.
- On each **existing** event's entry: points to the new event.

---

## Ingestion Workflow

```
1. Decode JSON payload
2. Normalize (fix reversed dates)
3. Validate with warnings (compare original vs normalized)
4. Check for existing reviews (deduplication)
   a. If rejected previously + same issues → Reject (400)
   b. If pending + now fixed → Approve and publish
   c. If pending + still broken → Update queue entry
5. If needs review:
   a. Insert into events table (lifecycle_state='pending_review')
   b. Insert into event_review_queue
   c. Return 202 Accepted with warnings
6. If no issues:
   a. Insert into events table (lifecycle_state='published')
   b. Return 201 Created
```

### Detailed Logic

```go
func (s *IngestService) IngestWithIdempotency(ctx context.Context, input EventInput, key string) (*IngestResult, error) {
    // 1. Handle idempotency key checks...
    
    // 2. Normalize (always fixes reversed dates)
    normalized := NormalizeEventInput(input)
    
    // 3. Validate (detects what was corrected)
    validationResult, err := ValidateEventInputWithWarnings(normalized, s.nodeDomain, &input)
    if err != nil {
        return nil, err  // Hard validation failure (missing required fields, etc.)
    }
    warnings := validationResult.Warnings
    needsReview := len(warnings) > 0
    
    // 4. Check for existing review
    if needsReview {
        existingReview, err := s.repo.FindReviewByDedup(ctx, sourceID, externalID, dedupHash)
        
        if existingReview != nil {
            switch existingReview.Status {
            case "rejected":
                // Check if rejection is still valid
                if !isEventPast(existingReview.EventEndTime) {
                    if stillHasSameIssues(existingReview.Warnings, warnings) {
                        return nil, ErrPreviouslyRejected{
                            Reason: existingReview.RejectionReason,
                            ReviewedAt: existingReview.ReviewedAt,
                        }
                    }
                }
                // Event passed or different issues - allow resubmission
                
            case "pending":
                // Already in queue - check if fixed
                if len(warnings) == 0 {
                    // Fixed! Approve and publish
                    return s.approveReview(ctx, existingReview.EventID, normalized)
                }
                // Still has issues - update queue entry
                return s.updateReview(ctx, existingReview.ID, input, normalized, warnings)
            }
        }
    }
    
    // 5. Create new event
    eventID := generateULID()
    lifecycleState := "published"
    if needsReview {
        lifecycleState = "pending_review"
    }
    
    event, err := s.repo.CreateEvent(ctx, EventCreateParams{
        ULID: eventID,
        // ... other fields from normalized input
        LifecycleState: lifecycleState,
    })
    
    // 6. If needs review, create queue entry
    if needsReview {
        err = s.repo.CreateReviewQueueEntry(ctx, ReviewQueueCreateParams{
            EventID: eventID,
            OriginalPayload: toJSON(input),
            NormalizedPayload: toJSON(normalized),
            Warnings: toJSON(warnings),
            SourceID: sourceID,
            SourceExternalID: externalID,
            DedupHash: dedupHash,
            EventStartTime: parseStartTime(normalized),
            EventEndTime: parseEndTime(normalized),
        })
    }
    
    return &IngestResult{
        Event: event,
        NeedsReview: needsReview,
        Warnings: warnings,
    }, nil
}
```

## Scenarios

### Scenario 1: High-Confidence Auto-Fix

**Input:**
```json
{
  "startDate": "2025-03-31T23:00:00Z",  // 11 PM
  "endDate": "2025-03-31T02:00:00Z"      // 2 AM
}
```

**Flow:**
1. Normalize: `endDate` → `2025-04-01T02:00:00Z` (add 24h)
2. Validate: End hour 2 AM (0-4), duration 3h (< 7h) → High confidence
3. Warning: `reversed_dates_timezone_likely`
4. Store: `lifecycle_state='pending_review'`, queue entry created
5. Response: 202 Accepted with warning

**Admin sees:** "Likely overnight event with timezone error. End time corrected from 02:00 to 02:00+1day."

---

### Scenario 2: Low-Confidence Correction

**Input:**
```json
{
  "startDate": "2025-03-31T23:00:00Z",  // 11 PM
  "endDate": "2025-03-31T10:00:00Z"      // 10 AM
}
```

**Flow:**
1. Normalize: `endDate` → `2025-04-01T10:00:00Z` (add 24h)
2. Validate: End hour 10 AM (not 0-4), duration 11h → Low confidence
3. Warning: `reversed_dates_corrected_needs_review`
4. Store: `lifecycle_state='pending_review'`, queue entry created
5. Response: 202 Accepted with warning

**Admin sees:** "Dates were reversed. Auto-corrected but unusual duration (11h). Please verify."

---

### Scenario 3: Fixed Resubmit (Before Review)

**Initial:**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-03-31T02:00:00Z"}
```
→ Queued for review

**Resubmit (Fixed):**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-04-01T02:00:00Z"}
```

**Flow:**
1. Normalize: No change needed (dates valid)
2. Validate: No warnings
3. Dedup: Finds pending review with same `source_id + source_external_id`
4. Check warnings: Now empty (fixed!)
5. Action: 
   - Update `events.lifecycle_state = 'published'`
   - Update `event_review_queue.status = 'merged'`
6. Response: 201 Created (or 409 if treating as duplicate)

---

### Scenario 4: Rejected, Then Resubmit (Still Wrong)

**Initial:**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-03-31T10:00:00Z"}
```
→ Admin reviews → Rejects "Cannot determine correct time"

**Resubmit (Same Bad Data):**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-03-31T10:00:00Z"}
```

**Flow:**
1. Normalize + Validate: Same warnings
2. Dedup: Finds rejected review
3. Check: Event hasn't passed yet (Mar 31), same warnings
4. Response: **400 Bad Request**
   ```json
   {
     "type": "https://sel.events/problems/previously-rejected",
     "title": "Previously Rejected",
     "detail": "This event was reviewed on 2025-02-07 and rejected: Cannot determine correct time. Please fix the data before resubmitting.",
     "reviewedAt": "2025-02-07T14:30:00Z",
     "reviewedBy": "admin@togather.ca"
   }
   ```

---

### Scenario 5: Rejected, Event Passes, Then Resubmit

**Initial (Feb 7):**
```json
{"startDate": "2025-02-10T23:00:00Z", "endDate": "2025-02-10T10:00:00Z"}
```
→ Admin reviews → Rejects

**Resubmit (Feb 20, After Event):**
```json
{"startDate": "2025-02-10T23:00:00Z", "endDate": "2025-02-10T10:00:00Z"}
```

**Flow:**
1. Dedup: Finds rejected review
2. Check: Event passed (Feb 10), rejection expired
3. Action: Allow resubmission (though likely will be rejected as past event by other logic)

---

## Admin Review API

### GET /admin/review-queue

List events pending review.

**Query Parameters:**
- `status` - Filter by status (pending, approved, rejected)
- `limit` - Page size (default 50)
- `cursor` - Pagination cursor

**Response:**
```json
{
  "items": [
    {
      "id": 123,
      "eventId": "01HQRS7T8G",
      "eventName": "Late Night Jazz",
      "eventStartTime": "2025-03-31T23:00:00Z",
      "warnings": [
        {
          "field": "endDate",
          "code": "reversed_dates_timezone_likely",
          "message": "endDate was 21h before startDate and ends at 02:00 - likely timezone error"
        }
      ],
      "status": "pending",
      "createdAt": "2025-02-07T14:00:00Z"
    }
  ],
  "nextCursor": "..."
}
```

---

### GET /admin/review-queue/:id

Get detailed review including original vs corrected data.

**Response:**
```json
{
  "id": 123,
  "eventId": "01HQRS7T8G",
  "status": "pending",
  "warnings": [...],
  "original": {
    "name": "Late Night Jazz",
    "startDate": "2025-03-31T23:00:00Z",
    "endDate": "2025-03-31T02:00:00Z",
    "location": {...}
  },
  "normalized": {
    "name": "Late Night Jazz",
    "startDate": "2025-03-31T23:00:00Z",
    "endDate": "2025-04-01T02:00:00Z",  // <-- Corrected
    "location": {...}
  },
  "changes": [
    {
      "field": "endDate",
      "original": "2025-03-31T02:00:00Z",
      "corrected": "2025-04-01T02:00:00Z",
      "reason": "Added 24 hours to fix reversed dates"
    }
  ],
  "createdAt": "2025-02-07T14:00:00Z"
}
```

---

### POST /admin/review-queue/:id/approve

Approve the auto-correction.

**Request:**
```json
{
  "notes": "Correction looks correct - typical overnight event"
}
```

**Action:**
- Update `events.lifecycle_state = 'published'`
- Update `event_review_queue.status = 'approved'`
- Record review metadata

---

### POST /admin/review-queue/:id/reject

Reject the event (cannot determine correct dates).

**Request:**
```json
{
  "reason": "Cannot contact organizer to verify correct times"
}
```

**Action:**
- Update `events.lifecycle_state = 'deleted'`
- Update `event_review_queue.status = 'rejected'`
- Record rejection reason
- Rejection expires after event passes

---

### POST /admin/review-queue/:id/fix

Manually correct the dates.

**Request:**
```json
{
  "corrections": {
    "startDate": "2025-03-31T19:00:00Z",  // Admin-verified correct time
    "endDate": "2025-04-01T01:00:00Z"
  },
  "notes": "Contacted organizer, confirmed 7 PM - 1 AM"
}
```

**Action:**
- Update event with corrected dates
- Update `events.lifecycle_state = 'published'`
- Update `event_review_queue.status = 'approved'`
- Record manual corrections

---

### POST /admin/review-queue/:id/merge (removed)

This endpoint has been removed. Use `POST /api/v1/admin/events/consolidate` with `event_ulid` (canonical) + `retire` (list of duplicates) instead. The service-layer method `MergeEventsWithReview` still exists and is invoked by the consolidate handler.

---

### POST /admin/review-queue/:id/add-occurrence (removed)

This endpoint has been removed. Use `POST /api/v1/admin/events/consolidate` with `event_ulid` (canonical) + `retire` (list of events to absorb) instead. The consolidate endpoint handles both the "add as occurrence" and "merge duplicate" semantics: it retires the non-canonical event with a tombstone and dismisses companion review entries atomically.

For reference, the consolidate request shape is:
```json
{ "event_ulid": "<canonical-ulid>", "retire": ["<other-ulid>"] }
```

---

### POST /admin/events/consolidate

See [POST /admin/events/consolidate (bead srv-won8q)](#post-admineventsconsolidate-bead-srv-won8q) under the Review Action State Machine section. This endpoint operates outside the per-review-entry action model: it resolves N events simultaneously, retiring them with tombstones and promoting or creating a single canonical. Refer to the full description above for request/response shape.

---

## Cleanup & Maintenance

### Background Job: Clean Expired Reviews

Runs daily to remove stale review queue entries.

```go
func CleanupExpiredReviews(ctx context.Context) error {
    // 1. Delete rejected reviews for past events (7 day grace period)
    db.Exec(`
        DELETE FROM event_review_queue
        WHERE status = 'rejected'
        AND (
            event_end_time < NOW() - INTERVAL '7 days'
            OR (event_end_time IS NULL AND event_start_time < NOW() - INTERVAL '7 days')
        )
    `)
    
    // 2. Mark unreviewed events as deleted BEFORE deleting queue entries
    // (Must run UPDATE before DELETE so subquery returns rows)
    db.Exec(`
        UPDATE events SET lifecycle_state = 'deleted'
        WHERE id IN (
            SELECT event_id FROM event_review_queue
            WHERE status = 'pending' AND event_start_time < NOW()
        )
    `)
    
    // 3. Delete unreviewed events that have started
    // (If not reviewed before event starts, too late - delete it)
    db.Exec(`
        DELETE FROM event_review_queue
        WHERE status = 'pending'
        AND event_start_time < NOW()
    `)
    
    // 4. Archive old approved/merged reviews (90 day retention)
    db.Exec(`
        DELETE FROM event_review_queue
        WHERE status IN ('approved', 'merged')
        AND reviewed_at < NOW() - INTERVAL '90 days'
    `)
}
```

---

## API Response Changes

### Success with Warnings (202 Accepted)

When `NeedsReview = true`:

**Response:**
```http
HTTP/1.1 202 Accepted
Content-Type: application/ld+json

{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.ca/events/01HQRS7T8G",
  "name": "Late Night Jazz",
  "warnings": [
    {
      "field": "endDate",
      "code": "reversed_dates_timezone_likely",
      "message": "endDate was 21h before startDate and ends at 02:00 - likely timezone error. Auto-corrected by adding 24 hours. Event queued for admin review."
    }
  ]
}
```

### Success without Warnings (201 Created)

Standard success response (no changes).

---

## Implementation Tasks

See related beads:
- Event review queue table migration
- Repository methods for review queue
- Ingestion logic updates
- Admin API endpoints
- Cleanup background job
- API handler response changes
- Tests for all scenarios

---

## Known Issues & Implementation Notes

### Critical Implementation Order

**MUST FIX FIRST (srv-629):** Normalization currently fixes reversed dates BEFORE validation runs, causing validation to see already-correct dates and generate zero warnings. This means high-confidence auto-fixes bypass the review workflow entirely and publish directly as `lifecycle_state='published'`.

**Fix:** Pass original input to `ValidateEventInputWithWarnings` so it can detect what was corrected by comparing original vs normalized. See srv-l02 for implementation plan.

**Impact:** Until fixed, the review workflow is non-functional for high-confidence cases.

---

### Edge Cases & Limitations

#### Concurrent Submission Race Conditions

1. **Same event from different sources simultaneously:**
   - Dedup by `source_external_id` is scoped to source
   - Dedup by `dedup_hash` can catch cross-source duplicates
   - But concurrent ingestion could create two pending reviews for the same real-world event
   - **Mitigation:** Dedup hash check happens in transaction, should catch most cases

2. **Admin approval vs source resubmission race:**
   - Admin approves review while source simultaneously resubmits fix
   - Both paths try to set `lifecycle_state='published'`
   - **Mitigation:** Last write wins (both outcomes are correct)

#### Occurrence-Only Events

Events with ONLY `occurrences` array (no top-level `startDate`/`endDate`) are not handled by current normalization/validation:
- `normalizeOccurrences()` doesn't apply timezone correction
- `validateOccurrences()` hard-rejects reversed dates (doesn't generate warnings)
- See srv-oad for fix

#### End Hour Boundary

Documentation says "0-4 AM" but code checks `endHour <= 4`, which covers 0:00-4:59:59 (nearly 5 hours). Minor discrepancy.

#### Transaction Boundaries

Event creation, occurrences, source creation, and review queue entry are separate DB operations. If a later step fails, earlier steps succeed, creating:
- Orphan event in draft state
- No review queue entry
- No way to recover

**Mitigation:** Wrap in transaction (future enhancement).

---

## Future Enhancements

1. **Learning System**: Track admin decisions to improve auto-fix heuristics
2. **Confidence Scoring**: More sophisticated ML-based confidence scores
3. **Bulk Review**: Allow admins to approve/reject multiple events at once
4. **Source Trust Levels**: Auto-approve high-confidence corrections from trusted sources
5. **Notification System**: Alert admins when review queue grows large
6. **Review Queue Metrics**: Dashboard showing review volume, approval rates, etc.

---

## Related Documentation

- `docs/interop/core-profile-v0.1.md` - SEL Core Interoperability Profile
- `docs/api/API_CONTRACT_v1.md` - API contract specifications
- `internal/domain/events/normalize.go` - Normalization logic
- `internal/domain/events/validation.go` - Validation logic

---

**Last Updated:** 2026-02-20
