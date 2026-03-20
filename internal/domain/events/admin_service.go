package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/validation"
	"github.com/rs/zerolog/log"
)

// AdminService provides admin operations for event management
type AdminService struct {
	repo             Repository
	requireHTTPS     bool
	defaultTZ        string
	validationConfig config.ValidationConfig
	baseURL          string // canonical base URL for tombstone/superseded_by URIs (e.g. "https://toronto.togather.foundation")
	ingestService    *IngestService
}

func NewAdminService(repo Repository, requireHTTPS bool, defaultTimezone string, validationConfig config.ValidationConfig, baseURL string) *AdminService {
	if baseURL == "" {
		panic("NewAdminService: baseURL must not be empty — set SERVER_BASE_URL (default: http://localhost:8080)")
	}
	svc := &AdminService{
		repo:             repo,
		requireHTTPS:     requireHTTPS,
		defaultTZ:        defaultTimezone,
		validationConfig: validationConfig.WithDefaults(),
		baseURL:          baseURL,
	}
	// Auto-create an internal IngestService so that existing tests that never
	// call WithIngestService still work.  Router.go will call WithIngestService
	// after both services are created to share the same configured instance.
	svc.ingestService = NewIngestService(repo, "", defaultTimezone, validationConfig)
	return svc
}

// WithIngestService wires a pre-configured IngestService into AdminService so
// that the Consolidate create path can share the full ingest pipeline.
// Returns the receiver for convenience.
func (s *AdminService) WithIngestService(svc *IngestService) *AdminService {
	s.ingestService = svc
	return s
}

// eventURI returns the canonical URI for an event ULID.
// baseURL is guaranteed non-empty by NewAdminService; if URI construction
// fails the error is returned rather than silently emitting a bare ULID into a
// tombstone or superseded_by field — either would violate the SEL spec.
func (s *AdminService) eventURI(ulid string) (string, error) {
	if ulid == "" {
		return "", fmt.Errorf("eventURI: ulid must not be empty")
	}
	uri, err := ids.BuildCanonicalURI(s.baseURL, "events", ulid)
	if err != nil {
		return "", fmt.Errorf("build canonical URI for event %s: %w", ulid, err)
	}
	return uri, nil
}

// UpdateEventParams contains fields that can be updated by admins
type UpdateEventParams struct {
	Name           *string
	Description    *string
	LifecycleState *string
	ImageURL       *string
	PublicURL      *string
	EventDomain    *string
	Keywords       []string
}

// MergeEventsParams contains IDs for merging duplicate events
type MergeEventsParams struct {
	PrimaryULID   string
	DuplicateULID string
}

// ConsolidateParams defines the request for atomic event consolidation.
// Exactly one of Event or EventULID must be set; Retire must have at least one ULID.
type ConsolidateParams struct {
	// Event creates a new canonical event. Mutually exclusive with EventULID.
	Event *EventInput
	// EventULID promotes an existing event as canonical. Mutually exclusive with Event.
	EventULID string
	// Retire lists event ULIDs to soft-delete with tombstones. Required, min 1.
	Retire []string
}

// ConsolidateResult contains the outcome of an atomic consolidation operation.
type ConsolidateResult struct {
	Event                  *Event
	LifecycleState         string
	IsDuplicate            bool
	IsMerged               bool
	NeedsReview            bool
	Warnings               []ValidationWarning
	Retired                []string
	ReviewEntriesDismissed []int
}

var (
	ErrInvalidUpdateParams            = errors.New("invalid update parameters")
	ErrCannotMergeSameEvent           = errors.New("cannot merge event with itself")
	ErrEventDeleted                   = errors.New("event has been deleted")
	ErrEventAlreadyMerged             = errors.New("event has already been merged")
	ErrAmbiguousOccurrenceSource      = errors.New("source event has multiple occurrences: cannot absorb ambiguously")
	ErrZeroOccurrenceSource           = errors.New("source event has no occurrences: cannot determine which occurrence to absorb")
	ErrAmbiguousOccurrenceDispatch    = errors.New("review entry has both potential_duplicate and near_duplicate_of_new_event warnings: cannot determine add-occurrence path unambiguously")
	ErrWrongOccurrencePath            = errors.New("review entry warnings do not match the requested add-occurrence path")
	ErrUnsupportedReviewForOccurrence = errors.New("review entry has no potential_duplicate or near_duplicate_of_new_event warning: add-occurrence requires a supported duplicate warning")
	// ErrMalformedWarnings is returned by occurrenceDispatchPath when the persisted
	// warnings column cannot be parsed as JSON.  This is a data-integrity fault —
	// the DB row was written with invalid JSON — and must surface as an internal
	// server error (500) rather than silently degrading to the "unsupported" path.
	ErrMalformedWarnings = errors.New("review entry warnings column contains malformed JSON: data integrity fault")
)

// OccurrenceDispatchPath classifies a raw warnings JSON blob into one of three
// outcomes for the add-as-occurrence dispatch:
//
//   - "forward"      — the review has at least one potential_duplicate warning and no
//     near_duplicate_of_new_event warning → use AddOccurrenceFromReview.
//   - "neardup"      — the review has at least one near_duplicate_of_new_event warning
//     and no potential_duplicate warning → use AddOccurrenceFromReviewNearDup.
//   - "unsupported"  — neither warning type is present (e.g. no warnings or only
//     quality/completeness warnings) → reject with ErrUnsupportedReviewForOccurrence.
//
// Returns ErrAmbiguousOccurrenceDispatch if BOTH warning types are present.
// Returns ErrMalformedWarnings if the warnings column cannot be parsed as JSON —
// this is a data-integrity fault and must surface as an internal server error.
func OccurrenceDispatchPath(warningsJSON []byte) (string, error) {
	return occurrenceDispatchPath(warningsJSON)
}

// occurrenceDispatchPath is the unexported implementation shared between the exported
// OccurrenceDispatchPath wrapper (handler pre-check) and internal service TX paths.
func occurrenceDispatchPath(warningsJSON []byte) (string, error) {
	if len(warningsJSON) == 0 {
		return "unsupported", nil
	}
	var warnings []ValidationWarning
	if err := json.Unmarshal(warningsJSON, &warnings); err != nil {
		// Malformed warnings JSON is a data-integrity fault: the DB row was written
		// with invalid JSON.  Do NOT silently degrade to "unsupported" — that would
		// produce a misleading unsupported-review 422.  Return ErrMalformedWarnings
		// so callers can surface this as an internal server error (500).
		return "", fmt.Errorf("parse warnings JSON: %w", ErrMalformedWarnings)
	}
	var hasNearDup, hasPotDup bool
	for _, w := range warnings {
		switch w.Code {
		case "near_duplicate_of_new_event":
			hasNearDup = true
		case "potential_duplicate":
			hasPotDup = true
		}
	}
	if hasNearDup && hasPotDup {
		return "", ErrAmbiguousOccurrenceDispatch
	}
	if hasNearDup {
		return "neardup", nil
	}
	if hasPotDup {
		return "forward", nil
	}
	return "unsupported", nil
}

