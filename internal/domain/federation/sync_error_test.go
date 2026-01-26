package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test error scenarios for federation sync

func TestSyncEvent_ErrorPaths(t *testing.T) {
	t.Run("missing required fields", func(t *testing.T) {
		tests := []struct {
			name    string
			payload map[string]any
			wantErr error
		}{
			{
				name:    "missing @context",
				payload: map[string]any{"@type": "Event", "@id": "https://example.org/e/1", "name": "Test"},
				wantErr: ErrInvalidJSONLD,
			},
			{
				name:    "missing @id",
				payload: map[string]any{"@context": "https://schema.org", "@type": "Event", "name": "Test"},
				wantErr: ErrMissingID,
			},
			{
				name:    "missing @type",
				payload: map[string]any{"@context": "https://schema.org", "@id": "https://example.org/e/1", "name": "Test"},
				wantErr: ErrMissingType,
			},
			{
				name:    "missing name",
				payload: map[string]any{"@context": "https://schema.org", "@type": "Event", "@id": "https://example.org/e/1", "startDate": time.Now().Format(time.RFC3339)},
				wantErr: ErrMissingRequiredField,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo := &mockSyncRepo{}
				service := NewSyncService(repo)

				_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: tt.payload})
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}
	})

	t.Run("unsupported @type", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context": "https://schema.org",
			"@type":    "Organization", // Not Event
			"@id":      "https://example.org/org/1",
			"name":     "Test Org",
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedType)
	})

	t.Run("empty @id", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context": "https://schema.org",
			"@type":    "Event",
			"@id":      "", // Empty string
			"name":     "Test Event",
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingID)
	})

	t.Run("invalid date format", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/e/1",
			"name":      "Test Event",
			"startDate": "not-a-date",
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidDateFormat)
	})

	t.Run("missing federation node", func(t *testing.T) {
		repo := &mockSyncRepoWithErrors{
			federationNodeErr: errors.New("federation node not found"),
		}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://unknown-node.org/e/1",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to determine origin node")
	})

	t.Run("upsert fails", func(t *testing.T) {
		repo := &mockSyncRepoWithErrors{
			upsertErr: errors.New("database error"),
		}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/e/1",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to upsert event")
	})

	t.Run("occurrence creation fails", func(t *testing.T) {
		repo := &mockSyncRepoWithErrors{
			occurrenceErr: errors.New("occurrence error"),
		}
		service := NewSyncService(repo)

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/e/1",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create occurrence")
	})

	t.Run("context cancelled before processing", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/e/1",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		_, err := service.SyncEvent(ctx, SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("context timeout during processing", func(t *testing.T) {
		repo := &slowMockSyncRepo{delay: 100 * time.Millisecond}
		service := NewSyncService(repo)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/e/1",
			"name":      "Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}

		_, err := service.SyncEvent(ctx, SyncEventParams{Payload: payload})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// Mock repository that returns errors
type mockSyncRepoWithErrors struct {
	federationNodeErr error
	upsertErr         error
	occurrenceErr     error
}

func (m *mockSyncRepoWithErrors) GetEventByFederationURI(ctx context.Context, federationUri string) (Event, error) {
	return Event{}, errors.New("not found")
}

func (m *mockSyncRepoWithErrors) UpsertFederatedEvent(ctx context.Context, arg UpsertFederatedEventParams) (Event, error) {
	if m.upsertErr != nil {
		return Event{}, m.upsertErr
	}
	return Event{
		ID:   pgtype.UUID{Valid: true, Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		ULID: arg.Ulid,
		Name: arg.Name,
	}, nil
}

func (m *mockSyncRepoWithErrors) GetFederationNodeByDomain(ctx context.Context, nodeDomain string) (FederationNode, error) {
	if m.federationNodeErr != nil {
		return FederationNode{}, m.federationNodeErr
	}
	return FederationNode{
		ID:         pgtype.UUID{Valid: true, Bytes: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
		NodeDomain: nodeDomain,
	}, nil
}

func (m *mockSyncRepoWithErrors) CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error {
	if m.occurrenceErr != nil {
		return m.occurrenceErr
	}
	return nil
}

func (m *mockSyncRepoWithErrors) WithTransaction(ctx context.Context, fn func(txRepo SyncRepository) error) error {
	return fn(m)
}

// Mock repository with artificial delay for timeout testing
type slowMockSyncRepo struct {
	delay time.Duration
}

func (m *slowMockSyncRepo) GetEventByFederationURI(ctx context.Context, federationUri string) (Event, error) {
	time.Sleep(m.delay)
	return Event{}, errors.New("not found")
}

func (m *slowMockSyncRepo) UpsertFederatedEvent(ctx context.Context, arg UpsertFederatedEventParams) (Event, error) {
	time.Sleep(m.delay)
	return Event{
		ID:   pgtype.UUID{Valid: true, Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		ULID: arg.Ulid,
		Name: arg.Name,
	}, nil
}

func (m *slowMockSyncRepo) GetFederationNodeByDomain(ctx context.Context, nodeDomain string) (FederationNode, error) {
	time.Sleep(m.delay)
	return FederationNode{
		ID:         pgtype.UUID{Valid: true, Bytes: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
		NodeDomain: nodeDomain,
	}, nil
}

func (m *slowMockSyncRepo) CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error {
	time.Sleep(m.delay)
	return nil
}

func (m *slowMockSyncRepo) WithTransaction(ctx context.Context, fn func(txRepo SyncRepository) error) error {
	return fn(m)
}
