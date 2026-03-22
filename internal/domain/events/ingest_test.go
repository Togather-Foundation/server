package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Togather-Foundation/server/internal/config"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// MockRepository implements Repository interface for testing
type MockRepository struct {
	mu sync.Mutex

	// Storage
	events            map[string]*Event
	idempotencyKeys   map[string]*IdempotencyKey
	sources           map[string]string            // sourceKey -> sourceID
	eventsBySources   map[string]map[string]*Event // sourceID -> sourceEventID -> Event
	eventsByDedupHash map[string]*Event
	places            map[string]*PlaceRecord
	organizations     map[string]*OrganizationRecord
	occurrences       map[string][]OccurrenceCreateParams // eventID -> occurrences
	reviewQueue       map[int]*ReviewQueueEntry           // id -> review queue entry
	nextReviewID      int

	// Enhanced storage for scenario tests
	sourceTrustLevels  map[string]int                   // sourceID -> trust level
	eventTrustOverride map[string]int                   // eventID -> trust level (overrides source-based lookup)
	notDuplicates      map[string]bool                  // "eventA|eventB" -> true (canonical order)
	nearDuplicates     []NearDuplicateCandidate         // configurable return for FindNearDuplicates
	similarPlaces      []SimilarPlaceCandidate          // configurable return for FindSimilarPlaces
	similarOrgs        []SimilarOrgCandidate            // configurable return for FindSimilarOrganizations
	reviewEntries      map[string]*ReviewQueueEntry     // lookup key -> entry (for FindReviewByDedup)
	occurrenceDates    map[string]*occurrenceDateUpdate // eventULID -> updated dates

	// Tracking for assertions
	approveReviewCalled        bool
	approveReviewID            int
	updateEventCalls           []updateEventCall
	mergePlacesCalled          bool
	mergePlacesDupID           string
	mergePlacesPriID           string
	getOrCreateSourceCallCount int

	// Behavior controls
	shouldFailCreate                 bool
	shouldFailGetIdempotencyKey      bool
	shouldFailInsertIdempotencyKey   bool
	shouldFailUpdateIdempotencyKey   bool
	shouldFailFindBySourceExternalID bool
	shouldFailFindByDedupHash        bool
	shouldFailGetOrCreateSource      bool
	shouldFailUpsertPlace            bool
	shouldFailGetPlaceByULID         bool
	shouldFailUpsertOrganization     bool
	shouldFailCreateOccurrence       bool
	shouldFailCreateSource           bool
	shouldFailFindNearDuplicates     bool
	shouldFailFindSimilarPlaces      bool
	shouldFailFindSimilarOrgs        bool
	shouldFailUpdateEvent            bool
	shouldFailApproveReview          bool
	shouldFailFindSeriesCompanion    bool
	seriesCompanion                  *CrossWeekCompanion
}

// occurrenceDateUpdate stores occurrence date updates for verification
type occurrenceDateUpdate struct {
	startTime time.Time
	endTime   *time.Time
}

// updateEventCall tracks calls to UpdateEvent for verification
type updateEventCall struct {
	ULID   string
	Params UpdateEventParams
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		events:             make(map[string]*Event),
		idempotencyKeys:    make(map[string]*IdempotencyKey),
		sources:            make(map[string]string),
		eventsBySources:    make(map[string]map[string]*Event),
		eventsByDedupHash:  make(map[string]*Event),
		places:             make(map[string]*PlaceRecord),
		organizations:      make(map[string]*OrganizationRecord),
		occurrences:        make(map[string][]OccurrenceCreateParams),
		reviewQueue:        make(map[int]*ReviewQueueEntry),
		nextReviewID:       1,
		sourceTrustLevels:  make(map[string]int),
		eventTrustOverride: make(map[string]int),
		notDuplicates:      make(map[string]bool),
		reviewEntries:      make(map[string]*ReviewQueueEntry),
		occurrenceDates:    make(map[string]*occurrenceDateUpdate),
	}
}

func (m *MockRepository) List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var events []Event
	for _, event := range m.events {
		events = append(events, *event)
	}
	return ListResult{Events: events, NextCursor: ""}, nil
}

func (m *MockRepository) GetByULID(ctx context.Context, ulid string) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.events[ulid]
	if !ok {
		return nil, ErrNotFound
	}
	return event, nil
}

func (m *MockRepository) Create(ctx context.Context, params EventCreateParams) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailCreate {
		return nil, errors.New("mock create error")
	}

	event := &Event{
		ID:             fmt.Sprintf("id-%s", params.ULID),
		ULID:           params.ULID,
		Name:           params.Name,
		Description:    params.Description,
		LicenseURL:     params.LicenseURL,
		LicenseStatus:  params.LicenseStatus,
		DedupHash:      params.DedupHash,
		LifecycleState: params.LifecycleState,
		EventDomain:    params.EventDomain,
		OrganizerID:    params.OrganizerID,
		PrimaryVenueID: params.PrimaryVenueID,
		VirtualURL:     params.VirtualURL,
		ImageURL:       params.ImageURL,
		PublicURL:      params.PublicURL,
		Confidence:     params.Confidence,
		QualityScore:   params.QualityScore,
		Keywords:       params.Keywords,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	m.events[params.ULID] = event

	if params.DedupHash != "" {
		m.eventsByDedupHash[params.DedupHash] = event
	}

	return event, nil
}

func (m *MockRepository) CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailCreateOccurrence {
		return errors.New("mock create occurrence error")
	}

	// Enforce the DB constraint: venue_id IS NOT NULL OR virtual_url IS NOT NULL.
	// Mirrors occurrence_location_required in migrations/000001_core.up.sql.
	if params.VenueID == nil && params.VirtualURL == nil {
		return errors.New("mock occurrence_location_required constraint: venue_id IS NOT NULL OR virtual_url IS NOT NULL")
	}

	m.occurrences[params.EventID] = append(m.occurrences[params.EventID], params)
	return nil
}

func (m *MockRepository) CreateSource(ctx context.Context, params EventSourceCreateParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailCreateSource {
		return errors.New("mock create source error")
	}

	return nil
}

func (m *MockRepository) FindBySourceExternalID(ctx context.Context, sourceID string, sourceEventID string) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailFindBySourceExternalID {
		return nil, errors.New("mock find by source external ID error")
	}

	if sources, ok := m.eventsBySources[sourceID]; ok {
		if event, ok := sources[sourceEventID]; ok {
			return event, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MockRepository) FindByDedupHash(ctx context.Context, dedupHash string) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailFindByDedupHash {
		return nil, errors.New("mock find by dedup hash error")
	}

	event, ok := m.eventsByDedupHash[dedupHash]
	if !ok {
		return nil, ErrNotFound
	}
	return event, nil
}

func (m *MockRepository) GetOrCreateSource(ctx context.Context, params SourceLookupParams) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getOrCreateSourceCallCount++

	if m.shouldFailGetOrCreateSource {
		return "", errors.New("mock get or create source error")
	}

	key := fmt.Sprintf("%s|%s", params.Name, params.BaseURL)
	if sourceID, ok := m.sources[key]; ok {
		return sourceID, nil
	}

	sourceID := fmt.Sprintf("source-%d", len(m.sources)+1)
	m.sources[key] = sourceID
	m.eventsBySources[sourceID] = make(map[string]*Event)
	return sourceID, nil
}

func (m *MockRepository) GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailGetIdempotencyKey {
		return nil, errors.New("mock get idempotency key error")
	}

	ikey, ok := m.idempotencyKeys[key]
	if !ok {
		return nil, ErrNotFound
	}
	return ikey, nil
}

func (m *MockRepository) InsertIdempotencyKey(ctx context.Context, params IdempotencyKeyCreateParams) (*IdempotencyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailInsertIdempotencyKey {
		return nil, errors.New("mock insert idempotency key error")
	}

	var eventID *string
	var eventULID *string
	if params.EventID != "" {
		eventID = &params.EventID
	}
	if params.EventULID != "" {
		eventULID = &params.EventULID
	}

	ikey := &IdempotencyKey{
		Key:         params.Key,
		RequestHash: params.RequestHash,
		EventID:     eventID,
		EventULID:   eventULID,
	}
	m.idempotencyKeys[params.Key] = ikey
	return ikey, nil
}

func (m *MockRepository) UpdateIdempotencyKeyEvent(ctx context.Context, key string, eventID string, eventULID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailUpdateIdempotencyKey {
		return errors.New("mock update idempotency key error")
	}

	ikey, ok := m.idempotencyKeys[key]
	if !ok {
		return ErrNotFound
	}

	ikey.EventID = &eventID
	ikey.EventULID = &eventULID
	return nil
}

