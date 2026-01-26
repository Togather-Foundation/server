package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
)

type stubEventsRepo struct {
	listFn      func(filters events.Filters, pagination events.Pagination) (events.ListResult, error)
	getFn       func(ulid string) (*events.Event, error)
	tombstoneFn func(ulid string) (*events.Tombstone, error)
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

func (s stubEventsRepo) GetIdempotencyKey(_ context.Context, _ string) (*events.IdempotencyKey, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) InsertIdempotencyKey(_ context.Context, _ events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) UpdateIdempotencyKeyEvent(_ context.Context, _ string, _ string, _ string) error {
	return nil
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

	h := NewEventsHandler(events.NewService(repo), nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2", nil)
	req.Header.Set("Accept", "application/ld+json")
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/ld+json", res.Header().Get("Content-Type"))

	var payload listResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.Len(t, payload.Items, 1)
	require.Equal(t, "Jazz Fest", payload.Items[0]["name"])
	require.Equal(t, "Event", payload.Items[0]["@type"])
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

	h := NewEventsHandler(events.NewService(repo), nil, nil, "test", "https://example.org")
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

	h := NewEventsHandler(events.NewService(repo), nil, nil, "test", "https://example.org")
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

	h := NewEventsHandler(events.NewService(repo), nil, nil, "test", "https://example.org")
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

	h := NewEventsHandler(events.NewService(repo), nil, nil, "test", "https://example.org")
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

	h := NewEventsHandler(events.NewService(repo), nil, nil, "test", "https://example.org")
	params := url.Values{}
	params.Set("limit", "1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?"+params.Encode(), nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}