// AddOccurrenceFromReview atomically adds the review entry's event occurrence to a target
// recurring-series event, soft-deletes the review's own event, and marks the review
// as "merged" — all in a single database transaction.
//
// The targetEventULID identifies the existing recurring-series event.  The new
// occurrence is constructed from the **locked source event's sole occurrence** —
// not from the review entry's snapshot EventStartTime / EventEndTime, which may be
// stale if the source event was edited after ingest.  The source event is fetched and
// re-read under a row-level lock inside the transaction to prevent TOCTOU races.
//
// Pre-conditions checked under the transaction lock:
//   - The review entry must be in "pending" status (else ErrConflict).
//   - The locked review warnings must indicate the "forward" (potential_duplicate) path
//     (else ErrWrongOccurrencePath or ErrUnsupportedReviewForOccurrence or
//     ErrAmbiguousOccurrenceDispatch).
//   - The source event must have exactly one occurrence (else ErrZeroOccurrenceSource
//     or ErrAmbiguousOccurrenceSource).
//   - The target event must not be deleted (else ErrEventDeleted).
//
// If the new time range overlaps any existing occurrence on the target event,
// ErrOccurrenceOverlap is returned and the transaction is rolled back.
func (s *AdminService) AddOccurrenceFromReview(ctx context.Context, reviewID int, targetEventULID string, reviewedBy string) (*ReviewQueueEntry, error) {
	if targetEventULID == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	// Lock the review queue row before inspecting its status.  This ensures that
	// two concurrent admin requests for the same review entry are serialised: the
	// second request will block on the lock, then re-read the already-updated
	// status and return ErrConflict — instead of both proceeding past the pending
	// check and producing a confusing downstream error.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("lock review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, fmt.Errorf("review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// Re-validate the dispatch path from the locked warnings to prevent a stale
	// pre-read in the handler from routing to the wrong path.  If the warnings
	// changed since the handler's advisory read, the locked row is authoritative.
	lockedPath, pathErr := occurrenceDispatchPath(review.Warnings)
	if pathErr != nil {
		// Both warning types present — ambiguous dispatch.
		return nil, fmt.Errorf("review entry %d: %w", reviewID, pathErr)
	}
	if lockedPath == "neardup" {
		// The locked warnings indicate the near-dup path; the caller chose the wrong method.
		return nil, fmt.Errorf("review entry %d warnings indicate near_duplicate_of_new_event path: %w", reviewID, ErrWrongOccurrencePath)
	}
	if lockedPath == "unsupported" {
		// No supported duplicate warning present — forward path requires potential_duplicate.
		return nil, fmt.Errorf("review entry %d: %w", reviewID, ErrUnsupportedReviewForOccurrence)
	}

	// Look up the companion review for the target event and lock it BEFORE locking
	// any event row.  This preserves the review-first lock ordering used by all admin
	// methods (review → companion review → target event → source event) and prevents
	// deadlocks.  The companion review is created by near-dup ingest on the target
	// event and cross-links to the source event (review.EventULID) via duplicate_of_event_id.
	// We narrow the lookup to the exact counterpart using both the target ULID and the
	// source (review) event ULID — preventing the wrong unrelated pending review from
	// being dismissed when the target has multiple pending review rows.
	companionReview, compErr := txRepo.GetPendingReviewByEventUlidAndDuplicateUlid(ctx, targetEventULID, review.EventULID)
	if compErr != nil && !errors.Is(compErr, ErrNotFound) {
		return nil, fmt.Errorf("lookup companion review for target %s: %w", targetEventULID, compErr)
	}
	// companionReview == nil when ErrNotFound or no pending entry exists — both are fine.

	var lockedCompanion *ReviewQueueEntry
	if companionReview != nil {
		lc, lockErr := txRepo.LockReviewQueueEntryForUpdate(ctx, companionReview.ID)
		if lockErr != nil {
			if !errors.Is(lockErr, ErrNotFound) {
				return nil, fmt.Errorf("lock companion review id=%d: %w", companionReview.ID, lockErr)
			}
			// Companion was deleted concurrently — not fatal.
		} else {
			lockedCompanion = lc
		}
	}

	target, err := txRepo.GetByULID(ctx, targetEventULID)
	if err != nil {
		return nil, fmt.Errorf("get target event %s: %w", targetEventULID, err)
	}

	// Acquire a row-level lock on the target event BEFORE the eligibility recheck so
	// that two concurrent add-occurrence requests for the same target cannot both pass
	// the read-then-write gap (TOCTOU).  The lock is held until the transaction commits
	// or rolls back.
	if err := txRepo.LockEventForUpdate(ctx, target.ID); err != nil {
		return nil, fmt.Errorf("lock target event %s: %w", targetEventULID, err)
	}

	// Re-read under lock and validate eligibility.  Any lifecycle state other than
	// "deleted" is acceptable — admins may add occurrences to draft, pending_review,
	// cancelled, or published series events.
	target, err = txRepo.GetByULID(ctx, targetEventULID)
	if err != nil {
		return nil, fmt.Errorf("get target event %s (post-lock): %w", targetEventULID, err)
	}
	if target.LifecycleState == "deleted" {
		return nil, fmt.Errorf("target event %s: %w", targetEventULID, ErrEventDeleted)
	}

	// Ensure the review's own event is not the same as the target
	if review.EventULID == targetEventULID {
		return nil, fmt.Errorf("review event and target event are the same (%s): %w", targetEventULID, ErrCannotMergeSameEvent)
	}

	// Fetch the review (source) event to get its internal ID for locking.
	// We lock the source AFTER the target lock, preserving the lock ordering:
	// review → target event → source event.  This prevents a TOCTOU race where a
	// concurrent request modifies the source event's occurrences between the fetch
	// and the soft-delete.
	reviewEvent, err := txRepo.GetByULID(ctx, review.EventULID)
	if err != nil {
		return nil, fmt.Errorf("get review event %s: %w", review.EventULID, err)
	}

	// Acquire a row-level lock on the source event so that its occurrence timestamps
	// cannot be mutated between our read and the soft-delete commit.
	if err := txRepo.LockEventForUpdate(ctx, reviewEvent.ID); err != nil {
		return nil, fmt.Errorf("lock source event %s: %w", review.EventULID, err)
	}

	// Re-read the source event under lock to get authoritative occurrence timestamps.
	// The review row's EventStartTime / EventEndTime are a snapshot taken at ingest
	// time and may diverge from the actual occurrence if the event was edited after
	// ingest.  Using stale snapshot timestamps would add the wrong time slot to the
	// target series and then soft-delete the source — data corruption.
	reviewEvent, err = txRepo.GetByULID(ctx, review.EventULID)
	if err != nil {
		return nil, fmt.Errorf("get review event %s (post-lock): %w", review.EventULID, err)
	}

	// Reject source events that were deleted (soft-deleted via any reason) between
	// the time the review was created and now.  The most common causes are a
	// concurrent admin absorbing the same event or a duplicate-merge that ran in
	// parallel.  Returning ErrEventDeleted here surfaces the same sentinel that the
	// target-deleted guard already uses, keeping error handling uniform in the handler.
	if reviewEvent.LifecycleState == "deleted" {
		return nil, fmt.Errorf("source event %s is deleted: %w", review.EventULID, ErrEventDeleted)
	}

	// Reject multi-occurrence and zero-occurrence sources.
	//
	// Multiple occurrences: only one occurrence can be absorbed (the one matching
	// the review entry's EventStartTime), so soft-deleting the entire source event
	// would silently lose the remaining occurrences — data loss.
	//
	// Zero occurrences: there is nothing to absorb; the review start/end timestamps
	// belong to the review entry, not to a real occurrence on the source event.
	//
	// In both cases the admin must resolve the source event before retrying.
	if len(reviewEvent.Occurrences) > 1 {
		return nil, fmt.Errorf("source event %s has %d occurrences: %w",
			review.EventULID, len(reviewEvent.Occurrences), ErrAmbiguousOccurrenceSource)
	}
	if len(reviewEvent.Occurrences) == 0 {
		return nil, fmt.Errorf("source event %s has no occurrences: %w",
			review.EventULID, ErrZeroOccurrenceSource)
	}

	// Find the sole occurrence and extract its locked timestamps together with all
	// occurrence-level metadata.  We always have exactly one occurrence at this point
	// (the multi/zero cases above already returned).  The matched occurrence is the
	// source of truth for start/end times — do NOT use review.EventStartTime /
	// review.EventEndTime (snapshot values that may be stale).
	matchedOcc := &reviewEvent.Occurrences[0]

	// Occurrence timestamps — from the source event, not the review snapshot.
	occStartTime := matchedOcc.StartTime
	occEndTime := matchedOcc.EndTime

	// Occurrence-level metadata defaults seeded from the target series.
	occTimezone := s.defaultTZ
	occVenueID := target.PrimaryVenueID
	var occVirtualURL *string
	var occDoorTime *time.Time
	var occTicketURL *string
	var occPriceMin *float64
	var occPriceMax *float64
	var occPriceCurrency string
	var occAvailability string

	// Override with occurrence-level values from the source event so that pricing,
	// door time, ticket URL, timezone, venue, and virtual URL survive absorption.
	if matchedOcc.Timezone != "" {
		occTimezone = matchedOcc.Timezone
	}
	if matchedOcc.VenueID != nil {
		occVenueID = matchedOcc.VenueID
	}
	if matchedOcc.VirtualURL != nil && *matchedOcc.VirtualURL != "" {
		occVirtualURL = matchedOcc.VirtualURL
	}
	if matchedOcc.DoorTime != nil {
		occDoorTime = matchedOcc.DoorTime
	}
	if matchedOcc.TicketURL != "" {
		occTicketURL = &matchedOcc.TicketURL
	}
	if matchedOcc.PriceMin != nil {
		occPriceMin = matchedOcc.PriceMin
	}
	if matchedOcc.PriceMax != nil {
		occPriceMax = matchedOcc.PriceMax
	}
	if matchedOcc.PriceCurrency != "" {
		occPriceCurrency = matchedOcc.PriceCurrency
	}
	if matchedOcc.Availability != "" {
		occAvailability = matchedOcc.Availability
	}

	// Overlap check uses the locked occurrence timestamps (not the review snapshot).
	overlaps, err := txRepo.CheckOccurrenceOverlap(ctx, target.ID, occStartTime, occEndTime)
	if err != nil {
		return nil, fmt.Errorf("check overlap: %w", err)
	}
	if overlaps {
		return nil, fmt.Errorf("new occurrence [%s, %v] on event %s: %w",
			occStartTime.Format(time.RFC3339),
			occEndTime,
			targetEventULID,
			ErrOccurrenceOverlap,
		)
	}

	err = txRepo.CreateOccurrence(ctx, OccurrenceCreateParams{
		EventID:       target.ID,
		StartTime:     occStartTime,
		EndTime:       occEndTime,
		Timezone:      occTimezone,
		DoorTime:      occDoorTime,
		VenueID:       occVenueID,
		VirtualURL:    occVirtualURL,
		TicketURL:     occTicketURL,
		PriceMin:      occPriceMin,
		PriceMax:      occPriceMax,
		PriceCurrency: occPriceCurrency,
		Availability:  occAvailability,
	})
	if err != nil {
		return nil, fmt.Errorf("create occurrence: %w", err)
	}

	err = txRepo.SoftDeleteEvent(ctx, review.EventULID, "absorbed_as_occurrence")
	if err != nil {
		return nil, fmt.Errorf("soft-delete review event: %w", err)
	}

	// Clean up the source event's occurrence rows.  Soft-delete (UPDATE) does not
	// trigger ON DELETE CASCADE, so orphaned occurrence rows would remain without
	// explicit cleanup.
	if err := txRepo.DeleteOccurrencesByEventULID(ctx, review.EventULID); err != nil {
		return nil, fmt.Errorf("delete source occurrences: %w", err)
	}

	// Tombstone for the absorbed event
	targetURI, err := s.eventURI(targetEventULID)
	if err != nil {
		return nil, fmt.Errorf("canonical URI for target: %w", err)
	}
	reviewEventURI, err := s.eventURI(reviewEvent.ULID)
	if err != nil {
		return nil, fmt.Errorf("canonical URI for review event: %w", err)
	}
	tombstonePayload, err := buildTombstonePayload(reviewEventURI, reviewEvent.Name, &targetURI, "absorbed_as_occurrence")
	if err != nil {
		return nil, fmt.Errorf("build tombstone: %w", err)
	}
	err = txRepo.CreateTombstone(ctx, TombstoneCreateParams{
		EventID:      reviewEvent.ID,
		EventURI:     reviewEventURI,
		DeletedAt:    time.Now(),
		Reason:       "absorbed_as_occurrence",
		SupersededBy: &targetURI,
		Payload:      tombstonePayload,
	})
	if err != nil {
		return nil, fmt.Errorf("create tombstone: %w", err)
	}

	// Mark the review as merged (re-using the MergeReview status path with the target ULID)
	reviewEntry, err := txRepo.MergeReview(ctx, reviewID, reviewedBy, targetEventULID)
	if err != nil {
		return nil, fmt.Errorf("update review status: %w", err)
	}

	// Dismiss the companion review entry on the target event (if present and still
	// pending).  This mirrors the companion handling in AddOccurrenceFromReviewNearDup.
	// Without this, the target event retains an orphaned pending review row that
	// references the now-deleted source event, polluting the review queue and
	// blocking retry attempts.
	if lockedCompanion != nil && lockedCompanion.Status == "pending" {
		if _, mergeErr := txRepo.MergeReview(ctx, lockedCompanion.ID, reviewedBy, targetEventULID); mergeErr != nil {
			if errors.Is(mergeErr, ErrNotFound) || errors.Is(mergeErr, ErrConflict) {
				// Race outcome: companion was already dismissed by a concurrent request.
				// Log-worthy but non-fatal — continue so the primary review is resolved.
				_ = mergeErr
			} else {
				return nil, fmt.Errorf("dismiss companion review id=%d: %w", lockedCompanion.ID, mergeErr)
			}
		}
	}

	// Recompute whether the target event should leave review.  The add-occurrence
	// action resolved the primary review (and its companion, if any), but the target
	// may have OTHER unresolved pending review rows (e.g., a second flagged
	// occurrence on the same series).  Only restore the lifecycle to "published" if
	// no further pending review rows remain.
	if target.LifecycleState == "pending_review" {
		remaining, remErr := txRepo.GetPendingReviewByEventUlid(ctx, targetEventULID)
		if remErr != nil && !errors.Is(remErr, ErrNotFound) {
			return nil, fmt.Errorf("recheck pending review for target %s: %w", targetEventULID, remErr)
		}
		if remaining == nil {
			publishedState := "published"
			if _, err := txRepo.UpdateEvent(ctx, targetEventULID, UpdateEventParams{
				LifecycleState: &publishedState,
			}); err != nil {
				return nil, fmt.Errorf("restore target lifecycle to published: %w", err)
			}
		}
		// If remaining != nil, other unresolved issues exist — leave lifecycle
		// as pending_review so the event stays invisible to the public API until
		// all review rows are resolved.
	}

	if err := txCommitter.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return reviewEntry, nil
}

