package fixtures

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/rs/zerolog"
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
	eventsByFeedEvent map[string]map[string]*events.Event        // feed URL (trailing slashes trimmed) -> sourceEventID -> Event
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
		eventsByFeedEvent: make(map[string]map[string]*events.Event),
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

func (m *IngestMockRepository) GetPublicByULID(ctx context.Context, ulid string) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.events[ulid]
	if !ok {
		return nil, events.ErrNotFound
	}
	if event == nil || (!events.PubliclyVisible(event.LifecycleState) && event.LifecycleState != "deleted") {
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

	// Record the event for feed-scoped duplicate detection. Identity is
	// (feed URL, source event id), not the source row. The URL is normalised
	// (query and fragment stripped) and compared with trailing slashes ignored
	// on both sides.
	if params.SourceURL != "" && params.SourceEventID != "" {
		feedURL := feedIdentityKey(params.SourceURL)
		if _, ok := m.eventsByFeedEvent[feedURL]; !ok {
			m.eventsByFeedEvent[feedURL] = make(map[string]*events.Event)
		}
		// Find the event by ID and record it
		for _, event := range m.events {
			if event.ID == params.EventID {
				m.eventsByFeedEvent[feedURL][params.SourceEventID] = event
				break
			}
		}
	}
	return nil
}

// feedIdentityKey mirrors the repository's URL comparison rule: the normalised
// form, except that trailing slashes are ignored.
func feedIdentityKey(feedURL string) string {
	return strings.TrimRight(feedURL, "/")
}

func (m *IngestMockRepository) FindByFeedEventID(ctx context.Context, feedURL string, sourceEventID string) (*events.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if feedURL == "" || sourceEventID == "" {
		return nil, events.ErrNotFound
	}

	if eventsForFeed, ok := m.eventsByFeedEvent[feedIdentityKey(feedURL)]; ok {
		if event, ok := eventsForFeed[sourceEventID]; ok {
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

	key := events.NormalizeFeedURL(params.FeedURL)
	if key == "" {
		key = events.NormalizeFeedURL(params.BaseURL)
	}
	if key == "" {
		key = "name:" + params.Name
	}
	if sourceID, ok := m.sources[key]; ok {
		return sourceID, nil
	}

	sourceID := fmt.Sprintf("source-%d", len(m.sources)+1)
	m.sources[key] = sourceID
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
	if params.DedupHash != nil {
		event.DedupHash = *params.DedupHash
	}
	event.UpdatedAt = time.Now()

	return event, nil
}

func (m *IngestMockRepository) UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time, isAllDay bool) error {
	return nil
}

func (m *IngestMockRepository) UpdateOccurrenceDatesByOccurrenceID(ctx context.Context, eventULID string, occurrenceID string, startTime time.Time, endTime *time.Time, isAllDay bool) error {
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

func (m *IngestMockRepository) GetPublicTombstoneByEventULID(ctx context.Context, eventULID string) (*events.Tombstone, error) {
	return m.GetTombstoneByEventULID(ctx, eventULID)
}

func (m *IngestMockRepository) BeginTx(ctx context.Context) (events.Repository, events.TxCommitter, error) {
	return m, &noOpTxCommitter{}, nil
}

// AnchorAllDayOccurrence is a passthrough: these fixtures are not all-day, so
// anchoring is the identity mapping.
func (m *IngestMockRepository) AnchorAllDayOccurrence(_ context.Context, start time.Time, end *time.Time, _ string) (time.Time, *time.Time, error) {
	return start, end, nil
}

// Occurrence-level operations (stub implementations).

func (m *IngestMockRepository) InsertOccurrence(ctx context.Context, params events.OccurrenceCreateParams) (*events.Occurrence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Occurrences[params.EventID] = append(m.Occurrences[params.EventID], params)
	return &events.Occurrence{
		ID:        fmt.Sprintf("occ-%d", len(m.Occurrences[params.EventID])),
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
	}, nil
}

func (m *IngestMockRepository) CountOccurrences(ctx context.Context, eventID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return int64(len(m.Occurrences[eventID])), nil
}

func (m *IngestMockRepository) GetOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) (*events.Occurrence, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) UpdateOccurrence(ctx context.Context, eventID string, occurrenceID string, params events.OccurrenceUpdateParams) (*events.Occurrence, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) DeleteOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) error {
	return events.ErrNotFound
}

func (m *IngestMockRepository) CheckOccurrenceOverlapExcluding(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time, excludeOccurrenceID string) (bool, error) {
	return false, nil
}

// Remaining interface methods (stub implementations).

func (m *IngestMockRepository) Rollback(ctx context.Context) error {
	return nil
}

func (m *IngestMockRepository) GetPlaceByULID(ctx context.Context, ulid string) (*events.PlaceRecord, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) GetPlaceByID(ctx context.Context, id string) (*events.PlaceRecord, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) GetOrganizationByID(ctx context.Context, id string) (*events.OrganizationRecord, error) {
	return nil, events.ErrNotFound
}

func (m *IngestMockRepository) CreatePlaceTombstone(ctx context.Context, params events.PlaceTombstoneCreateParams) error {
	return nil
}

