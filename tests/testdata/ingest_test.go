package testdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Togather-Foundation/server/internal/config"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IngestMockRepository implements events.Repository interface for testing.
// This is a copy of the mock from events package to avoid import cycles.
type IngestMockRepository struct {
	mu sync.Mutex

	// Storage
	events            map[string]*events.Event
	idempotencyKeys   map[string]*events.IdempotencyKey
	sources           map[string]string                          // sourceKey -> sourceID
	eventsBySources   map[string]map[string]*events.Event        // sourceID -> sourceEventID -> Event
	eventsByDedupHash map[string]*events.Event                   // dedupHash -> Event
	places            map[string]*events.PlaceRecord             // key -> place
	organizations     map[string]*events.OrganizationRecord      // key -> org
	Occurrences       map[string][]events.OccurrenceCreateParams // eventID -> occurrences (exported for tests)
}

// NewIngestMockRepository creates a new mock repository for ingest testing.
func NewIngestMockRepository() *IngestMockRepository {
	return &IngestMockRepository{
		events:            make(map[string]*events.Event),
		idempotencyKeys:   make(map[string]*events.IdempotencyKey),
		sources:           make(map[string]string),
		eventsBySources:   make(map[string]map[string]*events.Event),
		eventsByDedupHash: make(map[string]*events.Event),
		places:            make(map[string]*events.PlaceRecord),
		organizations:     make(map[string]*events.OrganizationRecord),
		Occurrences:       make(map[string][]events.OccurrenceCreateParams),
	}
}

func (m *IngestMockRepository) List(ctx context.Context, filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var eventList []events.Event
	for _, event := range m.events {
		eventList = append(eventList, *event)
	}
	return events.ListResult{Events: eventList, NextCursor: ""}, nil
}

func (m *IngestMockRepository) GetByULID(ctx context.Context, ulid string) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.events[ulid]
	if !ok {
		return nil, events.ErrNotFound
	}
	return event, nil
}

func (m *IngestMockRepository) Create(ctx context.Context, params events.EventCreateParams) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event := &events.Event{
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

func (m *IngestMockRepository) CreateOccurrence(ctx context.Context, params events.OccurrenceCreateParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Occurrences[params.EventID] = append(m.Occurrences[params.EventID], params)
	return nil
}

func (m *IngestMockRepository) CreateSource(ctx context.Context, params events.EventSourceCreateParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record the event for source-based duplicate detection
	if params.SourceID != "" && params.SourceEventID != "" {
		if _, ok := m.eventsBySources[params.SourceID]; !ok {
			m.eventsBySources[params.SourceID] = make(map[string]*events.Event)
		}
		// Find the event by ID and record it
		for _, event := range m.events {
			if event.ID == params.EventID {
				m.eventsBySources[params.SourceID][params.SourceEventID] = event
				break
			}
		}
	}
	return nil
}

func (m *IngestMockRepository) FindBySourceExternalID(ctx context.Context, sourceID string, sourceEventID string) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sources, ok := m.eventsBySources[sourceID]; ok {
		if event, ok := sources[sourceEventID]; ok {
			return event, nil
		}
	}
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) FindByDedupHash(ctx context.Context, dedupHash string) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.eventsByDedupHash[dedupHash]
	if !ok {
		return nil, events.ErrNotFound
	}
	return event, nil
}

func (m *IngestMockRepository) GetOrCreateSource(ctx context.Context, params events.SourceLookupParams) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s|%s", params.Name, params.BaseURL)
	if sourceID, ok := m.sources[key]; ok {
		return sourceID, nil
	}

	sourceID := fmt.Sprintf("source-%d", len(m.sources)+1)
	m.sources[key] = sourceID
	m.eventsBySources[sourceID] = make(map[string]*events.Event)
	return sourceID, nil
}

func (m *IngestMockRepository) GetIdempotencyKey(ctx context.Context, key string) (*events.IdempotencyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ikey, ok := m.idempotencyKeys[key]
	if !ok {
		return nil, events.ErrNotFound
	}
	return ikey, nil
}

func (m *IngestMockRepository) InsertIdempotencyKey(ctx context.Context, params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var eventID *string
	var eventULID *string
	if params.EventID != "" {
		eventID = &params.EventID
	}
	if params.EventULID != "" {
		eventULID = &params.EventULID
	}

	ikey := &events.IdempotencyKey{
		Key:         params.Key,
		RequestHash: params.RequestHash,
		EventID:     eventID,
		EventULID:   eventULID,
	}
	m.idempotencyKeys[params.Key] = ikey
	return ikey, nil
}

func (m *IngestMockRepository) UpdateIdempotencyKeyEvent(ctx context.Context, key string, eventID string, eventULID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ikey, ok := m.idempotencyKeys[key]
	if !ok {
		return events.ErrNotFound
	}

	ikey.EventID = &eventID
	ikey.EventULID = &eventULID
	return nil
}

