# Phase 1 Specification: Curator Protection Core

**Spec**: 006-curator-protection / Phase 1 | **Date**: 2026-04-16 | **Status**: Draft
**Parent**: specs/006-curator-protection/plan.md
**Goal**: Curated events and tombstoned events are protected from automatic scraper overwrites. Identical re-submissions are short-circuited. Source updates on curated events route to the existing review queue.

## Context

### What Exists Today

| Component | Status | Location |
|---|---|---|
| `Event` struct | Has `LifecycleState` | `repository.go:61-93` |
| `Event.LastCuratedAt` | **Missing** | — |
| Layer 1: source_event_id auto-merge | Works, no curation check | `create_event_core.go:139-173` |
| Layer 2: dedup_hash auto-merge | Works, no curation check | `create_event_core.go:175-234` |
| `AutoMergeFields` | Works | `merge.go:15-37` |
| `FindBySourceExternalID` | Returns deleted events (no lifecycle filter) | `events_repository.go:825-864` |
| `FindByDedupHash` | Returns deleted events (no lifecycle filter) | `events_repository.go:892-930` |
| `event_sources.payload_hash` | Written, indexed, **never queried for dedup** | `provenance.up.sql:55,64` |
| `event_tombstones` | Created on consolidate/delete, **never queried during ingest** | `000003_federation.up.sql:80-91` |
| `Tombstone.SupersededBy` | Set for consolidations, NULL for admin deletions | `repository.go:399-408` |
| Review queue `potential_duplicate` | Full workflow: create, approve, merge, reject | `admin_service.go`, review queue SQL |
| `ErrPreviouslyRejected` | Exists | `errors.go:8-19` |
| Review queue entry creation | Creates `pending_review` event + review entry in transaction | `create_event_core.go:750-803` |

### What This Phase Delivers

1. **Payload hash already-seen check** — short-circuit ingest when identical payload was previously ingested (any source)
2. **`last_curated_at` column on events** — timestamp set by meaningful admin actions
3. **Curator guard in Layer 1 and Layer 2** — curated events route to review instead of auto-merge
4. **Tombstone guard in Layer 1 and Layer 2** — deleted events reject or redirect; consolidated events follow redirect to canonical
5. **Shared `handleExistingMatch` function** — DRY guard logic called from both layers
6. **New error types** — `ErrPreviouslyDeleted`, `ErrAlreadySeen`

### Non-Goals

- **New review queue entry type** (`source_update`) — deferred. Curated updates use existing `potential_duplicate` path.
- **Per-field curation tracking** — deferred to Phase 3 if needed.
- **Permanent rejection flag** — deferred. Separate concern (spam sources).
- **`event_changes.user_id` tracking** — deferred. Useful but orthogonal.
- **Review queue diff view** — Phase 2.
- **Batch payload hash check** — optimization deferred until throughput is a concern.
- **Admin "un-protect" endpoint** — deferred. `UPDATE events SET last_curated_at = NULL` is the escape hatch.

### Design Constraint Reminders

- RFC 7807 error envelopes on all error responses
- `fmt.Errorf("...: %w", err)` for error wrapping
- HTTP handlers stay thin; logic in `internal/domain/`
- SQLc for standard queries; raw SQL only when structure varies at runtime
- CC0 license defaults; preserve source provenance

## User Scenarios & Testing

### User Story 1 — Identical Payload Short-Circuit (Priority: P0)

**Independent Test**: Ingest an event, then re-submit the exact same payload.

**Acceptance Scenarios**:

1. **Given** an event was previously ingested from Source A with payload hash `abc123`,
   **when** Source A (or any source) submits a payload that hashes to `abc123`,
   **then** ingest returns immediately with `ErrAlreadySeen` and no DB writes occur beyond the hash lookup.

2. **Given** an event was previously ingested with payload hash `abc123`,
   **when** Source A submits a payload that hashes to `def456` (different content),
   **then** ingest continues to Layer 1/Layer 2 matching (no short-circuit).

3. **Given** an event was ingested and then admin-deleted (tombstoned),
   **when** the same payload (same hash) is resubmitted,
   **then** ingest returns `ErrAlreadySeen` (the payload_hash row still exists in `event_sources`).

### User Story 2 — Curated Event Protection (Priority: P0)

**Independent Test**: Admin edits an event, then scraper resubmits with different data.

**Acceptance Scenarios**:

1. **Given** an event exists with `last_curated_at = 2026-04-15T10:00:00Z`,
   **when** a scraper submits a different payload matching the same `source_event_id`,
   **then** a new `pending_review` event is created with a `potential_duplicate` warning referencing the curated event, and the curated event is **not modified**.

2. **Given** an event exists with `last_curated_at = NULL` (never curated),
   **when** a scraper submits a different payload matching the same `source_event_id`,
   **then** auto-merge proceeds as today (trust comparison, field updates).

3. **Given** an event exists with `last_curated_at = 2026-04-15T10:00:00Z`,
   **when** a scraper submits a different payload matching the same `dedup_hash`,
   **then** a new `pending_review` event is created with a `potential_duplicate` warning referencing the curated event, and the curated event is **not modified**.

### User Story 3 — Tombstone Blocking (Priority: P1)

**Independent Test**: Admin deletes or consolidates an event, then scraper resubmits.

**Acceptance Scenarios**:

1. **Given** an event was admin-deleted (tombstone with `superseded_by_uri IS NULL`),
   **when** a scraper submits a payload matching the deleted event's `source_event_id`,
   **then** ingest returns `ErrPreviouslyDeleted` with the tombstone reason and deletion time.

2. **Given** an event was consolidated into canonical event C (tombstone with `superseded_by_uri = C`),
   **when** a scraper submits a payload matching the consolidated event's `source_event_id`,
   **then** the guard redirects to canonical event C and applies the match logic against C (curated check, auto-merge, etc.).

3. **Given** an event was admin-deleted,
   **when** a scraper submits a payload matching the deleted event's `dedup_hash`,
   **then** ingest returns `ErrPreviouslyDeleted`.

4. **Given** canonical event C is also tombstoned (chain),
   **when** a redirect from a consolidated event reaches C,
   **then** ingest returns an error (no recursive redirect, max depth = 1).

### User Story 4 — Admin Actions Set `last_curated_at` (Priority: P0)

**Independent Test**: Perform each admin action and verify `last_curated_at` state.

**Acceptance Scenarios**:

1. **Given** an event with `last_curated_at = NULL`,
   **when** `AdminService.UpdateEvent` is called with field changes,
   **then** `last_curated_at` is set to the current time.

2. **Given** an event with `last_curated_at = NULL`,
   **when** `AdminService.Consolidate` is called and this event is the canonical,
   **then** `last_curated_at` is set on the canonical event.

3. **Given** an event with `last_curated_at = NULL`,
   **when** `AdminService.FixAndApproveEventWithReview` is called,
   **then** `last_curated_at` is set to the current time.

4. **Given** an event with `last_curated_at = NULL`,
   **when** `AdminService.MergeEventsWithReview` is called,
   **then** `last_curated_at` is set on the surviving event.

5. **Given** an event with `last_curated_at = NULL`,
   **when** `AdminService.ApproveEventWithReview` is called **without** field edits,
   **then** `last_curated_at` remains NULL.

6. **Given** an event with `last_curated_at = NULL`,
   **when** `AdminService.PublishEvent`, `UnpublishEvent`, `CreateOccurrenceOnEvent`, `AddOccurrenceFromReview`, or `FixEventOccurrenceDates` is called,
   **then** `last_curated_at` remains NULL.

## Technical Design

### Migration

Single migration file. Pre-launch system, so no data concerns.

```sql
-- 000028_curator_protection.up.sql

-- Track when an event was last curated by an admin.
-- NULL = never curated (scraper auto-merge allowed).
-- Non-NULL = curated (scraper updates route to review queue).
ALTER TABLE events ADD COLUMN last_curated_at TIMESTAMPTZ;

-- Partial index: only curated events need to be found by this column.
-- Used by the guard function to check curation status after Layer 1/2 match.
CREATE INDEX idx_events_curated ON events (last_curated_at)
  WHERE last_curated_at IS NOT NULL;
```

```sql
-- 000028_curator_protection.down.sql
DROP INDEX IF EXISTS idx_events_curated;
ALTER TABLE events DROP COLUMN IF EXISTS last_curated_at;
```

**Note**: Check the next available migration number before creating. The spec uses `000028` as a placeholder.

### Data Structures

**Event struct** — add one field:

```go
// In repository.go, Event struct
type Event struct {
    // ... existing fields through UpdatedAt ...
    PublishedAt    *time.Time
    LastCuratedAt  *time.Time // Non-NULL = human-curated; auto-merge blocked
}
```

**New error types** — in `errors.go`:

```go
// ErrAlreadySeen indicates the exact payload (by hash) was previously ingested.
// This is a fast short-circuit — no further processing needed.
type ErrAlreadySeen struct {
    PayloadHash string
}

func (e ErrAlreadySeen) Error() string {
    return fmt.Sprintf("payload hash %s already ingested, skipping", e.PayloadHash)
}

// ErrPreviouslyDeleted indicates the matched event was admin-deleted.
// The tombstone has no superseded_by — the event is permanently dead.
type ErrPreviouslyDeleted struct {
    EventURI  string
    DeletedAt time.Time
    Reason    string
}

func (e ErrPreviouslyDeleted) Error() string {
    return fmt.Sprintf("event %s was admin-deleted (%s at %s), rejecting",
        e.EventURI, e.Reason, e.DeletedAt)
}
```

### New Repository Method

One new query method. Everything else reuses existing methods.

```go
// In repository.go, Repository interface
type Repository interface {
    // ... existing methods ...

    // PayloadHashExists returns true if any event_sources row has this payload_hash.
    // Used for the already-seen short-circuit in ingest.
    PayloadHashExists(ctx context.Context, payloadHash string) (bool, error)
}
```

**SQLc query** (in `provenance.sql`):

```sql
-- name: PayloadHashExists :one
SELECT EXISTS(
  SELECT 1 FROM event_sources WHERE payload_hash = @payload_hash
) AS exists;
```

### Shared Guard Function

This is the core of the feature. One function, called from both Layer 1 and Layer 2.

```go
// File: create_event_core.go (or a new file: curator_guard.go)

// matchAction describes what the guard decided.
type matchAction int

const (
    matchActionAutoMerge   matchAction = iota // uncurated, proceed with trust-based merge
    matchActionRouteReview                     // curated, route to review as potential_duplicate
    matchActionReject                          // tombstoned (admin-deleted), hard reject
    matchActionRedirect                        // tombstoned (consolidated), follow redirect
)

// matchResult holds the guard decision plus context for the caller.
type matchResult struct {
    Action          matchAction
    RedirectEventID string     // only set when Action == matchActionRedirect
    Err             error      // only set when Action == matchActionReject
}

// evaluateExistingMatch decides what to do when Layer 1 or Layer 2 finds
// an existing event matching the incoming submission.
//
// Decision tree:
//  1. lifecycle_state == "deleted" → look up tombstone
//     a. superseded_by IS NULL (admin-deleted) → matchActionReject
//     b. superseded_by IS NOT NULL (consolidated) → matchActionRedirect
//     c. no tombstone found (inconsistency) → matchActionReject with error
//  2. last_curated_at IS NOT NULL → matchActionRouteReview
//  3. otherwise → matchActionAutoMerge
func (s *IngestService) evaluateExistingMatch(
    ctx context.Context,
    existing *Event,
) (*matchResult, error) {
    if existing.LifecycleState == "deleted" {
        tombstone, err := s.repo.GetTombstoneByEventID(ctx, existing.ID)
        if err != nil {
            return nil, fmt.Errorf("get tombstone for deleted event %s: %w", existing.ULID, err)
        }
        if tombstone.SupersededBy != nil {
            // Consolidated — caller should redirect to canonical
            return &matchResult{
                Action:          matchActionRedirect,
                RedirectEventID: *tombstone.SupersededBy, // This is a URI, needs parsing
            }, nil
        }
        // Admin-deleted — hard reject
        return &matchResult{
            Action: matchActionReject,
            Err: ErrPreviouslyDeleted{
                EventURI:  tombstone.EventURI,
                DeletedAt: tombstone.DeletedAt,
                Reason:    tombstone.Reason,
            },
        }, nil
    }

    if existing.LastCuratedAt != nil {
        return &matchResult{Action: matchActionRouteReview}, nil
    }

    return &matchResult{Action: matchActionAutoMerge}, nil
}
```

### Integration Into Layer 1 (source_event_id match)

Current code (`create_event_core.go:139-173`). Changes are marked with `// NEW`:

```go
existing, err := s.repo.FindBySourceExternalID(ctx, sourceID, validated.Source.EventID)
if err == nil && existing != nil {
    if opts.SkipDedupAutoMerge {
        // Consolidation path — unchanged
        // ...
    } else {
        // Standard ingest path

        // NEW: evaluate the match before auto-merging
        result, err := s.evaluateExistingMatch(ctx, existing)
        if err != nil {
            return nil, fmt.Errorf("evaluate source-external-id match: %w", err)
        }

        switch result.Action {
        case matchActionReject:
            return nil, result.Err

        case matchActionRedirect:
            // Consolidated event — follow redirect to canonical.
            // Parse the superseded_by_uri to get the canonical event ULID,
            // look it up, and re-evaluate against the canonical.
            canonical, err := s.followTombstoneRedirect(ctx, result.RedirectEventID)
            if err != nil {
                return nil, fmt.Errorf("follow tombstone redirect: %w", err)
            }
            // Re-evaluate against canonical (max depth 1 — if canonical
            // is also deleted, we reject rather than recurse)
            result, err = s.evaluateExistingMatch(ctx, canonical)
            if err != nil {
                return nil, fmt.Errorf("evaluate canonical match: %w", err)
            }
            if result.Action == matchActionRedirect {
                return nil, fmt.Errorf("tombstone redirect chain: canonical %s is also consolidated", canonical.ULID)
            }
            // Use canonical as the existing event for merge/review
            existing = canonical
            if result.Action == matchActionReject {
                return nil, result.Err
            }
            if result.Action == matchActionRouteReview {
                // Fall through to create review entry below
                warnings = append(warnings, ValidationWarning{
                    Code:    "curator_protected",
                    Message: fmt.Sprintf("source update for curated event %s (via consolidated redirect)", existing.ULID),
                    Details: map[string]string{
                        "curated_event_ulid": existing.ULID,
                    },
                })
                needsReview = true
                // Don't return — fall through to event creation for review
                break
            }
            // matchActionAutoMerge — merge into canonical
            // (fall through to existing merge logic below, using `existing` = canonical)

        case matchActionRouteReview:
            // Curated event — don't auto-merge, route to review
            warnings = append(warnings, ValidationWarning{
                Code:    "curator_protected",
                Message: fmt.Sprintf("source update for curated event %s — routing to review", existing.ULID),
                Details: map[string]string{
                    "curated_event_ulid": existing.ULID,
                },
            })
            needsReview = true
            // Don't return — fall through to event creation for review
            break

        case matchActionAutoMerge:
            // Uncurated — proceed with existing auto-merge logic
            existingTrust, err := s.repo.GetSourceTrustLevel(ctx, existing.ID)
            if err != nil {
                return nil, fmt.Errorf("get existing source trust: %w", err)
            }
            newTrust, err := s.repo.GetSourceTrustLevelBySourceID(ctx, sourceID)
            if err != nil {
                return nil, fmt.Errorf("get new source trust: %w", err)
            }
            updates, changed := AutoMergeFields(existing, validated, existingTrust, newTrust)
            if changed {
                existing, err = s.repo.UpdateEvent(ctx, existing.ULID, updates)
                if err != nil {
                    return nil, fmt.Errorf("source-external-id auto-merge update: %w", err)
                }
            }
            _ = s.recordSourceForExisting(ctx, existing, validated, sourceID)
            return &CreateEventCoreResult{
                IngestResult: &IngestResult{Event: existing, IsDuplicate: true, IsMerged: changed, Warnings: warnings},
            }, nil
        }
    }
}
```

**Important**: When `matchActionRouteReview` is triggered, we do NOT return early from the source_event_id block. We set `needsReview = true` and let the existing event creation path run — it will create a `pending_review` event and review queue entry with the `curator_protected` warning. The `DuplicateOfEventID` field on the review entry will point to the curated event.

The same pattern applies to Layer 2 (dedup_hash match). The code change is structurally identical — replace the auto-merge block with `evaluateExistingMatch` + switch.

### followTombstoneRedirect Helper

