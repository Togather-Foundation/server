package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/stretchr/testify/require"
)

type stubPlacesRepo struct {
	listFn      func(filters places.Filters, pagination places.Pagination) (places.ListResult, error)
	getFn       func(ulid string) (*places.Place, error)
	tombstoneFn func(ulid string) (*places.Tombstone, error)
}

func (s stubPlacesRepo) List(_ context.Context, filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubPlacesRepo) GetByULID(_ context.Context, ulid string) (*places.Place, error) {
	return s.getFn(ulid)
}

func (s stubPlacesRepo) SoftDelete(_ context.Context, _ string, _ string) error {
	return errors.New("not implemented")
}

func (s stubPlacesRepo) CreateTombstone(_ context.Context, _ places.TombstoneCreateParams) error {
	return errors.New("not implemented")
}

func (s stubPlacesRepo) GetTombstoneByULID(_ context.Context, ulid string) (*places.Tombstone, error) {
	if s.tombstoneFn == nil {
		return nil, places.ErrNotFound
	}
	return s.tombstoneFn(ulid)
}

func (s stubPlacesRepo) Update(_ context.Context, _ string, _ places.UpdatePlaceParams) (*places.Place, error) {
	return nil, errors.New("not implemented")
}

func TestPlacesHandlerListSuccess(t *testing.T) {
	repo := stubPlacesRepo{
		listFn: func(filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
			return places.ListResult{Places: []places.Place{{Name: "Central Park"}}, NextCursor: "next"}, nil
		},
		getFn: func(_ string) (*places.Place, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*places.Tombstone, error) {
			return nil, places.ErrNotFound
		},
	}

	h := NewPlacesHandler(places.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload listResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	items, ok := payload.Items.([]any)
	require.True(t, ok, "Items should be a slice")
	require.Len(t, items, 1)
	item0, ok := items[0].(map[string]any)
	require.True(t, ok, "Item should be a map")
	require.Equal(t, "Central Park", item0["name"])
	require.Equal(t, "Place", item0["@type"])
	require.Equal(t, "next", payload.NextCursor)
}

func TestPlacesHandlerListValidationError(t *testing.T) {
	repo := stubPlacesRepo{
		listFn: func(filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
			return places.ListResult{}, nil
		},
		getFn: func(_ string) (*places.Place, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*places.Tombstone, error) {
			return nil, places.ErrNotFound
		},
	}

	h := NewPlacesHandler(places.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places?limit=abc", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestPlacesHandlerGetNotFound(t *testing.T) {
	repo := stubPlacesRepo{
		listFn: func(filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
			return places.ListResult{}, nil
		},
		getFn: func(_ string) (*places.Place, error) {
			return nil, places.ErrNotFound
		},
		tombstoneFn: func(_ string) (*places.Tombstone, error) {
			return nil, places.ErrNotFound
		},
	}

	h := NewPlacesHandler(places.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestPlacesHandlerGetInvalidID(t *testing.T) {
	repo := stubPlacesRepo{
		listFn: func(filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
			return places.ListResult{}, nil
		},
		getFn: func(_ string) (*places.Place, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*places.Tombstone, error) {
			return nil, places.ErrNotFound
		},
	}

	h := NewPlacesHandler(places.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/bad", nil)
	req.SetPathValue("id", "bad")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestPlacesHandlerListServiceError(t *testing.T) {
	repo := stubPlacesRepo{
		listFn: func(filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
			return places.ListResult{}, errors.New("boom")
		},
		getFn: func(_ string) (*places.Place, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*places.Tombstone, error) {
			return nil, places.ErrNotFound
		},
	}

	h := NewPlacesHandler(places.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}
