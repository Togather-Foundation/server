package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/validation"
	"github.com/rs/zerolog/log"
)

// AdminService provides admin operations for event management
type AdminService struct {
	repo             Repository
	requireHTTPS     bool
	defaultTZ        string
	validationConfig config.ValidationConfig
}

func NewAdminService(repo Repository, requireHTTPS bool, defaultTimezone string, validationConfig config.ValidationConfig) *AdminService {
	return &AdminService{
		repo:             repo,
		requireHTTPS:     requireHTTPS,
		defaultTZ:        defaultTimezone,
		validationConfig: validationConfig.WithDefaults(),
	}
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

var (
	ErrInvalidUpdateParams  = errors.New("invalid update parameters")
	ErrCannotMergeSameEvent = errors.New("cannot merge event with itself")
	ErrEventDeleted         = errors.New("event has been deleted")
	ErrEventAlreadyMerged   = errors.New("event has already been merged")
)

// AddOccurrenceFromReview atomically adds the review entry's event occurrence to a target
// recurring-series event, soft-deletes the review's own event, and marks the review
// as "merged" — all in a single database transaction.
//
// The targetEventULID identifies the existing recurring-series event.  The new
// occurrence is constructed from the review entry's EventStartTime / EventEndTime.
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

	// Fetch the target (recurring-series) event: a quick read to get its internal ID
	// so that we can acquire the row-level lock before any eligibility recheck.
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

	// Overlap check
	overlaps, err := txRepo.CheckOccurrenceOverlap(ctx, target.ID, review.EventStartTime, review.EventEndTime)
	if err != nil {
		return nil, fmt.Errorf("check overlap: %w", err)
	}
	if overlaps {
		return nil, fmt.Errorf("new occurrence [%s, %v] on event %s: %w",
			review.EventStartTime.Format(time.RFC3339),
			review.EventEndTime,
			targetEventULID,
			ErrOccurrenceOverlap,
		)
	}

	// Add the new occurrence to the target event.  Seed defaults from the target
	// series, then override with the review event's matched occurrence metadata so
	// that the absorbed instance's venue, timezone, pricing, door time, ticket URL,
	// and virtual URL are preserved rather than silently dropped.
	occTimezone := s.defaultTZ
	occVenueID := target.PrimaryVenueID
	var occVirtualURL *string

	// Fetch the review event to extract its occurrence-level metadata.
	reviewEvent, err := txRepo.GetByULID(ctx, review.EventULID)
	if err != nil {
		return nil, fmt.Errorf("get review event %s: %w", review.EventULID, err)
	}

	// Find the occurrence on the review event whose StartTime matches the review
	// entry's EventStartTime.  If no occurrence matches (e.g. multi-occurrence event
	// whose start times don't align), fall back to [0] only when there is exactly
	// one occurrence (unambiguous single-instance case from ingest).  If still no
	// match, the series-level defaults above are used as-is.
	//
	// Carry over all occurrence-level metadata from the review event so that
	// pricing, door time, ticket URL, etc. survive the absorption — not just
	// the three fields that were preserved in the first implementation.
	var occDoorTime *time.Time
	var occTicketURL *string
	var occPriceMin *float64
	var occPriceMax *float64
	var occPriceCurrency string
	var occAvailability string

	if len(reviewEvent.Occurrences) > 0 {
		var matchedOcc *Occurrence
		for i := range reviewEvent.Occurrences {
			if reviewEvent.Occurrences[i].StartTime.Equal(review.EventStartTime) {
				matchedOcc = &reviewEvent.Occurrences[i]
				break
			}
		}
		// If no exact match, fall back to [0] only when the event has exactly one
		// occurrence (unambiguous case, e.g. single-instance event from ingest).
		if matchedOcc == nil && len(reviewEvent.Occurrences) == 1 {
			matchedOcc = &reviewEvent.Occurrences[0]
		}
		if matchedOcc != nil {
			if matchedOcc.Timezone != "" {
				occTimezone = matchedOcc.Timezone
			}
			if matchedOcc.VenueID != nil {
				occVenueID = matchedOcc.VenueID
			}
			if matchedOcc.VirtualURL != nil && *matchedOcc.VirtualURL != "" {
				occVirtualURL = matchedOcc.VirtualURL
			}
			// Preserve remaining occurrence-level metadata
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
		}
	}

	err = txRepo.CreateOccurrence(ctx, OccurrenceCreateParams{
		EventID:       target.ID,
		StartTime:     review.EventStartTime,
		EndTime:       review.EventEndTime,
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

	// Tombstone for the absorbed event
	targetURI := fmt.Sprintf("https://togather.foundation/events/%s", targetEventULID)
	tombstonePayload, err := buildTombstonePayload(reviewEvent.ULID, reviewEvent.Name, &targetURI, "absorbed_as_occurrence")
	if err != nil {
		return nil, fmt.Errorf("build tombstone: %w", err)
	}
	err = txRepo.CreateTombstone(ctx, TombstoneCreateParams{
		EventID:      reviewEvent.ID,
		EventURI:     fmt.Sprintf("https://togather.foundation/events/%s", reviewEvent.ULID),
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
// The method atomically:
//  1. Locks the near-dup review entry and verifies it is still pending.
//  2. Locks and re-reads the target (existing series) event.
//  3. Checks overlap for the new event's start/end time on the target.
//  4. Creates the occurrence on the target.
//  5. Soft-deletes the new (source) event.
//  6. Creates a tombstone for the source event.
//  7. If the source event has a companion pending review entry, marks it merged.
//  8. Marks the near-dup review entry as merged (sets duplicateOfEventUlid to
//     the target ULID so the admin can navigate to the series after the action).
func (s *AdminService) AddOccurrenceFromReviewNearDup(ctx context.Context, reviewID int, reviewedBy string) (*ReviewQueueEntry, *string, error) {
	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = txCommitter.Rollback(ctx) }()

	// Lock the near-dup review entry first.
	review, err := txRepo.LockReviewQueueEntryForUpdate(ctx, reviewID)
	if err != nil {
		return nil, nil, fmt.Errorf("lock near-dup review entry: %w", err)
	}
	if review.Status != "pending" {
		return nil, nil, fmt.Errorf("near-dup review entry %d has already been %s: %w", reviewID, review.Status, ErrConflict)
	}

	// The near-dup review entry must have a DuplicateOfEventULID (the new event).
	if review.DuplicateOfEventULID == nil || *review.DuplicateOfEventULID == "" {
		return nil, nil, fmt.Errorf("near-dup review entry %d has no duplicate event ULID: %w", reviewID, ErrInvalidUpdateParams)
	}
	sourceEventULID := *review.DuplicateOfEventULID // new event → to be absorbed
	targetEventULID := review.EventULID             // existing series → kept

	if sourceEventULID == targetEventULID {
		return nil, nil, fmt.Errorf("near-dup review source and target are the same (%s): %w", targetEventULID, ErrCannotMergeSameEvent)
	}

	// Fetch target (existing series) for locking.
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

	// Fetch the source (new) event to extract timing and occurrence metadata.
	sourceEvent, err := txRepo.GetByULID(ctx, sourceEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("get source event %s: %w", sourceEventULID, err)
	}

	// Derive start/end times for the new occurrence from the source event.
	// Use the first occurrence if available; fall back to zero time (ingest always
	// creates at least one occurrence, so this should not happen in practice).
	var occStartTime time.Time
	var occEndTime *time.Time
	if len(sourceEvent.Occurrences) > 0 {
		occStartTime = sourceEvent.Occurrences[0].StartTime
		occEndTime = sourceEvent.Occurrences[0].EndTime
	} else {
		// Fallback: derive from the *review entry's* stored start/end times, which
		// are set from the existing event's first occurrence during ingest.
		// For near_duplicate_of_new_event the review entry holds the existing
		// event's times, not the new event's — so this branch is last resort only.
		occStartTime = review.EventStartTime
		occEndTime = review.EventEndTime
	}

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

	// Collect occurrence-level metadata from the source event's first occurrence.
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

	// Tombstone for the absorbed source event.
	targetURI := fmt.Sprintf("https://togather.foundation/events/%s", targetEventULID)
	tombstonePayload, err := buildTombstonePayload(sourceEvent.ULID, sourceEvent.Name, &targetURI, "absorbed_as_occurrence")
	if err != nil {
		return nil, nil, fmt.Errorf("build tombstone: %w", err)
	}
	if err = txRepo.CreateTombstone(ctx, TombstoneCreateParams{
		EventID:      sourceEvent.ID,
		EventURI:     fmt.Sprintf("https://togather.foundation/events/%s", sourceEvent.ULID),
		DeletedAt:    time.Now(),
		Reason:       "absorbed_as_occurrence",
		SupersededBy: &targetURI,
		Payload:      tombstonePayload,
	}); err != nil {
		return nil, nil, fmt.Errorf("create tombstone: %w", err)
	}

	// If the source event has its own pending review entry (the new-event side of the
	// near-dup pair), mark it merged too so it doesn't linger in the queue.
	companionReview, err := txRepo.GetPendingReviewByEventUlid(ctx, sourceEventULID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, nil, fmt.Errorf("lookup companion review for source %s: %w", sourceEventULID, err)
		}
		// ErrNotFound: no companion entry — nothing to dismiss.
	} else if companionReview != nil && companionReview.Status == "pending" {
		if _, mergeErr := txRepo.MergeReview(ctx, companionReview.ID, reviewedBy, targetEventULID); mergeErr != nil {
			if errors.Is(mergeErr, ErrNotFound) || errors.Is(mergeErr, ErrConflict) {
				// Race outcome: companion was already dismissed by a concurrent request.
				// Log-worthy but non-fatal — continue so the primary review is resolved.
				_ = mergeErr
			} else {
				return nil, nil, fmt.Errorf("dismiss companion review id=%d: %w", companionReview.ID, mergeErr)
			}
		}
	}

	// Mark the near-dup review entry as merged.
	// We call MergeReview with targetEventULID so that duplicateOfEventUlid is set to
	// the series ULID — giving the admin a direct navigation link after the action.
	reviewEntry, err := txRepo.MergeReview(ctx, reviewID, reviewedBy, targetEventULID)
	if err != nil {
		return nil, nil, fmt.Errorf("update near-dup review status: %w", err)
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

// MergeEvents merges a duplicate event into a primary event
// The duplicate event is soft-deleted with a sameAs link to the primary
// This operation is atomic and wrapped in a database transaction
func (s *AdminService) MergeEvents(ctx context.Context, params MergeEventsParams) error {
	if params.PrimaryULID == "" || params.DuplicateULID == "" {
		return ErrInvalidUpdateParams
	}

	if params.PrimaryULID == params.DuplicateULID {
		return ErrCannotMergeSameEvent
	}

	// Begin transaction
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on error (no-op after commit)
	defer func() {
		_ = txCommitter.Rollback(ctx)
	}()

	if err := s.executeMerge(ctx, txRepo, params); err != nil {
		return err
	}

	// Commit transaction
	err = txCommitter.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
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

	if err := s.executeMerge(ctx, txRepo, params); err != nil {
		return nil, err
	}

	// Update the review queue entry to "merged" status — within the same transaction
	reviewEntry, err := txRepo.MergeReview(ctx, reviewID, reviewedBy, params.PrimaryULID)
	if err != nil {
		return nil, fmt.Errorf("update review status: %w", err)
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
	primaryURI := fmt.Sprintf("https://togather.foundation/events/%s", params.PrimaryULID)

	tombstonePayload, err := buildTombstonePayload(duplicate.ULID, duplicate.Name, &primaryURI, "duplicate_merged")
	if err != nil {
		return fmt.Errorf("build tombstone: %w", err)
	}

	tombstoneParams := TombstoneCreateParams{
		EventID:      duplicate.ID,
		EventURI:     fmt.Sprintf("https://togather.foundation/events/%s", duplicate.ULID),
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
	tombstonePayload, err := buildTombstonePayload(event.ULID, event.Name, nil, reason)
	if err != nil {
		return nil, fmt.Errorf("build tombstone: %w", err)
	}

	tombstoneParams := TombstoneCreateParams{
		EventID:      event.ID,
		EventURI:     fmt.Sprintf("https://togather.foundation/events/%s", event.ULID),
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
	tombstonePayload, err := buildTombstonePayload(event.ULID, event.Name, nil, reason)
	if err != nil {
		return fmt.Errorf("build tombstone: %w", err)
	}

	// Create tombstone record
	tombstoneParams := TombstoneCreateParams{
		EventID:      event.ID,
		EventURI:     fmt.Sprintf("https://togather.foundation/events/%s", event.ULID),
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

// buildTombstonePayload generates a JSON-LD tombstone payload according to SEL spec
// See: docs/togather_SEL_Interoperability_Profile_v0.1.md section 1.6
func buildTombstonePayload(ulid, name string, supersededBy *string, reason string) ([]byte, error) {
	eventURI := fmt.Sprintf("https://togather.foundation/events/%s", ulid)

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
