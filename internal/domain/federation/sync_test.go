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
}

func (m *mockSyncRepo) GetEventByFederationURI(ctx context.Context, federationUri string) (Event, error) {
	// Simulate not found
	return Event{}, pgtype.ErrScanTargetTypeChanged
}

func (m *mockSyncRepo) UpsertFederatedEvent(ctx context.Context, arg UpsertFederatedEventParams) (Event, error) {
	return Event{
		ID:   pgtype.UUID{Valid: true, Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		ULID: arg.Ulid,
		Name: arg.Name,
	}, nil
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