// AddOccurrenceFromReviewNearDup handles the near_duplicate_of_new_event case:
// the review entry sits on the *existing* recurring-series event, and
// DuplicateOfEventULID points to the *newly ingested* event that should be
// absorbed as an occurrence.
//
// Semantics (reversed from AddOccurrenceFromReview):
//   - Target  = review.EventULID  (existing series — preserved)
//   - Source  = review.DuplicateOfEventULID (newly ingested event — absorbed)
//
// The source event must have exactly one occurrence.  If it has multiple
// occurrences the method returns ErrAmbiguousOccurrenceSource without making
// any changes — the admin must resolve the source event first.
//
// Lock ordering (review-first discipline, mirrors all other admin methods):
//  1. Lock the near-dup review entry.
//  2. Look up the companion review for the source event (read-only).
//  3. Lock the companion review (if found) — all review locks before any event locks.
//  4. Lock the target (existing series) event.
//  5. Fetch and lock the source event (after target lock to preserve ordering).
//
// The method atomically:
//  1. Locks the near-dup review entry and verifies it is still pending.
//  2. Looks up and locks the source event's companion review entry (if present).
//  3. Locks and re-reads the target (existing series) event.
//  4. Fetches and locks the source event; rejects if it has multiple occurrences.
//  5. Checks overlap for the source event's occurrence on the target.
//  6. Creates the occurrence on the target.
//  7. Soft-deletes the source event.
//  8. Creates a tombstone for the source event.
//  9. Marks the companion review as merged (if present and still pending).
//
// 10. Marks the near-dup review entry as merged.
func (s *AdminService) AddOccurrenceFromReviewNearDup(ctx context.Context, reviewID int, reviewedBy string) (*ReviewQueueEntry, *string, error) {
	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	// Step 1: Lock the near-dup review entry first.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, nil, fmt.Errorf("lock near-dup review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, nil, fmt.Errorf("near-dup review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// Re-validate the dispatch path from the locked warnings to prevent a stale
	// pre-read in the handler from routing to the wrong path.
	lockedPath, pathErr := occurrenceDispatchPath(review.Warnings)
	if pathErr != nil {
		return nil, nil, fmt.Errorf("near-dup review entry %d: %w", reviewID, pathErr)
	}
	if lockedPath == "forward" {
		// The locked warnings indicate the forward path; the caller chose the wrong method.
		return nil, nil, fmt.Errorf("near-dup review entry %d warnings do not indicate near_duplicate_of_new_event path: %w", reviewID, ErrWrongOccurrencePath)
	}
	if lockedPath == "unsupported" {
		// No supported duplicate warning present — near-dup path requires near_duplicate_of_new_event.
		return nil, nil, fmt.Errorf("near-dup review entry %d: %w", reviewID, ErrUnsupportedReviewForOccurrence)
	}
	if review.DuplicateOfEventULID == nil || *review.DuplicateOfEventULID == "" {
		return nil, nil, fmt.Errorf("near-dup review entry %d has no duplicate event ULID: %w", reviewID, ErrInvalidUpdateParams)
	}
	sourceEventULID := *review.DuplicateOfEventULID // new event → to be absorbed
	targetEventULID := review.EventULID             // existing series → kept

	if sourceEventULID == targetEventULID {
		return nil, nil, fmt.Errorf("near-dup review source and target are the same (%s): %w", targetEventULID, ErrCannotMergeSameEvent)
	}

	// Step 2+3: Look up the companion review for the source event and lock it BEFORE
	// locking any event row.  This preserves the review-first lock ordering used by
	// all other admin methods and eliminates the deadlock risk that arises when a
	// companion review lock is acquired after an event lock (lock-order inversion).
	//
	// We narrow the lookup to the exact counterpart using both the source event ULID
	// and the target (existing series) ULID via duplicate_of_event_id — preventing the
	// wrong unrelated pending review from being dismissed when the source event has
	// multiple pending review rows.
	//
	// A concurrent admin action on the companion may have committed between our
	// GetPending read and the LockForUpdate; we re-read status under lock and skip
	// the merge if it is no longer pending.
	companionReview, err := txRepo.GetPendingReviewByEventUlidAndDuplicateUlid(ctx, sourceEventULID, targetEventULID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, nil, fmt.Errorf("lookup companion review for source %s: %w", sourceEventULID, err)
	}
	// companionReview == nil when ErrNotFound or no pending entry exists — both are fine.

	var lockedCompanion *ReviewQueueEntry
	if companionReview != nil {
		lc, lockErr := txRepo.LockReviewQueueEntryForUpdate(ctx, companionReview.ID)
		if lockErr != nil {
			if !errors.Is(lockErr, ErrNotFound) {
				// ErrNotFound means the companion was concurrently deleted — not fatal.
				return nil, nil, fmt.Errorf("lock companion review id=%d: %w", companionReview.ID, lockErr)
			}
			// Companion was deleted; treat as if there is no companion.
		} else {
			lockedCompanion = lc
		}
	}

	// Step 4: Fetch target (existing series) for locking.
	target, err := txRepo.GetByULID(ctx, targetEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("get target event %s: %w", targetEventULID, err)
	}

	// Lock target before eligibility recheck (TOCTOU guard).
	if err := txRepo.LockEventForUpdate(ctx, target.ID); err != nil {
		return nil, nil, fmt.Errorf("lock target event %s: %w", targetEventULID, err)
	}

	// Re-read target under lock.
	target, err = txRepo.GetByULID(ctx, targetEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("get target event %s (post-lock): %w", targetEventULID, err)
	}
	if target.LifecycleState == "deleted" {
		return nil, nil, fmt.Errorf("target event %s: %w", targetEventULID, ErrEventDeleted)
	}

	// Step 5: Fetch and lock the source (new) event.
	// Lock ordering: review rows → target event → source event.  The source is locked
	// AFTER the target event to avoid lock-order inversion.  Without this lock a
	// concurrent request that edits the source event's occurrence timestamps could
	// race between our GetByULID read and the eventual soft-delete, causing us to
	// absorb the wrong time slot (TOCTOU data corruption).
	sourceEvent, err := txRepo.GetByULID(ctx, sourceEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("get source event %s: %w", sourceEventULID, err)
	}

	if err := txRepo.LockEventForUpdate(ctx, sourceEvent.ID); err != nil {
		return nil, nil, fmt.Errorf("lock source event %s: %w", sourceEventULID, err)
	}

	// Re-read source event under lock to get authoritative occurrence timestamps.
	sourceEvent, err = txRepo.GetByULID(ctx, sourceEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("get source event %s (post-lock): %w", sourceEventULID, err)
	}

	// Reject source events that were deleted between review creation and now.
	// This mirrors the guard in AddOccurrenceFromReview and uses the same sentinel
	// so the handler can treat both paths uniformly.
	if sourceEvent.LifecycleState == "deleted" {
		return nil, nil, fmt.Errorf("source event %s is deleted: %w", sourceEventULID, ErrEventDeleted)
	}

	// Reject ambiguous multi-occurrence sources.  The near-dup ingest path always
	// creates single-occurrence events, so more than one occurrence indicates a
	// non-standard state that would silently drop occurrences if we absorbed only [0].
	if len(sourceEvent.Occurrences) > 1 {
		return nil, nil, fmt.Errorf("source event %s has %d occurrences: %w",
			sourceEventULID, len(sourceEvent.Occurrences), ErrAmbiguousOccurrenceSource)
	}

	// Reject zero-occurrence sources.  The review entry's EventStartTime belongs to
	// the *existing series* (target), not the newly-ingested source event — using it
	// as the occurrence start time would absorb the wrong date.  There is nothing safe
	// to absorb without a real occurrence on the source event.
	if len(sourceEvent.Occurrences) == 0 {
		return nil, nil, fmt.Errorf("source event %s has no occurrences: %w",
			sourceEventULID, ErrZeroOccurrenceSource)
	}

	// Derive start/end times for the new occurrence from the source event's sole occurrence.
	occStartTime := sourceEvent.Occurrences[0].StartTime
	occEndTime := sourceEvent.Occurrences[0].EndTime

	// Overlap check on the target.
	overlaps, err := txRepo.CheckOccurrenceOverlap(ctx, target.ID, occStartTime, occEndTime)
	if err != nil {
		return nil, nil, fmt.Errorf("check overlap: %w", err)
	}
	if overlaps {
		return nil, nil, fmt.Errorf("new occurrence [%s, %v] on event %s: %w",
			occStartTime.Format(time.RFC3339),
			occEndTime,
			targetEventULID,
			ErrOccurrenceOverlap,
		)
	}

	// Collect occurrence-level metadata from the source event's sole occurrence.
	occTimezone := s.defaultTZ
	occVenueID := target.PrimaryVenueID
	var occVirtualURL *string
	var occDoorTime *time.Time
	var occTicketURL *string
	var occPriceMin *float64
	var occPriceMax *float64
	var occPriceCurrency string
	var occAvailability string

	if len(sourceEvent.Occurrences) > 0 {
		occ := &sourceEvent.Occurrences[0]
		if occ.Timezone != "" {
			occTimezone = occ.Timezone
		}
		if occ.VenueID != nil {
			occVenueID = occ.VenueID
		}
		if occ.VirtualURL != nil && *occ.VirtualURL != "" {
			occVirtualURL = occ.VirtualURL
		}
		if occ.DoorTime != nil {
			occDoorTime = occ.DoorTime
		}
		if occ.TicketURL != "" {
			occTicketURL = &occ.TicketURL
		}
		if occ.PriceMin != nil {
			occPriceMin = occ.PriceMin
		}
		if occ.PriceMax != nil {
			occPriceMax = occ.PriceMax
		}
		if occ.PriceCurrency != "" {
			occPriceCurrency = occ.PriceCurrency
		}
		if occ.Availability != "" {
			occAvailability = occ.Availability
		}
	}

	// Add the occurrence to the target series.
	if err = txRepo.CreateOccurrence(ctx, OccurrenceCreateParams{
		EventID:       target.ID,
		StartTime:     occStartTime,
		EndTime:       occEndTime,
		Timezone:      occTimezone,
		DoorTime:      occDoorTime,
		VenueID:       occVenueID,
		VirtualURL:    occVirtualURL,
		TicketURL:     occTicketURL,
		PriceMin:      occPriceMin,
		PriceMax:      occPriceMax,
		PriceCurrency: occPriceCurrency,
		Availability:  occAvailability,
	}); err != nil {
		return nil, nil, fmt.Errorf("create occurrence: %w", err)
	}

	// Soft-delete the source (new) event.
	if err = txRepo.SoftDeleteEvent(ctx, sourceEventULID, "absorbed_as_occurrence"); err != nil {
		return nil, nil, fmt.Errorf("soft-delete source event: %w", err)
	}

	// Clean up the source event's occurrence rows.  Soft-delete (UPDATE) does not
	// trigger ON DELETE CASCADE, so orphaned occurrence rows would remain without
	// explicit cleanup.
	if err := txRepo.DeleteOccurrencesByEventULID(ctx, sourceEventULID); err != nil {
		return nil, nil, fmt.Errorf("delete source occurrences: %w", err)
	}

	// Tombstone for the absorbed source event.
	targetURI, err := s.eventURI(targetEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("canonical URI for target: %w", err)
	}
	sourceEventURI, err := s.eventURI(sourceEvent.ULID)
	if err != nil {
		return nil, nil, fmt.Errorf("canonical URI for source event: %w", err)
	}
	tombstonePayload, err := buildTombstonePayload(sourceEventURI, sourceEvent.Name, &targetURI, "absorbed_as_occurrence")
	if err != nil {
		return nil, nil, fmt.Errorf("build tombstone: %w", err)
	}
	if err = txRepo.CreateTombstone(ctx, TombstoneCreateParams{
		EventID:      sourceEvent.ID,
		EventURI:     sourceEventURI,
		DeletedAt:    time.Now(),
		Reason:       "absorbed_as_occurrence",
		SupersededBy: &targetURI,
		Payload:      tombstonePayload,
	}); err != nil {
		return nil, nil, fmt.Errorf("create tombstone: %w", err)
	}

	// Step 9: Dismiss the companion review entry (if present and still pending).
	// The lock was already acquired in step 3 above, maintaining review-first order.
	if lockedCompanion != nil && lockedCompanion.Status == "pending" {
		if _, mergeErr := txRepo.MergeReview(ctx, lockedCompanion.ID, reviewedBy, targetEventULID); mergeErr != nil {
			if errors.Is(mergeErr, ErrNotFound) || errors.Is(mergeErr, ErrConflict) {
				// Race outcome: companion was already dismissed by a concurrent request.
				// Log-worthy but non-fatal — continue so the primary review is resolved.
				_ = mergeErr
			} else {
				return nil, nil, fmt.Errorf("dismiss companion review id=%d: %w", lockedCompanion.ID, mergeErr)
			}
		}
	}

	// Step 10: Mark the near-dup review entry as merged.
	// We call MergeReview with targetEventULID so that duplicateOfEventUlid is set to
	// the series ULID — giving the admin a direct navigation link after the action.
	reviewEntry, err := txRepo.MergeReview(ctx, reviewID, reviewedBy, targetEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("update near-dup review status: %w", err)
	}

	// Step 11: Recompute whether the target event should leave review.  The
	// add-occurrence action resolved the near-dup review (and its companion, if
	// any), but the target may have OTHER unresolved pending review rows (e.g., a
	// second flagged occurrence on the same series).  Only restore the lifecycle to
	// "published" if no further pending review rows remain.
	if target.LifecycleState == "pending_review" {
		remaining, remErr := txRepo.GetPendingReviewByEventUlid(ctx, targetEventULID)
		if remErr != nil && !errors.Is(remErr, ErrNotFound) {
			return nil, nil, fmt.Errorf("recheck pending review for target %s: %w", targetEventULID, remErr)
		}
		if remaining == nil {
			publishedState := "published"
			if _, err := txRepo.UpdateEvent(ctx, targetEventULID, UpdateEventParams{
				LifecycleState: &publishedState,
			}); err != nil {
				return nil, nil, fmt.Errorf("restore target lifecycle to published: %w", err)
			}
		}
		// If remaining != nil, other unresolved issues exist — leave lifecycle
		// as pending_review so the event stays invisible to the public API until
		// all review rows are resolved.
	}

	if err := txCommitter.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit transaction: %w", err)
	}

	return reviewEntry, &targetEventULID, nil
}