func (m *MockRepository) UpsertPlace(ctx context.Context, params PlaceCreateParams) (*PlaceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailUpsertPlace {
		return nil, errors.New("mock upsert place error")
	}

	key := params.Name + params.AddressLocality
	if place, ok := m.places[key]; ok {
		return place, nil
	}

	place := &PlaceRecord{
		ID:   fmt.Sprintf("place-id-%d", len(m.places)+1),
		ULID: params.ULID,
	}
	m.places[key] = place
	// Also index by ULID so GetPlaceByULID can find places created via UpsertPlace.
	m.places["ulid:"+params.ULID] = place
	return place, nil
}

func (m *MockRepository) GetPlaceByULID(ctx context.Context, ulid string) (*PlaceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailGetPlaceByULID {
		return nil, errors.New("mock get place by ulid error")
	}
	if place, ok := m.places["ulid:"+ulid]; ok {
		return place, nil
	}
	return nil, ErrNotFound
}

func (m *MockRepository) UpsertOrganization(ctx context.Context, params OrganizationCreateParams) (*OrganizationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailUpsertOrganization {
		return nil, errors.New("mock upsert organization error")
	}

	key := params.Name + params.AddressLocality
	if org, ok := m.organizations[key]; ok {
		return org, nil
	}

	org := &OrganizationRecord{
		ID:   fmt.Sprintf("org-id-%d", len(m.organizations)+1),
		ULID: params.ULID,
	}
	m.organizations[key] = org
	return org, nil
}

// Admin operations (stub implementations for testing)
func (m *MockRepository) UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailUpdateEvent {
		return nil, errors.New("mock update event error")
	}

	event, ok := m.events[ulid]
	if !ok {
		return nil, ErrNotFound
	}

	// Track calls for assertions
	m.updateEventCalls = append(m.updateEventCalls, updateEventCall{ULID: ulid, Params: params})

	// Apply updates in memory
	if params.Name != nil {
		event.Name = *params.Name
	}
	if params.Description != nil {
		event.Description = *params.Description
	}
	if params.LifecycleState != nil {
		event.LifecycleState = *params.LifecycleState
	}
	if params.ImageURL != nil {
		event.ImageURL = *params.ImageURL
	}
	if params.PublicURL != nil {
		event.PublicURL = *params.PublicURL
	}
	if params.EventDomain != nil {
		event.EventDomain = *params.EventDomain
	}
	if len(params.Keywords) > 0 {
		event.Keywords = params.Keywords
	}
	event.UpdatedAt = time.Now()

	return event, nil
}

func (m *MockRepository) DeleteOccurrencesByEventULID(ctx context.Context, eventULID string) error {
	return nil
}
func (m *MockRepository) UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.occurrenceDates[eventULID] = &occurrenceDateUpdate{
		startTime: startTime,
		endTime:   endTime,
	}
	return nil
}

func (m *MockRepository) SoftDeleteEvent(ctx context.Context, ulid string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.events[ulid]
	if !ok {
		return ErrNotFound
	}

	// Mark as deleted (in real implementation, would set deleted_at)
	return nil
}

func (m *MockRepository) MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.events[duplicateULID]
	if !ok {
		return ErrNotFound
	}

	_, ok = m.events[primaryULID]
	if !ok {
		return ErrNotFound
	}

	// Mark duplicate as merged (in real implementation, would set merged_into_id)
	return nil
}

func (m *MockRepository) CreateTombstone(ctx context.Context, params TombstoneCreateParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stub implementation - in real implementation would insert into event_tombstones table
	return nil
}

func (m *MockRepository) GetTombstoneByEventID(ctx context.Context, eventID string) (*Tombstone, error) {
	return nil, ErrNotFound
}

func (m *MockRepository) GetTombstoneByEventULID(ctx context.Context, eventULID string) (*Tombstone, error) {
	return nil, ErrNotFound
}

// Review Queue methods
func (m *MockRepository) FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Search by source+externalID first
	if sourceID != nil && externalID != nil {
		key := *sourceID + "|" + *externalID
		if entry, ok := m.reviewEntries[key]; ok {
			return entry, nil
		}
	}
	// Search by dedupHash
	if dedupHash != nil {
		key := "hash:" + *dedupHash
		if entry, ok := m.reviewEntries[key]; ok {
			return entry, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MockRepository) CreateReviewQueueEntry(ctx context.Context, params ReviewQueueCreateParams) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := &ReviewQueueEntry{
		ID:                 m.nextReviewID,
		EventID:            params.EventID,
		OriginalPayload:    params.OriginalPayload,
		NormalizedPayload:  params.NormalizedPayload,
		Warnings:           params.Warnings,
		SourceID:           params.SourceID,
		SourceExternalID:   params.SourceExternalID,
		DedupHash:          params.DedupHash,
		EventStartTime:     params.EventStartTime,
		EventEndTime:       params.EventEndTime,
		DuplicateOfEventID: params.DuplicateOfEventID,
		Status:             "pending",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	m.reviewQueue[m.nextReviewID] = entry
	m.nextReviewID++
	return entry, nil
}

func (m *MockRepository) UpdateReviewQueueEntry(ctx context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.reviewQueue[id]
	if !ok {
		return nil, ErrNotFound
	}

	if params.OriginalPayload != nil {
		entry.OriginalPayload = *params.OriginalPayload
	}
	if params.NormalizedPayload != nil {
		entry.NormalizedPayload = *params.NormalizedPayload
	}
	if params.Warnings != nil {
		entry.Warnings = *params.Warnings
	}
	if params.DuplicateOfEventID != nil {
		entry.DuplicateOfEventID = params.DuplicateOfEventID
	}
	entry.UpdatedAt = time.Now()

	return entry, nil
}

func (m *MockRepository) GetReviewQueueEntry(ctx context.Context, id int) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.reviewQueue[id]
	if !ok {
		return nil, ErrNotFound
	}
	return entry, nil
}

func (m *MockRepository) LockReviewQueueEntryForUpdate(ctx context.Context, id int) (*ReviewQueueEntry, error) {
	// In tests, locking is a no-op — delegate to GetReviewQueueEntry.
	return m.GetReviewQueueEntry(ctx, id)
}

// GetReviewQueueByEventID is a test helper that returns all review queue entries for a given event UUID.
func (m *MockRepository) GetReviewQueueByEventID(eventID string) []*ReviewQueueEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	var entries []*ReviewQueueEntry
	for _, e := range m.reviewQueue {
		if e.EventID == eventID {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *MockRepository) ListReviewQueue(ctx context.Context, filters ReviewQueueFilters) (*ReviewQueueListResult, error) {
	return &ReviewQueueListResult{Entries: []ReviewQueueEntry{}, NextCursor: nil}, nil
}

func (m *MockRepository) ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailApproveReview {
		return nil, errors.New("mock approve review error")
	}

	m.approveReviewCalled = true
	m.approveReviewID = id

	entry, ok := m.reviewQueue[id]
	if !ok {
		return nil, ErrNotFound
	}

	entry.Status = "approved"
	entry.ReviewedBy = &reviewedBy
	now := time.Now()
	entry.ReviewedAt = &now
	if notes != nil {
		entry.ReviewNotes = notes
	}
	entry.UpdatedAt = now

	return entry, nil
}

func (m *MockRepository) RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.reviewQueue[id]
	if !ok {
		return nil, ErrNotFound
	}

	entry.Status = "rejected"
	entry.ReviewedBy = &reviewedBy
	now := time.Now()
	entry.ReviewedAt = &now
	entry.RejectionReason = &reason
	entry.UpdatedAt = now

	return entry, nil
}
func (m *MockRepository) MergeReview(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.reviewQueue[id]
	if !ok {
		return nil, ErrNotFound
	}

	if entry.Status != "pending" {
		return nil, ErrNotFound // only pending can be merged
	}

	entry.Status = "merged"
	entry.ReviewedBy = &reviewedBy
	now := time.Now()
	entry.ReviewedAt = &now
	entry.DuplicateOfEventID = &primaryEventULID
	entry.UpdatedAt = now

	return entry, nil
}

func (m *MockRepository) CleanupExpiredReviews(ctx context.Context) error {
	return nil
}

// AddEvent is a test helper that pre-populates the mock with an existing event,
// making it available via GetByULID. Used in near-duplicate scenario tests.
func (m *MockRepository) AddEvent(event *Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[event.ULID] = event
}

// GetUpdateEventCalls is a test helper that returns all recorded UpdateEvent calls.
func (m *MockRepository) GetUpdateEventCalls() []updateEventCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]updateEventCall, len(m.updateEventCalls))
	copy(result, m.updateEventCalls)
	return result
}

