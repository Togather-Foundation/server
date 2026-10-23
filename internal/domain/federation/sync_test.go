package federation

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSyncRepo struct {
	occurrenceCreated bool
	occurrenceParams  *OccurrenceCreateParams
	idempotencyKeys   map[string]*IdempotencyKey
	events            map[string]Event // Track events by federation URI

	eventUpserts int
	placeUpserts int
	orgUpserts   int

	// node overrides the federation node returned by GetFederationNodeByDomain.
	// When nil, an active/bidirectional node with mockNodeID is returned.
	node *FederationNode

	// lastNodeDomain records the domain passed to GetFederationNodeByDomain so
	// tests can assert host normalization actually happened.
	lastNodeDomain string
}

// mockNodeID/mockNodeIDString identify the default federation node used by the
// sync mocks. CallerNodeID must match this value for authorized syncs.
var mockNodeID = pgtype.UUID{Valid: true, Bytes: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}}

const mockNodeIDString = "01010101-0101-0101-0101-010101010101"

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
	m.eventUpserts++
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
	m.lastNodeDomain = nodeDomain
	if m.node != nil {
		node := *m.node
		node.NodeDomain = nodeDomain
		return node, nil
	}
	return FederationNode{
		ID:               mockNodeID,
		NodeDomain:       nodeDomain,
		FederationStatus: FederationStatusActive,
		SyncEnabled:      true,
		SyncDirection:    SyncDirectionBidirectional,
		TrustLevel:       5,
	}, nil
}

func (m *mockSyncRepo) CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error {
	m.occurrenceCreated = true
	m.occurrenceParams = &params
	return nil
}

func (m *mockSyncRepo) UpsertPlace(ctx context.Context, params PlaceCreateParams) (*PlaceRecord, error) {
	m.placeUpserts++
	return &PlaceRecord{
		ID:   "00000000-0000-0000-0000-000000000011",
		ULID: params.ULID,
	}, nil
}

func (m *mockSyncRepo) UpsertOrganization(ctx context.Context, params OrganizationCreateParams) (*OrganizationRecord, error) {
	m.orgUpserts++
	return &OrganizationRecord{
		ID:   "00000000-0000-0000-0000-000000000012",
		ULID: params.ULID,
	}, nil
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
	service := NewSyncService(repo, nil, zerolog.Nop())

	payload := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "Event",
		"@id":       "https://example.org/events/123",
		"name":      "Test Event",
		"startDate": time.Now().Format(time.RFC3339),
		"url":       "https://example.org/event-page",
	}

	result, err := service.SyncEvent(context.Background(), SyncEventParams{
		Payload:      payload,
		CallerNodeID: mockNodeIDString,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.EventULID)
	assert.True(t, repo.occurrenceCreated, "occurrence should have been created")
	assert.NotNil(t, repo.occurrenceParams)
	assert.NotNil(t, repo.occurrenceParams.VirtualURL)
	assert.Equal(t, "https://example.org/event-page", *repo.occurrenceParams.VirtualURL)
}

func TestSyncEvent_RejectsInvalidIDScheme(t *testing.T) {
	repo := &mockSyncRepo{}
	service := NewSyncService(repo, nil, zerolog.Nop())

	payload := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "Event",
		"@id":       "ftp://example.org/events/123",
		"name":      "Test Event",
		"startDate": time.Now().Format(time.RFC3339),
	}

	_, err := service.SyncEvent(context.Background(), SyncEventParams{Payload: payload, CallerNodeID: mockNodeIDString})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingID)
}

func TestSyncEvent_WithoutURL_UsesFederationURI(t *testing.T) {
	repo := &mockSyncRepo{}
	service := NewSyncService(repo, nil, zerolog.Nop())

	payload := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "Event",
		"@id":       "https://example.org/events/123",
		"name":      "Test Event",
		"startDate": time.Now().Format(time.RFC3339),
		// No url field
	}

	result, err := service.SyncEvent(context.Background(), SyncEventParams{
		Payload:      payload,
		CallerNodeID: mockNodeIDString,
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
		service := NewSyncService(repo, nil, zerolog.Nop())

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
			CallerNodeID:   mockNodeIDString,
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
		service := NewSyncService(repo, nil, zerolog.Nop())

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
			CallerNodeID:   mockNodeIDString,
		})
		require.NoError(t, err)
		ulid1 := result1.EventULID
		assert.False(t, result1.IsDuplicate, "first request should not be duplicate")

		// Second request with same key and payload should return existing event
		result2, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:        payload,
			IdempotencyKey: "test-key-456",
			CallerNodeID:   mockNodeIDString,
		})

		require.NoError(t, err)
		assert.Equal(t, ulid1, result2.EventULID, "should return same event ULID")
		assert.True(t, result2.IsDuplicate, "should be marked as duplicate")
	})

	t.Run("different payload with same key returns error", func(t *testing.T) {
		repo := &mockSyncRepo{
			idempotencyKeys: make(map[string]*IdempotencyKey),
		}
		service := NewSyncService(repo, nil, zerolog.Nop())

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
			CallerNodeID:   mockNodeIDString,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "idempotency key conflict")
	})
}

