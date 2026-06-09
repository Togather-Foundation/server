# Event Review Workflow

## Overview

The Event Review Workflow handles events with data quality issues (e.g., reversed dates, near-duplicates) by storing them in a review queue for admin approval rather than rejecting them outright. This document is the **definitive, auditable state machine and decision table** for the admin review queue system. Every claim maps to a specific line number in the codebase.

## Problem Statement

Event providers sometimes submit events with data quality issues, most commonly:
- **Reversed dates**: `endDate` appears chronologically before `startDate`
- **Timezone errors**: Overnight events incorrectly converted from local time to UTC
- **Near-duplicates**: pg_trgm similarity match against an existing event at the same venue on the same date
- **Cross-week series**: Event at same venue, same name, 7-21 days apart, same time-of-day (±30 min)

Rather than rejecting these events (400 error), the system:
1. Accepts them with warnings (202 Accepted)
2. Auto-corrects obvious errors (reversed dates)
3. Queues ambiguous cases for admin review
4. Allows sources to fix and resubmit

## Design Rationale

### Why Always Auto-Fix Reversed Dates?

**Decision:** Normalization always corrects reversed dates by adding 24 hours to `endDate`, regardless of confidence level.

**Rationale:**
1. **Database Constraint**: The `event_occurrences` table has a CHECK constraint `end_time >= start_time`. Without auto-fixing, we cannot store the event at all.
2. **Accept Don't Reject**: The requirement is to *accept* events with issues, not reject them. Auto-fixing allows storage while preserving the signal via warnings.
3. **Most Likely Cause**: The vast majority of reversed dates are timezone errors on overnight events. Adding 24 hours is almost always correct.
4. **Admin Can Override**: If the auto-fix is wrong, admins can manually correct during review.

---

### Why Store in Both `events` and `event_review_queue`?

**Decision:** Events needing review are stored in BOTH tables simultaneously with `lifecycle_state='pending_review'`.

**Rationale:**
1. **Deduplication Works**: Events get a real ID immediately, allowing deduplication to catch resubmissions.
2. **Idempotency Works**: Can track idempotency keys against real event IDs.
3. **Federation Ready**: Other nodes can reference the event ID even while under review.
4. **Simpler Rollback**: Approval is just updating `lifecycle_state`, not re-running full insert logic.
5. **Audit Trail**: Both tables form complete history.

---

### Why Track Rejection History?