func (m *MockRepository) GetSourceTrustLevel(ctx context.Context, eventID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for explicit event trust override first
	if trust, ok := m.eventTrustOverride[eventID]; ok {
		return trust, nil
	}

	// Match real DB behavior: return MAX trust level across all sources linked to this event.
	// The real SQL query is: SELECT COALESCE(MAX(s.trust_level), 5) FROM sources s JOIN event_sources es ON es.event_id = $1
	maxTrust := -1
	for sourceID, events := range m.eventsBySources {
		for _, evt := range events {
			if evt.ID == eventID {
				if trust, ok := m.sourceTrustLevels[sourceID]; ok {
					if trust > maxTrust {
						maxTrust = trust
					}
				}
			}
		}
	}
	// Also check by event ULID in case the eventID is a UUID mapped via events map
	for _, evt := range m.events {
		if evt.ID == eventID {
			for sourceID, events := range m.eventsBySources {
				for _, srcEvt := range events {
					if srcEvt.ULID == evt.ULID {
						if trust, ok := m.sourceTrustLevels[sourceID]; ok {
							if trust > maxTrust {
								maxTrust = trust
							}
						}
					}
				}
			}
		}
	}
	if maxTrust >= 0 {
		return maxTrust, nil
	}
	return 5, nil // default trust level
}

func (m *MockRepository) GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if trust, ok := m.sourceTrustLevels[sourceID]; ok {
		return trust, nil
	}
	return 5, nil // default trust level
}

func (m *MockRepository) FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]NearDuplicateCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailFindNearDuplicates {
		return nil, errors.New("mock find near duplicates error")
	}
	return m.nearDuplicates, nil
}
func (m *MockRepository) FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarPlaceCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailFindSimilarPlaces {
		return nil, errors.New("mock find similar places error")
	}
	return m.similarPlaces, nil
}
func (m *MockRepository) FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarOrgCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailFindSimilarOrgs {
		return nil, errors.New("mock find similar orgs error")
	}
	return m.similarOrgs, nil
}
func (m *MockRepository) FindSeriesCompanion(ctx context.Context, params SeriesCompanionQuery) (*CrossWeekCompanion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailFindSeriesCompanion {
		return nil, errors.New("mock find series companion error")
	}
	return m.seriesCompanion, nil
}
func (m *MockRepository) MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mergePlacesCalled = true
	m.mergePlacesDupID = duplicateID
	m.mergePlacesPriID = primaryID
	return &MergeResult{CanonicalID: primaryID}, nil
}
func (m *MockRepository) MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error) {
	return &MergeResult{CanonicalID: primaryID}, nil
}
func (m *MockRepository) InsertNotDuplicate(ctx context.Context, eventIDa string, eventIDb string, createdBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store in canonical order (alphabetical)
	a, b := eventIDa, eventIDb
	if a > b {
		a, b = b, a
	}
	m.notDuplicates[a+"|"+b] = true
	return nil
}
func (m *MockRepository) IsNotDuplicate(ctx context.Context, eventIDa string, eventIDb string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check in canonical order
	a, b := eventIDa, eventIDb
	if a > b {
		a, b = b, a
	}
	return m.notDuplicates[a+"|"+b], nil
}

func (m *MockRepository) GetPendingReviewByEventUlid(_ context.Context, eventULID string) (*ReviewQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range m.reviewQueue {
		if entry.EventULID == eventULID && entry.Status == "pending_review" {
			return entry, nil
		}
	}
	return nil, nil
}

func (m *MockRepository) GetPendingReviewByEventUlidAndDuplicateUlid(_ context.Context, _ string, _ string) (*ReviewQueueEntry, error) {
	return nil, nil
}

func (m *MockRepository) UpdateReviewWarnings(_ context.Context, _ int, _ []byte) error {
	return nil
}

func (m *MockRepository) DismissCompanionWarningMatch(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *MockRepository) DismissWarningMatchByReviewID(_ context.Context, _ int, _ string) error {
	return nil
}

func (m *MockRepository) CheckOccurrenceOverlap(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
	return false, nil
}

func (m *MockRepository) LockEventForUpdate(_ context.Context, _ string) error {
	return nil
}

func (m *MockRepository) InsertOccurrence(_ context.Context, params OccurrenceCreateParams) (*Occurrence, error) {
	return &Occurrence{StartTime: params.StartTime}, nil
}

func (m *MockRepository) GetOccurrenceByID(_ context.Context, _, _ string) (*Occurrence, error) {
	return nil, ErrNotFound
}

func (m *MockRepository) UpdateOccurrence(_ context.Context, _, occurrenceID string, _ OccurrenceUpdateParams) (*Occurrence, error) {
	return &Occurrence{ID: occurrenceID}, nil
}

func (m *MockRepository) DeleteOccurrenceByID(_ context.Context, _, _ string) error {
	return nil
}

func (m *MockRepository) CountOccurrences(_ context.Context, _ string) (int64, error) {
	return 2, nil
}

func (m *MockRepository) CheckOccurrenceOverlapExcluding(_ context.Context, _ string, _ time.Time, _ *time.Time, _ string) (bool, error) {
	return false, nil
}

func (m *MockRepository) DismissPendingReviewsByEventULIDs(_ context.Context, _ []string, _ string) ([]int, error) {
	return nil, nil
}

func (m *MockRepository) BeginTx(ctx context.Context) (Repository, TxCommitter, error) {
	// For testing, return self and a no-op committer
	return m, &noOpTxCommitter{}, nil
}

type noOpTxCommitter struct{}

func (n *noOpTxCommitter) Commit(ctx context.Context) error {
	return nil
}

func (n *noOpTxCommitter) Rollback(ctx context.Context) error {
	return nil
}

// Helper methods for testing
func (m *MockRepository) AddExistingEvent(sourceID, sourceEventID string, event *Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.eventsBySources[sourceID]; !ok {
		m.eventsBySources[sourceID] = make(map[string]*Event)
	}
	m.eventsBySources[sourceID][sourceEventID] = event
	m.events[event.ULID] = event
}

// SetSourceTrust sets the trust level for a source ID
func (m *MockRepository) SetSourceTrust(sourceID string, trust int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sourceTrustLevels[sourceID] = trust
}

// SetEventTrust sets a trust level override for a specific event ID.
// This takes precedence over the source-based trust lookup in GetSourceTrustLevel.
func (m *MockRepository) SetEventTrust(eventID string, trust int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventTrustOverride[eventID] = trust
}

// AddReviewEntry adds a review queue entry that FindReviewByDedup can find.
// It indexes the entry by source+externalID and/or dedupHash.
func (m *MockRepository) AddReviewEntry(entry *ReviewQueueEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store in reviewQueue by ID
	m.reviewQueue[entry.ID] = entry

	// Index for FindReviewByDedup
	if entry.SourceID != nil && entry.SourceExternalID != nil {
		key := *entry.SourceID + "|" + *entry.SourceExternalID
		m.reviewEntries[key] = entry
	}
	if entry.DedupHash != nil {
		key := "hash:" + *entry.DedupHash
		m.reviewEntries[key] = entry
	}
}

// SetNearDuplicates configures the candidates returned by FindNearDuplicates
func (m *MockRepository) SetNearDuplicates(candidates []NearDuplicateCandidate) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nearDuplicates = candidates
}

// SetSimilarPlaces configures the candidates returned by FindSimilarPlaces
func (m *MockRepository) SetSimilarPlaces(candidates []SimilarPlaceCandidate) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.similarPlaces = candidates
}

// SetSimilarOrgs configures the candidates returned by FindSimilarOrganizations
func (m *MockRepository) SetSimilarOrgs(candidates []SimilarOrgCandidate) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.similarOrgs = candidates
}

// SeedPlaceByULID pre-populates the mock so that GetPlaceByULID will return a
// PlaceRecord for the given ULID.  Use this in tests that exercise the
// occurrences[].venueId canonical-URI resolution path.
func (m *MockRepository) SeedPlaceByULID(ulid, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.places["ulid:"+ulid] = &PlaceRecord{ID: id, ULID: ulid}
}

// SeedPlaceByULIDWithName pre-populates the mock with a place ULID, UUID, and Name.
// Use in tests that exercise the parent location.@id resolution path where name
// mismatch detection is needed.
func (m *MockRepository) SeedPlaceByULIDWithName(ulid, id, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.places["ulid:"+ulid] = &PlaceRecord{ID: id, ULID: ulid, Name: name}
}

// AddNotDuplicate records a known not-duplicate pair
func (m *MockRepository) AddNotDuplicate(eventA, eventB string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, b := eventA, eventB
	if a > b {
		a, b = b, a
	}
	m.notDuplicates[a+"|"+b] = true
}

// AddEventByDedupHash adds an event to the dedup hash index
func (m *MockRepository) AddEventByDedupHash(hash string, event *Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsByDedupHash[hash] = event
	m.events[event.ULID] = event
}

