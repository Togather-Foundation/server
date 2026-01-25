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
	listFn func(filters places.Filters, pagination places.Pagination) (places.ListResult, error)
	getFn  func(ulid string) (*places.Place, error)
}

func (s stubPlacesRepo) List(_ context.Context, filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubPlacesRepo) GetByULID(_ context.Context, ulid string) (*places.Place, error) {
	return s.getFn(ulid)
}

func TestPlacesHandlerListSuccess(t *testing.T) {
	repo := stubPlacesRepo{
		listFn: func(filters places.Filters, pagination places.Pagination) (places.ListResult, error) {
			return places.ListResult{Places: []places.Place{{Name: "Central Park"}}, NextCursor: "next"}, nil
		},
		getFn: func(_ string) (*places.Place, error) {
			return nil, nil
		},
	}

	h := NewPlacesHandler(places.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload placeListResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.Len(t, payload.Items, 1)
	require.Equal(t, "Central Park", payload.Items[0]["name"])
	require.Equal(t, "Place", payload.Items[0]["@type"])
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
	}

	h := NewPlacesHandler(places.NewService(repo), "test")
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
	}

	h := NewPlacesHandler(places.NewService(repo), "test")
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
	}

	h := NewPlacesHandler(places.NewService(repo), "test")
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
	}

	h := NewPlacesHandler(places.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}