```go
// followTombstoneRedirect resolves a superseded_by_uri to the canonical Event.
// The URI is a full canonical URI like "https://toronto.togather.foundation/events/01JRZX..."
// We parse out the ULID and look up the event.
func (s *IngestService) followTombstoneRedirect(ctx context.Context, supersededByURI string) (*Event, error) {
    // Parse the ULID from the URI.
    // superseded_by_uri format: "https://{domain}/events/{ULID}"
    parsed, err := ids.ParseEntityURI(s.nodeDomain, "events", supersededByURI, "")
    if err != nil {
        // Try treating it as a bare ULID (defensive)
        canonical, lookupErr := s.repo.GetByULID(ctx, supersededByURI)
        if lookupErr != nil {
            return nil, fmt.Errorf("parse superseded_by_uri %q: %w", supersededByURI, err)
        }
        return canonical, nil
    }
    canonical, err := s.repo.GetByULID(ctx, parsed.ULID)
    if err != nil {
        return nil, fmt.Errorf("get canonical event %s: %w", parsed.ULID, err)
    }
    return canonical, nil
}
```

### Payload Hash Already-Seen Check

Inserted as Step 0, before any Layer 1/Layer 2 matching:

```go
// In create_event_core.go, near the top of CreateEventCore, after validation
// and payload hash computation but before source resolution.

// ── 0. Already-seen check (payload hash) ────────────────────────────────
// If this exact payload was previously ingested (from any source), skip.
// This is the cheapest possible short-circuit: single index lookup.
if payloadHash != [32]byte{} {
    hashHex := hex.EncodeToString(payloadHash[:])
    seen, err := s.repo.PayloadHashExists(ctx, hashHex)
    if err != nil {
        return nil, fmt.Errorf("check payload hash: %w", err)
    }
    if seen {
        return nil, ErrAlreadySeen{PayloadHash: hashHex}
    }
}
```

**Where is `payloadHash` computed?** Currently it's computed in `ingest.go:304` inside `recordSourceForExisting`. We need to compute it earlier in the pipeline — at the `CreateEventCore` entry point — so the already-seen check can use it. This means extracting the hash computation to a shared utility:

```go
// In ingest.go or a new file: payload_hash.go

// ComputePayloadHash computes a SHA-256 hash of the canonical JSON representation
// of the event input. Used for both the already-seen check and event_sources recording.
func ComputePayloadHash(input EventInput) ([32]byte, error) {
    canonical, err := json.Marshal(input)
    if err != nil {
        return [32]byte{}, fmt.Errorf("marshal payload for hash: %w", err)
    }
    return sha256.Sum256(canonical), nil
}
```

**Note**: The exact payload hash computation must be verified against the existing code in `ingest.go:304` to ensure the hash is computed from the same representation. If `ingest.go` hashes the raw/original payload vs. the normalized payload, we need to match that behavior. Check `ingest.go:290-310` during implementation.

### Setting `last_curated_at` on Admin Actions

Each qualifying admin action sets `last_curated_at` as part of its existing transaction. No new transaction needed.

**Pattern**: After the action's core DB write, call:

```go
// SetCuratedAt sets last_curated_at on an event. Called within admin action transactions.
// Uses the event's existing ULID (not ID) for consistency with other admin service methods.
_, err := txRepo.UpdateEvent(ctx, eventULID, UpdateEventParams{
    LastCuratedAt: timePtr(time.Now()),
})
```

This requires adding `LastCuratedAt *time.Time` to `UpdateEventParams`:

```go
// In repository.go
type UpdateEventParams struct {
    // ... existing fields ...
    LastCuratedAt  *time.Time // Set to non-nil to mark event as curated
}
```

And updating the `UpdateEvent` SQL to include the column when non-nil. The existing `UpdateEvent` in `events_repository.go` uses dynamic SQL (builds SET clauses from non-nil params), so this follows the established pattern.

**Admin actions to wire up:**

| Method | Where to add SetCuratedAt | Notes |
|---|---|---|
| `UpdateEvent` | After the existing `repo.UpdateEvent` call — actually, include in same call | `LastCuratedAt` is just another field in `UpdateEventParams` |
| `Consolidate` (`consolidateRetireEvents`) | After enriching canonical event, line ~1840 | Set on canonical event only |
| `FixAndApproveEventWithReview` | After applying fixes, before approval | Line ~1560 area |
| `MergeEventsWithReview` | After merge completes, on surviving event | Line ~1070 area |

**Admin actions that do NOT set it:**