func TestIngestService_Ingest(t *testing.T) {
	tests := []struct {
		name       string
		input      EventInput
		setupRepo  func(*MockRepository)
		wantErr    bool
		wantDup    bool
		errMessage string
	}{
		{
			name: "successful ingest without source",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			setupRepo: func(m *MockRepository) {},
			wantErr:   false,
			wantDup:   false,
		},
		{
			name: "successful ingest with source - new event",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
				Source: &SourceInput{
					URL:     "https://example.com/events/123",
					EventID: "ext-123",
					Name:    "Example Source",
				},
			},
			setupRepo: func(m *MockRepository) {},
			wantErr:   false,
			wantDup:   false,
		},
		{
			name: "duplicate event by source external ID",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
				Source: &SourceInput{
					URL:     "https://example.com/events/123",
					EventID: "ext-123",
					Name:    "Example Source",
				},
			},
			setupRepo: func(m *MockRepository) {
				ulid, _ := ids.NewULID()
				existingEvent := &Event{
					ID:   "existing-1",
					ULID: ulid,
					Name: "Existing Event",
				}
				m.AddExistingEvent("source-1", "ext-123", existingEvent)
				// Pre-create the source so it matches
				m.sources["Example Source|https://example.com"] = "source-1"
			},
			wantErr: false,
			wantDup: true,
		},
		{
			name: "nil repository",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			setupRepo:  func(m *MockRepository) {},
			wantErr:    true,
			errMessage: "repository not configured",
		},
		{
			name: "validation error - missing name",
			input: EventInput{
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			setupRepo:  func(m *MockRepository) {},
			wantErr:    true,
			errMessage: "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var repo *MockRepository
			var service *IngestService

			if tt.name == "nil repository" {
				service = &IngestService{repo: nil, nodeDomain: "https://test.com"}
			} else {
				repo = NewMockRepository()
				tt.setupRepo(repo)
				service = NewIngestService(repo, "https://test.com", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true})
			}

			result, err := service.Ingest(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Ingest() expected error, got nil")
				} else if tt.errMessage != "" && !contains(err.Error(), tt.errMessage) {
					t.Errorf("Ingest() error = %v, want error containing %v", err, tt.errMessage)
				}
				return
			}

			if err != nil {
				t.Errorf("Ingest() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("Ingest() returned nil result")
				return
			}

			if result.IsDuplicate != tt.wantDup {
				t.Errorf("Ingest() IsDuplicate = %v, want %v", result.IsDuplicate, tt.wantDup)
			}

			if result.Event == nil {
				t.Error("Ingest() returned nil Event")
			}
		})
	}
}

func TestIngestService_IngestWithIdempotency(t *testing.T) {
	tests := []struct {
		name           string
		input          EventInput
		idempotencyKey string
		setupRepo      func(*MockRepository)
		wantErr        bool
		wantDup        bool
		errType        error
	}{
		{
			name: "first request with idempotency key",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			idempotencyKey: "test-key-1",
			setupRepo:      func(m *MockRepository) {},
			wantErr:        false,
			wantDup:        false,
		},
		{
			name: "duplicate request with same idempotency key and payload",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2024-06-15T10:00:00Z",
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			idempotencyKey: "test-key-2",
			setupRepo: func(m *MockRepository) {
				ulid, _ := ids.NewULID()
				event := &Event{
					ID:   "evt-1",
					ULID: ulid,
					Name: "Test Event",
				}
				m.events[ulid] = event

				// Pre-insert idempotency key with matching hash
				input := EventInput{
					Name:      "Test Event",
					StartDate: "2024-06-15T10:00:00Z",
					License:   "CC0-1.0",
					Location:  &PlaceInput{Name: "Test Venue"},
				}
				hash, _ := hashInput(NormalizeEventInput(input))
				m.idempotencyKeys["test-key-2"] = &IdempotencyKey{
					Key:         "test-key-2",
					RequestHash: hash,
					EventID:     &event.ID,
					EventULID:   &ulid,
				}
			},
			wantErr: false,
			wantDup: true,
		},
		{
			name: "conflict - same key different payload",
			input: EventInput{
				Name:      "Different Event",
				StartDate: "2024-06-15T10:00:00Z",
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			idempotencyKey: "test-key-3",
			setupRepo: func(m *MockRepository) {
				ulid, _ := ids.NewULID()
				event := &Event{
					ID:   "evt-1",
					ULID: ulid,
					Name: "Original Event",
				}
				m.events[ulid] = event

				// Pre-insert idempotency key with different hash
				m.idempotencyKeys["test-key-3"] = &IdempotencyKey{
					Key:         "test-key-3",
					RequestHash: "different-hash",
					EventID:     &event.ID,
					EventULID:   &ulid,
				}
			},
			wantErr: true,
			errType: ErrConflict,
		},
		{
			name: "empty idempotency key falls back to normal ingest",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			idempotencyKey: "",
			setupRepo:      func(m *MockRepository) {},
			wantErr:        false,
			wantDup:        false,
		},
		{
			name: "whitespace idempotency key falls back to normal ingest",
			input: EventInput{
				Name:      "Test Event",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			idempotencyKey: "   ",
			setupRepo:      func(m *MockRepository) {},
			wantErr:        false,
			wantDup:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			tt.setupRepo(repo)
			service := NewIngestService(repo, "https://test.com", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true})

			result, err := service.IngestWithIdempotency(context.Background(), tt.input, tt.idempotencyKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("IngestWithIdempotency() expected error, got nil")
					return
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("IngestWithIdempotency() error = %v, want %v", err, tt.errType)
				}
				return
			}

			if err != nil {
				t.Errorf("IngestWithIdempotency() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("IngestWithIdempotency() returned nil result")
				return
			}

			if result.IsDuplicate != tt.wantDup {
				t.Errorf("IngestWithIdempotency() IsDuplicate = %v, want %v", result.IsDuplicate, tt.wantDup)
			}
		})
	}
}

