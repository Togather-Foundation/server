package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type stubSyncRepo struct {
	lastIdempotencyKey string
	getByURICalls      int
}

func (s *stubSyncRepo) GetEventByFederationURI(_ context.Context, _ string) (federation.Event, error) {
	s.getByURICalls++
	return federation.Event{}, errors.New("not found")
}

func (s *stubSyncRepo) UpsertFederatedEvent(_ context.Context, arg federation.UpsertFederatedEventParams) (federation.Event, error) {
	return federation.Event{ID: pgtype.UUID{Valid: true}, ULID: arg.Ulid, FederationURI: arg.FederationUri}, nil
}

func (s *stubSyncRepo) GetFederationNodeByDomain(_ context.Context, nodeDomain string) (federation.FederationNode, error) {
	return federation.FederationNode{ID: pgtype.UUID{Valid: true}, NodeDomain: nodeDomain}, nil
}

func (s *stubSyncRepo) CreateOccurrence(_ context.Context, _ federation.OccurrenceCreateParams) error {
	return nil
}

func (s *stubSyncRepo) UpsertPlace(_ context.Context, _ federation.PlaceCreateParams) (*federation.PlaceRecord, error) {
	return &federation.PlaceRecord{ID: "place-id", ULID: "place-ulid"}, nil
}

func (s *stubSyncRepo) UpsertOrganization(_ context.Context, _ federation.OrganizationCreateParams) (*federation.OrganizationRecord, error) {
	return &federation.OrganizationRecord{ID: "org-id", ULID: "org-ulid"}, nil
}

func (s *stubSyncRepo) WithTransaction(_ context.Context, fn func(txRepo federation.SyncRepository) error) error {
	return fn(s)
}

func (s *stubSyncRepo) GetIdempotencyKey(_ context.Context, _ string) (*federation.IdempotencyKey, error) {
	return nil, nil
}

func (s *stubSyncRepo) InsertIdempotencyKey(_ context.Context, params federation.IdempotencyKeyParams) error {
	s.lastIdempotencyKey = params.Key
	return nil
}

func TestFederationHandlerSyncSuccess(t *testing.T) {
	repo := &stubSyncRepo{}
	syncService := federation.NewSyncService(repo, nil, zerolog.Nop())
	h := NewFederationHandler(nil, syncService, "test")

	body := `{"@context":"https://schema.org","@type":"Event","@id":"https://example.org/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP","name":"Jazz Fest","startDate":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set(middleware.IdempotencyHeader, "idem-123")
	rec := httptest.NewRecorder()

	middleware.Idempotency(http.HandlerFunc(h.Sync)).ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "idem-123", repo.lastIdempotencyKey)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
	require.Equal(t, "https://example.org/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", payload["federationUri"])
}

func TestFederationHandlerSyncInvalidJSON(t *testing.T) {
	syncService := federation.NewSyncService(&stubSyncRepo{}, nil, zerolog.Nop())
	h := NewFederationHandler(nil, syncService, "test")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/sync", strings.NewReader("{invalid"))
	rec := httptest.NewRecorder()

	h.Sync(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFederationHandlerSyncValidationError(t *testing.T) {
	syncService := federation.NewSyncService(&stubSyncRepo{}, nil, zerolog.Nop())
	h := NewFederationHandler(nil, syncService, "test")

	body := `{"@context":"https://schema.org","@type":"Event"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/sync", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Sync(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
