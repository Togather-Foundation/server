package federation

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSyncRepo struct {
	occurrenceCreated bool
	occurrenceParams  *OccurrenceCreateParams
	idempotencyKeys   map[string]*IdempotencyKey
	events            map[string]Event // Track events by federation URI
}

func (m *mockSyncRepo) GetEventByFederationURI(ctx context.Context, federationUri string) (Event, error) {
	if m.events != nil {
		if event, ok := m.events[federationUri]; ok {
			return event, nil
		}
	}
	// Simulate not found
	return Event{}, pgtype.ErrScanTargetTypeChanged
}

func (m *mockSyncRepo) UpsertFederatedEvent(ctx context.Context, arg UpsertFederatedEventParams) (Event, error) {
	event := Event{
		ID:            pgtype.UUID{Valid: true, Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		ULID:          arg.Ulid,
		Name:          arg.Name,
		FederationURI: arg.FederationUri,
	}

	// Store event for future lookups
	if m.events == nil {
		m.events = make(map[string]Event)
	}
	if arg.FederationUri.Valid {
		m.events[arg.FederationUri.String] = event
	}

	return event, nil
}

func (m *mockSyncRepo) GetFederationNodeByDomain(ctx context.Context, nodeDomain string) (FederationNode, error) {
	return FederationNode{
		ID:         pgtype.UUID{Valid: true, Bytes: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
		NodeDomain: nodeDomain,
	}, nil
}

func (m *mockSyncRepo) CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error {
	m.occurrenceCreated = true
	m.occurrenceParams = &params
	return nil
}

func (m *mockSyncRepo) WithTransaction(ctx context.Context, fn func(txRepo SyncRepository) error) error {
	// For mock, just execute the function with the same repo (no real transaction)
	return fn(m)
}

func (m *mockSyncRepo) GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyKey, error) {
	if m.idempotencyKeys == nil {
		return nil, nil
	}
	return m.idempotencyKeys[key], nil
}

func (m *mockSyncRepo) InsertIdempotencyKey(ctx context.Context, params IdempotencyKeyParams) error {
	if m.idempotencyKeys == nil {
		m.idempotencyKeys = make(map[string]*IdempotencyKey)
	}
	m.idempotencyKeys[params.Key] = &IdempotencyKey{
		Key:         params.Key,
		RequestHash: params.RequestHash,
		EventULID:   &params.EventULID,
		CreatedAt:   time.Now(),
	}
	return nil
}

func TestSyncEvent_CreatesOccurrence(t *testing.T) {
	repo := &mockSyncRepo{}
	service := NewSyncService(repo)

	payload := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "Event",
		"@id":       "https://example.org/events/123",
		"name":      "Test Event",
		"startDate": time.Now().Format(time.RFC3339),
		"url":       "https://example.org/event-page",
	}

	result, err := service.SyncEvent(context.Background(), SyncEventParams{
		Payload: payload,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.EventULID)
	assert.True(t, repo.occurrenceCreated, "occurrence should have been created")
	assert.NotNil(t, repo.occurrenceParams)
	assert.NotNil(t, repo.occurrenceParams.VirtualURL)
	assert.Equal(t, "https://example.org/event-page", *repo.occurrenceParams.VirtualURL)
}

func TestSyncEvent_WithoutURL_UsesFederationURI(t *testing.T) {
	repo := &mockSyncRepo{}
	service := NewSyncService(repo)

	payload := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "Event",
		"@id":       "https://example.org/events/123",
		"name":      "Test Event",
		"startDate": time.Now().Format(time.RFC3339),
		// No url field
	}

	result, err := service.SyncEvent(context.Background(), SyncEventParams{
		Payload: payload,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.EventULID)
	assert.True(t, repo.occurrenceCreated, "occurrence should have been created")
	assert.NotNil(t, repo.occurrenceParams)
	assert.NotNil(t, repo.occurrenceParams.VirtualURL)
	assert.Equal(t, "https://example.org/events/123", *repo.occurrenceParams.VirtualURL, "should use federation URI as fallback")
}

func TestSyncEvent_IdempotencyKey(t *testing.T) {
	t.Run("first request with idempotency key stores it", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/events/123",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:        payload,
			IdempotencyKey: "test-key-123",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
		assert.False(t, result.IsDuplicate)
		assert.NotNil(t, repo.idempotencyKeys["test-key-123"])
	})

	t.Run("duplicate request with same payload returns existing event", func(t *testing.T) {
		repo := &mockSyncRepo{
			idempotencyKeys: make(map[string]*IdempotencyKey),
		}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/events/123",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		// First request
		result1, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:        payload,
			IdempotencyKey: "test-key-456",
		})
		require.NoError(t, err)
		ulid1 := result1.EventULID
		assert.False(t, result1.IsDuplicate, "first request should not be duplicate")

		// Second request with same key and payload should return existing event
		result2, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:        payload,
			IdempotencyKey: "test-key-456",
		})

		require.NoError(t, err)
		assert.Equal(t, ulid1, result2.EventULID, "should return same event ULID")
		assert.True(t, result2.IsDuplicate, "should be marked as duplicate")
	})

	t.Run("different payload with same key returns error", func(t *testing.T) {
		repo := &mockSyncRepo{
			idempotencyKeys: make(map[string]*IdempotencyKey),
		}
		service := NewSyncService(repo)

		payload1 := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/events/123",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		// Pre-populate with different hash
		ulid := "01HYX3KQW7ERTV9XNBM2P8QJZF"
		repo.idempotencyKeys["test-key-789"] = &IdempotencyKey{
			Key:         "test-key-789",
			RequestHash: "different-hash",
			EventULID:   &ulid,
			CreatedAt:   time.Now(),
		}

		// Try with different payload
		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:        payload1,
			IdempotencyKey: "test-key-789",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "idempotency key conflict")
	})
}