// UpdateEvent updates event fields with admin attribution
// Returns the updated event
func (s *AdminService) UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error) {
	if ulid == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Validate parameters
	if err := s.validateUpdateParams(params); err != nil {
		return nil, err
	}

	// Get existing event
	existing, err := s.repo.GetByULID(ctx, ulid)
	if err != nil {
		return nil, err
	}

	// Apply updates
	updates := buildUpdateMap(existing, params)
	if len(updates) == 0 {
		// No changes, return existing
		return existing, nil
	}

	// Persist updates via repository
	updated, err := s.repo.UpdateEvent(ctx, ulid, params)
	if err != nil {
		return nil, fmt.Errorf("update event: %w", err)
	}

	return updated, nil
}

// FixEventOccurrenceDates updates occurrence dates for an event during the fix review workflow.
// If only startDate is provided, the existing end_time is preserved.
// If only endDate is provided, the existing start_time is preserved.
func (s *AdminService) FixEventOccurrenceDates(ctx context.Context, eventULID string, startDate *time.Time, endDate *time.Time) error {
	if eventULID == "" {
		return ErrInvalidUpdateParams
	}

	// Verify the event exists and get current occurrence data
	existing, err := s.repo.GetByULID(ctx, eventULID)
	if err != nil {
		return fmt.Errorf("get event for fix: %w", err)
	}

	if len(existing.Occurrences) == 0 {
		return fmt.Errorf("event %s has no occurrences to fix", eventULID)
	}

	// Determine the effective start and end times
	var effectiveStart time.Time
	var effectiveEnd *time.Time

	if startDate != nil {
		effectiveStart = *startDate
	} else {
		// Keep existing start time
		effectiveStart = existing.Occurrences[0].StartTime
	}

	if endDate != nil {
		effectiveEnd = endDate
	} else {
		// Keep existing end time
		effectiveEnd = existing.Occurrences[0].EndTime
	}

	// Validate: end must not be before start
	if effectiveEnd != nil && effectiveEnd.Before(effectiveStart) {
		return FilterError{Field: "endDate", Message: "end date cannot be before start date"}
	}

	return s.repo.UpdateOccurrenceDates(ctx, eventULID, effectiveStart, effectiveEnd)
}

// PublishEvent changes lifecycle_state from draft to published
func (s *AdminService) PublishEvent(ctx context.Context, ulid string) (*Event, error) {
	if ulid == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Get existing event
	existing, err := s.repo.GetByULID(ctx, ulid)
	if err != nil {
		return nil, err
	}

	// Check if already published
	if existing.LifecycleState == "published" {
		return existing, nil
	}

	// Update lifecycle state to published
	published := "published"
	params := UpdateEventParams{
		LifecycleState: &published,
	}

	return s.UpdateEvent(ctx, ulid, params)
}

// UnpublishEvent changes lifecycle_state from published to draft
func (s *AdminService) UnpublishEvent(ctx context.Context, ulid string) (*Event, error) {
	if ulid == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Get existing event
	existing, err := s.repo.GetByULID(ctx, ulid)
	if err != nil {
		return nil, err
	}

	// Check if already draft
	if existing.LifecycleState == "draft" {
		return existing, nil
	}

	// Update lifecycle state to draft
	draft := "draft"
	params := UpdateEventParams{
		LifecycleState: &draft,
	}

	return s.UpdateEvent(ctx, ulid, params)
}

// MergeEventsWithReview atomically merges a duplicate event into a primary event AND
// updates the review queue entry status to "merged" in a single database transaction.
// This prevents inconsistency where the merge could succeed but the review status update fails.
//
// Lock ordering: review row is locked first, then event rows are acquired by
// executeMerge.  This matches the ordering used by all other review-based
// admin methods and prevents deadlocks when concurrent admin actions touch
// the same review/event set.
func (s *AdminService) MergeEventsWithReview(ctx context.Context, params MergeEventsParams, reviewID int, reviewedBy string) (*ReviewQueueEntry, error) {
	if params.PrimaryULID == "" || params.DuplicateULID == "" {
		return nil, ErrInvalidUpdateParams
	}

	if params.PrimaryULID == params.DuplicateULID {
		return nil, ErrCannotMergeSameEvent
	}

	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on error (no-op after commit)
	defer func() {
		_ = txCommitter.Rollback(ctx)
	}()

	// Lock the review row FIRST so that concurrent requests on the same review
	// are serialised here.  The second request will block, then re-read the
	// already-processed status and return ErrConflict.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("lock review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, fmt.Errorf("review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// Look up the companion review on the primary event (the near_duplicate_of_new_event
	// row that points back to the duplicate) and lock it BEFORE any event locks.
	// This matches the review-first lock ordering used by add-occurrence and prevents
	// deadlocks when concurrent admin actions touch the same review/event set.
	companionReview, compErr := txRepo.GetPendingReviewByEventUlidAndDuplicateUlid(ctx, params.PrimaryULID, params.DuplicateULID)
	if compErr != nil && !errors.Is(compErr, ErrNotFound) {
		return nil, fmt.Errorf("lookup companion review for primary %s: %w", params.PrimaryULID, compErr)
	}
	// companionReview == nil when ErrNotFound or no pending entry — both are fine.

	var lockedCompanion *ReviewQueueEntry
	if companionReview != nil {
		lc, lockErr := txRepo.LockReviewQueueEntryForUpdate(ctx, companionReview.ID)
		if lockErr != nil {
			if errors.Is(lockErr, ErrNotFound) {
				// Companion was concurrently deleted — treat as absent.
				lockedCompanion = nil
			} else {
				return nil, fmt.Errorf("lock companion review id=%d: %w", companionReview.ID, lockErr)
			}
		} else {
			lockedCompanion = lc
		}
	}

	if err := s.executeMerge(ctx, txRepo, params); err != nil {
		return nil, err
	}

	// Dismiss the companion review entry on the primary event (if present and still
	// pending).  This mirrors the companion handling in AddOccurrenceFromReview.
	// IMPORTANT: pass params.PrimaryULID here — the duplicate has already been
	// soft-deleted by executeMerge, so MergeReview's live-event lookup
	// (WHERE ulid = $1 AND deleted_at IS NULL) would fail if given DuplicateULID.
	if lockedCompanion != nil && lockedCompanion.Status == "pending" {
		if _, mergeErr := txRepo.MergeReview(ctx, lockedCompanion.ID, reviewedBy, params.PrimaryULID); mergeErr != nil {
			if errors.Is(mergeErr, ErrNotFound) || errors.Is(mergeErr, ErrConflict) {
				// Race outcome: companion was already dismissed — non-fatal.
				_ = mergeErr
			} else {
				return nil, fmt.Errorf("dismiss companion review id=%d: %w", lockedCompanion.ID, mergeErr)
			}
		}
	}

	// Update the review queue entry to "merged" status — within the same transaction
	reviewEntry, err := txRepo.MergeReview(ctx, reviewID, reviewedBy, params.PrimaryULID)
	if err != nil {
		return nil, fmt.Errorf("update review status: %w", err)
	}

	// Recompute whether the primary event should leave pending_review.  The merge
	// resolved the companion review (if any), but the primary may have OTHER
	// unresolved review rows.  Only restore lifecycle to "published" if none remain.
	primary, primaryErr := txRepo.GetByULID(ctx, params.PrimaryULID)
	if primaryErr != nil {
		return nil, fmt.Errorf("get primary event post-merge: %w", primaryErr)
	}
	if primary.LifecycleState == "pending_review" {
		remaining, remErr := txRepo.GetPendingReviewByEventUlid(ctx, params.PrimaryULID)
		if remErr != nil && !errors.Is(remErr, ErrNotFound) {
			return nil, fmt.Errorf("recheck pending review for primary %s: %w", params.PrimaryULID, remErr)
		}
		if remaining == nil {
			publishedState := "published"
			if _, err := txRepo.UpdateEvent(ctx, params.PrimaryULID, UpdateEventParams{
				LifecycleState: &publishedState,
			}); err != nil {
				return nil, fmt.Errorf("restore primary lifecycle to published: %w", err)
			}
		}
	}

	// Commit transaction — all operations succeed or none do
	err = txCommitter.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return reviewEntry, nil
}

