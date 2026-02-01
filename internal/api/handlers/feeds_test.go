package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type mockChangeFeedRepo struct {
	rows      []federation.ListEventChangesRow
	lastArgs  federation.ListEventChangesParams
	returnErr error
}

func (m *mockChangeFeedRepo) ListEventChanges(_ context.Context, arg federation.ListEventChangesParams) ([]federation.ListEventChangesRow, error) {
	m.lastArgs = arg
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return m.rows, nil
}

func TestFeedsHandlerListChangesSuccess(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	eventID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	now := time.Now().UTC()

	row := federation.ListEventChangesRow{
		ID:                pgtype.UUID{Bytes: id, Valid: true},
		EventID:           pgtype.UUID{Bytes: eventID, Valid: true},
		Action:            "create",
		ChangedFields:     json.RawMessage(`["/name"]`),
		Snapshot:          json.RawMessage(`{"ulid":"01J0KXMQZ8RPXJPN8J9Q6TK0WP","name":"Jazz Fest"}`),
		ChangedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		SequenceNumber:    pgtype.Int8{Int64: 42, Valid: true},
		EventUlid:         "01J0KXMQZ8RPXJPN8J9Q6TK0WP",
		FederationUri:     pgtype.Text{String: "https://example.org/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", Valid: true},
		LicenseUrl:        "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseStatus:     "cc0",
		SourceTimestamp:   pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true},
		ReceivedTimestamp: pgtype.Timestamptz{Time: now, Valid: true},
	}

	repo := &mockChangeFeedRepo{rows: []federation.ListEventChangesRow{row}}
	service := federation.NewChangeFeedService(repo, zerolog.Nop(), "https://example.org")
	h := NewFeedsHandler(service, "test", "https://example.org")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/changes?include_snapshot=true", nil)
	res := httptest.NewRecorder()

	h.ListChanges(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	changesValue, ok := payload["changes"].([]any)
	require.True(t, ok)
	require.Len(t, changesValue, 1)
	change, ok := changesValue[0].(map[string]any)
	require.True(t, ok)
	snapshot, ok := change["snapshot"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Event", snapshot["@type"])
	require.Equal(t, "https://example.org/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", snapshot["@id"])
}

func TestFeedsHandlerListChangesInvalidLimit(t *testing.T) {
	repo := &mockChangeFeedRepo{}
	service := federation.NewChangeFeedService(repo, zerolog.Nop(), "https://example.org")
	h := NewFeedsHandler(service, "test", "https://example.org")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/changes?limit=9999", nil)
	res := httptest.NewRecorder()

	h.ListChanges(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestFeedsHandlerListChangesInvalidCursor(t *testing.T) {
	repo := &mockChangeFeedRepo{}
	service := federation.NewChangeFeedService(repo, zerolog.Nop(), "https://example.org")
	h := NewFeedsHandler(service, "test", "https://example.org")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/changes?since=invalid-cursor", nil)
	res := httptest.NewRecorder()

	h.ListChanges(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}