| Method | Why not |
|---|---|
| `ApproveEventWithReview` | Routine unblock, not curation |
| `AddOccurrenceFromReview` | Schedule data, not content |
| `AddOccurrenceFromReviewNearDup` | Schedule data, not content |
| `CreateOccurrenceOnEvent` | Schedule data, not content |
| `UpdateOccurrenceOnEvent` | Schedule data, not content |
| `DeleteOccurrenceOnEvent` | Schedule data, not content |
| `FixEventOccurrenceDates` | Schedule correction, not content |
| `PublishEvent` | Workflow state change |
| `UnpublishEvent` | Workflow state change |
| `RejectEventWithReview` | Event gets tombstoned |
| `DeleteEvent` | Event gets tombstoned |

### Error Handling

| Error | HTTP Status | RFC 7807 `type` | When |
|---|---|---|---|
| `ErrAlreadySeen` | Not surfaced via HTTP (ingest-internal) | — | Payload hash match |
| `ErrPreviouslyDeleted` | Not surfaced via HTTP (ingest-internal) | — | Tombstoned event, admin-deleted |
| `ErrPreviouslyRejected` (existing) | Not surfaced via HTTP (ingest-internal) | — | Review queue rejected |

These errors are returned from `CreateEventCore` to the calling ingest pipeline. The ingest pipeline logs them and increments counters. They are NOT surfaced as HTTP errors because scraped events arrive via internal job processing, not direct HTTP calls.

### SQLc Changes Summary

1. **New query**: `PayloadHashExists` in `provenance.sql`
2. **Modified query**: `UpdateEvent` in `events.sql` — add `last_curated_at` to the dynamic SET logic
3. **Read `last_curated_at`**: All event-reading queries need to SELECT the new column. This affects `eventRow` scan in `events_repository.go`.

### Repository Changes Summary

1. **`Event` struct**: add `LastCuratedAt *time.Time`
2. **`UpdateEventParams`**: add `LastCuratedAt *time.Time`
3. **`Repository` interface**: add `PayloadHashExists(ctx, hash) (bool, error)`
4. **`eventRow` / scan functions**: add `last_curated_at` to all SELECT lists and Scan calls
5. **`EventCreateParams`**: no change (new events are never curated at creation time)

### Warning Codes

One new warning code:

| Warning Code | Source | When |
|---|---|---|
| `curator_protected` | ingest Layer 1/2 | Existing event has `last_curated_at` set; routing to review |

This code is informational — it tells the admin reviewer why the event is in the queue. The review workflow (approve, reject, merge) is unchanged.

## Implementation Tasks

### Task 1: Migration — add `last_curated_at` to events

**What**: Create migration `000028_curator_protection` (verify number) adding `last_curated_at TIMESTAMPTZ` column and partial index.

**Test**: `make migrate-up` succeeds; `make migrate-down` cleanly reverses.

**Acceptance**: Column exists, partial index exists, down migration drops both cleanly.

### Task 2: Domain model updates

**What**: Add `LastCuratedAt *time.Time` to `Event` struct and `UpdateEventParams`. Add `ErrAlreadySeen` and `ErrPreviouslyDeleted` error types. Add `PayloadHashExists` to `Repository` interface. Update all `eventRow` scan sites to include `last_curated_at`.

**Test**: `make build` succeeds (all interface implementations compile). `make sqlc` regenerates cleanly.

**Acceptance**: No compilation errors. All mock repositories implement the new method. `Event.LastCuratedAt` is populated from DB reads.

### Task 3: Payload hash already-seen check

**What**: Implement `PayloadHashExists` query. Extract `ComputePayloadHash` to a shared function. Add Step 0 to `CreateEventCore` that short-circuits when hash exists.

**Test**: Table-driven test:
- Payload hash exists → returns `ErrAlreadySeen`
- Payload hash doesn't exist → proceeds to Layer 1
- Empty/zero hash → skips check, proceeds to Layer 1

**Acceptance**: Identical payload resubmission returns `ErrAlreadySeen` with no other side effects.

### Task 4: `evaluateExistingMatch` guard function

**What**: Implement `evaluateExistingMatch` and `followTombstoneRedirect` as described in Technical Design. Pure decision logic — no side effects beyond DB reads.

**Test**: Table-driven test with mock repository:
- Existing event, `lifecycle_state = "published"`, `last_curated_at = nil` → `matchActionAutoMerge`
- Existing event, `lifecycle_state = "published"`, `last_curated_at` set → `matchActionRouteReview`
- Existing event, `lifecycle_state = "deleted"`, tombstone `superseded_by = nil` → `matchActionReject` with `ErrPreviouslyDeleted`
- Existing event, `lifecycle_state = "deleted"`, tombstone `superseded_by` set → `matchActionRedirect`
- Redirect to canonical that is also deleted → error (no recursive redirect)
- Redirect to canonical that is curated → `matchActionRouteReview`
- Redirect to canonical that is uncurated → `matchActionAutoMerge`