// executeMerge performs the core merge operations within an existing transaction:
// verify events, enrich primary, soft-delete duplicate, create tombstone.
func (s *AdminService) executeMerge(ctx context.Context, txRepo Repository, params MergeEventsParams) error {
	// Verify both events exist
	primary, err := txRepo.GetByULID(ctx, params.PrimaryULID)
	if err != nil {
		return fmt.Errorf("primary event not found: %w", err)
	}

	duplicate, err := txRepo.GetByULID(ctx, params.DuplicateULID)
	if err != nil {
		return fmt.Errorf("duplicate event not found: %w", err)
	}

	// Verify neither event is deleted or already merged
	if primary.LifecycleState == "deleted" {
		log.Warn().
			Str("primary_ulid", params.PrimaryULID).
			Str("duplicate_ulid", params.DuplicateULID).
			Msg("merge rejected: primary event is deleted")
		return fmt.Errorf("primary event %s: %w", params.PrimaryULID, ErrEventDeleted)
	}
	if duplicate.LifecycleState == "deleted" {
		log.Warn().
			Str("primary_ulid", params.PrimaryULID).
			Str("duplicate_ulid", params.DuplicateULID).
			Msg("merge rejected: duplicate event is already deleted or merged")
		return fmt.Errorf("duplicate event %s: %w", params.DuplicateULID, ErrEventDeleted)
	}

	// Enrich primary event with data from the duplicate before soft-deleting it.
	// Admin merges use equal trust (0, 0) so only gap-filling occurs — the
	// duplicate's data fills empty fields on the primary but never overwrites.
	dupInput := EventInputFromEvent(duplicate)
	enrichParams, enriched := AutoMergeFields(primary, dupInput, 0, 0)
	if enriched {
		_, err = txRepo.UpdateEvent(ctx, params.PrimaryULID, enrichParams)
		if err != nil {
			return fmt.Errorf("enrich primary event: %w", err)
		}
		log.Info().
			Str("primary_ulid", params.PrimaryULID).
			Str("duplicate_ulid", params.DuplicateULID).
			Msg("enriched primary event with duplicate data during merge")
	}

	// Merge duplicate into primary (soft delete + set merged_into_id)
	err = txRepo.MergeEvents(ctx, params.DuplicateULID, params.PrimaryULID)
	if err != nil {
		return fmt.Errorf("merge events: %w", err)
	}

	// Generate tombstone for the duplicate event
	primaryURI, err := s.eventURI(params.PrimaryULID)
	if err != nil {
		return fmt.Errorf("canonical URI for primary event: %w", err)
	}
	duplicateURI, err := s.eventURI(duplicate.ULID)
	if err != nil {
		return fmt.Errorf("canonical URI for duplicate event: %w", err)
	}
	tombstonePayload, err := buildTombstonePayload(duplicateURI, duplicate.Name, &primaryURI, "duplicate_merged")
	if err != nil {
		return fmt.Errorf("build tombstone: %w", err)
	}

	tombstoneParams := TombstoneCreateParams{
		EventID:      duplicate.ID,
		EventURI:     duplicateURI,
		DeletedAt:    time.Now(),
		Reason:       "duplicate_merged",
		SupersededBy: &primaryURI,
		Payload:      tombstonePayload,
	}

	err = txRepo.CreateTombstone(ctx, tombstoneParams)
	if err != nil {
		return fmt.Errorf("create tombstone: %w", err)
	}

	return nil
}

// ApproveEventWithReview atomically publishes an event AND marks its review queue entry
// as approved in a single database transaction. This prevents inconsistency where the
// event is published but the review stays pending.
//
// Lock ordering: review row is locked first, then the event row is read/written.
// This matches the ordering used by all other review-based admin methods and
// prevents deadlocks when concurrent admin actions touch the same review/event set.
func (s *AdminService) ApproveEventWithReview(ctx context.Context, eventULID string, reviewID int, reviewedBy string, notes *string) (*ReviewQueueEntry, error) {
	if eventULID == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on error (no-op after commit)
	defer func() {
		_ = txCommitter.Rollback(ctx)
	}()

	// Lock the review row FIRST so that concurrent requests on the same review
	// are serialised here.  The second request will block, then re-read the
	// already-processed status and return ErrConflict.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("lock review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, fmt.Errorf("review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// Publish the event within the transaction
	existing, err := txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}

	if existing.LifecycleState != "published" {
		published := "published"
		_, err = txRepo.UpdateEvent(ctx, eventULID, UpdateEventParams{
			LifecycleState: &published,
		})
		if err != nil {
			return nil, fmt.Errorf("publish event: %w", err)
		}
	}

	// Mark review as approved within the same transaction
	reviewEntry, err := txRepo.ApproveReview(ctx, reviewID, reviewedBy, notes)
	if err != nil {
		return nil, fmt.Errorf("approve review: %w", err)
	}

	// Commit transaction — both operations succeed or neither does
	err = txCommitter.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return reviewEntry, nil
}

// RejectEventWithReview atomically soft-deletes an event with a tombstone AND marks its
// review queue entry as rejected in a single database transaction. This prevents inconsistency
// where the event is deleted but the review stays pending.
//
// Lock ordering: review row is locked first, then the event row is read/written.
// This matches the ordering used by all other review-based admin methods and
// prevents deadlocks when concurrent admin actions touch the same review/event set.
func (s *AdminService) RejectEventWithReview(ctx context.Context, eventULID string, reviewID int, reviewedBy string, reason string) (*ReviewQueueEntry, error) {
	if eventULID == "" || reason == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on error (no-op after commit)
	defer func() {
		_ = txCommitter.Rollback(ctx)
	}()

	// Lock the review row FIRST so that concurrent requests on the same review
	// are serialised here.  The second request will block, then re-read the
	// already-processed status and return ErrConflict.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("lock review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, fmt.Errorf("review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// Get the event for tombstone generation
	event, err := txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}

	// Soft delete the event
	err = txRepo.SoftDeleteEvent(ctx, eventULID, reason)
	if err != nil {
		return nil, fmt.Errorf("soft delete event: %w", err)
	}

	// Generate tombstone
	eventURI, err := s.eventURI(event.ULID)
	if err != nil {
		return nil, fmt.Errorf("canonical URI for event: %w", err)
	}
	tombstonePayload, err := buildTombstonePayload(eventURI, event.Name, nil, reason)
	if err != nil {
		return nil, fmt.Errorf("build tombstone: %w", err)
	}

	tombstoneParams := TombstoneCreateParams{
		EventID:      event.ID,
		EventURI:     eventURI,
		DeletedAt:    time.Now(),
		Reason:       reason,
		SupersededBy: nil,
		Payload:      tombstonePayload,
	}

	err = txRepo.CreateTombstone(ctx, tombstoneParams)
	if err != nil {
		return nil, fmt.Errorf("create tombstone: %w", err)
	}

	// Mark review as rejected within the same transaction
	reviewEntry, err := txRepo.RejectReview(ctx, reviewID, reviewedBy, reason)
	if err != nil {
		return nil, fmt.Errorf("reject review: %w", err)
	}

	// Commit transaction — all operations succeed or none do
	err = txCommitter.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return reviewEntry, nil
}

// FixAndApproveEventWithReview atomically fixes occurrence dates, publishes the event,
// AND marks the review queue entry as approved in a single database transaction.
//
// Lock ordering: review row is locked first, then the event row is read/written.
// This matches the ordering used by all other review-based admin methods and
// prevents deadlocks when concurrent admin actions touch the same review/event set.
func (s *AdminService) FixAndApproveEventWithReview(ctx context.Context, eventULID string, reviewID int, reviewedBy string, notes *string, startDate *time.Time, endDate *time.Time) (*ReviewQueueEntry, error) {
	if eventULID == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on error (no-op after commit)
	defer func() {
		_ = txCommitter.Rollback(ctx)
	}()

	// Lock the review row FIRST so that concurrent requests on the same review
	// are serialised here.  The second request will block, then re-read the
	// already-processed status and return ErrConflict.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("lock review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, fmt.Errorf("review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// Get the event and its occurrences
	existing, err := txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}

	// Determine effective start and end times
	var effectiveStart time.Time
	var effectiveEnd *time.Time

	if len(existing.Occurrences) == 0 {
		// Event was ingested with reversed dates — occurrence was intentionally
		// skipped during ingest (ingest.go:713-715). Create one now with corrected dates.
		// Both start and end dates MUST be provided when creating from scratch.
		if startDate == nil || endDate == nil {
			return nil, fmt.Errorf("both startDate and endDate are required to fix events without occurrences")
		}

		// Validate corrected dates
		if endDate.Before(*startDate) {
			return nil, FilterError{Field: "endDate", Message: "end date cannot be before start date"}
		}

		effectiveStart = *startDate
		effectiveEnd = endDate

		// Create the missing occurrence using event metadata (venue, timezone, etc.)
		err = txRepo.CreateOccurrence(ctx, OccurrenceCreateParams{
			EventID:    existing.ID,
			StartTime:  effectiveStart,
			EndTime:    effectiveEnd,
			Timezone:   s.defaultTZ,
			VenueID:    existing.PrimaryVenueID,
			VirtualURL: nil, // Use event's VirtualURL if set
		})
		if err != nil {
			return nil, fmt.Errorf("create missing occurrence: %w", err)
		}
	} else {
		// Event has existing occurrences - update them with corrected dates
		if startDate != nil {
			effectiveStart = *startDate
		} else {
			effectiveStart = existing.Occurrences[0].StartTime
		}

		if endDate != nil {
			effectiveEnd = endDate
		} else {
			effectiveEnd = existing.Occurrences[0].EndTime
		}

		// Validate: end must not be before start
		if effectiveEnd != nil && effectiveEnd.Before(effectiveStart) {
			return nil, FilterError{Field: "endDate", Message: "end date cannot be before start date"}
		}

		// Fix occurrence dates within the transaction
		err = txRepo.UpdateOccurrenceDates(ctx, eventULID, effectiveStart, effectiveEnd)
		if err != nil {
			return nil, fmt.Errorf("fix occurrence dates: %w", err)
		}
	}

	// Publish the event within the transaction
	if existing.LifecycleState != "published" {
		published := "published"
		_, err = txRepo.UpdateEvent(ctx, eventULID, UpdateEventParams{
			LifecycleState: &published,
		})
		if err != nil {
			return nil, fmt.Errorf("publish event: %w", err)
		}
	}

	// Mark review as approved within the same transaction
	reviewEntry, err := txRepo.ApproveReview(ctx, reviewID, reviewedBy, notes)
	if err != nil {
		return nil, fmt.Errorf("approve review: %w", err)
	}

	// Commit transaction — all operations succeed or none do
	err = txCommitter.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return reviewEntry, nil
}