func TestIngestService_ReversedDates(t *testing.T) {
	tests := []struct {
		name            string
		input           EventInput
		wantErr         bool
		wantWarning     bool
		wantWarningCode string
		wantLifecycle   string
		wantNeedsReview bool
	}{
		{
			name: "reversed dates - ending at 2 AM (early morning) - auto-fixed by normalization",
			input: EventInput{
				Name:        "Monday Latin Nights with Latin Grooves and Dancing",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-03-31T23:00:00Z", // 11 PM
				EndDate:     "2025-03-31T02:00:00Z", // 2 AM (early morning, within 0-4 range)
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "DROM Taberna"},
			},
			wantErr:         false,
			wantWarning:     true, // Auto-fixed but still needs review per design doc
			wantWarningCode: "reversed_dates_timezone_likely",
			wantLifecycle:   "pending_review", // Changed from "published" - per design doc
			wantNeedsReview: true,             // Changed from false - per design doc
		},
		{
			name: "reversed dates - ending at 4 AM (early morning) - auto-fixed",
			input: EventInput{
				Name:        "Late Night Event",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-01T22:00:00Z", // 10 PM
				EndDate:     "2025-04-01T04:00:00Z", // 4 AM (early morning, boundary of 0-4)
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarning:     true, // Auto-fixed but still needs review per design doc
			wantWarningCode: "reversed_dates_timezone_likely",
			wantLifecycle:   "pending_review", // Changed from "published" - per design doc
			wantNeedsReview: true,             // Changed from false - per design doc
		},
		{
			name: "reversed dates - afternoon end (2 PM) - NOT auto-fixed",
			input: EventInput{
				Name:        "Suspicious Event",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-01T22:00:00Z",
				EndDate:     "2025-04-01T14:00:00Z", // 2 PM (not early morning)
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarning:     true,
			wantWarningCode: "reversed_dates_corrected_needs_review", // NOT early morning → corrected_needs_review warning
			wantLifecycle:   "pending_review",                        // Changed from "draft" - per design doc
			wantNeedsReview: true,
		},
		{
			name: "reversed dates - large gap (25 hours) - cannot be auto-fixed",
			input: EventInput{
				Name:        "Test Event",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-03T10:00:00Z",
				EndDate:     "2025-04-02T09:00:00Z", // 25 hours before
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarning:     true,
			wantWarningCode: "reversed_dates_corrected_needs_review", // Needs review
			wantLifecycle:   "pending_review",                        // Changed from "draft" - per design doc
			wantNeedsReview: true,
		},
		{
			name: "correct dates - no warning",
			input: EventInput{
				Name:        "Normal Event",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-01T10:00:00Z",
				EndDate:     "2025-04-01T12:00:00Z",
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarning:     false,
			wantLifecycle:   "published",
			wantNeedsReview: false,
		},
		{
			name: "no end date - but missing description/image",
			input: EventInput{
				Name:      "Event without end time",
				StartDate: "2025-04-01T10:00:00Z",
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarning:     true,                  // Now generates quality warnings
			wantWarningCode: "missing_description", // First warning is for missing description
			wantLifecycle:   "pending_review",      // Changed from "draft" - missing description/image triggers review
			wantNeedsReview: true,
		},
		{
			name: "reversed dates - exactly 24 hours - cannot be auto-fixed",
			input: EventInput{
				Name:        "Event at 24 hour boundary",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-02T10:00:00Z",
				EndDate:     "2025-04-01T10:00:00Z", // exactly 24 hours before
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarning:     true,
			wantWarningCode: "reversed_dates_corrected_needs_review", // Needs review
			wantLifecycle:   "pending_review",                        // Changed from "draft" - per design doc
			wantNeedsReview: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			service := NewIngestService(repo, "https://test.com", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true})

			result, err := service.Ingest(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Ingest() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Ingest() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("Ingest() returned nil result")
				return
			}

			// Check warnings
			if tt.wantWarning {
				if len(result.Warnings) == 0 {
					t.Errorf("Ingest() expected warnings, got none")
					return
				}
				foundCode := false
				for _, w := range result.Warnings {
					if w.Code == tt.wantWarningCode {
						foundCode = true
						// Skip field check for quality warnings (they don't always relate to endDate)
						break
					}
				}
				if !foundCode {
					t.Errorf("Ingest() expected warning code %v, got %v", tt.wantWarningCode, result.Warnings)
				}
			} else {
				if len(result.Warnings) > 0 {
					t.Errorf("Ingest() expected no warnings, got %v", result.Warnings)
				}
			}

			// Check NeedsReview flag
			if result.NeedsReview != tt.wantNeedsReview {
				t.Errorf("Ingest() NeedsReview = %v, want %v", result.NeedsReview, tt.wantNeedsReview)
			}

			// Check lifecycle state
			if result.Event.LifecycleState != tt.wantLifecycle {
				t.Errorf("Ingest() LifecycleState = %v, want %v", result.Event.LifecycleState, tt.wantLifecycle)
			}
		})
	}
}

func TestIngestService_PipelineOrder(t *testing.T) {
	// This test verifies that normalization runs BEFORE validation
	// Previously, validation rejected events before normalization could fix them
	tests := []struct {
		name           string
		input          EventInput
		wantErr        bool
		wantNormalized bool
		expectWarning  bool
	}{
		{
			name: "normalization should run before validation",
			input: EventInput{
				Name:        "Event with whitespace",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "  2025-04-01T10:00:00Z  ", // Extra whitespace
				EndDate:     "  2025-04-01T12:00:00Z  ",
				License:     "  CC0-1.0  ",
				Location:    &PlaceInput{Name: "  Test Venue  "},
			},
			wantErr:        false,
			wantNormalized: true,
			expectWarning:  false,
		},
		{
			name: "normalization fixes timezone with early morning end - generates warning for review",
			input: EventInput{
				Name:        "Event that normalization CAN fix",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-03-31T23:00:00Z",
				EndDate:     "2025-03-31T02:00:00Z", // 2 AM - auto-fixed (early morning 0-4, duration 3h)
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:        false,
			wantNormalized: true,
			expectWarning:  true, // Changed: per design doc, auto-corrected dates ALWAYS generate warnings
		},
		{
			name: "normalization cannot fix afternoon end - should warn",
			input: EventInput{
				Name:        "Event with afternoon end time",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-01T22:00:00Z",
				EndDate:     "2025-04-01T14:00:00Z", // 2 PM - NOT early morning
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:        false,
			wantNormalized: true,
			expectWarning:  true, // Not auto-fixed (end not early morning)
		},
		{
			name: "normalization cannot fix large gap - should warn",
			input: EventInput{
				Name:        "Event with 25 hour reversed gap",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-03T10:00:00Z",
				EndDate:     "2025-04-02T09:00:00Z", // 25 hours before (cannot be fixed)
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
			},
			wantErr:        false,
			wantNormalized: true,
			expectWarning:  true, // Normalization runs but can't fix this case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			service := NewIngestService(repo, "https://test.com", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true})

			result, err := service.Ingest(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Ingest() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Ingest() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("Ingest() returned nil result")
				return
			}

			// Verify normalization happened (check for trimmed whitespace in event name)
			if tt.wantNormalized && result.Event != nil {
				if result.Event.Name != strings.TrimSpace(tt.input.Name) {
					t.Errorf("Normalization did not run - Name = %q, want trimmed version", result.Event.Name)
				}
			}

			// Check if we got expected warnings
			if tt.expectWarning && len(result.Warnings) == 0 {
				t.Error("Expected warnings but got none")
			}
			if !tt.expectWarning && len(result.Warnings) > 0 {
				t.Errorf("Expected no warnings but got %v", result.Warnings)
			}
		})
	}
}

func TestIngestService_WarningsInDuplicateDetection(t *testing.T) {
	// Test that warnings are returned even when duplicate is detected
	tests := []struct {
		name          string
		input         EventInput
		setupRepo     func(*MockRepository)
		wantDuplicate bool
		wantWarnings  bool
	}{
		{
			name: "duplicate by source - warnings still returned for ambiguous reversed dates",
			input: EventInput{
				Name:        "Test Event",
				Description: "Test description",
				Image:       "https://example.com/image.jpg",
				StartDate:   "2025-04-01T22:00:00Z",
				EndDate:     "2025-04-01T14:00:00Z", // 8h reversed, afternoon end (ambiguous)
				License:     "CC0-1.0",
				Location:    &PlaceInput{Name: "Test Venue"},
				Source: &SourceInput{
					URL:     "https://example.com/events/123",
					EventID: "ext-123",
					Name:    "Example Source",
				},
			},
			setupRepo: func(m *MockRepository) {
				ulid, _ := ids.NewULID()
				existingEvent := &Event{
					ID:   "existing-1",
					ULID: ulid,
					Name: "Existing Event",
				}
				m.AddExistingEvent("source-1", "ext-123", existingEvent)
				m.sources["Example Source|https://example.com"] = "source-1"
			},
			wantDuplicate: true,
			wantWarnings:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			tt.setupRepo(repo)
			service := NewIngestService(repo, "https://test.com", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true})

			result, err := service.Ingest(context.Background(), tt.input)

			if err != nil {
				t.Errorf("Ingest() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("Ingest() returned nil result")
				return
			}

			if result.IsDuplicate != tt.wantDuplicate {
				t.Errorf("Ingest() IsDuplicate = %v, want %v", result.IsDuplicate, tt.wantDuplicate)
			}

			if tt.wantWarnings && len(result.Warnings) == 0 {
				t.Error("Expected warnings even for duplicate, got none")
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNearDuplicateWarningsWithDetails(t *testing.T) {
	t.Run("embeds_new_event_details", func(t *testing.T) {
		existingEvent := &Event{Name: "Jazz at the Rex"}
		newEventULID := "01ABCDEF0123456789ABCDEF"

		data := nearDupNewEventData{
			Name:      "Rex Jazz Night",
			StartDate: "2026-06-15T20:00:00Z",
			EndDate:   "2026-06-15T23:00:00Z",
			VenueName: "The Rex Hotel",
		}

		b, err := nearDuplicateWarnings(existingEvent, newEventULID, data)
		if err != nil {
			t.Fatalf("nearDuplicateWarnings() error = %v", err)
		}

		var warnings []struct {
			Field   string         `json:"field"`
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		}
		if err := json.Unmarshal(b, &warnings); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(warnings) != 1 {
			t.Fatalf("expected 1 warning, got %d", len(warnings))
		}
		w := warnings[0]
		if w.Code != "near_duplicate_of_new_event" {
			t.Errorf("code = %v, want near_duplicate_of_new_event", w.Code)
		}
		if w.Details == nil {
			t.Fatal("expected Details to be non-nil")
		}
		if w.Details["new_event_name"] != "Rex Jazz Night" {
			t.Errorf("new_event_name = %v, want Rex Jazz Night", w.Details["new_event_name"])
		}
		if w.Details["new_event_startDate"] != "2026-06-15T20:00:00Z" {
			t.Errorf("new_event_startDate = %v, want 2026-06-15T20:00:00Z", w.Details["new_event_startDate"])
		}
		if w.Details["new_event_endDate"] != "2026-06-15T23:00:00Z" {
			t.Errorf("new_event_endDate = %v, want 2026-06-15T23:00:00Z", w.Details["new_event_endDate"])
		}
		if w.Details["new_event_venue"] != "The Rex Hotel" {
			t.Errorf("new_event_venue = %v, want The Rex Hotel", w.Details["new_event_venue"])
		}
	})

	t.Run("omits_empty_optional_fields", func(t *testing.T) {
		// Zero values for StartDate, EndDate, VenueName should not appear in Details.
		existingEvent := &Event{Name: "Jazz"}
		newEventULID := "01ABCDEF0123456789ABCDEF"

		data := nearDupNewEventData{
			Name:      "Jazz Night",
			StartDate: "",
			EndDate:   "",
			VenueName: "",
		}

		b, err := nearDuplicateWarnings(existingEvent, newEventULID, data)
		if err != nil {
			t.Fatalf("nearDuplicateWarnings() error = %v", err)
		}

		var warnings []struct {
			Details map[string]any `json:"details"`
		}
		if err := json.Unmarshal(b, &warnings); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(warnings) != 1 {
			t.Fatalf("expected 1 warning")
		}
		d := warnings[0].Details
		if _, ok := d["new_event_startDate"]; ok {
			t.Error("new_event_startDate should be absent for empty StartDate")
		}
		if _, ok := d["new_event_endDate"]; ok {
			t.Error("new_event_endDate should be absent for empty EndDate")
		}
		if _, ok := d["new_event_venue"]; ok {
			t.Error("new_event_venue should be absent for empty VenueName")
		}
		// name must always be present
		if d["new_event_name"] != "Jazz Night" {
			t.Errorf("new_event_name = %v, want Jazz Night", d["new_event_name"])
		}
	})

	t.Run("message_includes_existing_event_name", func(t *testing.T) {
		existingEvent := &Event{Name: "Old Jazz Night"}
		b, err := nearDuplicateWarnings(existingEvent, "NEW-ULID", nearDupNewEventData{Name: "New Jazz"})
		if err != nil {
			t.Fatalf("nearDuplicateWarnings() error = %v", err)
		}
		var warnings []struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(b, &warnings); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !hasSubstring(warnings[0].Message, "Old Jazz Night") {
			t.Errorf("message %q should contain existing event name", warnings[0].Message)
		}
	})
}

// TestIngestService_MultiOccurrenceParentVenue is a regression test for the staging bug where
// fixture events with an occurrences array and a parent-only location failed with
// "occurrence_location_required" DB constraint violation.
//
// Root cause: validateOccurrences did not enforce that each occurrence must have a venue
// resolvable at create time; createOccurrencesWithRepo inherited event.PrimaryVenueID, which
// is nil when the parent event's location wasn't persisted (or when submitted without location).
//
// Fix: validateOccurrences now rejects occurrences that have neither venueId nor virtualUrl
// when the parent event also provides no location or virtualLocation. The mock enforces the
// same DB constraint so unit tests catch the regression.
func TestIngestService_MultiOccurrenceParentVenue(t *testing.T) {
	ctx := context.Background()
	futureDateFn := func(offset time.Duration) string {
		return time.Now().Add(offset).Format(time.RFC3339)
	}

	t.Run("multi_occurrence_inherits_parent_venue", func(t *testing.T) {
		// Regression: recurring event with parent location and bare occurrences must ingest
		// successfully on all environments. The mock now enforces occurrence_location_required
		// so this will fail if inheritance is broken.
		repo := NewMockRepository()
		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Weekly Yoga at Studio",
			Description: "A recurring yoga class every week.",
			Image:       "https://images.example.com/yoga.jpg",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				Name:            "The Studio",
				AddressLocality: "Toronto",
				AddressRegion:   "ON",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				// bare occurrences: no venueId, no virtualUrl — rely on parent location
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
					Timezone:  "America/Toronto",
				},
				{
					StartDate: futureDateFn(14 * 24 * time.Hour),
					EndDate:   futureDateFn(14*24*time.Hour + 90*time.Minute),
					Timezone:  "America/Toronto",
				},
				{
					StartDate: futureDateFn(21 * 24 * time.Hour),
					EndDate:   futureDateFn(21*24*time.Hour + 90*time.Minute),
					Timezone:  "America/Toronto",
				},
			},
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil (occurrence should inherit parent venue)", err)
		}
		if result == nil || result.Event == nil {
			t.Fatal("Ingest() returned nil result or event")
		}

		// All 3 occurrences must have been created with a venue inherited from the parent event.
		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 3 {
			t.Fatalf("expected 3 occurrences, got %d", len(occs))
		}
		for i, occ := range occs {
			if occ.VenueID == nil && occ.VirtualURL == nil {
				t.Errorf("occurrence[%d] has no venue or virtual URL after inheritance", i)
			}
		}
	})

	t.Run("multi_occurrence_inherits_parent_virtual_location", func(t *testing.T) {
		// Regression: bare occurrences (no venueId/virtualUrl) must also work when the
		// parent event has a virtualLocation instead of a physical location. The occurrence
		// should inherit the parent's VirtualURL and satisfy occurrence_location_required.
		repo := NewMockRepository()
		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Online Yoga Series",
			Description: "A recurring online yoga class.",
			License:     "CC0-1.0",
			VirtualLocation: &VirtualLocationInput{
				URL: "https://zoom.us/j/meeting123",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			// Bare occurrences: no venueId/virtualUrl — rely on parent virtualLocation.
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
				},
			},
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil (occurrence should inherit parent virtualLocation)", err)
		}
		if result == nil || result.Event == nil {
			t.Fatal("Ingest() returned nil result or event")
		}
		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 1 {
			t.Fatalf("expected 1 occurrence, got %d", len(occs))
		}
		if occs[0].VirtualURL == nil {
			t.Error("occurrence should have inherited virtualURL from parent event")
		}
	})

	t.Run("occurrence_with_resolved_venueId_must_not_inherit_parent_virtualLocation", func(t *testing.T) {
		// Regression: when a multi-occurrence event has a parent virtualLocation AND an
		// occurrence that resolves its own physical venueId, the occurrence must NOT
		// inherit the parent's virtualURL. Doing so creates a hybrid occurrence with both
		// venue_id and virtual_url set, violating the location contract (an occurrence is
		// either physical or virtual, not both).
		//
		// Fix: virtualURL inheritance is skipped when venueID is already resolved.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		repo := NewMockRepository()
		repo.SeedPlaceByULID(placeULID, placeUUID)

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Hybrid Series",
			Description: "Event with a parent virtualLocation but an occurrence that specifies a physical venue.",
			License:     "CC0-1.0",
			VirtualLocation: &VirtualLocationInput{
				URL: "https://stream.example.com/live",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
					// Explicitly references a physical venue — must NOT inherit parent virtualURL.
					VenueID: "https://test.togather.ca/places/" + placeULID,
				},
			},
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil", err)
		}
		if result == nil || result.Event == nil {
			t.Fatal("Ingest() returned nil result or event")
		}

		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 1 {
			t.Fatalf("expected 1 occurrence, got %d", len(occs))
		}
		occ := occs[0]
		// VenueID must be the resolved UUID.
		if occ.VenueID == nil || *occ.VenueID != placeUUID {
			t.Errorf("occurrence VenueID = %v; want %q", occ.VenueID, placeUUID)
		}
		// VirtualURL must NOT be set — no hybrid occurrences.
		if occ.VirtualURL != nil {
			t.Errorf("occurrence VirtualURL = %q; want nil (physical venue must not inherit parent virtualURL)", *occ.VirtualURL)
		}
	})

	t.Run("parent_location_atid_only_not_found_rejected", func(t *testing.T) {
		// Regression: when parent location supplies only a canonical @id and the place
		// does not exist in the DB, ingest must return a clear error.
		// The mock has no seeded place, so GetPlaceByULID returns ErrNotFound.
		repo := NewMockRepository()
		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Event at Canonical Venue (not found)",
			Description: "Parent location @id references a non-existent place.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				ID: "https://test.togather.ca/places/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
				},
			},
		}

		_, err := svc.Ingest(ctx, input)
		if err == nil {
			t.Fatal("Ingest() expected error for @id referencing non-existent place, got nil")
		}
		if !strings.Contains(err.Error(), "location.@id") {
			t.Errorf("Ingest() error = %v; want error mentioning 'location.@id'", err)
		}
	})

	t.Run("parent_location_atid_only_valid_resolves_primary_venue", func(t *testing.T) {
		// Regression: parent location with only canonical @id (no name) must resolve the
		// place via GetPlaceByULID and set PrimaryVenueID on the event.
		// Occurrences that omit venueId should inherit this resolved venue.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
		repo := NewMockRepository()
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, "The Canonical Hall")

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Event at Canonical Hall",
			Description: "Parent location uses only canonical @id; no name supplied.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				ID: "https://test.togather.ca/places/" + placeULID,
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
				},
			},
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil", err)
		}
		if result.Event.PrimaryVenueID == nil {
			t.Fatal("Event.PrimaryVenueID is nil; want the canonical place UUID")
		}
		if *result.Event.PrimaryVenueID != placeUUID {
			t.Errorf("Event.PrimaryVenueID = %q; want %q", *result.Event.PrimaryVenueID, placeUUID)
		}
		// Occurrence must inherit the resolved venue.
		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 1 {
			t.Fatalf("expected 1 occurrence, got %d", len(occs))
		}
		if occs[0].VenueID == nil || *occs[0].VenueID != placeUUID {
			t.Errorf("occurrence VenueID = %v; want %q (inherited from parent @id)", occs[0].VenueID, placeUUID)
		}
	})

	t.Run("parent_location_atid_plus_matching_name_accepted", func(t *testing.T) {
		// Regression: parent location with both @id and a name that matches the canonical
		// place must succeed — @id is authoritative and name check passes.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
		const canonicalName = "The Canonical Hall"
		repo := NewMockRepository()
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, canonicalName)

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Event at Canonical Hall (with name)",
			Description: "Parent location uses @id and a matching name.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				ID:   "https://test.togather.ca/places/" + placeULID,
				Name: canonicalName, // matches canonical — should pass
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
				},
			},
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil (matching name should be accepted)", err)
		}
		if result.Event.PrimaryVenueID == nil || *result.Event.PrimaryVenueID != placeUUID {
			t.Errorf("Event.PrimaryVenueID = %v; want %q", result.Event.PrimaryVenueID, placeUUID)
		}
	})

	t.Run("parent_location_atid_plus_mismatched_name_rejected", func(t *testing.T) {
		// Regression: parent location with both @id and a name that DOES NOT match the
		// canonical place name must be rejected with a clear error.
		// This prevents silent venue misattribution when a submitter has the wrong @id.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
		repo := NewMockRepository()
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, "The Canonical Hall")

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Event at Wrong Venue",
			Description: "Parent location @id and name disagree.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				ID:   "https://test.togather.ca/places/" + placeULID,
				Name: "Totally Different Venue", // mismatches canonical — must reject
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
				},
			},
		}

		_, err := svc.Ingest(ctx, input)
		if err == nil {
			t.Fatal("Ingest() expected error when location.@id and location.name disagree, got nil")
		}
		if !strings.Contains(err.Error(), "location.name") {
			t.Errorf("Ingest() error = %v; want error mentioning 'location.name'", err)
		}
		if !strings.Contains(err.Error(), "does not match") {
			t.Errorf("Ingest() error = %v; want error mentioning 'does not match'", err)
		}
	})

	t.Run("parent_location_atid_only_single_occurrence_resolves_and_inherits", func(t *testing.T) {
		// Regression: single-occurrence event (no Occurrences array) whose parent
		// location uses only canonical @id must resolve the place and inherit venue
		// into the implicit occurrence.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
		repo := NewMockRepository()
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, "The Canonical Hall")

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Single-Occ Event at Canonical Hall",
			Description: "Single-occurrence with @id-only parent location.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				ID: "https://test.togather.ca/places/" + placeULID,
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			// No Occurrences array — single-occurrence path.
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil (single-occ @id-only should resolve)", err)
		}
		if result.Event.PrimaryVenueID == nil || *result.Event.PrimaryVenueID != placeUUID {
			t.Errorf("Event.PrimaryVenueID = %v; want %q", result.Event.PrimaryVenueID, placeUUID)
		}
		// Implicit single occurrence must also carry the resolved venue.
		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 1 {
			t.Fatalf("expected 1 implicit occurrence, got %d", len(occs))
		}
		if occs[0].VenueID == nil || *occs[0].VenueID != placeUUID {
			t.Errorf("implicit occurrence VenueID = %v; want %q", occs[0].VenueID, placeUUID)
		}
	})

	t.Run("occurrence_venueId_canonical_uri_resolves_to_uuid", func(t *testing.T) {
		// Regression (Issue 2 — venueId resolution): an occurrence with venueId set to a
		// canonical place URI must have the URI resolved to the underlying place UUID before
		// being persisted.  Previously the raw URI string was passed directly to the UUID
		// venue_id column, which would cause a DB type mismatch.
		//
		// Arrange: seed the mock with a known place ULID → UUID mapping.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		repo := NewMockRepository()
		repo.SeedPlaceByULID(placeULID, placeUUID)

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Event with Explicit venueId",
			Description: "Occurrence references a known place via canonical URI.",
			License:     "CC0-1.0",
			// Parent has a named location to satisfy the location-required check, but each
			// occurrence overrides it with an explicit venueId.
			Location: &PlaceInput{
				Name:            "Fallback Venue",
				AddressLocality: "Toronto",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate: futureDateFn(7 * 24 * time.Hour),
					EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
					Timezone:  "America/Toronto",
					// Canonical place URI — must be resolved to placeUUID.
					VenueID: "https://test.togather.ca/places/" + placeULID,
				},
			},
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil", err)
		}
		if result == nil || result.Event == nil {
			t.Fatal("Ingest() returned nil result or event")
		}

		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 1 {
			t.Fatalf("expected 1 occurrence, got %d", len(occs))
		}
		occ := occs[0]
		if occ.VenueID == nil {
			t.Fatal("occurrence VenueID is nil; want a UUID")
		}
		if *occ.VenueID != placeUUID {
			t.Errorf("occurrence VenueID = %q; want %q (UUID, not canonical URI)", *occ.VenueID, placeUUID)
		}
	})

	t.Run("single_occurrence_physical_location_must_not_inherit_parent_virtual_url", func(t *testing.T) {
		// Regression (Issue 1 — single-occ hybrid): when the parent event has BOTH a
		// physical location (location.name) AND a virtualLocation, the implicit single
		// occurrence must receive the physical venue only.  Previously virtualURL(input)
		// was unconditionally assigned, creating a hybrid occurrence with both venue_id
		// AND virtual_url set — violating the location contract.
		repo := NewMockRepository()
		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true, RequireImage: false})

		input := EventInput{
			Name:        "Hybrid-parent single occurrence",
			Description: "Parent has physical and virtual; occurrence must be physical-only.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				Name:            "Studio 42",
				AddressLocality: "Toronto",
			},
			VirtualLocation: &VirtualLocationInput{
				URL: "https://stream.example.com/live",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			// No Occurrences slice — single-occurrence path.
		}

		result, err := svc.Ingest(ctx, input)
		if err != nil {
			t.Fatalf("Ingest() error = %v; want nil", err)
		}
		if result == nil || result.Event == nil {
			t.Fatal("Ingest() returned nil result or event")
		}

		occs := repo.occurrences[result.Event.ID]
		if len(occs) != 1 {
			t.Fatalf("expected 1 occurrence, got %d", len(occs))
		}
		occ := occs[0]
		// Must have the physical venue.
		if occ.VenueID == nil {
			t.Error("occurrence VenueID is nil; want the resolved physical place UUID")
		}
		// Must NOT have the virtual URL — no hybrid occurrences.
		if occ.VirtualURL != nil {
			t.Errorf("occurrence VirtualURL = %q; want nil (physical location takes priority, no hybrid)", *occ.VirtualURL)
		}
	})

	t.Run("explicit_occurrence_with_both_venueId_and_virtualUrl_is_rejected", func(t *testing.T) {
		// Regression (Issue 2 — explicit hybrid): a caller that supplies both venueId
		// AND virtualUrl on the same occurrence must be rejected at validation time with
		// a clear error, not silently persisted as a hybrid occurrence.
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		repo := NewMockRepository()
		repo.SeedPlaceByULID(placeULID, placeUUID)

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true})

		input := EventInput{
			Name:        "Explicit Hybrid Occurrence",
			Description: "Occurrence supplies both venueId and virtualUrl — must be rejected.",
			License:     "CC0-1.0",
			Location: &PlaceInput{
				Name:            "Fallback Venue",
				AddressLocality: "Toronto",
			},
			StartDate: futureDateFn(7 * 24 * time.Hour),
			EndDate:   futureDateFn(7*24*time.Hour + 90*time.Minute),
			Occurrences: []OccurrenceInput{
				{
					StartDate:  futureDateFn(7 * 24 * time.Hour),
					EndDate:    futureDateFn(7*24*time.Hour + 90*time.Minute),
					Timezone:   "America/Toronto",
					VenueID:    "https://test.togather.ca/places/" + placeULID,
					VirtualURL: "https://stream.example.com/live",
				},
			},
		}

		_, err := svc.Ingest(ctx, input)
		if err == nil {
			t.Fatal("Ingest() returned nil error; want a validation error for hybrid occurrence")
		}
		var ve ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("Ingest() error = %T (%v); want ValidationError", err, err)
		}
	})
}