**Acceptance**: All 7 test cases pass. Function has no side effects beyond reads.

### Task 5: Wire guard into Layer 1 and Layer 2

**What**: Replace the unconditional auto-merge in both `source_event_id` match (lines 148-169) and `dedup_hash` match (lines 203-230) with `evaluateExistingMatch` + switch. When `matchActionRouteReview`, add `curator_protected` warning, set `needsReview = true`, and set `DuplicateOfEventID` to the curated event.

**Test**: Integration-style tests using `MockRepository`:
- Layer 1 match + curated → creates review entry with `curator_protected` warning and `duplicate_of_event_id` pointing to curated event
- Layer 1 match + uncurated → auto-merges as before (regression test)
- Layer 1 match + deleted (admin) → returns `ErrPreviouslyDeleted`
- Layer 1 match + deleted (consolidated) → follows redirect, merges/reviews canonical
- Layer 2 match + curated → creates review entry
- Layer 2 match + uncurated → auto-merges as before
- Existing tests continue to pass (regression)

**Acceptance**: All new tests pass. All existing ingest tests pass unchanged.

### Task 6: Set `last_curated_at` on admin actions

**What**: Wire `last_curated_at = now()` into `UpdateEvent`, `Consolidate` (canonical), `FixAndApproveEventWithReview`, and `MergeEventsWithReview` (surviving event). For `UpdateEvent`, add `last_curated_at` to the dynamic SET builder in `events_repository.go`.

**Test**: For each admin action:
- Call action on event with `last_curated_at = NULL`
- Verify `last_curated_at` is now non-NULL
- For non-qualifying actions (`ApproveEventWithReview`, `PublishEvent`, etc.), verify `last_curated_at` remains NULL

**Acceptance**: All qualifying actions set the timestamp. Non-qualifying actions don't. Existing admin tests pass.

### Task 7: End-to-end integration test

**What**: Full pipeline test: ingest → admin edit → re-ingest. Verify the complete flow including payload hash check, curator guard, and review queue routing.

**Test**:
1. Ingest event E from source S → published
2. Same payload from S → `ErrAlreadySeen`
3. Admin edits E (description change) → `last_curated_at` set
4. Source S submits updated payload (different hash, same source_event_id) → creates review entry with `curator_protected` warning, E unchanged
5. Admin deletes event F → tombstone created
6. Source submits payload matching F's source_event_id → `ErrPreviouslyDeleted`
7. Admin consolidates G into H → tombstone on G with superseded_by = H
8. Source submits payload matching G's source_event_id → follows redirect to H, merges or reviews H

**Acceptance**: All 8 steps produce expected outcomes. No regressions in existing test suite.

## Configuration

No new configuration. `last_curated_at` logic is unconditional — there's no feature flag or threshold to tune. If we later need to make the already-seen check optional (e.g., for re-processing), we can add a `SkipPayloadHashCheck bool` to `CreateEventCoreOpts`.

## Success Criteria

1. Identical payload resubmission is short-circuited (no DB writes beyond hash lookup)
2. Admin-edited events are not modified by subsequent scraper runs
3. Admin-deleted events are not resurrected by scraper runs
4. Consolidated events redirect to the canonical event
5. All existing ingest and admin tests pass unchanged
6. New tests cover all decision branches in `evaluateExistingMatch`

## Open Questions

1. **Payload hash computation source**: The existing hash in `ingest.go:304` — is it computed from the raw input or the normalized input? The already-seen check must use the same representation. Verify during Task 3 implementation.
2. **`superseded_by_uri` format**: The tombstone stores a full URI like `https://toronto.togather.foundation/events/01JRZX...`. The `followTombstoneRedirect` helper needs to parse the ULID out of this. Verify the URI format during Task 4 implementation and use `ids.ParseEntityURI` if it fits.
3. **Review queue for curated + redirected events**: When a consolidated event redirects to a curated canonical, the review entry's `duplicate_of_event_id` should point to the canonical. The `curator_protected` warning should mention the redirect for admin context. Verify this provides enough info in the existing review UI.