// Consolidate atomically consolidates multiple events into one canonical event while
// retiring the others. The canonical event is either an existing event (promoted via
// EventULID) or a newly created event (supplied via Event). All events in the Retire
// list are soft-deleted with tombstones pointing to the canonical event, and any
// pending review queue entries for retired events are dismissed.
//
// Lock ordering: retired events are sorted by ULID before locking to prevent deadlocks.
// The canonical event (promote path) is locked after retired events.
//
// Returns ConsolidateResult with the canonical event and summary of side effects.
func (s *AdminService) Consolidate(ctx context.Context, params ConsolidateParams) (*ConsolidateResult, error) {
	// Step 1: Validate params.
	if params.Event != nil && params.EventULID != "" {
		return nil, ErrConsolidateBothEventFields
	}
	if params.Event == nil && params.EventULID == "" {
		return nil, ErrConsolidateNoEventField
	}
	if len(params.Retire) == 0 {
		return nil, ErrConsolidateNoRetire
	}
	// Check canonical ULID not in retire list (only applicable for promote path).
	if params.EventULID != "" {
		for _, r := range params.Retire {
			if r == params.EventULID {
				return nil, ErrConsolidateCanonicalInRetire
			}
		}
	}

	// Step 2: Begin transaction.
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	// Step 3: Lock retired events in sorted order (deadlock prevention).
	sorted := make([]string, len(params.Retire))
	copy(sorted, params.Retire)
	slices.Sort(sorted)

	retiredEvents := make([]*Event, 0, len(sorted))
	for _, ulid := range sorted {
		ev, err := txRepo.GetByULID(ctx, ulid)
		if err != nil {
			return nil, fmt.Errorf("get retire target %s: %w", ulid, err)
		}
		if ev.LifecycleState == "deleted" {
			return nil, fmt.Errorf("retire target %s is already deleted: %w", ulid, ErrConsolidateRetiredAlreadyDeleted)
		}
		if err := txRepo.LockEventForUpdate(ctx, ev.ID); err != nil {
			return nil, fmt.Errorf("lock retire target %s: %w", ulid, err)
		}
		// Re-read after lock (TOCTOU guard).
		ev, err = txRepo.GetByULID(ctx, ulid)
		if err != nil {
			return nil, fmt.Errorf("re-read retire target %s after lock: %w", ulid, err)
		}
		if ev.LifecycleState == "deleted" {
			return nil, fmt.Errorf("retire target %s was deleted concurrently: %w", ulid, ErrConsolidateRetiredAlreadyDeleted)
		}
		retiredEvents = append(retiredEvents, ev)
	}

	// Step 4: Resolve canonical event.
	var canonicalEvent *Event
	var warnings []ValidationWarning
	needsReview := false
	isDuplicate := false

	if params.EventULID != "" {
		// Promote path: fetch and lock existing event.
		canon, err := txRepo.GetByULID(ctx, params.EventULID)
		if err != nil {
			return nil, fmt.Errorf("get canonical event %s: %w", params.EventULID, err)
		}
		if canon.LifecycleState == "deleted" {
			return nil, fmt.Errorf("canonical event %s is deleted: %w", params.EventULID, ErrEventDeleted)
		}
		if err := txRepo.LockEventForUpdate(ctx, canon.ID); err != nil {
			return nil, fmt.Errorf("lock canonical event %s: %w", params.EventULID, err)
		}
		// Re-read after lock (TOCTOU).
		canon, err = txRepo.GetByULID(ctx, params.EventULID)
		if err != nil {
			return nil, fmt.Errorf("re-read canonical event %s after lock: %w", params.EventULID, err)
		}
		if canon.LifecycleState == "deleted" {
			return nil, fmt.Errorf("canonical event %s was deleted concurrently: %w", params.EventULID, ErrEventDeleted)
		}
		canonicalEvent = canon
	} else {
		// Create path: delegate to the shared ingest pipeline.
		// SkipDedupAutoMerge=true: warn instead of auto-merging (admin has explicitly
		// chosen to create a new canonical event).
		// ExcludeFromNearDup=params.Retire: don't flag the events we are retiring as
		// near-duplicates of the new canonical.
		// TxRepo=txRepo: participate in the caller's transaction so that event
		// creation and retirement are atomic.
		coreResult, coreErr := s.ingestService.createEventCore(ctx, *params.Event, CreateEventCoreOptions{
			SkipDedupAutoMerge: true,
			ExcludeFromNearDup: params.Retire,
			TxRepo:             txRepo,
		})
		if coreErr != nil {
			return nil, fmt.Errorf("create canonical event: %w", coreErr)
		}
		canonicalEvent = coreResult.Event
		warnings = append(warnings, coreResult.Warnings...)
		if coreResult.IsDuplicate {
			isDuplicate = true
		}
		if coreResult.NeedsReview {
			needsReview = true
		}
	}

	// Steps 5–6: Retire events — soft-delete + tombstone, then dismiss pending reviews.
	retiredULIDs, dismissedIDs, err := s.consolidateRetireEvents(ctx, txRepo, retiredEvents, canonicalEvent.ULID, params.Retire)
	if err != nil {
		return nil, err
	}

	// Step 6b: Strip stale dup warnings from the canonical's existing review entry.
	dismissedIDs, err = s.consolidateStripRetiredDupWarnings(ctx, txRepo, canonicalEvent, params.Retire, dismissedIDs)
	if err != nil {
		return nil, err
	}

	// Step 7: Post-consolidation near-dup check (only if canonical has a venue).
	// The error return from consolidatePostValidation is always nil — non-fatal
	// errors are logged internally and do not propagate to avoid blocking the
	// consolidation transaction on a best-effort check.
	dupResult, dupWarnings, _ := s.consolidatePostValidation(ctx, txRepo, canonicalEvent, params.Retire, s.ingestService.dedupConfig.NearDuplicateThreshold)
	if dupResult {
		isDuplicate = true
		needsReview = true
	}
	warnings = append(warnings, dupWarnings...)

	// Step 8: If needs review, set lifecycle to pending_review.
	if needsReview {
		if err := s.consolidateResolvePending(ctx, txRepo, canonicalEvent, warnings); err != nil {
			return nil, err
		}
	}

	// Step 9: Commit transaction.
	if err := txCommitter.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &ConsolidateResult{
		Event:                  canonicalEvent,
		LifecycleState:         canonicalEvent.LifecycleState,
		IsDuplicate:            isDuplicate,
		IsMerged:               false,
		NeedsReview:            needsReview,
		Warnings:               warnings,
		Retired:                retiredULIDs,
		ReviewEntriesDismissed: dismissedIDs,
	}, nil
}

// consolidateRetireEvents encapsulates steps 5 and 6 of the Consolidate algorithm:
// soft-delete each retired event with a tombstone pointing to the canonical event,
// then dismiss any pending review queue entries for those events.
func (s *AdminService) consolidateRetireEvents(
	ctx context.Context,
	txRepo Repository,
	retiredEvents []*Event,
	canonicalULID string,
	retireULIDs []string,
) (retiredULIDs []string, dismissedIDs []int, err error) {
	// Step 5: Retire events — soft-delete + tombstone.
	canonicalURI, err := s.eventURI(canonicalULID)
	if err != nil {
		return nil, nil, fmt.Errorf("canonical URI for consolidation target: %w", err)
	}

	retiredULIDs = make([]string, 0, len(retiredEvents))
	for _, ev := range retiredEvents {
		if err := txRepo.SoftDeleteEvent(ctx, ev.ULID, "consolidated"); err != nil {
			return nil, nil, fmt.Errorf("soft-delete retired event %s: %w", ev.ULID, err)
		}
		retiredURI, err := s.eventURI(ev.ULID)
		if err != nil {
			return nil, nil, fmt.Errorf("canonical URI for retired event %s: %w", ev.ULID, err)
		}
		tombstonePayload, err := buildTombstonePayload(retiredURI, ev.Name, &canonicalURI, "consolidated")
		if err != nil {
			return nil, nil, fmt.Errorf("build tombstone for %s: %w", ev.ULID, err)
		}
		if err := txRepo.CreateTombstone(ctx, TombstoneCreateParams{
			EventID:      ev.ID,
			EventURI:     retiredURI,
			DeletedAt:    time.Now(),
			Reason:       "consolidated",
			SupersededBy: &canonicalURI,
			Payload:      tombstonePayload,
		}); err != nil {
			return nil, nil, fmt.Errorf("create tombstone for %s: %w", ev.ULID, err)
		}
		retiredULIDs = append(retiredULIDs, ev.ULID)
	}

	// Step 6: Dismiss pending reviews for retired events.
	dismissedIDs, err = txRepo.DismissPendingReviewsByEventULIDs(ctx, retireULIDs, "system")
	if err != nil {
		return nil, nil, fmt.Errorf("dismiss pending reviews for retired events: %w", err)
	}

	return retiredULIDs, dismissedIDs, nil
}