func TestAppendWarnings(t *testing.T) {
	mustMarshal := func(w []ValidationWarning) []byte {
		b, err := json.Marshal(w)
		if err != nil {
			t.Fatalf("failed to marshal warnings: %v", err)
		}
		return b
	}

	t.Run("nil inputs return empty array", func(t *testing.T) {
		result, err := appendWarnings(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out []ValidationWarning
		if err := json.Unmarshal(result, &out); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("want 0 warnings, got %d", len(out))
		}
	})

	t.Run("appends new warnings to existing", func(t *testing.T) {
		existing := mustMarshal([]ValidationWarning{
			{Field: "multi_session", Code: "multi_session_likely", Message: "may recur"},
		})
		toAdd := mustMarshal([]ValidationWarning{
			{Field: "near_duplicate", Code: "near_duplicate_of_new_event", Message: "may be a near-dup of NEW-1"},
		})
		result, err := appendWarnings(existing, toAdd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out []ValidationWarning
		if err := json.Unmarshal(result, &out); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if len(out) != 2 {
			t.Errorf("want 2 warnings, got %d", len(out))
		}
	})

	t.Run("deduplicates identical warnings by field+code+message", func(t *testing.T) {
		w := ValidationWarning{Field: "near_duplicate", Code: "near_duplicate_of_new_event", Message: "dup of NEW-1"}
		existing := mustMarshal([]ValidationWarning{w})
		toAdd := mustMarshal([]ValidationWarning{w})
		result, err := appendWarnings(existing, toAdd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out []ValidationWarning
		if err := json.Unmarshal(result, &out); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if len(out) != 1 {
			t.Errorf("want 1 warning (deduped), got %d", len(out))
		}
	})

	t.Run("same code different message are both retained", func(t *testing.T) {
		// Two near_duplicate_of_new_event warnings for different new events must both be kept.
		existing := mustMarshal([]ValidationWarning{
			{Field: "near_duplicate", Code: "near_duplicate_of_new_event", Message: "dup of NEW-1"},
		})
		toAdd := mustMarshal([]ValidationWarning{
			{Field: "near_duplicate", Code: "near_duplicate_of_new_event", Message: "dup of NEW-2"},
		})
		result, err := appendWarnings(existing, toAdd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out []ValidationWarning
		if err := json.Unmarshal(result, &out); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if len(out) != 2 {
			t.Errorf("want 2 warnings (distinct near-dup targets), got %d", len(out))
		}
	})

	t.Run("invalid JSON in existing returns error", func(t *testing.T) {
		_, err := appendWarnings([]byte("not-json"), nil)
		if err == nil {
			t.Error("expected error for invalid existing JSON, got nil")
		}
	})

	t.Run("invalid JSON in toAdd returns error", func(t *testing.T) {
		existing := mustMarshal([]ValidationWarning{
			{Field: "f", Code: "c", Message: "m"},
		})
		_, err := appendWarnings(existing, []byte("not-json"))
		if err == nil {
			t.Error("expected error for invalid toAdd JSON, got nil")
		}
	})

	t.Run("empty existing plus new warnings", func(t *testing.T) {
		toAdd := mustMarshal([]ValidationWarning{
			{Field: "near_duplicate", Code: "near_duplicate_of_new_event", Message: "dup of NEW-1"},
		})
		result, err := appendWarnings([]byte("[]"), toAdd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out []ValidationWarning
		if err := json.Unmarshal(result, &out); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if len(out) != 1 {
			t.Errorf("want 1 warning, got %d", len(out))
		}
	})
}

func TestCrossWeekSeriesCompanion(t *testing.T) {
	t.Run("flags_event_with_cross_week_series_companion", func(t *testing.T) {
		repo := NewMockRepository()
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "c3d4e5f6-a7b8-9012-cdef-123456789012"
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, "Community Centre")

		companionStartDate := time.Date(2026, 3, 8, 19, 0, 0, 0, time.UTC)
		repo.seriesCompanion = &CrossWeekCompanion{
			ULID:      "01ARZ3NDEKTSV4RRFFQ69G5FBV",
			Name:      "Weekly Pottery",
			StartDate: companionStartDate.Format(time.RFC3339),
			StartTime: "19:00:00",
			VenueName: "Community Centre",
		}

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true}).
			WithDedupConfig(config.DedupConfig{NearDuplicateThreshold: 0.4})

		input := EventInput{
			Name:        "Weekly Pottery",
			Description: "Test event",
			License:     "CC0-1.0",
			StartDate:   "2026-03-15T19:00:00Z",
			Location: &PlaceInput{
				ID: "https://test.togather.ca/places/" + placeULID,
			},
			Occurrences: []OccurrenceInput{
				{StartDate: "2026-03-15T19:00:00Z"},
			},
		}

		result, err := svc.Ingest(context.Background(), input)
		if err != nil {
			t.Fatalf("Ingest() error = %v", err)
		}

		if !result.NeedsReview {
			t.Error("expected NeedsReview = true when cross-week companion found")
		}

		var crossWeekWarning *ValidationWarning
		for _, w := range result.Warnings {
			if w.Code == "cross_week_series_companion" {
				crossWeekWarning = &w
				break
			}
		}
		if crossWeekWarning == nil {
			t.Fatal("cross_week_series_companion warning not found in result.Warnings")
		}
		if crossWeekWarning.Field != "name" {
			t.Errorf("Field = %q, want %q", crossWeekWarning.Field, "name")
		}
		if !strings.Contains(crossWeekWarning.Message, "Weekly Pottery") {
			t.Errorf("Message = %q, want to contain %q", crossWeekWarning.Message, "Weekly Pottery")
		}
		if !strings.Contains(crossWeekWarning.Message, "2026-03-08") {
			t.Errorf("Message = %q, want to contain %q", crossWeekWarning.Message, "2026-03-08")
		}
		if crossWeekWarning.Details["companion_ulid"] != "01ARZ3NDEKTSV4RRFFQ69G5FBV" {
			t.Errorf("companion_ulid = %v, want %q", crossWeekWarning.Details["companion_ulid"], "01ARZ3NDEKTSV4RRFFQ69G5FBV")
		}
		if crossWeekWarning.Details["companion_name"] != "Weekly Pottery" {
			t.Errorf("companion_name = %v, want %q", crossWeekWarning.Details["companion_name"], "Weekly Pottery")
		}
		if crossWeekWarning.Details["companion_date"] != "2026-03-08T19:00:00Z" {
			t.Errorf("companion_date = %v, want %q", crossWeekWarning.Details["companion_date"], "2026-03-08T19:00:00Z")
		}
		if crossWeekWarning.Details["companion_time"] != "19:00:00" {
			t.Errorf("companion_time = %v, want %q", crossWeekWarning.Details["companion_time"], "19:00:00")
		}
		if crossWeekWarning.Details["venue_name"] != "Community Centre" {
			t.Errorf("venue_name = %v, want %q", crossWeekWarning.Details["venue_name"], "Community Centre")
		}
	})

	t.Run("no_warning_when_no_series_companion", func(t *testing.T) {
		repo := NewMockRepository()
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "d4e5f6a7-b8c9-0123-def0-234567890123"
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, "Art Gallery")
		repo.seriesCompanion = nil

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true}).
			WithDedupConfig(config.DedupConfig{NearDuplicateThreshold: 0.4})

		input := EventInput{
			Name:        "Unique Art Show",
			Description: "Test",
			License:     "CC0-1.0",
			StartDate:   "2026-03-15T19:00:00Z",
			Location: &PlaceInput{
				ID: "https://test.togather.ca/places/" + placeULID,
			},
			Occurrences: []OccurrenceInput{
				{StartDate: "2026-03-15T19:00:00Z"},
			},
		}

		result, err := svc.Ingest(context.Background(), input)
		if err != nil {
			t.Fatalf("Ingest() error = %v", err)
		}

		for _, w := range result.Warnings {
			if w.Code == "cross_week_series_companion" {
				t.Error("cross_week_series_companion warning should not be present when no companion found")
			}
		}
	})

	t.Run("error_in_series_companion_check_is_non_fatal", func(t *testing.T) {
		repo := NewMockRepository()
		const placeULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		const placeUUID = "e5f6a7b8-c9d0-1234-ef01-345678901234"
		repo.SeedPlaceByULIDWithName(placeULID, placeUUID, "Studio")
		repo.shouldFailFindSeriesCompanion = true

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true}).
			WithDedupConfig(config.DedupConfig{NearDuplicateThreshold: 0.4})

		input := EventInput{
			Name:        "Art Workshop",
			Description: "Test",
			License:     "CC0-1.0",
			StartDate:   "2026-03-15T19:00:00Z",
			Location: &PlaceInput{
				ID: "https://test.togather.ca/places/" + placeULID,
			},
			Occurrences: []OccurrenceInput{
				{StartDate: "2026-03-15T19:00:00Z"},
			},
		}

		result, err := svc.Ingest(context.Background(), input)
		if err != nil {
			t.Fatalf("Ingest() error = %v (expected non-fatal)", err)
		}

		for _, w := range result.Warnings {
			if w.Code == "cross_week_series_companion" {
				t.Error("cross_week_series_companion warning should not be present when check failed")
			}
		}
	})

	t.Run("no_check_when_no_venue", func(t *testing.T) {
		repo := NewMockRepository()
		repo.seriesCompanion = &CrossWeekCompanion{ULID: "01ARZ3NDEKTSV4RRFFQ69G5FBV"}

		svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
			config.ValidationConfig{AllowTestDomains: true}).
			WithDedupConfig(config.DedupConfig{NearDuplicateThreshold: 0.4})

		input := EventInput{
			Name:        "Virtual Workshop",
			Description: "Test",
			License:     "CC0-1.0",
			StartDate:   "2026-03-15T19:00:00Z",
			VirtualLocation: &VirtualLocationInput{
				URL: "https://zoom.us/j/123456",
			},
		}

		result, err := svc.Ingest(context.Background(), input)
		if err != nil {
			t.Fatalf("Ingest() error = %v", err)
		}

		for _, w := range result.Warnings {
			if w.Code == "cross_week_series_companion" {
				t.Error("cross_week_series_companion warning should not appear when no venue")
			}
		}
	})
}
