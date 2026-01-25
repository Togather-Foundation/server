package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/stretchr/testify/require"
)

type stubOrganizationsRepo struct {
	listFn func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error)
	getFn  func(ulid string) (*organizations.Organization, error)
}

func (s stubOrganizationsRepo) List(_ context.Context, filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubOrganizationsRepo) GetByULID(_ context.Context, ulid string) (*organizations.Organization, error) {
	return s.getFn(ulid)
}

func TestOrganizationsHandlerListSuccess(t *testing.T) {
	repo := stubOrganizationsRepo{
		listFn: func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{Organizations: []organizations.Organization{{Name: "Arts Org"}}, NextCursor: "next"}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return nil, nil
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload organizationListResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.Len(t, payload.Items, 1)
	require.Equal(t, "Arts Org", payload.Items[0]["name"])
	require.Equal(t, "Organization", payload.Items[0]["@type"])
	require.Equal(t, "next", payload.NextCursor)
}

func TestOrganizationsHandlerListValidationError(t *testing.T) {
	repo := stubOrganizationsRepo{
		listFn: func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return nil, nil
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations?limit=abc", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestOrganizationsHandlerGetNotFound(t *testing.T) {
	repo := stubOrganizationsRepo{
		listFn: func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return nil, organizations.ErrNotFound
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestOrganizationsHandlerGetInvalidID(t *testing.T) {
	repo := stubOrganizationsRepo{
		listFn: func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return nil, nil
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/bad", nil)
	req.SetPathValue("id", "bad")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestOrganizationsHandlerListServiceError(t *testing.T) {
	repo := stubOrganizationsRepo{
		listFn: func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, errors.New("boom")
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return nil, nil
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}