func TestComputePayloadHash_Deterministic(t *testing.T) {
	t.Run("same payload produces same hash", func(t *testing.T) {
		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/events/123",
			"name":      "Test Event",
			"startDate": "2026-07-10T19:00:00Z",
		}

		hash1, err := computePayloadHash(payload)
		require.NoError(t, err)

		hash2, err := computePayloadHash(payload)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "same payload should produce identical hash")
	})

	t.Run("map insertion order does not affect hash", func(t *testing.T) {
		// Create two payloads with identical content but different construction order
		payload1 := map[string]any{
			"name":      "Test Event",
			"@context":  "https://schema.org",
			"startDate": "2026-07-10T19:00:00Z",
			"@type":     "Event",
			"@id":       "https://example.org/events/123",
		}

		payload2 := map[string]any{
			"@id":       "https://example.org/events/123",
			"@type":     "Event",
			"name":      "Test Event",
			"startDate": "2026-07-10T19:00:00Z",
			"@context":  "https://schema.org",
		}

		hash1, err := computePayloadHash(payload1)
		require.NoError(t, err)

		hash2, err := computePayloadHash(payload2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "map insertion order should not affect hash")
	})

	t.Run("nested maps are normalized", func(t *testing.T) {
		payload1 := map[string]any{
			"@type": "Event",
			"name":  "Test Event",
			"location": map[string]any{
				"name":            "Venue",
				"addressLocality": "Toronto",
				"@type":           "Place",
			},
		}

		payload2 := map[string]any{
			"name":  "Test Event",
			"@type": "Event",
			"location": map[string]any{
				"@type":           "Place",
				"name":            "Venue",
				"addressLocality": "Toronto",
			},
		}

		hash1, err := computePayloadHash(payload1)
		require.NoError(t, err)

		hash2, err := computePayloadHash(payload2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "nested map order should not affect hash")
	})

	t.Run("arrays maintain order", func(t *testing.T) {
		payload1 := map[string]any{
			"@type": "Event",
			"tags":  []any{"music", "concert", "rock"},
		}

		payload2 := map[string]any{
			"@type": "Event",
			"tags":  []any{"rock", "music", "concert"}, // Different order
		}

		hash1, err := computePayloadHash(payload1)
		require.NoError(t, err)

		hash2, err := computePayloadHash(payload2)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2, "array element order should affect hash")
	})

	t.Run("deep nested structures are normalized", func(t *testing.T) {
		payload1 := map[string]any{
			"@type": "Event",
			"organizer": map[string]any{
				"name": "Org Name",
				"address": map[string]any{
					"city":    "Toronto",
					"country": "Canada",
					"street":  "123 Main St",
				},
			},
		}

		payload2 := map[string]any{
			"organizer": map[string]any{
				"address": map[string]any{
					"street":  "123 Main St",
					"city":    "Toronto",
					"country": "Canada",
				},
				"name": "Org Name",
			},
			"@type": "Event",
		}

		hash1, err := computePayloadHash(payload1)
		require.NoError(t, err)

		hash2, err := computePayloadHash(payload2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "deep nested structures should normalize correctly")
	})

	t.Run("different payloads produce different hashes", func(t *testing.T) {
		payload1 := map[string]any{
			"@type": "Event",
			"name":  "Event A",
		}

		payload2 := map[string]any{
			"@type": "Event",
			"name":  "Event B",
		}

		hash1, err := computePayloadHash(payload1)
		require.NoError(t, err)

		hash2, err := computePayloadHash(payload2)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2, "different payloads should produce different hashes")
	})

	t.Run("multiple iterations produce consistent hash", func(t *testing.T) {
		// Create payload and compute hash 100 times to catch any non-determinism
		payload := map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       "https://example.org/events/123",
			"name":      "Test Event",
			"startDate": "2026-07-10T19:00:00Z",
			"location": map[string]any{
				"@type":           "Place",
				"name":            "Test Venue",
				"addressLocality": "Toronto",
			},
		}

		var firstHash string
		for i := 0; i < 100; i++ {
			hash, err := computePayloadHash(payload)
			require.NoError(t, err)

			if i == 0 {
				firstHash = hash
			} else {
				assert.Equal(t, firstHash, hash, "hash should be identical on iteration %d", i)
			}
		}
	})
}

