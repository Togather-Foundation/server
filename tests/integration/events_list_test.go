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

	today := time.Now().In(time.UTC)
	filters := url.Values{}
	filters.Set("startDate", today.Format("2006-01-02"))
	filters.Set("endDate", today.AddDate(0, 1, 0).Format("2006-01-02"))
	filters.Set("limit", "1")

	// city= is a no-op on a single-city node, so all three seeded events are in
	// the window (Toronto + Ottawa venues) and pagination walks all of them.
	first := fetchEventsList(t, env, filters)
	require.Len(t, first.Items, 1)
	require.NotEmpty(t, first.NextCursor)
	require.Equal(t, seed.EventAName, eventName(first.Items[0]))

	filters.Set("after", first.NextCursor)
	second := fetchEventsList(t, env, filters)
	require.Len(t, second.Items, 1)
	require.NotEqual(t, first.Items[0], second.Items[0])
	require.Equal(t, seed.EventBName, eventName(second.Items[0]))

	filters.Set("after", second.NextCursor)
	third := fetchEventsList(t, env, filters)
	require.Len(t, third.Items, 1)
	require.Equal(t, "Ottawa Winter Fest", eventName(third.Items[0]))
	require.Empty(t, third.NextCursor)

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

// TestEventsListCityFilterDoesNotExclude is the regression guard for #19: the
// deployment is single-city (staging.toronto.togather.foundation is all Toronto),
// so the `city` query parameter must NOT affect which events are returned.
// Previously it filtered on p.address_locality, silently dropping events whose
// venue had a null or mis-parsed addressLocality (up to ~74% of the catalogue).
func TestEventsListCityFilterDoesNotExclude(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "City Arts Org")
	placeToronto := insertPlace(t, env, "Centennial Park", "Toronto")
	placeOttawa := insertPlace(t, env, "Ottawa Arena", "Ottawa")

	now := time.Now().UTC()
	_ = insertEventWithOccurrence(t, env, "Toronto Night", org.ID, placeToronto.ID, "music", "published", nil, now.AddDate(0, 0, 1))
	_ = insertEventWithOccurrence(t, env, "Ottawa Night", org.ID, placeOttawa.ID, "culture", "published", nil, now.AddDate(0, 0, 2))

	filters := url.Values{}
	filters.Set("city", "Toronto")
	filters.Set("startDate", now.Format("2006-01-02"))
	filters.Set("endDate", now.AddDate(0, 1, 0).Format("2006-01-02"))

	payload := fetchEventsList(t, env, filters)

	names := eventNames(payload.Items)
	require.ElementsMatch(t, []string{"Toronto Night", "Ottawa Night"}, names,
		"city= must not exclude events; the node is single-city, so every event belongs to it")
}

// TestEventsListEndDateIncludesEntireDay is the end-to-end regression guard for
// GitHub issue #11: a bare endDate previously resolved to midnight and was used
// as an inclusive upper bound, silently dropping every event later in the day.
// endDate must include events starting any time on the endDate day, and must
// exclude events starting exactly at the following day's midnight (the range is
// a half-open interval [startDate, endDate+1day)).
func TestEventsListEndDateIncludesEntireDay(t *testing.T) {
	env := setupTestEnv(t)

	loc, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")

	// endDay: a calendar day well in the future so the startDate=today default
	// can never interfere with the explicit startDate/endDate we pass.
	now := time.Now().In(loc)
	endDay := time.Date(now.Year(), now.Month(), now.Day()+7, 0, 0, 0, 0, loc)

	// 23:30 local on endDay — the core #11 symptom (was silently dropped pre-fix).
	late := "Late Night Jazz"
	_ = insertEventWithOccurrence(t, env, late, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 23, 30, 0, 0, loc))

	// Exactly next-day midnight — must be excluded (strict half-open bound).
	next := "Next Midnight Event"
	_ = insertEventWithOccurrence(t, env, next, org.ID, place.ID, "culture", "published", []string{}, endDay.AddDate(0, 0, 1))

	// 23:00 the day before endDay — must be excluded by startDate=endDay.
	earlier := "Earlier Day Event"
	_ = insertEventWithOccurrence(t, env, earlier, org.ID, place.ID, "sports", "published", []string{}, endDay.Add(-1*time.Hour))

	day := endDay.Format("2006-01-02")
	filters := url.Values{}
	filters.Set("startDate", day)
	filters.Set("endDate", day)
	filters.Set("limit", "50")

	resp := fetchEventsList(t, env, filters)
	require.ElementsMatch(t, []string{late}, eventNames(resp.Items),
		"endDate must include the entire endDate day and nothing beyond it")
}

// TestEventsListOpenAPILinkHeader verifies the RFC 8631 service-desc Link
// header coexists with the ICS alternate Link header on the real events route.
// The events handler sets its own Link via Header.Set, which previously
// clobbered the middleware's service-desc Link (Togather-Foundation/server#16).
func TestEventsListOpenAPILinkHeader(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events?limit=1", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	joined := ""
	for _, l := range resp.Header.Values("Link") {
		joined += l + "\n"
	}
	require.Contains(t, joined, "rel=\"service-desc\"", "service-desc Link should be present on /api/v1/events")
	require.Contains(t, joined, "/api/v1/openapi.json", "service-desc Link should point at the OpenAPI spec")
	require.Contains(t, joined, "rel=\"alternate\"", "the handler's ICS alternate Link should survive")
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

	now := time.Now().UTC()
	_ = insertEventWithOccurrence(t, env, eventAName, orgA.ID, placeA.ID, "music", "published", []string{"jazz", "summer"}, now.AddDate(0, 0, 1))
	_ = insertEventWithOccurrence(t, env, eventBName, orgB.ID, placeB.ID, "arts", "draft", []string{"gallery"}, now.AddDate(0, 0, 10))
	_ = insertEventWithOccurrence(t, env, "Ottawa Winter Fest", orgB.ID, placeC.ID, "culture", "published", []string{"winter"}, now.AddDate(0, 0, 22))

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

func insertEventWithOccurrence(t *testing.T, env *testEnv, name string, organizerID string, venueID string, domain string, state string, keywords []string, start time.Time) string {
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

	return ulidValue
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