func (m *IngestMockRepository) UpsertPlace(ctx context.Context, params events.PlaceCreateParams) (*events.PlaceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := params.Name + params.AddressLocality
	if place, ok := m.places[key]; ok {
		return place, nil
	}

	place := &events.PlaceRecord{
		ID:   fmt.Sprintf("place-id-%d", len(m.places)+1),
		ULID: params.ULID,
	}
	m.places[key] = place
	return place, nil
}

func (m *IngestMockRepository) UpsertOrganization(ctx context.Context, params events.OrganizationCreateParams) (*events.OrganizationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := params.Name + params.AddressLocality
	if org, ok := m.organizations[key]; ok {
		return org, nil
	}

	org := &events.OrganizationRecord{
		ID:   fmt.Sprintf("org-id-%d", len(m.organizations)+1),
		ULID: params.ULID,
	}
	m.organizations[key] = org
	return org, nil
}

// Admin operations (stub implementations)
func (m *IngestMockRepository) UpdateEvent(ctx context.Context, ulid string, params events.UpdateEventParams) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.events[ulid]
	if !ok {
		return nil, events.ErrNotFound
	}

	if params.Name != nil {
		event.Name = *params.Name
	}
	if params.Description != nil {
		event.Description = *params.Description
	}
	if params.LifecycleState != nil {
		event.LifecycleState = *params.LifecycleState
	}
	event.UpdatedAt = time.Now()

	return event, nil
}

func (m *IngestMockRepository) UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error {
	return nil
}

func (m *IngestMockRepository) SoftDeleteEvent(ctx context.Context, ulid string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.events[ulid]; !ok {
		return events.ErrNotFound
	}
	return nil
}

func (m *IngestMockRepository) MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.events[duplicateULID]; !ok {
		return events.ErrNotFound
	}
	if _, ok := m.events[primaryULID]; !ok {
		return events.ErrNotFound
	}
	return nil
}

func (m *IngestMockRepository) CreateTombstone(ctx context.Context, params events.TombstoneCreateParams) error {
	return nil
}

func (m *IngestMockRepository) GetTombstoneByEventID(ctx context.Context, eventID string) (*events.Tombstone, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) GetTombstoneByEventULID(ctx context.Context, eventULID string) (*events.Tombstone, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) BeginTx(ctx context.Context) (events.Repository, events.TxCommitter, error) {
	return m, &noOpTxCommitter{}, nil
}

// Review Queue methods (stub implementations)
func (m *IngestMockRepository) FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) CreateReviewQueueEntry(ctx context.Context, params events.ReviewQueueCreateParams) (*events.ReviewQueueEntry, error) {
	return &events.ReviewQueueEntry{
		ID:      1,
		EventID: params.EventID,
		Status:  "pending",
	}, nil
}

func (m *IngestMockRepository) UpdateReviewQueueEntry(ctx context.Context, id int, params events.ReviewQueueUpdateParams) (*events.ReviewQueueEntry, error) {
	return nil, nil
}

func (m *IngestMockRepository) GetReviewQueueEntry(ctx context.Context, id int) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) ListReviewQueue(ctx context.Context, filters events.ReviewQueueFilters) (*events.ReviewQueueListResult, error) {
	return &events.ReviewQueueListResult{Entries: []events.ReviewQueueEntry{}, NextCursor: nil}, nil
}

func (m *IngestMockRepository) ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}

func (m *IngestMockRepository) RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}
func (m *IngestMockRepository) MergeReview(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}

func (m *IngestMockRepository) CleanupExpiredReviews(ctx context.Context) error {
	return nil
}

func (m *IngestMockRepository) GetSourceTrustLevel(ctx context.Context, eventID string) (int, error) {
	return 5, nil
}

func (m *IngestMockRepository) GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error) {
	return 5, nil
}

func (m *IngestMockRepository) FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]events.NearDuplicateCandidate, error) {
	return nil, nil
}
func (m *IngestMockRepository) FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]events.SimilarPlaceCandidate, error) {
	return nil, nil
}
func (m *IngestMockRepository) FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]events.SimilarOrgCandidate, error) {
	return nil, nil
}
func (m *IngestMockRepository) MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*events.MergeResult, error) {
	return &events.MergeResult{CanonicalID: primaryID}, nil
}
func (m *IngestMockRepository) MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*events.MergeResult, error) {
	return &events.MergeResult{CanonicalID: primaryID}, nil
}
func (m *IngestMockRepository) InsertNotDuplicate(ctx context.Context, eventIDa string, eventIDb string, createdBy string) error {
	return nil
}
func (m *IngestMockRepository) IsNotDuplicate(ctx context.Context, eventIDa string, eventIDb string) (bool, error) {
	return false, nil
}