func TestSyncEvent_Authorization(t *testing.T) {
	const otherNodeIDString = "02020202-0202-0202-0202-020202020202"

	basePayload := func(eventURI string) map[string]any {
		return map[string]any{
			"@context":  "https://schema.org",
			"@type":     "Event",
			"@id":       eventURI,
			"name":      "Authorization Test Event",
			"startDate": time.Now().Format(time.RFC3339),
		}
	}

	t.Run("unbound key is rejected before any write", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload: basePayload("https://example.org/events/unbound"),
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFederationNodeNotBound)
		assert.Zero(t, repo.eventUpserts, "rejected sync must not write events")
		assert.Zero(t, repo.placeUpserts, "rejected sync must not write places")
		assert.Zero(t, repo.orgUpserts, "rejected sync must not write organizations")
	})

	t.Run("malformed caller node id is rejected as unbound", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      basePayload("https://example.org/events/malformed"),
			CallerNodeID: "not-a-uuid",
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFederationNodeNotBound)
		assert.Zero(t, repo.eventUpserts)
	})

	t.Run("key bound to a different node is rejected", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      basePayload("https://example.org/events/mismatch"),
			CallerNodeID: otherNodeIDString,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFederationNodeMismatch)
		assert.Zero(t, repo.eventUpserts, "rejected sync must not write events")
		assert.Zero(t, repo.placeUpserts)
		assert.Zero(t, repo.orgUpserts)
	})

	inactiveCases := []struct {
		name string
		node FederationNode
	}{
		{
			name: "blocked node",
			node: FederationNode{ID: mockNodeID, FederationStatus: FederationStatusBlocked, SyncEnabled: true, SyncDirection: SyncDirectionBidirectional},
		},
		{
			name: "paused node",
			node: FederationNode{ID: mockNodeID, FederationStatus: FederationStatusPaused, SyncEnabled: true, SyncDirection: SyncDirectionBidirectional},
		},
		{
			name: "pending node",
			node: FederationNode{ID: mockNodeID, FederationStatus: FederationStatusPending, SyncEnabled: true, SyncDirection: SyncDirectionBidirectional},
		},
		{
			name: "sync disabled",
			node: FederationNode{ID: mockNodeID, FederationStatus: FederationStatusActive, SyncEnabled: false, SyncDirection: SyncDirectionBidirectional},
		},
		{
			name: "push only direction",
			node: FederationNode{ID: mockNodeID, FederationStatus: FederationStatusActive, SyncEnabled: true, SyncDirection: SyncDirectionPushOnly},
		},
		{
			name: "disabled direction",
			node: FederationNode{ID: mockNodeID, FederationStatus: FederationStatusActive, SyncEnabled: true, SyncDirection: SyncDirectionDisabled},
		},
	}

	for _, tc := range inactiveCases {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			node := tc.node
			repo := &mockSyncRepo{node: &node}
			service := NewSyncService(repo, nil, zerolog.Nop())

			_, err := service.SyncEvent(context.Background(), SyncEventParams{
				Payload:      basePayload("https://example.org/events/inactive"),
				CallerNodeID: mockNodeIDString,
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrFederationNodeInactive)
			assert.Zero(t, repo.eventUpserts, "rejected sync must not write events")
			assert.Zero(t, repo.placeUpserts)
			assert.Zero(t, repo.orgUpserts)
		})
	}

	t.Run("pull only direction is accepted", func(t *testing.T) {
		repo := &mockSyncRepo{node: &FederationNode{
			ID:               mockNodeID,
			FederationStatus: FederationStatusActive,
			SyncEnabled:      true,
			SyncDirection:    SyncDirectionPullOnly,
		}}
		service := NewSyncService(repo, nil, zerolog.Nop())

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      basePayload("https://example.org/events/pull-only"),
			CallerNodeID: mockNodeIDString,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
	})

	t.Run("foreign nested place uri is rejected before any write", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		payload := basePayload("https://example.org/events/foreign-place")
		payload["location"] = map[string]any{
			"@type": "Place",
			"@id":   "https://other-node.example.org/places/123",
			"name":  "Foreign Venue",
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      payload,
			CallerNodeID: mockNodeIDString,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrForeignEntityURI)
		assert.Zero(t, repo.eventUpserts, "rejected sync must not write events")
		assert.Zero(t, repo.placeUpserts, "rejected sync must not write places")
		assert.Zero(t, repo.orgUpserts)
	})

	t.Run("foreign nested organizer uri is rejected before any write", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		payload := basePayload("https://example.org/events/foreign-org")
		payload["organizer"] = map[string]any{
			"@type": "Organization",
			"@id":   "https://other-node.example.org/organizations/456",
			"name":  "Foreign Org",
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      payload,
			CallerNodeID: mockNodeIDString,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrForeignEntityURI)
		assert.Zero(t, repo.eventUpserts, "rejected sync must not write events")
		assert.Zero(t, repo.orgUpserts, "rejected sync must not write organizations")
	})

	t.Run("uppercase origin host is normalized", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      basePayload("https://EXAMPLE.ORG/events/uppercase-host"),
			CallerNodeID: mockNodeIDString,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
		assert.Equal(t, "example.org", repo.lastNodeDomain,
			"uppercase origin host must be normalized before the node lookup")
	})

	t.Run("origin host with explicit port is normalized", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      basePayload("https://example.org:443/events/explicit-port"),
			CallerNodeID: mockNodeIDString,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
		assert.Equal(t, "example.org", repo.lastNodeDomain,
			"the port must be stripped before the node lookup")
	})

	t.Run("origin host with trailing dot is normalized", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      basePayload("https://example.org./events/trailing-dot"),
			CallerNodeID: mockNodeIDString,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
		assert.Equal(t, "example.org", repo.lastNodeDomain,
			"a trailing dot must be stripped before the node lookup")
	})

	t.Run("uppercase nested @id with port is normalized", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		payload := basePayload("https://example.org/events/uppercase-nested")
		payload["location"] = map[string]any{
			"@type": "Place",
			"@id":   "https://EXAMPLE.ORG:443/places/123",
			"name":  "Local Venue",
		}
		payload["organizer"] = map[string]any{
			"@type": "Organization",
			"@id":   "https://EXAMPLE.ORG.:443/organizations/456", // trailing dot + port
			"name":  "Local Org",
		}

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      payload,
			CallerNodeID: mockNodeIDString,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
		assert.Equal(t, 1, repo.placeUpserts)
		assert.Equal(t, 1, repo.orgUpserts)
	})

	t.Run("nested @id on a genuinely different host is rejected", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		payload := basePayload("https://example.org/events/different-host")
		payload["location"] = map[string]any{
			"@type": "Place",
			"@id":   "https://EXAMPLE.NET/places/123",
			"name":  "Foreign Venue",
		}

		_, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      payload,
			CallerNodeID: mockNodeIDString,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrForeignEntityURI)
		assert.Zero(t, repo.placeUpserts)
	})

	t.Run("nested uri on the bound node is accepted", func(t *testing.T) {
		repo := &mockSyncRepo{}
		service := NewSyncService(repo, nil, zerolog.Nop())

		payload := basePayload("https://example.org/events/local-nested")
		payload["location"] = map[string]any{
			"@type": "Place",
			"@id":   "https://EXAMPLE.org/places/123", // case-insensitive match
			"name":  "Local Venue",
		}
		payload["organizer"] = map[string]any{
			"@type": "Organization",
			"@id":   "https://example.org:443/organizations/456", // port stripped
			"name":  "Local Org",
		}

		result, err := service.SyncEvent(context.Background(), SyncEventParams{
			Payload:      payload,
			CallerNodeID: mockNodeIDString,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.EventULID)
		assert.Equal(t, 1, repo.eventUpserts)
		assert.Equal(t, 1, repo.placeUpserts)
		assert.Equal(t, 1, repo.orgUpserts)
	})
}
