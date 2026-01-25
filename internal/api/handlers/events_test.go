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
	listFn func(filters events.Filters, pagination events.Pagination) (events.ListResult, error)
	getFn  func(ulid string) (*events.Event, error)
}

func (s stubEventsRepo) List(_ context.Context, filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubEventsRepo) GetByULID(_ context.Context, ulid string) (*events.Event, error) {
	return s.getFn(ulid)
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
	}

	h := NewEventsHandler(events.NewService(repo), "test")
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

	h := NewEventsHandler(events.NewService(repo), "test")
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

	h := NewEventsHandler(events.NewService(repo), "test")
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

	h := NewEventsHandler(events.NewService(repo), "test")
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

	h := NewEventsHandler(events.NewService(repo), "test")
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

	h := NewEventsHandler(events.NewService(repo), "test")
	params := url.Values{}
	params.Set("limit", "1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?"+params.Encode(), nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}