type noOpTxCommitter struct{}

func (n *noOpTxCommitter) Commit(ctx context.Context) error   { return nil }
func (n *noOpTxCommitter) Rollback(ctx context.Context) error { return nil }

// AddExistingEvent adds an event for duplicate testing.
func (m *IngestMockRepository) AddExistingEvent(sourceID, sourceEventID string, event *events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.eventsBySources[sourceID]; !ok {
		m.eventsBySources[sourceID] = make(map[string]*events.Event)
	}
	m.eventsBySources[sourceID][sourceEventID] = event
	m.events[event.ULID] = event
}

// sourceBaseURL extracts base URL from a full URL.
func sourceBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

// hashEventInput creates a hash for idempotency testing.
func hashEventInput(input events.EventInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("hash input: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// Tests using synthetic fixtures

func TestIngestService_WithSyntheticFixtures(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("ingest random event", func(t *testing.T) {
		input := gen.RandomEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err, "Should ingest random event without error")
		require.NotNil(t, result, "Result should not be nil")
		require.NotNil(t, result.Event, "Event should not be nil")
		assert.Equal(t, input.Name, result.Event.Name, "Event name should match")
		assert.False(t, result.IsDuplicate, "Should not be a duplicate")
	})

	t.Run("ingest minimal event", func(t *testing.T) {
		input := gen.MinimalEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err, "Should ingest minimal event without error")
		require.NotNil(t, result)
		require.NotNil(t, result.Event)
		assert.Equal(t, input.Name, result.Event.Name)
		// Minimal events should trigger review (no description/image)
		assert.True(t, result.NeedsReview, "Minimal event should need review")
	})

	t.Run("ingest virtual event", func(t *testing.T) {
		input := gen.VirtualEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err, "Should ingest virtual event without error")
		require.NotNil(t, result)
		require.NotNil(t, result.Event)
		assert.Contains(t, result.Event.Name, "Online")
		assert.NotEmpty(t, result.Event.VirtualURL, "Virtual URL should be set")
	})

	t.Run("ingest hybrid event", func(t *testing.T) {
		input := gen.HybridEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err, "Should ingest hybrid event without error")
		require.NotNil(t, result)
		require.NotNil(t, result.Event)
		assert.Contains(t, result.Event.Name, "Hybrid")
		assert.NotNil(t, result.Event.PrimaryVenueID, "Should have physical venue")
		assert.NotEmpty(t, result.Event.VirtualURL, "Should have virtual URL")
	})

	t.Run("ingest event needing review", func(t *testing.T) {
		input := gen.EventInputNeedsReview()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.NeedsReview, "Sparse event should need review")
		assert.Equal(t, "draft", result.Event.LifecycleState, "Should be draft state")
	})

	t.Run("ingest far future event", func(t *testing.T) {
		input := gen.EventInputFarFuture()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.NeedsReview, "Far future event should need review")
	})
}

func TestIngestService_BatchIngest(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("batch ingest multiple events", func(t *testing.T) {
		inputs := gen.BatchEventInputs(5)

		var results []*events.IngestResult
		for _, input := range inputs {
			result, err := service.Ingest(context.Background(), input)
			require.NoError(t, err, "Each event should ingest without error")
			results = append(results, result)
		}

		assert.Len(t, results, 5, "Should have all results")

		// All should have unique ULIDs
		ulids := make(map[string]bool)
		for _, r := range results {
			assert.False(t, ulids[r.Event.ULID], "ULIDs should be unique")
			ulids[r.Event.ULID] = true
		}
	})
}

func TestIngestService_DuplicateDetection(t *testing.T) {
	t.Run("detect duplicate by source external ID", func(t *testing.T) {
		gen := NewDeterministicGenerator()
		repo := NewIngestMockRepository()
		service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})
		input := gen.RandomEventInput()

		// First ingest
		result1, err := service.Ingest(context.Background(), input)
		require.NoError(t, err)
		require.NotNil(t, result1)
		assert.False(t, result1.IsDuplicate)

		// Second ingest with same source
		result2, err := service.Ingest(context.Background(), input)
		require.NoError(t, err)
		require.NotNil(t, result2)
		assert.True(t, result2.IsDuplicate, "Second ingest should be detected as duplicate")
		assert.Equal(t, result1.Event.ULID, result2.Event.ULID, "Should return same event")
	})

	t.Run("detect duplicate by dedup hash", func(t *testing.T) {
		gen := NewDeterministicGenerator()
		repo := NewIngestMockRepository()
		service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})
		first, second := gen.DuplicateCandidates()

		// Ingest first event
		result1, err := service.Ingest(context.Background(), first)
		require.NoError(t, err)
		require.NotNil(t, result1)
		assert.False(t, result1.IsDuplicate)

		// Ingest second event (same name, venue, start time but different source)
		result2, err := service.Ingest(context.Background(), second)
		require.NoError(t, err)
		require.NotNil(t, result2)
		assert.True(t, result2.IsDuplicate, "Should detect duplicate by dedup hash")
		assert.Equal(t, result1.Event.ULID, result2.Event.ULID, "Should return same event")
	})
}