**Decision:** Keep rejected reviews in the database (don't delete) and block resubmission of same bad data.

**Rationale:**
1. **Avoid Spam**: Prevents sources from repeatedly submitting known-bad data.
2. **Save Admin Time**: Don't re-review the exact same issue multiple times.
3. **Signal Data Quality**: Repeated rejections from a source indicate systemic issues.

**Expiry:** Rejections expire after the event passes (event can't happen anymore).

---

### Why Compare Original vs Normalized in Validation?

**Decision:** Pass both original and normalized input to validation to detect what changed.

**Rationale:**
1. **Transparency**: Admins need to see what was submitted vs what was auto-corrected.
2. **Confidence Levels**: Detection logic (early morning + duration check) runs on original dates.
3. **Warning Messages**: Can include specific details about corrections.

---

### Why Two Warning Codes (Not Three)?

**Decision:** Use two warning codes:
- `reversed_dates_timezone_likely` (high confidence: 0-4 AM, < 7h)
- `reversed_dates_corrected_needs_review` (low confidence: everything else)

**Rationale:** Two clear categories: "probably right" vs "needs human judgment". Early morning threshold (0-4 AM) minimizes false positives.

---

### Why No Auto-Merge for Consolidated Events?

**Decision:** When a consolidated canonical event is flagged as a near-duplicate, it is sent to the review queue rather than auto-merged.

**Rationale:** Admin explicitly chose this event as canonical — auto-merging would silently override that decision.

---

### Why Expire Reviews After Event Passes?

**Decision:** Delete rejected reviews 7 days after event ends. Delete pending reviews when event starts.

**Rationale:** No value in fixing past events. Database hygiene for *future* events only.

---

### Why Hybrid Status (pending_review + queue)?

**Decision:** Use `lifecycle_state='pending_review'` in events table, separate from `draft` and `published`.

- `draft` = intentionally unpublished (user choice)
- `pending_review` = system-flagged for quality issues
- `published` = live and approved
- `deleted` = tombstoned

---

## Architecture

### Data Flow

```
Submission → Normalization → Validation → Deduplication → Storage → Review (if needed)
```

### Components

1. **Normalization** (`internal/domain/events/normalize.go`) — Always corrects reversed dates by adding 24 hours.
2. **Validation** (`internal/domain/events/validation.go`) — Compares original vs normalized input, generates warnings.
3. **Ingestion** (`internal/domain/events/ingest.go`) — Coordinates normalization → validation → storage.
4. **Review Queue** (`event_review_queue` table) — Stores original payload for admin inspection.
5. **Admin API** — Approve/reject/fix/merge/add-occurrence/consolidate actions.

---

## 1. State Machine Diagram

```
                    ┌──────────────────────────────────────────────────────────┐
                    │                     INGESTION                            │
                    │  Normalize → Validate → Needs review? ───No──→ published │
                    │       │                                    │             │
                    │       Yes                                   │             │
                    │       ▼                                     │             │
                    │  ┌─────────┐                                │             │
                    │  │  PENDING │◄── resubmit (still broken) ───┘             │
                    │  └────┬─────┘                                              │
                    │       │                                                   │
                    └───────┼───────────────────────────────────────────────────┘
                            │
            ┌───────────────┼───────────────────────────────┐
            │               │           ADMIN ACTIONS       │
            │               │                               │
    ┌───────▼──────┐  ┌─────▼──────┐  ┌──────────────┐     │
    │              │  │            │  │              │      │
    │  APPROVED    │  │  REJECTED  │  │   MERGED     │      │
    │              │  │            │  │              │      │
    └──────┬───────┘  └─────┬──────┘  └──────┬───────┘      │
           │                 │                 │             │
           │  approve        │  reject         │  merge      │
           │  fix             │                 │  add-occ    │
           │  not-a-dup       │                 │             │
           │                 │                 │             │
    ┌──────▼──────┐         │          ┌──────▼───────┐      │
    │  event →    │         │          │  source      │      │
    │  published  │         │          │  event →     │      │
    │             │         │          │  soft-delete │      │
    └─────────────┘         │          │  (absorbed   │      │
                             │          │   or merged) │      │
    ┌─────────────┐         │          └──────────────┘      │
    │  companion  │         │                                │
    │  recheck    │         │   ┌──────────────────┐         │
    │  (not-a-dup │         │   │  DISMISSED        │        │
    │   path)     │         │   │  (consolidation   │        │
    └─────────────┘         │   │   retire list)    │        │
                             │   └──────────────────┘        │
                    ┌───────▼──────┐                         │
                    │  event →     │                         │
                    │  soft-delete │                         │
                    │  (deleted)   │                         │
                    └──────────────┘                         │

                    ┌──────────────────┐
                    │  CLEANUP JOB     │
                    │  Daily River job │
                    │                  │
                    │  rejected:       │
                    │    DELETE after  │
                    │    event end +7d │
                    │                  │
                    │  pending:        │
                    │    event →       │
                    │    deleted,      │
                    │    DELETE after  │
                    │    event start   │
                    │                  │
                    │  approved/       │
                    │  superseded:     │
                    │    DELETE after  │
                    │    90 days       │
                    └──────────────────┘
```

**5 Review Statuses:** `pending`, `approved`, `rejected`, `merged`, `dismissed`

- `pending` — initial state at ingest (`ReviewQueueCreateParams`; `admin_service.go:2170` for consolidate create-path)
- `approved` — set by `ApproveReview` (`events_repository.go:2838`), invoked by `ApproveEventWithReview` (`admin_service.go:1294`) and `FixAndApproveEventWithReview` (`admin_service.go:1616`)
- `rejected` — set by `RejectReview` (`events_repository.go:2862`), invoked by `RejectEventWithReview` (`admin_service.go:1493`)
- `merged` — set by `MergeReview` (`events_repository.go:2885`, line 2900), invoked by `MergeEventsWithReview` (`admin_service.go:1094`), `AddOccurrenceFromReview` (line 490), `AddOccurrenceFromReviewNearDup` (line 844)
- `dismissed` — set by `DismissPendingReviewsByEventULIDs` (`events_repository.go:1557`, line 1562), invoked by consolidation retire logic (`admin_service.go:1863`)

**Action Triggers:**

| Trigger | Method | admin_service.go |
|---------|--------|:---------------:|
| approve | `ApproveEventWithReview` | :1220 |
| approve (not-a-dup) | `ApproveEventWithReview` + `recordNotDuplicatesFromWarnings` | :1220 + handler :347 |
| reject | `RejectEventWithReview` | :1429 |
| fix | `FixAndApproveEventWithReview` | :1513 |
| merge | `MergeEventsWithReview` | :1017 |
| add-occurrence (forward) | `AddOccurrenceFromReview` | :216 |
| add-occurrence (near-dup) | `AddOccurrenceFromReviewNearDup` | :574 |
| consolidate | `Consolidate` | :1640 |
| cleanup (job) | `ReviewQueueCleanupWorker.Work` | `cleanup_review_queue.go:37` |

---

## 2. Decision Table

> **"Companion review"** = the cross-linked review row on the counterpart event created during near-duplicate ingest (the row linked via `duplicate_of_event_id`). Not all actions have a companion review row.

| Action | Source event effect | Target/canonical effect | Companion review handling | Review status set | Recompute? | Notes |
|--------|--------------------|-------------------------|---------------------------|-------------------|------------|-------|
| **approve** | `lifecycle_state` → `published` (if not already; `admin_service.go:1283-1291`) | N/A (same event) | Companion dismissed via `approveStripCompanionDupWarning` if `duplicate_of_event_id` is set (`:1299-1311`) | `approved` | No | Companion strip uses atomic SQL (`DismissAllCompanionWarnings`) covering all 3 warning types + `duplicate_of_event_id` clear. |
| **approve (not-a-dup)** | Same as approve | N/A | `recordNotDuplicatesFromWarnings` in handler (`admin_review_queue.go:347-349`) records not-duplicate pairs, then `recheckCompanionNotDuplicateReview` (`:1075`) strips dup warnings and auto-approves the companion if no other warnings remain | `approved` | No | Best-effort: errors logged not propagated. Key path: `filterPotentialDuplicateWarning` (`:1168`) strips specific ULID from matches array. |
| **reject** | Soft-deleted (`lifecycle_state='deleted'`, tombstone created; `admin_service.go:1463-1490`) | N/A | Handler calls `recordNotDuplicatesFromWarnings` (`admin_review_queue.go:468`) post-transaction — records not-duplicate pairs but does NOT dismiss companion reviews | `rejected` | No | Companion reviews left as-is. If the rejected event had a companion row linked via `duplicate_of_event_id`, that row persists with a stale reference to a now-deleted event. |
| **fix** | Occurrence dates corrected; then `lifecycle_state` → `published` (`admin_service.go:1598-1613`) | N/A | Not touched — no companion lookup | `approved` | No | Handles both events with no occurrences (creates one; `:1550-1577`) and events with existing occurrences (updates them; `:1579-1601`). |
| **merge** | Soft-deleted (`lifecycle_state='deleted'`, tombstone reason `duplicate_merged`, `superseded_by` → primary URI; `admin_service.go:1177-1201`) | Primary enriched with gap-filling data from duplicate (`:1163-1174`). Lifecycle state NOT modified. | Companion review on primary is looked up, locked, and dismissed via `MergeReview` if pending (`:1052-1091`) | `merged` | Yes (via `recomputeLifecycleAfterReview`) | Recompute on primary via shared helper. Companion dismissal passes `primaryULID` not `duplicateULID` to `MergeReview` (`:1083`). |
| **add-occurrence (forward — `potential_duplicate`)** | Soft-deleted (`lifecycle_state='deleted'`, tombstone reason `absorbed_as_occurrence`, `superseded_by` → target URI; `:452-487`). Occurrences deleted (`:460-462`). | New occurrence created on target from source event's sole occurrence (`:434-450`) | Companion review on target is located via `GetPendingReviewByEventUlidAndDuplicateUlid(targetULID, review.EventULID)`, locked, and dismissed via `MergeReview` if pending (`:266-510`) | `merged` | Yes (via `recomputeLifecycleAfterReview`) | Target lifecycle recomputed via shared helper. Source event must have exactly 1 occurrence (`:359-366`). Dispatch path validated from locked warnings (`:244-256`). |
| **add-occurrence (near-dup — `near_duplicate_of_new_event`)** | Source = `review.DuplicateOfEventULID` (new event), soft-deleted with `absorbed_as_occurrence` tombstone (`:792-825`). Target = `review.EventULID` (existing series event). | Occurrence added to target from source event's sole occurrence (`:774-789`) | Companion review on source event is located via `GetPendingReviewByEventUlidAndDuplicateUlid(sourceULID, targetULID)`, locked, and dismissed via `MergeReview` if pending (`:628-646`, `:829-838`) | `merged` | Yes (via `recomputeLifecycleAfterReview`) | Reversed semantics from forward path. Target lifecycle recomputed via shared helper. Source dispatched from `DuplicateOfEventULID` field (`:605-608`). |
| **consolidate** | Each retired event: soft-deleted (`lifecycle_state='deleted'`, tombstone reason `consolidated`, `superseded_by` → canonical URI; `:1833-1858`). All pending reviews dismissed via `DismissPendingReviewsByEventULIDs` → `dismissed` (`:1862-1866`) | Canonical: promoted (`event_ulid`) or created (full ingest pipeline). Cross-week series companion re-synced (`consolidateSyncSeriesCompanion`; `:2039-2126`). Retired dup warnings stripped from canonical's existing review entry via atomic SQL (`StripRetiredDupWarnings`) then companion replacement in Go (`consolidateStripRetiredDupWarnings`; `:1879-1941`) | Retired events' review entries batch-dismissed to `dismissed` (`:1862-1866`). Canonical's companion reviews have retired-warning entries stripped via atomic SQL then companion replacement in Go (`:1895-1922`) | Retired entries → `dismissed`; Canonical → `pending` or `published` depending on warnings | Yes (via `recomputeLifecycleAfterReview`) | See Consolidation Decision Table below. Retired events MUST not include the canonical ULID (`:1569-1574`). If `transferOccurrences=true`, occurrences copied to canonical before retire (`consolidateRetireEvents`; `:1773-1869`). Recompute now runs in Step 8, checking for any pre-existing pending reviews that were not dismissed. |
| **cleanup (job)** | Events with expired pending reviews: `lifecycle_state` → `deleted` (`cleanup_review_queue.go:142-148`) then review row deleted (`:162-164`) | N/A | N/A (rows are physically deleted) | Row deleted | No | Runs daily via River (`:17`; `JobKindReviewQueueCleanup`). Three phases: rejected (event end +7d → delete; `:119-135`), pending past start (event deleted + row deleted; `:140-168`), archived (`approved`/`superseded` → delete after 90d; `:171-184`). |

---

## 3. Consolidation Decision Table

The `Consolidate` method (`admin_service.go:1560`) is the atomic resolution path for N duplicate events into a single canonical.

### Dispatch Paths

| Path | Condition | How | Canonical lifecycle |
|------|-----------|-----|---------------------|
| **Promote** | `event_ulid` provided, no `event` | Existing event locked, re-read, promoted as canonical. Post-promotion dedup checks run (Layer 2 near-dupes + cross-week series). | Unchanged unless warnings generated → `pending_review` |
| **Promote + Patch** | Both `event_ulid` and `event` provided | Existing event locked; patchable fields (`name`, `description`, `url`, `image`, `keywords`, `eventDomain`) applied atomically (`:1724-1737`). Canonical re-read post-patch, then dedup checks run. | Unchanged unless warnings generated → `pending_review` |
| **Create** | `event` provided, no `event_ulid` | Full ingest pipeline: normalize → validate → dedup via `createEventCore` (`:1765-1769`). `SkipDedupAutoMerge=true`, `ExcludeFromNearDup=retire list`. | `published` unless warnings → `pending_review` |

### Retire Processing

Each retired event (`consolidateRetireEvents`, `admin_service.go:1773-1869`):
1. If `transferOccurrences=true`: each occurrence copied to canonical after overlap check (`:1792-1831`)
2. Soft-deleted with `lifecycle_state='deleted'`, tombstone reason `"consolidated"`, `supersededBy` → canonical URI (`:1833-1858`)
3. All occurrence rows cleaned up (`:1838`)
4. All pending review queue entries dismissed → `dismissed` via `DismissPendingReviewsByEventULIDs` (`:1862-1866`)

### When consolidateStripRetiredDupWarnings Triggers

`consolidateStripRetiredDupWarnings` (`admin_service.go:1879`) runs on every consolidate. It:
1. Looks up the canonical's pending review entry via `GetPendingReviewByEventUlid` (`:1887`)
2. Delegates to `StripRetiredDupWarnings` (`:1895`) — atomic SQL that strips `near_duplicate_of_new_event`, `potential_duplicate`, and `cross_week_series_companion` warnings whose matches/companion ULIDs are in the retire set; also clears `duplicate_of_event_id` if it pointed to a retired event
3. If a replacement companion exists, re-reads the entry and replaces stale cross-week companion warnings in Go (`:1900-1922`)
4. If **all** warnings stripped → dismisses the review entry (`MergeReview`, `:1925`) and publishes the canonical (`:1930-1931`)
5. If **some** warnings remain → returns (the atomic SQL already persisted the pruned warnings)

### Series Companion Cross-Linking

`consolidateSyncSeriesCompanion` (`admin_service.go:2039`) re-runs cross-week companion detection against the post-retirement event graph:
- Finds a surviving companion (not in the retire list) via `FindSeriesCompanion` (`:2049`)
- Creates a `cross_week_series_companion` warning on the companion event's review entry
- If the companion was `published`, flips it to `pending_review` and creates a new review entry (`:2100-2123`)
- If the companion already had a pending review, updates its warnings with the new cross-week warning (`:2084-2092`)

---

## 4. Post-Action Recomputation

### Which Actions Trigger Recompute

| Action | Recomputes? | Location |
|--------|:-----------:|----------|
| **AddOccurrenceFromReview** (forward) | Yes | `admin_service.go` via `recomputeLifecycleAfterReview` |
| **AddOccurrenceFromReviewNearDup** (near-dup) | Yes | `admin_service.go` via `recomputeLifecycleAfterReview` |
| **MergeEventsWithReview** | Yes | `admin_service.go` via `recomputeLifecycleAfterReview` |
| **Consolidate** | Yes | `admin_service.go` via `recomputeLifecycleAfterReview` (Step 8) |
| **ApproveEventWithReview** | No (sets `published` directly) | `admin_service.go:1283-1291` |
| **RejectEventWithReview** | No (soft-deletes directly) | `admin_service.go:1463` |
| **FixAndApproveEventWithReview** | No (sets `published` directly) | `admin_service.go:1605-1613` |

### The Recompute Helper

The recompute logic is centralised in a single shared method:

```go
func (s *AdminService) recomputeLifecycleAfterReview(ctx, txRepo, eventULID, event) error
```

**Location:** `admin_service.go` (declared before `AddOccurrenceFromReview`).

Previously copy-pasted 3 times, now called from 4 locations (the 3 original sites plus Consolidate).

The helper checks `GetPendingReviewByEventUlid` — if it returns `nil` (no pending reviews remain for the event), lifecycle is promoted to `published`. If other pending reviews still exist, lifecycle stays `pending_review`.

### Consolidate Recompute (Added)

`Consolidate` (`admin_service.go:1560`) now calls `recomputeLifecycleAfterReview` in Step 8 (after `consolidateStripRetiredDupWarnings` and `consolidateSyncSeriesCompanion`). This closes the gap where the canonical could remain `pending_review` with stale pending reviews after consolidation. The recompute accounts for any pre-existing pending reviews that were not dismissed by the consolidation steps.

---

## 5. Companion Dismissal Mechanisms

Three different mechanisms handle companion review cleanup, each with different scope and guarantees:

### Mechanism 1: MergeReview — Full Row Dismissal

**SQL:** `UPDATE event_review_queue SET status = 'merged' ... WHERE id = $1 AND status = 'pending'` (`events_repository.go:2899-2912`)

**Scope:** Sets the entire review row to `merged` status in a single atomic UPDATE. Also sets `duplicate_of_event_id` and `review_notes`.

**Used by:**
- `AddOccurrenceFromReview` (forward) — dismisses companion on target (`admin_service.go:501`)
- `AddOccurrenceFromReviewNearDup` (near-dup) — dismisses companion on source (`admin_service.go:830`)
- `MergeEventsWithReview` (merge) — dismisses companion on primary (`admin_service.go:1083`)
- `consolidateStripRetiredDupWarnings` — dismisses canonical entry when all warnings stripped (`admin_service.go:1925`)

**Characteristics:** Full dismissal. Atomic. No race window. Handles all warning types (the whole row is resolved).

### Mechanism 2: DismissWarningMatchByReviewID — Atomic JSONB Warning Removal

**SQL:** JSONB manipulation within a single UPDATE (`event_review_queue.sql:306-329`). Removes `potential_duplicate` entries where `m->>'ulid' = eventULID` from the warnings array.

**Scope:** Target exactly one review row by primary key `id`. Only handles `potential_duplicate` warnings.

**Used by:** Ingest-level companion filtering (not directly by admin actions; used in `internal/domain/events/ingest.go`).

### Mechanism 3: approveStripCompanionDupWarning — Atomic SQL via DismissAllCompanionWarnings

**Location:** `admin_service.go` (method body delegates to `DismissAllCompanionWarnings`)

**SQL:** Atomic UPDATE in `event_review_queue.sql` (`DismissAllCompanionWarnings`). Handles all three warning types in a single statement:
- `near_duplicate_of_new_event` — stripped when `duplicate_of_event_id` matches
- `potential_duplicate` — specific match entries filtered; warning nullified when matches empty
- `cross_week_series_companion` — stripped when `details->>'companion_ulid'` matches

**Also clears `duplicate_of_event_id`** if it points at the just-approved event (within the same UPDATE).

**Returns** `warnings_empty` (bool) so the caller can decide whether to approve.

**Two outcomes:**
1. **All warnings stripped** → caller dismisses the companion entry via `ApproveReview` and publishes companion event
2. **Some warnings remain** → the atomic UPDATE already handled pruning and `duplicate_of_event_id` clear; caller returns

**Used by:** `ApproveEventWithReview` (`admin_service.go:1304`)

**Previously:** Go-level JSON unmarshal → filter → marshal → write cycle with silent-failure risk on Unmarshal error. Now: single atomic SQL UPDATE — no race window, no marshalling risk.

### Which Actions Use Which Mechanism

| Action | Mechanism | Location |
|--------|-----------|----------|
| approve (companion strip) | Mechanism 3 (approveStripCompanionDupWarning) | `admin_service.go:1304` |
| approve (not-a-dup, companion recheck) | Go-level filtering (`filteredCompanionWarnings` → `filterPotentialDuplicateWarning` → `UpdateReviewWarnings`) | `admin_review_queue.go:1094-1111` |
| merge (companion dismiss) | Mechanism 1 (MergeReview) | `admin_service.go:1083` |
| add-occurrence forward (companion dismiss) | Mechanism 1 (MergeReview) | `admin_service.go:501` |
| add-occurrence near-dup (companion dismiss) | Mechanism 1 (MergeReview) | `admin_service.go:830` |
| consolidate (retire-list reviews) | `DismissPendingReviewsByEventULIDs` → `dismissed` | `admin_service.go:1863` |
| consolidate (canonical strip) | Atomic SQL (`StripRetiredDupWarnings`) + Go companion replacement (`consolidateStripRetiredDupWarnings`) | `admin_service.go:1879` |

### All Three Warning Types Now Handled by Atomic SQL

`DismissAllCompanionWarnings` (`event_review_queue.sql`) handles all three warning types in a single atomic UPDATE:
1. **`near_duplicate_of_new_event`** — stripped when `duplicate_of_event_id` matches the target event
2. **`potential_duplicate`** — specific match entries with matching `ulid` are filtered from the matches array; warning nullified when no matches remain
3. **`cross_week_series_companion`** — stripped when `details->>'companion_ulid'` matches the target event

`DismissCompanionWarningMatch` and `DismissWarningMatchByReviewID` remain for lower-level `potential_duplicate`-only operations in ingest.

---

## 6. Known Gaps and Technical Debt

### G1: `dismissed` Status Not Covered by CleanupArchivedReviews

**Severity:** Low (eventual table bloat) — **FIXED**

Both the SQLc query `CleanupArchivedReviews` and the River job's raw SQL now include `dismissed` and `merged` in their status filters.

### G2: Recompute Logic Centralised

**Status:** FIXED. The recompute pattern has been extracted into a shared `recomputeLifecycleAfterReview` method on `AdminService`.

### G3: Companion Dismissal Duplicated 4+ Times

Companion review lookup + lock + dismiss logic is repeated across:
- `admin_service.go:266-283` + `:500-510` (AddOccurrenceFromReview)
- `admin_service.go:628-646` + `:829-838` (AddOccurrenceFromReviewNearDup)
- `admin_service.go:1052-1071` + `:1082-1091` (MergeEventsWithReview)
- `admin_service.go:1257-1275` + `:1299-1311` (ApproveEventWithReview)

Each copy follows the same pattern: `GetPendingReviewByEventUlidAndDuplicateUlid` → `LockReviewQueueEntryForUpdate` → `MergeReview` (or `approveStripCompanionDupWarning`). Should be extracted to a shared method.

### G4: Consolidate Now Recomputes

**Status:** FIXED. `Consolidate` (`admin_service.go:1560`) now calls `recomputeLifecycleAfterReview` in Step 8 after `consolidateStripRetiredDupWarnings` and `consolidateSyncSeriesCompanion`. This ensures the canonical event's lifecycle accounts for any pre-existing pending reviews that were not dismissed by the consolidation steps.

### G5: OpenAPI Spec Now Includes `dismissed`

**Status:** FIXED. The `event_review_queue.status` field in `docs/api/openapi.yaml` now includes `dismissed` in its enum.

### G6: All Warning Types Now Handled by Atomic SQL

`approveStripCompanionDupWarning` now delegates to `DismissAllCompanionWarnings` — a single atomic SQL UPDATE that handles all three warning types (`near_duplicate_of_new_event`, `potential_duplicate`, `cross_week_series_companion`) via JSONB manipulation. No Go-level read-modify-write.

### G7: Approve Companion Strip is Now Atomic

**Status:** FIXED. `approveStripCompanionDupWarning` no longer performs Go-level JSON parsing. It delegates to `DismissAllCompanionWarnings` which atomically strips warnings and clears `duplicate_of_event_id` in a single SQL UPDATE. Silent-failure risk from JSON unmarshal errors is eliminated.

### G8: Reject Leaves Companion Reviews Orphaned

`RejectEventWithReview` (`admin_service.go:1429`) does not look up or dismiss companion reviews. The handler's `recordNotDuplicatesFromWarnings` (`admin_review_queue.go:468`) records not-duplicate pairs but does not dismiss the companion review rows. If a rejected event had a companion cross-linked via `duplicate_of_event_id`, that companion persists with a stale reference.

---

## 7. Lifecycle State Transitions

### Event Lifecycle Transitions

| From | To | Trigger | Location |
|------|----|---------|----------|
| `pending_review` | `published` | approve | `admin_service.go:1283-1291` |
| `pending_review` | `published` | fix | `admin_service.go:1605-1613` |
| `pending_review` | `published` | recompute (no remaining reviews) | `admin_service.go` via `recomputeLifecycleAfterReview` |
| `pending_review` | `deleted` | reject | `admin_service.go:1463` |
| `pending_review` | `deleted` | merge (source event) | `admin_service.go:1177` |
| `pending_review` | `deleted` | add-occurrence (absorbed event) | `admin_service.go:452` |
| `pending_review` | `deleted` | consolidate (retired event) | `admin_service.go:1833` |
| `pending_review` | `deleted` | cleanup job (unreviewed past event) | `cleanup_review_queue.go:142-148` |
| `pending_review` | `pending_review` | fix (with remaining review issues — not applicable; fix always publishes) | — |
| `pending_review` | `pending_review` | consolidate with needsReview=true | `admin_service.go:1970` |
| `published` | `pending_review` | new review created (near-dup companion back-link) | `ingest.go` (via near-dup cross-linking) |
| `published` | `pending_review` | consolidate companion re-sync (cross-week series) | `admin_service.go:2100-2106` |
| `published` | `deleted` | delete (admin) | `admin_service.go:2411-2456` |
| `any` | `deleted` | consolidate retire list | `admin_service.go:1833` |

### Review Queue Status Transitions

| From | To | Trigger | Location |
|------|----|---------|----------|
| `pending` | `approved` | ApproveReview | `events_repository.go:2838` |
| `pending` | `rejected` | RejectReview | `events_repository.go:2862` |
| `pending` | `merged` | MergeReview | `events_repository.go:2900` |
| `pending` | `dismissed` | DismissPendingReviewsByEventULIDs | `events_repository.go:1562` |
| `pending` | (deleted) | cleanup job — pending past event start | `cleanup_review_queue.go:162-164` |
| `rejected` | (deleted) | cleanup job — event end + 7 days | `cleanup_review_queue.go:121-127` |
| `approved` | (deleted) | cleanup job — 90 days after reviewed_at | `cleanup_review_queue.go:175` |
| `superseded` | (deleted) | cleanup job — 90 days after reviewed_at | `cleanup_review_queue.go:175` |

---

## Database Schema

### `event_review_queue` Table

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
  status TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, merged, dismissed
  reviewed_by TEXT,
  reviewed_at TIMESTAMPTZ,
  review_notes TEXT,
  rejection_reason TEXT,

  -- Near-duplicate cross-linking
  duplicate_of_event_id TEXT REFERENCES events(id) ON DELETE SET NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  CONSTRAINT fk_event FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
);
```

### `events` Lifecycle State

```sql
ALTER TABLE events ADD CONSTRAINT events_lifecycle_state_check
  CHECK (lifecycle_state IN ('draft', 'published', 'deleted', 'pending_review'));
```

- `draft` = User-created, intentionally unpublished
- `pending_review` = System-flagged for data quality review
- `published` = Approved and live
- `deleted` = Tombstoned

---

## Warning Codes

| Code | Confidence | Description |
|------|-----------|-------------|
| `reversed_dates_timezone_likely` | High | End time 0-4 AM, duration < 7h after correction. Likely overnight event with timezone error. |
| `reversed_dates_corrected_needs_review` | Low | Reversed dates corrected but doesn't match high-confidence pattern. |
| `multi_session_likely` | — | Event appears to be a multi-session course or recurring series. |
| `potential_duplicate` | — | pg_trgm near-duplicate match against existing event at same venue, same date. |
| `near_duplicate_of_new_event` | — | Companion warning on the *existing* event during near-dup cross-linking. References the newly ingested event. |
| `cross_week_series_companion` | — | Same venue, same name, 7-21 days apart, same time-of-day (±30 min). |
| `exact_duplicate` | — | Layer 1 dedup hash match (generated during consolidate create path only). |

---

## Near-Duplicate Cross-Linking

When Layer 2 (pg_trgm) near-duplicate detection matches a new event against one or more existing published events, both sides of the match enter the review queue:

1. **New event** — stored with `lifecycle_state = 'pending_review'`. Its review queue entry has `duplicate_of_event_id` pointing to the first matched existing event.
2. **Each matched existing event** — if currently `published`, its `lifecycle_state` is set to `'pending_review'` and a new review queue entry is created, with `duplicate_of_event_id` pointing to the new event.

**Timing:** The existing-event updates happen after the transaction commits (for FK safety). Failures are non-critical — logged and skipped.

**Scope:** Only Layer 2 (near-duplicate) matches trigger this behaviour. Layer 1 exact dedup-hash matches still auto-merge as before.

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

---

## Admin Review API

The `server review` CLI wraps these endpoints for efficient automated review (see `docs/integration/tg-review.md`).

### GET /api/v1/admin/review-queue
List events pending review. Query params: `status` (default: pending), `limit` (1-100), `cursor`.

### GET /api/v1/admin/review-queue/:id
Get detailed review including original vs corrected data, related events.

### POST /api/v1/admin/review-queue/:id/approve
Approve the auto-correction. Publishes event, sets review status to `approved`. Optional `record_not_duplicates: true` triggers the not-a-duplicate companion reconciliation path.

### POST /api/v1/admin/review-queue/:id/reject
Reject the event. Soft-deletes event with tombstone, sets review status to `rejected`. Requires `reason`.

### POST /api/v1/admin/review-queue/:id/fix
Manually correct occurrence dates. Applies corrections, publishes event, sets review status to `approved`.

### POST /api/v1/admin/events/consolidate
Atomically resolve N duplicate events into a single canonical event. See Consolidation Decision Table (Section 3).

---

## Cleanup & Maintenance

### Background Job: `ReviewQueueCleanupWorker`

Runs daily via River (`cleanup_review_queue.go:37`). Three phases:

1. **Expired rejections** (`:119-135`): DELETE `event_review_queue` WHERE `status = 'rejected'` AND (`event_end_time < NOW() - 7 days` OR event already passed).

2. **Unreviewed past events** (`:140-168`): UPDATE `events` SET `lifecycle_state = 'deleted'` WHERE pending review with `event_start_time < NOW()`. Then DELETE the review row.

3. **Archived reviews** (`:171-184`): DELETE `event_review_queue` WHERE `status IN ('approved', 'superseded')` AND `reviewed_at < NOW() - 90 days`.

**Gap:** `merged` and `dismissed` statuses are not cleaned up by the running River job (see Known Gap G1).

---

## Implementation Tasks

See related beads for implementation details.

---

## Related Documentation

- `docs/interop/core-profile-v0.1.md` - SEL Core Interoperability Profile
- `docs/api/API_CONTRACT_v1.md` - API contract specifications
- `internal/domain/events/admin_service.go` - All state machine logic
- `internal/domain/events/repository.go` - Repository interface + ReviewQueueEntry struct
- `internal/domain/events/normalize.go` - Normalization logic
- `internal/domain/events/validation.go` - Validation logic
- `internal/storage/postgres/events_repository.go` - MergeReview, DismissPendingReviewsByEventULIDs, CleanupExpiredReviews
- `internal/storage/postgres/queries/event_review_queue.sql` - SQLc queries for review queue
- `internal/api/handlers/admin_review_queue.go` - Admin review queue HTTP handlers
- `internal/jobs/cleanup_review_queue.go` - Periodic cleanup River job
- `cmd/server/cmd/review.go` - tg-review CLI entrypoint
- `docs/integration/tg-review.md` - tg-review CLI reference

---

**Last Updated:** 2026-06-03
