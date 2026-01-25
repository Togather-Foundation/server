package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type placeListResponse struct {
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

func TestPlacesListFiltersAndPagination(t *testing.T) {
	env := setupTestEnv(t)

	_ = insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertPlace(t, env, "Riverside Gallery", "Toronto")
	_ = insertPlace(t, env, "Ottawa Arena", "Ottawa")

	params := url.Values{}
	params.Set("city", "Toronto")
	params.Set("limit", "1")

	first := fetchPlacesList(t, env, params)
	require.Len(t, first.Items, 1)
	require.NotEmpty(t, first.NextCursor)

	params.Set("after", first.NextCursor)
	second := fetchPlacesList(t, env, params)
	require.Len(t, second.Items, 1)
	require.Empty(t, second.NextCursor)

	got := placeNames(append(first.Items, second.Items...))
	require.ElementsMatch(t, []string{"Centennial Park", "Riverside Gallery"}, got)
}

func TestGetPlaceByID(t *testing.T) {
	env := setupTestEnv(t)

	place := insertPlace(t, env, "Centennial Park", "Toronto")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/places/"+place.ULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "Centennial Park", placeNameFromPayload(payload))
}

func TestGetPlaceByIDNotFound(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/places/01HYX3KQW7ERTV9XNBM2P8QJZF", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func fetchPlacesList(t *testing.T, env *testEnv, params url.Values) placeListResponse {
	t.Helper()

	u := env.Server.URL + "/api/v1/places"
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

	var payload placeListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	return payload
}

func placeNames(items []map[string]any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, placeNameFromPayload(item))
	}
	sort.Strings(result)
	return result
}

func placeNameFromPayload(payload map[string]any) string {
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