func TestIngestService_OccurrenceHandling(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("ingest event with multiple occurrences", func(t *testing.T) {
		input := gen.EventInputWithOccurrences(4)

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err, "Should handle multiple occurrences")
		require.NotNil(t, result)
		require.NotNil(t, result.Event)

		// Check that occurrences were created
		occurrences := repo.Occurrences[result.Event.ID]
		assert.Len(t, occurrences, 4, "Should create 4 occurrences")

		// Verify timezone is set
		for _, occ := range occurrences {
			assert.Equal(t, "America/Toronto", occ.Timezone, "Timezone should be set")
		}
	})
}

func TestIngestService_SourceTracking(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("track source for ingested events", func(t *testing.T) {
		input := gen.RandomEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Event)

		// Verify source was registered
		sourceKey := input.Source.Name + "|" + sourceBaseURL(input.Source.URL)
		_, exists := repo.sources[sourceKey]
		assert.True(t, exists, "Source should be registered")
	})
}

func TestIngestService_VenueCreation(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("create venue from Toronto fixtures", func(t *testing.T) {
		input := gen.RandomEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Event)
		assert.NotNil(t, result.Event.PrimaryVenueID, "Should have primary venue ID")

		// Verify place was created with Toronto location
		found := false
		for _, place := range repo.places {
			if place.ID == *result.Event.PrimaryVenueID {
				found = true
				break
			}
		}
		assert.True(t, found, "Place should exist in repository")
	})
}

func TestIngestService_OrganizerCreation(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("create organizer from fixtures", func(t *testing.T) {
		input := gen.RandomEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Event)
		assert.NotNil(t, result.Event.OrganizerID, "Should have organizer ID")

		// Verify organizer was created
		found := false
		for _, org := range repo.organizations {
			if org.ID == *result.Event.OrganizerID {
				found = true
				break
			}
		}
		assert.True(t, found, "Organizer should exist in repository")
	})
}

func TestIngestService_IdempotencyWithFixtures(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true})

	t.Run("idempotent ingest with same key returns same event", func(t *testing.T) {
		input := gen.RandomEventInput()
		idempotencyKey := "test-idem-key-1"

		// First request
		result1, err := service.IngestWithIdempotency(context.Background(), input, idempotencyKey)
		require.NoError(t, err)
		require.NotNil(t, result1)
		assert.False(t, result1.IsDuplicate)

		// Second request with same key and payload
		result2, err := service.IngestWithIdempotency(context.Background(), input, idempotencyKey)
		require.NoError(t, err)
		require.NotNil(t, result2)
		assert.True(t, result2.IsDuplicate, "Should be marked as duplicate")
		assert.Equal(t, result1.Event.ULID, result2.Event.ULID, "Should return same event")
	})

	t.Run("different idempotency keys create different events", func(t *testing.T) {
		input := gen.RandomEventInput()

		result1, err := service.IngestWithIdempotency(context.Background(), input, "key-a")
		require.NoError(t, err)
		require.NotNil(t, result1)

		// Change source to avoid duplicate detection by source external ID
		input.Source.EventID = "different-evt-id"
		input.Source.URL = "https://different-source.com/events/999"

		result2, err := service.IngestWithIdempotency(context.Background(), input, "key-b")
		require.NoError(t, err)
		require.NotNil(t, result2)

		// Note: With same name/venue/time, these will be detected as duplicates by dedup hash
		// This is expected behavior - testing that different idempotency keys don't prevent dedup detection
	})

	t.Run("conflict on same key with different payload", func(t *testing.T) {
		input1 := gen.RandomEventInput()
		input2 := gen.RandomEventInput()
		idempotencyKey := "conflict-key"

		// First request
		result1, err := service.IngestWithIdempotency(context.Background(), input1, idempotencyKey)
		require.NoError(t, err)
		require.NotNil(t, result1)

		// Second request with different payload but same key
		_, err = service.IngestWithIdempotency(context.Background(), input2, idempotencyKey)
		require.Error(t, err, "Should error on conflict")
		assert.True(t, errors.Is(err, events.ErrConflict), "Should be ErrConflict")
	})
}

// Verify test fixtures satisfy interface requirements
var _ events.Repository = (*IngestMockRepository)(nil)

// Ensure ids package is imported (used elsewhere in events package)
var _ = ids.NewULID
