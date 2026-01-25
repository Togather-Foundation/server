package contracts_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type eventListPayload struct {
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

func TestJSONLDEventListAndDetailFraming(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	listReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	require.NoError(t, err)
	listReq.Header.Set("Accept", "application/ld+json")

	listResp, err := env.Server.Client().Do(listReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listResp.Body.Close() })
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listPayload eventListPayload
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listPayload))
	require.NotEmpty(t, listPayload.Items)
	first := listPayload.Items[0]
	require.NotEmpty(t, first["@context"])
	require.NotEmpty(t, first["@type"])

	detailReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	detailReq.Header.Set("Accept", "application/ld+json")

	detailResp, err := env.Server.Client().Do(detailReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = detailResp.Body.Close() })
	require.Equal(t, http.StatusOK, detailResp.StatusCode)

	var detailPayload map[string]any
	require.NoError(t, json.NewDecoder(detailResp.Body).Decode(&detailPayload))
	require.NotEmpty(t, detailPayload["@context"])
	require.NotEmpty(t, detailPayload["@type"])
}