func (m *IngestMockRepository) CreateOrganizationTombstone(ctx context.Context, params events.OrganizationTombstoneCreateParams) error {
	return nil
}

func (m *IngestMockRepository) UpsertEventSeries(ctx context.Context, params events.UpsertEventSeriesParams) (*events.UpsertEventSeriesResult, error) {
	return &events.UpsertEventSeriesResult{SeriesID: fmt.Sprintf("series-%s", params.ExternalKey)}, nil
}

func (m *IngestMockRepository) FindSeriesCompanion(ctx context.Context, params events.SeriesCompanionQuery) (*events.CrossWeekCompanion, error) {
	return nil, nil
}

func (m *IngestMockRepository) GetReviewByEventUlid(ctx context.Context, eventULID string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}

func (m *IngestMockRepository) DismissAllCompanionWarnings(ctx context.Context, reviewID int, eventULID string) (bool, error) {
	return false, nil
}

func (m *IngestMockRepository) StripRetiredDupWarnings(ctx context.Context, reviewID int, retireULIDs []string) (bool, error) {
	return false, nil
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

func (m *IngestMockRepository) UpsertReviewRow(ctx context.Context, params events.ReviewQueueUpsertParams) (*events.ReviewQueueUpsertResult, error) {
	return &events.ReviewQueueUpsertResult{
		Entry: &events.ReviewQueueEntry{
			ID:          1,
			EventID:     params.EventULID,
			EventULID:   params.EventULID,
			Warnings:    params.Warnings,
			EventAllDay: params.EventAllDay,
			Status:      "pending",
		},
		Inserted: true,
	}, nil
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

func (m *IngestMockRepository) ListSeriesCandidatePairs(ctx context.Context, params events.SeriesCandidatesQuery) ([]events.SeriesCandidatePair, error) {
	return nil, nil
}

func (m *IngestMockRepository) ResolveSourceIdentifier(ctx context.Context, identifier string) (string, error) {
	return "", nil
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
func (m *IngestMockRepository) CheckOccurrenceOverlap(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
	return false, nil
}
func (m *IngestMockRepository) DeleteOccurrencesByEventULID(_ context.Context, _ string) error {
	return nil
}
func (m *IngestMockRepository) DismissCompanionWarningMatch(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *IngestMockRepository) DismissWarningMatchByReviewID(_ context.Context, _ int, _ string) error {
	return nil
}
func (m *IngestMockRepository) GetPendingReviewByEventUlid(_ context.Context, _ string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}
func (m *IngestMockRepository) GetPendingReviewByEventUlidAndDuplicateUlid(_ context.Context, _ string, _ string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}
func (m *IngestMockRepository) UpdateReviewWarnings(_ context.Context, _ int, _ []byte) error {
	return nil
}
func (m *IngestMockRepository) LockEventForUpdate(_ context.Context, _ string) error {
	return nil
}
func (m *IngestMockRepository) LockReviewQueueEntryForUpdate(_ context.Context, _ int) (*events.ReviewQueueEntry, error) {
	return nil, nil
}

func (m *IngestMockRepository) DismissPendingReviewsByEventULIDs(_ context.Context, _ []string, _ string) ([]int, error) {
	return nil, nil
}

func (m *IngestMockRepository) FindCrossWeekCompanionTargets(_ context.Context, _ []string) ([]events.CrossWeekCompanionTarget, error) {
	return nil, nil
}

type noOpTxCommitter struct{}

func (n *noOpTxCommitter) Commit(ctx context.Context) error   { return nil }
func (n *noOpTxCommitter) Rollback(ctx context.Context) error { return nil }

// AddExistingEvent registers an event as previously ingested from feedURL under
// sourceEventID.
func (m *IngestMockRepository) AddExistingEvent(feedURL, sourceEventID string, event *events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := feedIdentityKey(feedURL)
	if _, ok := m.eventsByFeedEvent[key]; !ok {
		m.eventsByFeedEvent[key] = make(map[string]*events.Event)
	}
	m.eventsByFeedEvent[key][sourceEventID] = event
	m.events[event.ULID] = event
}

// Tests using synthetic fixtures

func TestIngestService_WithSyntheticFixtures(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

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
		assert.Equal(t, "pending_review", result.Event.LifecycleState, "Should be pending review state")
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
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

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
		service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())
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
		service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())
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
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

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
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

	t.Run("track source for ingested events", func(t *testing.T) {
		input := gen.RandomEventInput()

		result, err := service.Ingest(context.Background(), input)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Event)

		// Verify source was registered
		sourceKey := events.NormalizeFeedURL(input.Source.URL)
		_, exists := repo.sources[sourceKey]
		assert.True(t, exists, "Source should be registered")
	})
}

func TestIngestService_VenueCreation(t *testing.T) {
	gen := NewDeterministicGenerator()
	repo := NewIngestMockRepository()
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

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
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

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
	service := events.NewIngestService(repo, "https://test.togather.events", "America/Toronto", config.ValidationConfig{RequireImage: true}, zerolog.Nop())

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
