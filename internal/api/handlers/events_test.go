package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Togather-Foundation/server/internal/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
)

type stubEventsRepo struct {
	listFn      func(filters events.Filters, pagination events.Pagination) (events.ListResult, error)
	getFn       func(ulid string) (*events.Event, error)
	tombstoneFn func(ulid string) (*events.Tombstone, error)
	idemKeyFn   func(key string) (*events.IdempotencyKey, error)
	idemInsert  func(params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error)
	idemUpdate  func(key string, eventID string, eventULID string) error
}

func (s stubEventsRepo) List(_ context.Context, filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubEventsRepo) GetByULID(_ context.Context, ulid string) (*events.Event, error) {
	return s.getFn(ulid)
}

func (s stubEventsRepo) GetTombstoneByEventID(_ context.Context, _ string) (*events.Tombstone, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) GetTombstoneByEventULID(_ context.Context, ulid string) (*events.Tombstone, error) {
	if s.tombstoneFn == nil {
		return nil, events.ErrNotFound
	}
	return s.tombstoneFn(ulid)
}

func (s stubEventsRepo) Create(_ context.Context, _ events.EventCreateParams) (*events.Event, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) CreateOccurrence(_ context.Context, _ events.OccurrenceCreateParams) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) CreateSource(_ context.Context, _ events.EventSourceCreateParams) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) FindBySourceExternalID(_ context.Context, _ string, _ string) (*events.Event, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) FindByDedupHash(_ context.Context, _ string) (*events.Event, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) GetOrCreateSource(_ context.Context, _ events.SourceLookupParams) (string, error) {
	return "", errors.New("not implemented")
}

func (s stubEventsRepo) GetIdempotencyKey(_ context.Context, key string) (*events.IdempotencyKey, error) {
	if s.idemKeyFn == nil {
		return nil, events.ErrNotFound
	}
	return s.idemKeyFn(key)
}

func (s stubEventsRepo) InsertIdempotencyKey(_ context.Context, params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
	if s.idemInsert == nil {
		return nil, errors.New("not implemented")
	}
	return s.idemInsert(params)
}

func (s stubEventsRepo) UpdateIdempotencyKeyEvent(_ context.Context, key string, eventID string, eventULID string) error {
	if s.idemUpdate == nil {
		return nil
	}
	return s.idemUpdate(key, eventID, eventULID)
}

func (s stubEventsRepo) UpsertPlace(_ context.Context, _ events.PlaceCreateParams) (*events.PlaceRecord, error) {
	return &events.PlaceRecord{ID: "place-id", ULID: "place-ulid"}, nil
}

func (s stubEventsRepo) UpsertOrganization(_ context.Context, _ events.OrganizationCreateParams) (*events.OrganizationRecord, error) {
	return &events.OrganizationRecord{ID: "org-id", ULID: "org-ulid"}, nil
}

func (s stubEventsRepo) UpdateEvent(_ context.Context, _ string, _ events.UpdateEventParams) (*events.Event, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) SoftDeleteEvent(_ context.Context, _ string, _ string) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) MergeEvents(_ context.Context, _ string, _ string) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) CreateTombstone(_ context.Context, _ events.TombstoneCreateParams) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) BeginTx(_ context.Context) (events.Repository, events.TxCommitter, error) {
	return nil, nil, errors.New("not implemented")
}

// Review Queue methods
func (s stubEventsRepo) FindReviewByDedup(_ context.Context, _ *string, _ *string, _ *string) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) CreateReviewQueueEntry(_ context.Context, _ events.ReviewQueueCreateParams) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) UpdateReviewQueueEntry(_ context.Context, _ int, _ events.ReviewQueueUpdateParams) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) GetReviewQueueEntry(_ context.Context, _ int) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) ListReviewQueue(_ context.Context, _ events.ReviewQueueFilters) (*events.ReviewQueueListResult, error) {
	return &events.ReviewQueueListResult{Entries: []events.ReviewQueueEntry{}, NextCursor: nil}, nil
}

func (s stubEventsRepo) ApproveReview(_ context.Context, _ int, _ string, _ *string) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) RejectReview(_ context.Context, _ int, _ string, _ string) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) CleanupExpiredReviews(_ context.Context) error {
	return nil
}

func TestEventsHandlerListSuccess(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			require.Equal(t, 2, pagination.Limit)
			return events.ListResult{
				Events:     []events.Event{{Name: "Jazz Fest"}},
				NextCursor: "next",
			}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*events.Tombstone, error) {
			return nil, events.ErrNotFound
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2", nil)
	req.Header.Set("Accept", "application/ld+json")
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/ld+json", res.Header().Get("Content-Type"))

	var payload listResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	items, ok := payload.Items.([]any)
	require.True(t, ok, "Items should be a slice")
	require.Len(t, items, 1)
	item0, ok := items[0].(map[string]any)
	require.True(t, ok, "Item should be a map")
	require.Equal(t, "Jazz Fest", item0["name"])
	require.Equal(t, "Event", item0["@type"])
	require.Equal(t, "next", payload.NextCursor)
}

func TestEventsHandlerListValidationError(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=abc", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Equal(t, "application/problem+json", res.Header().Get("Content-Type"))
}

func TestEventsHandlerGetNotFound(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, events.ErrNotFound
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestEventsHandlerGetSuccess(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return &events.Event{Name: "Jazz Fest"}, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.Equal(t, "Jazz Fest", payload["name"])
}

func TestEventsHandlerGetInvalidID(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/bad", nil)
	req.SetPathValue("id", "bad")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestEventsHandlerListServiceError(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, errors.New("boom")
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	params := url.Values{}
	params.Set("limit", "1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?"+params.Encode(), nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}

func TestEventsHandlerCreateUsesIdempotencyHeader(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
		idemKeyFn: func(key string) (*events.IdempotencyKey, error) {
			require.Equal(t, "abc-123", key)
			return nil, events.ErrNotFound
		},
		idemInsert: func(params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
			require.Equal(t, "abc-123", params.Key)
			return &events.IdempotencyKey{Key: params.Key}, nil
		},
		idemUpdate: func(key string, eventID string, eventULID string) error {
			require.Equal(t, "abc-123", key)
			return nil
		},
	}

	service := events.NewService(repo)
	ingest := events.NewIngestService(repo, "example.org", config.ValidationConfig{RequireImage: true})
	h := NewEventsHandler(service, ingest, nil, nil, nil, "test", "https://example.org")

	body := `{"name":"Jazz","startDate":"2026-07-10T19:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.IdempotencyHeader, "abc-123")
	rec := httptest.NewRecorder()

	middleware.Idempotency(http.HandlerFunc(h.Create)).ServeHTTP(rec, req)

	require.Contains(t, []int{http.StatusCreated, http.StatusConflict, http.StatusBadRequest}, rec.Code)
}
