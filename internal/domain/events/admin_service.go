package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/validation"
	"github.com/rs/zerolog/log"
)

// AdminService provides admin operations for event management
type AdminService struct {
	repo         Repository
	requireHTTPS bool
}

func NewAdminService(repo Repository, requireHTTPS bool) *AdminService {
	return &AdminService{
		repo:         repo,
		requireHTTPS: requireHTTPS,
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

	// Ensure rollback on error
	defer func() {
		if err != nil {
			_ = txCommitter.Rollback(ctx)
		}
	}()

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
	// Build canonical URI for primary event (supersededBy)
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

	// Verify both events exist
	primary, err := txRepo.GetByULID(ctx, params.PrimaryULID)
	if err != nil {
		return nil, fmt.Errorf("primary event not found: %w", err)
	}

	duplicate, err := txRepo.GetByULID(ctx, params.DuplicateULID)
	if err != nil {
		return nil, fmt.Errorf("duplicate event not found: %w", err)
	}

	// Verify neither event is deleted or already merged
	if primary.LifecycleState == "deleted" {
		log.Warn().
			Str("primary_ulid", params.PrimaryULID).
			Str("duplicate_ulid", params.DuplicateULID).
			Int("review_id", reviewID).
			Msg("merge rejected: primary event is deleted")
		return nil, fmt.Errorf("primary event %s: %w", params.PrimaryULID, ErrEventDeleted)
	}
	if duplicate.LifecycleState == "deleted" {
		log.Warn().
			Str("primary_ulid", params.PrimaryULID).
			Str("duplicate_ulid", params.DuplicateULID).
			Int("review_id", reviewID).
			Msg("merge rejected: duplicate event is already deleted or merged")
		return nil, fmt.Errorf("duplicate event %s: %w", params.DuplicateULID, ErrEventDeleted)
	}

	// Enrich primary event with data from the duplicate before soft-deleting it.
	// Admin merges use equal trust (0, 0) so only gap-filling occurs — the
	// duplicate's data fills empty fields on the primary but never overwrites.
	dupInput := EventInputFromEvent(duplicate)
	enrichParams, enriched := AutoMergeFields(primary, dupInput, 0, 0)
	if enriched {
		_, err = txRepo.UpdateEvent(ctx, params.PrimaryULID, enrichParams)
		if err != nil {
			return nil, fmt.Errorf("enrich primary event: %w", err)
		}
		log.Info().
			Str("primary_ulid", params.PrimaryULID).
			Str("duplicate_ulid", params.DuplicateULID).
			Int("review_id", reviewID).
			Msg("enriched primary event with duplicate data during merge")
	}

	// Merge duplicate into primary (soft delete + set merged_into_id)
	err = txRepo.MergeEvents(ctx, params.DuplicateULID, params.PrimaryULID)
	if err != nil {
		return nil, fmt.Errorf("merge events: %w", err)
	}

	// Generate tombstone for the duplicate event
	primaryURI := fmt.Sprintf("https://togather.foundation/events/%s", params.PrimaryULID)

	tombstonePayload, err := buildTombstonePayload(duplicate.ULID, duplicate.Name, &primaryURI, "duplicate_merged")
	if err != nil {
		return nil, fmt.Errorf("build tombstone: %w", err)
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
		return nil, fmt.Errorf("create tombstone: %w", err)
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
		if len(name) > 500 {
			return FilterError{Field: "name", Message: "exceeds maximum length of 500 characters"}
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