// consolidateStripRetiredDupWarnings is Step 6b of the Consolidate algorithm.
// It removes stale near-dup and potential-duplicate warnings from the canonical
// event's pending review entry — warnings that point to events that have just
// been retired. If stripping leaves zero warnings, the entry is dismissed and
// the canonical is published.
//
// The dismissedIDs slice is extended and returned (canonical entry ID appended
// if fully dismissed).
func (s *AdminService) consolidateStripRetiredDupWarnings(
	ctx context.Context,
	txRepo Repository,
	canonicalEvent *Event,
	retireULIDs []string,
	dismissedIDs []int,
) (updatedDismissedIDs []int, err error) {
	// Look up the canonical event's pending review entry.
	entry, err := txRepo.GetPendingReviewByEventUlid(ctx, canonicalEvent.ULID)
	if err != nil {
		return dismissedIDs, fmt.Errorf("look up canonical pending review: %w", err)
	}
	if entry == nil {
		// No pending review entry — nothing to strip.
		return dismissedIDs, nil
	}

	// Build a fast-lookup set of retired ULIDs.
	retireSet := make(map[string]struct{}, len(retireULIDs))
	for _, u := range retireULIDs {
		retireSet[u] = struct{}{}
	}

	// Parse the warnings JSON from the review entry.
	var warnings []ValidationWarning
	if len(entry.Warnings) > 0 {
		if err := json.Unmarshal(entry.Warnings, &warnings); err != nil {
			// Malformed warnings — log and leave the entry untouched.
			log.Warn().Err(err).
				Int("review_id", entry.ID).
				Str("canonical_ulid", canonicalEvent.ULID).
				Msg("Consolidate: failed to parse canonical review warnings; skipping strip")
			return dismissedIDs, nil
		}
	}

	// Filter out dup warnings whose matches reference a retired ULID.
	filtered := warnings[:0]
	changed := false
	for _, w := range warnings {
		if w.Code != "near_duplicate_of_new_event" && w.Code != "potential_duplicate" {
			filtered = append(filtered, w)
			continue
		}

		// Check if the warning's matches reference any retired ULID.
		// potential_duplicate warnings carry details.matches[].ulid;
		// near_duplicate_of_new_event carries companion info via
		// DuplicateOfEventULID (no matches array — strip when DuplicateOfEventULID
		// points to a retired event, or when there are no matches at all).
		referencesRetired := false

		if details, ok := w.Details["matches"]; ok {
			// Marshal + unmarshal to normalise type — Details is map[string]any
			// so matches may be []any after JSON round-trip.
			matchesRaw, _ := json.Marshal(details)
			var matchList []map[string]any
			if json.Unmarshal(matchesRaw, &matchList) == nil {
				for _, m := range matchList {
					if ulid, ok := m["ulid"].(string); ok {
						if _, retired := retireSet[ulid]; retired {
							referencesRetired = true
							break
						}
					}
				}
			}
			// If matchList is empty, the warning is a bare companion warning — strip it.
			if len(matchList) == 0 {
				referencesRetired = true
			}
		} else {
			// No matches field: bare near_duplicate_of_new_event companion warning.
			// Strip it — the companion relationship is gone after the retire.
			referencesRetired = true
		}

		// Also check DuplicateOfEventULID on the entry itself.
		if !referencesRetired && entry.DuplicateOfEventULID != nil {
			if _, retired := retireSet[*entry.DuplicateOfEventULID]; retired {
				referencesRetired = true
			}
		}

		if referencesRetired {
			changed = true
			// Do not append — this warning is stripped.
		} else {
			filtered = append(filtered, w)
		}
	}

	if !changed {
		// Nothing was stripped — leave the entry as-is.
		return dismissedIDs, nil
	}

	if len(filtered) == 0 {
		// All warnings stripped — dismiss the entry and publish the canonical.
		if _, dismissErr := txRepo.MergeReview(ctx, entry.ID, "system", canonicalEvent.ULID); dismissErr != nil {
			return dismissedIDs, fmt.Errorf("dismiss canonical review entry %d after stripping dup warnings: %w", entry.ID, dismissErr)
		}
		dismissedIDs = append(dismissedIDs, entry.ID)

		published := "published"
		if _, updateErr := txRepo.UpdateEvent(ctx, canonicalEvent.ULID, UpdateEventParams{
			LifecycleState: &published,
		}); updateErr != nil {
			return dismissedIDs, fmt.Errorf("restore canonical event %s to published after stripping dup warnings: %w", canonicalEvent.ULID, updateErr)
		}
		canonicalEvent.LifecycleState = "published"
		return dismissedIDs, nil
	}

	// Warnings remain — update the entry with the pruned warnings list.
	// TODO: also clear DuplicateOfEventID when it points to a retired event;
	// ReviewQueueUpdateParams does not yet expose DuplicateOfEventID clearing.
	updatedJSON, err := json.Marshal(filtered)
	if err != nil {
		return dismissedIDs, fmt.Errorf("marshal updated warnings for canonical review entry %d: %w", entry.ID, err)
	}
	if _, err := txRepo.UpdateReviewQueueEntry(ctx, entry.ID, ReviewQueueUpdateParams{
		Warnings: &updatedJSON,
	}); err != nil {
		return dismissedIDs, fmt.Errorf("update canonical review entry %d with stripped warnings: %w", entry.ID, err)
	}
	return dismissedIDs, nil
}

// consolidatePostValidation encapsulates step 7 of the Consolidate algorithm:
// post-consolidation near-duplicate check. Only runs if the canonical event has a venue
// and occurrences. Non-fatal — errors are logged and not returned. Filters out
// self-match and events being retired.
func (s *AdminService) consolidatePostValidation(
	ctx context.Context,
	txRepo Repository,
	canonical *Event,
	retireULIDs []string,
	threshold float64,
) (isDuplicate bool, warnings []ValidationWarning, err error) {
	// Step 7: Post-consolidation near-dup check (only if canonical has a venue).
	// This mirrors the ingest pipeline: a consolidated event is not more valid than
	// an ingested one and must go through the same dedup checks.
	if canonical.PrimaryVenueID == nil || len(canonical.Occurrences) == 0 {
		return false, nil, nil
	}

	candidates, nearDupErr := txRepo.FindNearDuplicates(ctx,
		*canonical.PrimaryVenueID,
		canonical.Occurrences[0].StartTime,
		canonical.Name,
		threshold)
	if nearDupErr != nil {
		// Non-fatal — log and continue. The retire+create/promote has already
		// succeeded; failing the whole transaction for a dedup check error would
		// be wrong (mirrors ingest.go line 573-576).
		log.Warn().Err(nearDupErr).
			Str("canonical_ulid", canonical.ULID).
			Msg("Consolidate: near-duplicate check failed, continuing")
		return false, nil, nil
	}

	// Filter self-match and retired events — FindNearDuplicates does not
	// exclude the canonical event itself or the events being retired in
	// this consolidation, so we do it here (mirrors ingest.go 580-593).
	retireSet := make(map[string]struct{}, len(retireULIDs))
	for _, r := range retireULIDs {
		retireSet[r] = struct{}{}
	}
	filtered := make([]NearDuplicateCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.ULID == canonical.ULID {
			continue
		}
		if _, retiring := retireSet[c.ULID]; retiring {
			continue
		}
		filtered = append(filtered, c)
	}

	if len(filtered) == 0 {
		return false, nil, nil
	}

	// Append near-dup warnings.
	dupWarnings := make([]ValidationWarning, 0, len(filtered))
	for _, c := range filtered {
		dupWarnings = append(dupWarnings, ValidationWarning{
			Code:    "potential_duplicate",
			Message: fmt.Sprintf("near-duplicate of existing event %s (similarity %.2f)", c.ULID, c.Similarity),
		})
	}
	return true, dupWarnings, nil
}

// consolidateResolvePending encapsulates step 8 of the Consolidate algorithm:
// if the canonical event needs review, update its lifecycle to pending_review and
// create a review queue entry with the supplied warnings.
func (s *AdminService) consolidateResolvePending(
	ctx context.Context,
	txRepo Repository,
	canonical *Event,
	warnings []ValidationWarning,
) error {
	pendingReview := "pending_review"
	if _, err := txRepo.UpdateEvent(ctx, canonical.ULID, UpdateEventParams{
		LifecycleState: &pendingReview,
	}); err != nil {
		return fmt.Errorf("set canonical event to pending_review: %w", err)
	}
	canonical.LifecycleState = "pending_review"

	// Create review queue entry for canonical event.
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		return fmt.Errorf("marshal consolidation warnings: %w", err)
	}

	// Build a display payload from the canonical event's fields so that
	// OriginalPayload and NormalizedPayload are never nil (NOT NULL columns).
	payloadMap := map[string]any{
		"name": canonical.Name,
	}
	if canonical.Description != "" {
		payloadMap["description"] = canonical.Description
	}
	if canonical.PublicURL != "" {
		payloadMap["url"] = canonical.PublicURL
	}
	if len(canonical.Occurrences) > 0 {
		payloadMap["startDate"] = canonical.Occurrences[0].StartTime.Format(time.RFC3339)
		if canonical.Occurrences[0].EndTime != nil {
			payloadMap["endDate"] = canonical.Occurrences[0].EndTime.Format(time.RFC3339)
		}
	}
	if canonical.PrimaryVenueName != nil {
		payloadMap["location"] = map[string]any{"name": *canonical.PrimaryVenueName}
	}
	payloadJSON, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("marshal canonical payload for review queue: %w", err)
	}

	var eventStartTime time.Time
	if len(canonical.Occurrences) > 0 {
		eventStartTime = canonical.Occurrences[0].StartTime
	}

	if _, err := txRepo.CreateReviewQueueEntry(ctx, ReviewQueueCreateParams{
		EventID:           canonical.ID,
		OriginalPayload:   payloadJSON,
		NormalizedPayload: payloadJSON,
		Warnings:          warningsJSON,
		EventStartTime:    eventStartTime,
	}); err != nil {
		return fmt.Errorf("create review queue entry for canonical event: %w", err)
	}
	return nil
}

// DeleteEvent soft-deletes an event and generates a tombstone
// Returns the deleted event for tombstone generation
func (s *AdminService) DeleteEvent(ctx context.Context, ulid string, reason string) error {
	if ulid == "" {
		return ErrInvalidUpdateParams
	}

	// Get existing event before deletion
	event, err := s.repo.GetByULID(ctx, ulid)
	if err != nil {
		return fmt.Errorf("get event by ULID: %w", err)
	}

	// Soft delete the event
	err = s.repo.SoftDeleteEvent(ctx, ulid, reason)
	if err != nil {
		return fmt.Errorf("soft delete event: %w", err)
	}

	// Generate tombstone JSON-LD payload
	eventURI, err := s.eventURI(event.ULID)
	if err != nil {
		return fmt.Errorf("canonical URI for event: %w", err)
	}
	tombstonePayload, err := buildTombstonePayload(eventURI, event.Name, nil, reason)
	if err != nil {
		return fmt.Errorf("build tombstone: %w", err)
	}

	// Create tombstone record
	tombstoneParams := TombstoneCreateParams{
		EventID:      event.ID,
		EventURI:     eventURI,
		DeletedAt:    time.Now(),
		Reason:       reason,
		SupersededBy: nil,
		Payload:      tombstonePayload,
	}

	err = s.repo.CreateTombstone(ctx, tombstoneParams)
	if err != nil {
		return fmt.Errorf("create tombstone: %w", err)
	}

	return nil
}

// validateUpdateParams validates update parameters
func (s *AdminService) validateUpdateParams(params UpdateEventParams) error {
	if params.Name != nil {
		name := strings.TrimSpace(*params.Name)
		if name == "" {
			return FilterError{Field: "name", Message: "cannot be empty"}
		}
		if len(name) > s.validationConfig.MaxEventNameLength {
			return FilterError{Field: "name", Message: fmt.Sprintf("exceeds maximum length of %d characters", s.validationConfig.MaxEventNameLength)}
		}
	}

	if params.LifecycleState != nil {
		state := strings.ToLower(strings.TrimSpace(*params.LifecycleState))
		validStates := map[string]bool{
			"draft":       true,
			"published":   true,
			"postponed":   true,
			"rescheduled": true,
			"sold_out":    true,
			"cancelled":   true,
			"completed":   true,
		}
		if !validStates[state] {
			return FilterError{Field: "lifecycle_state", Message: "invalid state"}
		}
	}

	if params.EventDomain != nil {
		domain := strings.ToLower(strings.TrimSpace(*params.EventDomain))
		validDomains := map[string]bool{
			"arts":      true,
			"music":     true,
			"culture":   true,
			"sports":    true,
			"community": true,
			"education": true,
			"general":   true,
		}
		if !validDomains[domain] {
			return FilterError{Field: "event_domain", Message: "invalid domain"}
		}
	}

	// Validate image_url
	if params.ImageURL != nil && *params.ImageURL != "" {
		if err := validation.ValidateURL(*params.ImageURL, "image_url", s.requireHTTPS); err != nil {
			return fmt.Errorf("validate image_url: %w", err)
		}
	}

	// Validate public_url
	if params.PublicURL != nil && *params.PublicURL != "" {
		if err := validation.ValidateURL(*params.PublicURL, "public_url", s.requireHTTPS); err != nil {
			return fmt.Errorf("validate public_url: %w", err)
		}
	}

	return nil
}

