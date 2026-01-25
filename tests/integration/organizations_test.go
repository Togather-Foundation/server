package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type organizationListResponse struct {
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

func TestOrganizationsListPaginationAndQuery(t *testing.T) {
	env := setupTestEnv(t)

	_ = insertOrganization(t, env, "Toronto Arts Org")
	_ = insertOrganization(t, env, "City Gallery")
	_ = insertOrganization(t, env, "Ottawa Culture Hub")

	params := url.Values{}
	params.Set("q", "Toronto")
	params.Set("limit", "1")

	first := fetchOrganizationsList(t, env, params)
	require.Len(t, first.Items, 1)
	require.NotEmpty(t, first.NextCursor)
	require.Equal(t, "Toronto Arts Org", organizationNameFromPayload(first.Items[0]))

	params.Set("after", first.NextCursor)
	second := fetchOrganizationsList(t, env, params)
	require.Len(t, second.Items, 0)

	params = url.Values{}
	params.Set("limit", "2")
	all := fetchOrganizationsList(t, env, params)
	require.ElementsMatch(t, []string{"Toronto Arts Org", "City Gallery", "Ottawa Culture Hub"}, organizationNames(all.Items))
}

func TestGetOrganizationByID(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/organizations/"+org.ULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "Toronto Arts Org", organizationNameFromPayload(payload))
}

func TestGetOrganizationByIDNotFound(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/organizations/01HYX3KQW7ERTV9XNBM2P8QJZF", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func fetchOrganizationsList(t *testing.T, env *testEnv, params url.Values) organizationListResponse {
	t.Helper()

	u := env.Server.URL + "/api/v1/organizations"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload organizationListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	return payload
}

func organizationNames(items []map[string]any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, organizationNameFromPayload(item))
	}
	sort.Strings(result)
	return result
}

func organizationNameFromPayload(payload map[string]any) string {
	if value, ok := payload["name"].(string); ok {
		return value
	}
	if value, ok := payload["name"].(map[string]any); ok {
		if text, ok := value["value"].(string); ok {
			return text
		}
	}
	return ""
}
