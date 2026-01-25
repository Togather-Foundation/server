package contracts_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateEventRejectsNonCC0License(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-license")
	payload := map[string]any{
		"name":      "Licensed Event",
		"startDate": time.Date(2026, 9, 12, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Centennial Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"license": "https://creativecommons.org/licenses/by/4.0/",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}
