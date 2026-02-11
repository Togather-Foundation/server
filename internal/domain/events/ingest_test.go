package events

import (
	"context"
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
	approveReviewCalled bool
	approveReviewID     int
	updateEventCalls    []updateEventCall
	mergePlacesCalled   bool
	mergePlacesDupID    string
	mergePlacesPriID    string

	// Behavior controls
	shouldFailCreate                 bool
	shouldFailGetIdempotencyKey      bool
	shouldFailInsertIdempotencyKey   bool
	shouldFailUpdateIdempotencyKey   bool
	shouldFailFindBySourceExternalID bool
	shouldFailFindByDedupHash        bool
	shouldFailGetOrCreateSource      bool
	shouldFailUpsertPlace            bool
	shouldFailUpsertOrganization     bool
	shouldFailCreateOccurrence       bool
	shouldFailCreateSource           bool
	shouldFailFindNearDuplicates     bool
	shouldFailFindSimilarPlaces      bool
	shouldFailFindSimilarOrgs        bool
	shouldFailUpdateEvent            bool
	shouldFailApproveReview          bool
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
	return place, nil
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
		ID:                m.nextReviewID,
		EventID:           params.EventID,
		OriginalPayload:   params.OriginalPayload,
		NormalizedPayload: params.NormalizedPayload,
		Warnings:          params.Warnings,
		SourceID:          params.SourceID,
		SourceExternalID:  params.SourceExternalID,
		DedupHash:         params.DedupHash,
		EventStartTime:    params.EventStartTime,
		EventEndTime:      params.EventEndTime,
		Status:            "pending",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
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
				service = NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: true})
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
			service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: true})

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
			service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: true})

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
			service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: true})

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
			service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: true})

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
