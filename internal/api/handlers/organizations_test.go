package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/organizations"
	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/stretchr/testify/require"
)

type stubOrganizationsRepo struct {
	listFn      func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error)
	getFn       func(ulid string) (*organizations.Organization, error)
	tombstoneFn func(ulid string) (*organizations.Tombstone, error)
}

func (s stubOrganizationsRepo) List(_ context.Context, filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubOrganizationsRepo) GetByULID(_ context.Context, ulid string) (*organizations.Organization, error) {
	return s.getFn(ulid)
}

func (s stubOrganizationsRepo) Create(_ context.Context, _ organizations.CreateParams) (*organizations.Organization, error) {
	return nil, errors.New("not implemented")
}

func (s stubOrganizationsRepo) SoftDelete(_ context.Context, _ string, _ string) error {
	return errors.New("not implemented")
}

func (s stubOrganizationsRepo) CreateTombstone(_ context.Context, _ organizations.TombstoneCreateParams) error {
	return errors.New("not implemented")
}

func (s stubOrganizationsRepo) GetTombstoneByULID(_ context.Context, ulid string) (*organizations.Tombstone, error) {
	if s.tombstoneFn == nil {
		return nil, organizations.ErrNotFound
	}
	return s.tombstoneFn(ulid)
}

func (s stubOrganizationsRepo) Update(_ context.Context, _ string, _ organizations.UpdateOrganizationParams) (*organizations.Organization, error) {
	return nil, errors.New("not implemented")
}

func TestOrganizationsHandlerListSuccess(t *testing.T) {
	repo := stubOrganizationsRepo{
		listFn: func(filters organizations.Filters, pagination organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{Organizations: []organizations.Organization{{Name: "Arts Org"}}, NextCursor: "next"}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
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
	require.Equal(t, "Arts Org", item0["name"])
	require.Equal(t, "Organization", item0["@type"])
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
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test", "https://example.org")
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
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test", "https://example.org")
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
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test", "https://example.org")
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
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(repo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}

func TestOrganizationsHandlerGetScraperSourcesPresent(t *testing.T) {
	const ulid = "01J0KXMQZ8RPXJPN8J9Q6TK0WP"
	orgRepo := stubOrganizationsRepo{
		listFn: func(_ organizations.Filters, _ organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return &organizations.Organization{ULID: ulid, Name: "Arts Org"}, nil
		},
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}
	scraperRepo := stubScraperSourceRepo{
		listByOrgFn: func(_ context.Context, _ string) ([]domainScraper.Source, error) {
			return []domainScraper.Source{
				{Name: "arts-org-events", URL: "https://arts.example.org/events", Tier: 0, Notes: "main calendar"},
			}, nil
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(orgRepo), "test", "https://example.org").
		WithScraperSourceRepo(scraperRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+ulid, nil)
	req.SetPathValue("id", ulid)
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	sources, ok := payload["sel:scraperSource"].([]any)
	require.True(t, ok, "sel:scraperSource should be a slice")
	require.Len(t, sources, 1)
	src, ok := sources[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "arts-org-events", src["name"])
	require.Equal(t, "https://arts.example.org/events", src["url"])
}

func TestOrganizationsHandlerGetScraperSourcesEmpty(t *testing.T) {
	const ulid = "01J0KXMQZ8RPXJPN8J9Q6TK0WP"
	orgRepo := stubOrganizationsRepo{
		listFn: func(_ organizations.Filters, _ organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return &organizations.Organization{ULID: ulid, Name: "Arts Org"}, nil
		},
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}
	scraperRepo := stubScraperSourceRepo{
		listByOrgFn: func(_ context.Context, _ string) ([]domainScraper.Source, error) {
			return nil, nil // no linked sources
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(orgRepo), "test", "https://example.org").
		WithScraperSourceRepo(scraperRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+ulid, nil)
	req.SetPathValue("id", ulid)
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	_, present := payload["sel:scraperSource"]
	require.False(t, present, "sel:scraperSource should be omitted when empty")
}

func TestOrganizationsHandlerGetScraperSourcesError(t *testing.T) {
	const ulid = "01J0KXMQZ8RPXJPN8J9Q6TK0WP"
	orgRepo := stubOrganizationsRepo{
		listFn: func(_ organizations.Filters, _ organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return &organizations.Organization{ULID: ulid, Name: "Arts Org"}, nil
		},
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}
	scraperRepo := stubScraperSourceRepo{
		listByOrgFn: func(_ context.Context, _ string) ([]domainScraper.Source, error) {
			return nil, errors.New("db error")
		},
	}

	h := NewOrganizationsHandler(organizations.NewService(orgRepo), "test", "https://example.org").
		WithScraperSourceRepo(scraperRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+ulid, nil)
	req.SetPathValue("id", ulid)
	res := httptest.NewRecorder()

	h.Get(res, req)

	// best-effort: error must NOT bubble up as 500; response is still 200 OK
	require.Equal(t, http.StatusOK, res.Code)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	_, present := payload["sel:scraperSource"]
	require.False(t, present, "sel:scraperSource must be omitted when ListByOrg returns an error")
}

func TestOrganizationsHandlerGetNoScraperRepo(t *testing.T) {
	const ulid = "01J0KXMQZ8RPXJPN8J9Q6TK0WP"
	orgRepo := stubOrganizationsRepo{
		listFn: func(_ organizations.Filters, _ organizations.Pagination) (organizations.ListResult, error) {
			return organizations.ListResult{}, nil
		},
		getFn: func(_ string) (*organizations.Organization, error) {
			return &organizations.Organization{ULID: ulid, Name: "Arts Org"}, nil
		},
		tombstoneFn: func(_ string) (*organizations.Tombstone, error) {
			return nil, organizations.ErrNotFound
		},
	}

	// No WithScraperSourceRepo — scraperRepo is nil
	h := NewOrganizationsHandler(organizations.NewService(orgRepo), "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+ulid, nil)
	req.SetPathValue("id", ulid)
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	_, present := payload["sel:scraperSource"]
	require.False(t, present, "sel:scraperSource should be omitted when scraperRepo is nil")
}
