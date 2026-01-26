package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AdminService provides admin operations for event management
type AdminService struct {
	repo Repository
}

func NewAdminService(repo Repository) *AdminService {
	return &AdminService{repo: repo}
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
)

// UpdateEvent updates event fields with admin attribution
// Returns the updated event
func (s *AdminService) UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error) {
	if ulid == "" {
		return nil, ErrInvalidUpdateParams
	}

	// Validate parameters
	if err := validateUpdateParams(params); err != nil {
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

	// TODO: Persist updates via repository
	// For now, return the existing event with applied changes in memory
	// This will be completed when we add UpdateEvent to the repository interface

	updated := applyUpdatesInMemory(existing, params)
	return updated, nil
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
func (s *AdminService) MergeEvents(ctx context.Context, params MergeEventsParams) error {
	if params.PrimaryULID == "" || params.DuplicateULID == "" {
		return ErrInvalidUpdateParams
	}

	if params.PrimaryULID == params.DuplicateULID {
		return ErrCannotMergeSameEvent
	}

	// Verify both events exist
	primary, err := s.repo.GetByULID(ctx, params.PrimaryULID)
	if err != nil {
		return fmt.Errorf("primary event not found: %w", err)
	}

	duplicate, err := s.repo.GetByULID(ctx, params.DuplicateULID)
	if err != nil {
		return fmt.Errorf("duplicate event not found: %w", err)
	}

	// TODO: Implement merge logic:
	// 1. Soft delete duplicate event (set deleted_at)
	// 2. Add sameAs link pointing to primary
	// 3. Optionally merge data from duplicate into primary (enrich)
	// 4. Record merge in event_changes table for audit trail

	// For now, just validate that both events exist
	_ = primary
	_ = duplicate

	return nil
}

// validateUpdateParams validates update parameters
func validateUpdateParams(params UpdateEventParams) error {
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

// applyUpdatesInMemory applies updates to an event struct (for testing until persistence is implemented)
func applyUpdatesInMemory(event *Event, params UpdateEventParams) *Event {
	updated := *event
	updated.UpdatedAt = time.Now()

	if params.Name != nil {
		updated.Name = *params.Name
	}
	if params.Description != nil {
		updated.Description = *params.Description
	}
	if params.LifecycleState != nil {
		updated.LifecycleState = *params.LifecycleState
	}
	if params.ImageURL != nil {
		updated.ImageURL = *params.ImageURL
	}
	if params.PublicURL != nil {
		updated.PublicURL = *params.PublicURL
	}
	if params.EventDomain != nil {
		updated.EventDomain = *params.EventDomain
	}
	if len(params.Keywords) > 0 {
		updated.Keywords = params.Keywords
	}

	return &updated
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
