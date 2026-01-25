package contracts_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/stretchr/testify/require"
)

type problemPayload struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

func TestProblemDetailsInvalidULID(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/not-a-ulid", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))

	problem := decodeProblemDetails(t, resp)
	require.Equal(t, http.StatusBadRequest, problem.Status)
	require.NotEmpty(t, problem.Type)
	require.NotEmpty(t, problem.Title)
	require.Contains(t, strings.ToLower(problem.Detail), "ulid")
}

func TestProblemDetailsMalformedDateRange(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events?startDate=2026-13-40&endDate=2026-01-01", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))

	problem := decodeProblemDetails(t, resp)
	require.Equal(t, http.StatusBadRequest, problem.Status)
	require.NotEmpty(t, problem.Type)
	require.NotEmpty(t, problem.Title)
	require.NotEmpty(t, problem.Detail)
}

func TestAcceptHeaderDefaultsToJSON(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json"))

	reqInvalid, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	require.NoError(t, err)
	reqInvalid.Header.Set("Accept", "application/invalid")

	respInvalid, err := env.Server.Client().Do(reqInvalid)
	require.NoError(t, err)
	t.Cleanup(func() { _ = respInvalid.Body.Close() })

	require.Equal(t, http.StatusOK, respInvalid.StatusCode)
	require.True(t, strings.HasPrefix(respInvalid.Header.Get("Content-Type"), "application/json"))
}

func TestProblemDetailsDetailLevelsByEnvironment(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)

	problem.Write(rec, req, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", errors.New("missing required field"), "test")

	var payload problemPayload
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
	require.Equal(t, http.StatusBadRequest, payload.Status)
	require.Equal(t, "missing required field", payload.Detail)

	recProd := httptest.NewRecorder()
	problem.Write(recProd, req, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", errors.New("missing required field"), "production")

	var payloadProd problemPayload
	require.NoError(t, json.NewDecoder(recProd.Body).Decode(&payloadProd))
	require.Equal(t, http.StatusBadRequest, payloadProd.Status)
	require.Equal(t, http.StatusText(http.StatusBadRequest), payloadProd.Detail)
}

func decodeProblemDetails(t *testing.T, resp *http.Response) problemPayload {
	t.Helper()

	var payload problemPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	return payload
}
