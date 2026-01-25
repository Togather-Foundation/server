package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

type eventListResponse struct {
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

func TestEventsListFiltersAndPagination(t *testing.T) {
	env := setupTestEnv(t)

	seed := seedEventsListData(t, env)

	filters := url.Values{}
	filters.Set("city", "Toronto")
	filters.Set("startDate", "2026-07-01")
	filters.Set("endDate", "2026-07-31")
	filters.Set("limit", "1")

	first := fetchEventsList(t, env, filters)
	require.Len(t, first.Items, 1)
	require.NotEmpty(t, first.NextCursor)
	require.Equal(t, seed.EventAName, eventName(first.Items[0]))

	filters.Set("after", first.NextCursor)
	second := fetchEventsList(t, env, filters)
	require.Len(t, second.Items, 1)
	require.NotEqual(t, first.Items[0], second.Items[0])
	require.Equal(t, seed.EventBName, eventName(second.Items[0]))

	filters = url.Values{}
	filters.Set("venueId", seed.PlaceAULID)
	venueResp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{seed.EventAName}, eventNames(venueResp.Items))

	filters = url.Values{}
	filters.Set("organizerId", seed.OrgAULID)
	orgResp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{seed.EventAName}, eventNames(orgResp.Items))

	filters = url.Values{}
	filters.Set("state", "draft")
	stateResp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{seed.EventBName}, eventNames(stateResp.Items))

	filters = url.Values{}
	filters.Set("domain", "arts")
	domainResp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{seed.EventBName}, eventNames(domainResp.Items))

	filters = url.Values{}
	filters.Set("q", "Jazz")
	queryResp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{seed.EventAName}, eventNames(queryResp.Items))

	filters = url.Values{}
	filters.Set("keywords", "jazz")
	keywordResp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{seed.EventAName}, eventNames(keywordResp.Items))
}

type listSeedData struct {
	EventAName string
	EventBName string
	PlaceAULID string
	OrgAULID   string
}

func seedEventsListData(t *testing.T, env *testEnv) listSeedData {
	t.Helper()

	orgA := insertOrganization(t, env, "Toronto Arts Org")
	orgB := insertOrganization(t, env, "City Gallery")
	placeA := insertPlace(t, env, "Centennial Park", "Toronto")
	placeB := insertPlace(t, env, "Riverside Gallery", "Toronto")
	placeC := insertPlace(t, env, "Ottawa Arena", "Ottawa")

	eventAName := "Jazz in the Park"
	eventBName := "Summer Arts Expo"

	insertEventWithOccurrence(t, env, eventAName, orgA.ID, placeA.ID, "music", "published", []string{"jazz", "summer"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	insertEventWithOccurrence(t, env, eventBName, orgB.ID, placeB.ID, "arts", "draft", []string{"gallery"}, time.Date(2026, 7, 20, 18, 0, 0, 0, time.UTC))
	insertEventWithOccurrence(t, env, "Ottawa Winter Fest", orgB.ID, placeC.ID, "culture", "published", []string{"winter"}, time.Date(2026, 8, 1, 20, 0, 0, 0, time.UTC))

	return listSeedData{
		EventAName: eventAName,
		EventBName: eventBName,
		PlaceAULID: placeA.ULID,
		OrgAULID:   orgA.ULID,
	}
}

func fetchEventsList(t *testing.T, env *testEnv, params url.Values) eventListResponse {
	t.Helper()

	u := env.Server.URL + "/api/v1/events"
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

	var payload eventListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	return payload
}

type seededEntity struct {
	ID   string
	ULID string
}

func insertOrganization(t *testing.T, env *testEnv, name string) seededEntity {
	t.Helper()
	ulidValue := ulid.Make().String()
	var id string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO organizations (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		ulidValue, name, "Toronto",
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}

func insertPlace(t *testing.T, env *testEnv, name string, city string) seededEntity {
	t.Helper()
	ulidValue := ulid.Make().String()
	var id string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality, address_region) VALUES ($1, $2, $3, $4) RETURNING id`,
		ulidValue, name, city, "ON",
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}

func insertEventWithOccurrence(t *testing.T, env *testEnv, name string, organizerID string, venueID string, domain string, state string, keywords []string, start time.Time) {
	t.Helper()

	ulidValue := ulid.Make().String()
	var eventID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, organizer_id, primary_venue_id, event_domain, lifecycle_state, keywords)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		ulidValue, name, organizerID, venueID, domain, state, keywords,
	).Scan(&eventID)
	require.NoError(t, err)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
		 VALUES ($1, $2, $3, $4)`,
		eventID, start, start.Add(2*time.Hour), venueID,
	)
	require.NoError(t, err)
}

func eventNames(items []map[string]any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, eventName(item))
	}
	sort.Strings(result)
	return result
}

func eventName(item map[string]any) string {
	if value, ok := item["name"].(string); ok {
		return value
	}
	if value, ok := item["name"].(map[string]any); ok {
		if text, ok := value["value"].(string); ok {
			return text
		}
	}
	return ""
}