// buildUpdateMap creates a map of fields to update
func buildUpdateMap(existing *Event, params UpdateEventParams) map[string]any {
	updates := make(map[string]any)

	if params.Name != nil && *params.Name != existing.Name {
		updates["name"] = *params.Name
	}
	if params.Description != nil && *params.Description != existing.Description {
		updates["description"] = *params.Description
	}
	if params.LifecycleState != nil && *params.LifecycleState != existing.LifecycleState {
		updates["lifecycle_state"] = *params.LifecycleState
	}
	if params.ImageURL != nil && *params.ImageURL != existing.ImageURL {
		updates["image_url"] = *params.ImageURL
	}
	if params.PublicURL != nil && *params.PublicURL != existing.PublicURL {
		updates["public_url"] = *params.PublicURL
	}
	if params.EventDomain != nil && *params.EventDomain != existing.EventDomain {
		updates["event_domain"] = *params.EventDomain
	}
	if len(params.Keywords) > 0 {
		// Compare keywords
		if !equalKeywords(existing.Keywords, params.Keywords) {
			updates["keywords"] = params.Keywords
		}
	}

	return updates
}

// equalKeywords compares two keyword slices
func equalKeywords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildTombstonePayload generates a JSON-LD tombstone payload according to SEL spec.
// eventURI must be the canonical URI for the deleted event (e.g. from AdminService.eventURI).
// See: docs/togather_SEL_Interoperability_Profile_v0.1.md section 1.6
func buildTombstonePayload(eventURI, name string, supersededBy *string, reason string) ([]byte, error) {
	tombstone := map[string]interface{}{
		"@context":           "https://schema.org",
		"@type":              "Event",
		"@id":                eventURI,
		"name":               name,
		"eventStatus":        "https://schema.org/EventCancelled",
		"sel:tombstone":      true,
		"sel:deletedAt":      time.Now().Format(time.RFC3339),
		"sel:deletionReason": reason,
	}

	if supersededBy != nil {
		tombstone["sel:supersededBy"] = *supersededBy
	}

	payload, err := json.Marshal(tombstone)
	if err != nil {
		return nil, fmt.Errorf("marshal tombstone: %w", err)
	}

	return payload, nil
}

// FindSimilarPlaces returns places with similar names in the same locality/region.
func (s *AdminService) FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarPlaceCandidate, error) {
	return s.repo.FindSimilarPlaces(ctx, name, locality, normalizeRegion(region), threshold)
}

// GetPlaceByULID looks up a place by its ULID, returning the PlaceRecord (ID + ULID).
func (s *AdminService) GetPlaceByULID(ctx context.Context, ulid string) (*PlaceRecord, error) {
	return s.repo.GetPlaceByULID(ctx, ulid)
}

// FindSimilarOrganizations returns organizations with similar names in the same locality/region.
func (s *AdminService) FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarOrgCandidate, error) {
	return s.repo.FindSimilarOrganizations(ctx, name, locality, normalizeRegion(region), threshold)
}

// MergePlaces merges a duplicate place into a primary place.
// Parameters are internal UUIDs (not ULIDs). The handler must resolve ULIDs to UUIDs
// via Places.GetByULID() before calling this method.
func (s *AdminService) MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error) {
	if duplicateID == primaryID {
		return nil, fmt.Errorf("cannot merge place with itself")
	}
	return s.repo.MergePlaces(ctx, duplicateID, primaryID)
}

// MergeOrganizations merges a duplicate organization into a primary organization.
// Parameters are internal UUIDs (not ULIDs). The handler must resolve ULIDs to UUIDs
// via Organizations.GetByULID() before calling this method.
func (s *AdminService) MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error) {
	if duplicateID == primaryID {
		return nil, fmt.Errorf("cannot merge organization with itself")
	}
	return s.repo.MergeOrganizations(ctx, duplicateID, primaryID)
}

// CreateOccurrenceOnEvent adds a new occurrence to an existing event.
// It enforces:
//   - The event must not be deleted.
//   - The new occurrence must not overlap any existing occurrence on the event.
//   - Overlap check and insert are atomic (transaction + lock).
//
// Returns the created Occurrence.
func (s *AdminService) CreateOccurrenceOnEvent(ctx context.Context, eventULID string, params OccurrenceCreateParams) (*Occurrence, error) {
	if eventULID == "" {
		return nil, ErrInvalidUpdateParams
	}

	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	event, err := txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("get event %s: %w", eventULID, err)
	}
	if event.LifecycleState == "deleted" {
		return nil, ErrEventDeleted
	}

	if err := txRepo.LockEventForUpdate(ctx, event.ID); err != nil {
		return nil, fmt.Errorf("lock event %s: %w", eventULID, err)
	}

	// Re-read lifecycle_state after acquiring the lock to avoid TOCTOU race
	// with a concurrent delete/state-change between GetByULID and the lock.
	event, err = txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("re-read event %s after lock: %w", eventULID, err)
	}
	if event.LifecycleState == "deleted" {
		return nil, ErrEventDeleted
	}

	// Bind the occurrence to this event's internal UUID (required by InsertOccurrence).
	params.EventID = event.ID

	// Enforce the occurrence_location_required DB constraint before touching the DB.
	if params.VenueID == nil && params.VirtualURL == nil {
		return nil, ErrOccurrenceLocationRequired
	}

	overlaps, err := txRepo.CheckOccurrenceOverlap(ctx, event.ID, params.StartTime, params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("check overlap: %w", err)
	}
	if overlaps {
		return nil, ErrOccurrenceOverlap
	}

	occ, err := txRepo.InsertOccurrence(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("insert occurrence: %w", err)
	}

	if err := txCommitter.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return occ, nil
}

// UpdateOccurrenceOnEvent applies a PATCH-semantic partial update to a specific occurrence
// on an existing event.
// It enforces:
//   - The event must not be deleted.
//   - If start_time/end_time are changed, the updated window must not overlap any other
//     occurrence on the same event.
//   - Read-check-write is atomic (transaction + lock).
//
// Returns the updated Occurrence.
func (s *AdminService) UpdateOccurrenceOnEvent(ctx context.Context, eventULID string, occurrenceID string, params OccurrenceUpdateParams) (*Occurrence, error) {
	if eventULID == "" || occurrenceID == "" {
		return nil, ErrInvalidUpdateParams
	}

	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	event, err := txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("get event %s: %w", eventULID, err)
	}
	if event.LifecycleState == "deleted" {
		return nil, ErrEventDeleted
	}

	if err := txRepo.LockEventForUpdate(ctx, event.ID); err != nil {
		return nil, fmt.Errorf("lock event %s: %w", eventULID, err)
	}

	// Re-read lifecycle_state after acquiring the lock to avoid TOCTOU race.
	event, err = txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return nil, fmt.Errorf("re-read event %s after lock: %w", eventULID, err)
	}
	if event.LifecycleState == "deleted" {
		return nil, ErrEventDeleted
	}

	// Fetch the current occurrence for overlap + location constraint checks.
	current, err := txRepo.GetOccurrenceByID(ctx, event.ID, occurrenceID)
	if err != nil {
		return nil, fmt.Errorf("get occurrence %s: %w", occurrenceID, err)
	}

	// Only check overlap if time fields are being updated.
	if params.StartTime != nil || params.EndTimeSet {
		proposedStart := current.StartTime
		if params.StartTime != nil {
			proposedStart = *params.StartTime
		}
		var proposedEnd *time.Time
		if params.EndTimeSet {
			proposedEnd = params.EndTime // may be nil (clearing end_time)
		} else {
			proposedEnd = current.EndTime
		}

		overlaps, err := txRepo.CheckOccurrenceOverlapExcluding(ctx, event.ID, proposedStart, proposedEnd, occurrenceID)
		if err != nil {
			return nil, fmt.Errorf("check overlap: %w", err)
		}
		if overlaps {
			return nil, ErrOccurrenceOverlap
		}
	}

	// Pre-check the occurrence_location_required DB constraint (venue_id OR virtual_url must be non-null).
	// Simulate the result of the update to catch violations before hitting the DB.
	var proposedVenueID *string
	if params.VenueIDSet {
		proposedVenueID = params.VenueID // may be nil (clearing)
	} else {
		proposedVenueID = current.VenueID
	}
	var proposedVirtualURL *string
	if params.VirtualURLSet {
		proposedVirtualURL = params.VirtualURL // may be nil (clearing)
	} else {
		proposedVirtualURL = current.VirtualURL
	}
	if proposedVenueID == nil && proposedVirtualURL == nil {
		return nil, ErrOccurrenceLocationRequired
	}

	occ, err := txRepo.UpdateOccurrence(ctx, event.ID, occurrenceID, params)
	if err != nil {
		return nil, fmt.Errorf("update occurrence: %w", err)
	}

	if err := txCommitter.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return occ, nil
}

// DeleteOccurrenceOnEvent deletes a specific occurrence from an event.
// Returns ErrLastOccurrence if this is the only occurrence (would leave event in invalid state).
func (s *AdminService) DeleteOccurrenceOnEvent(ctx context.Context, eventULID string, occurrenceID string) error {
	if eventULID == "" || occurrenceID == "" {
		return ErrInvalidUpdateParams
	}

	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	event, err := txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return fmt.Errorf("get event %s: %w", eventULID, err)
	}
	if event.LifecycleState == "deleted" {
		return ErrEventDeleted
	}

	if err := txRepo.LockEventForUpdate(ctx, event.ID); err != nil {
		return fmt.Errorf("lock event %s: %w", eventULID, err)
	}

	// Re-read lifecycle_state after acquiring the lock to avoid TOCTOU race.
	event, err = txRepo.GetByULID(ctx, eventULID)
	if err != nil {
		return fmt.Errorf("re-read event %s after lock: %w", eventULID, err)
	}
	if event.LifecycleState == "deleted" {
		return ErrEventDeleted
	}

	count, err := txRepo.CountOccurrences(ctx, event.ID)
	if err != nil {
		return fmt.Errorf("count occurrences: %w", err)
	}
	if count <= 1 {
		return ErrLastOccurrence
	}

	if err := txRepo.DeleteOccurrenceByID(ctx, event.ID, occurrenceID); err != nil {
		return fmt.Errorf("delete occurrence: %w", err)
	}

	if err := txCommitter.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
