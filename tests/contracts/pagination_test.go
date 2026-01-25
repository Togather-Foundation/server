package contracts_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPaginationCursorEncodingAndNextCursor(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	_ = insertEventWithOccurrence(t, env, "Summer Arts Expo", org.ID, place.ID, "arts", "published", []string{"summer"}, time.Date(2026, 7, 11, 19, 0, 0, 0, time.UTC))

	params := url.Values{}
	params.Set("limit", "1")

	listReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events?"+params.Encode(), nil)
	require.NoError(t, err)
	listReq.Header.Set("Accept", "application/ld+json")

	listResp, err := env.Server.Client().Do(listReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listResp.Body.Close() })
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var payload eventListPayload
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&payload))
	require.Len(t, payload.Items, 1)
	require.NotEmpty(t, payload.NextCursor)

	cursorPattern := regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)
	require.True(t, cursorPattern.MatchString(payload.NextCursor))

	params.Set("after", payload.NextCursor)
	nextReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events?"+params.Encode(), nil)
	require.NoError(t, err)
	nextReq.Header.Set("Accept", "application/ld+json")

	nextResp, err := env.Server.Client().Do(nextReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = nextResp.Body.Close() })
	require.Equal(t, http.StatusOK, nextResp.StatusCode)

	var nextPayload eventListPayload
	require.NoError(t, json.NewDecoder(nextResp.Body).Decode(&nextPayload))
	require.Len(t, nextPayload.Items, 1)
}
