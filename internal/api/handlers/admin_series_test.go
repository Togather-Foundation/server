package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func seriesMember(ulid, name, startDate string) events.SeriesCandidateMember {
	next := startDate
	return events.SeriesCandidateMember{
		ULID:                ulid,
		Name:                name,
		StartDate:           startDate,
		LastDate:            startDate,
		OccurrenceCount:     1,
		LifecycleState:      "published",
		NextOccurrenceDate:  &next,
		FirstLocalStartTime: "18:00",
		FirstDayOfWeek:      2,
		VenueULID:           "01HPLACE000000000000000001",
		VenueName:           "Main Branch",
	}
}

func TestGetSeriesCandidates_400Validation(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"days zero", "?days=0"},
		{"days too large", "?days=366"},
		{"days not a number", "?days=lots"},
		{"lookback negative", "?lookback_days=-1"},
		{"lookback too large", "?lookback_days=91"},
		{"min_events too small", "?min_events=1"},
		{"min_events too large", "?min_events=51"},
		{"limit zero", "?limit=0"},
		{"limit too large", "?limit=201"},
		{"venue invalid", "?venue_id=not-a-ulid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			h := newTestAdminHandler(repo)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates"+tc.query, nil)
			req = withAdminUser(req, "admin@example.com")
			rec := httptest.NewRecorder()

			h.GetSeriesCandidates(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")

			var problem map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.Equal(t, float64(http.StatusBadRequest), problem["status"])
			repo.AssertNotCalled(t, "ListSeriesCandidatePairs", mock.Anything, mock.Anything)
		})
	}
}

func TestGetSeriesCandidates_200Shape(t *testing.T) {
	now := time.Now().UTC()
	first := now.AddDate(0, 0, 2).Format(time.DateOnly)
	second := now.AddDate(0, 0, 9).Format(time.DateOnly)
	third := now.AddDate(0, 0, 16).Format(time.DateOnly)

	a := seriesMember("01HEVENT000000000000000001", "Family Storytime", first)
	b := seriesMember("01HEVENT000000000000000002", "Family Story Time", second)
	c := seriesMember("01HEVENT000000000000000003", "Family Storytime", third)

	repo := new(MockRepository)
	repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).Return([]events.SeriesCandidatePair{
		{A: a, B: b},
		{A: b, B: c},
	}, nil)

	h := newTestAdminHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?days=30&lookback_days=0&min_events=2&limit=5", nil)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.GetSeriesCandidates(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var resp struct {
		Items []struct {
			SuggestedCanonicalULID string `json:"suggested_canonical_ulid"`
			Name                   string `json:"name"`
			Venue                  struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"venue"`
			DayOfWeek       int    `json:"day_of_week"`
			LocalStartTime  string `json:"local_start_time"`
			EventCount      int    `json:"event_count"`
			OccurrenceCount int    `json:"occurrence_count"`
			FirstDate       string `json:"first_date"`
			LastDate        string `json:"last_date"`
			Members         []struct {
				ULID            string `json:"ulid"`
				Name            string `json:"name"`
				StartDate       string `json:"start_date"`
				OccurrenceCount int    `json:"occurrence_count"`
				LifecycleState  string `json:"lifecycle_state"`
			} `json:"members"`
		} `json:"items"`
		TotalClusters int  `json:"total_clusters"`
		Truncated     bool `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.Items, 1)
	item := resp.Items[0]
	assert.Equal(t, a.ULID, item.SuggestedCanonicalULID)
	assert.Equal(t, "Family Storytime", item.Name)
	assert.Equal(t, "01HPLACE000000000000000001", item.Venue.ID)
	assert.Equal(t, "Main Branch", item.Venue.Name)
	assert.Equal(t, 2, item.DayOfWeek)
	assert.Equal(t, "18:00", item.LocalStartTime)
	assert.Equal(t, 3, item.EventCount)
	assert.Equal(t, 3, item.OccurrenceCount)
	assert.Equal(t, first, item.FirstDate)
	assert.Equal(t, third, item.LastDate)
	assert.Len(t, item.Members, 3)
	assert.Equal(t, 1, resp.TotalClusters)
	assert.False(t, resp.Truncated)

	repo.AssertExpectations(t)
}

func TestGetSeriesCandidates_RepositoryParams(t *testing.T) {
	const venueULID = "01HP1ACE000000000000000009"

	cases := []struct {
		name         string
		query        string
		lookbackDays int
		expectVenue  string
	}{
		{name: "defaults", query: "?days=30&min_events=2&limit=7&venue_id=" + venueULID, lookbackDays: 14, expectVenue: venueULID},
		{name: "lowercase venue normalized", query: "?days=30&min_events=2&limit=7&venue_id=01hp1ace000000000000000009", lookbackDays: 14, expectVenue: venueULID},
		{name: "lookback zero", query: "?days=30&lookback_days=0&min_events=2&limit=7", lookbackDays: 0},
		{name: "lookback explicit", query: "?days=30&lookback_days=45&min_events=2&limit=7", lookbackDays: 45},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			var captured events.SeriesCandidatesQuery
			repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					captured = args.Get(1).(events.SeriesCandidatesQuery)
				}).
				Return(nil, nil)

			h := newTestAdminHandler(repo)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates"+tc.query, nil)
			req = withAdminUser(req, "admin@example.com")
			rec := httptest.NewRecorder()
			h.GetSeriesCandidates(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)

			require.Equal(t, tc.expectVenue, captured.VenueULID)
			require.Equal(t, "UTC", captured.Now.Location().String())

			nowDate := time.Date(captured.Now.Year(), captured.Now.Month(), captured.Now.Day(), 0, 0, 0, 0, time.UTC)
			assert.Equal(t, nowDate.AddDate(0, 0, -tc.lookbackDays), captured.WindowStart,
				"lookback_days=%d must not silently become the default", tc.lookbackDays)
			assert.Equal(t, nowDate.AddDate(0, 0, 30), captured.WindowEnd)
		})
	}
}

func TestGetSeriesCandidates_MinEventsFilters(t *testing.T) {
	now := time.Now().UTC()
	a := seriesMember("01HEVENT000000000000000001", "Family Storytime", now.AddDate(0, 0, 2).Format(time.DateOnly))
	b := seriesMember("01HEVENT000000000000000002", "Family Storytime", now.AddDate(0, 0, 9).Format(time.DateOnly))
	c := seriesMember("01HEVENT000000000000000003", "Family Storytime", now.AddDate(0, 0, 16).Format(time.DateOnly))

	repo := new(MockRepository)
	repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).Return([]events.SeriesCandidatePair{
		{A: a, B: b},
		{A: b, B: c},
	}, nil)

	h := newTestAdminHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?min_events=10", nil)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()
	h.GetSeriesCandidates(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items         []json.RawMessage `json:"items"`
		TotalClusters int               `json:"total_clusters"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items, "min_events must reach the domain cluster builder")
	assert.Equal(t, 0, resp.TotalClusters)
}

func TestGetSeriesCandidates_Truncated(t *testing.T) {
	now := time.Now().UTC()
	first := now.AddDate(0, 0, 2).Format(time.DateOnly)
	second := now.AddDate(0, 0, 9).Format(time.DateOnly)

	venueOneA := seriesMember("01HEVENT000000000000000001", "One", first)
	venueOneB := seriesMember("01HEVENT000000000000000002", "One", second)
	venueTwoA := seriesMember("01HEVENT000000000000000003", "Two", first)
	venueTwoB := seriesMember("01HEVENT000000000000000004", "Two", second)
	venueTwoA.VenueULID = "01HPLACE000000000000000002"
	venueTwoA.VenueName = "Other Branch"
	venueTwoB.VenueULID = "01HPLACE000000000000000002"
	venueTwoB.VenueName = "Other Branch"

	repo := new(MockRepository)
	repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).Return([]events.SeriesCandidatePair{
		{A: venueOneA, B: venueOneB},
		{A: venueTwoA, B: venueTwoB},
	}, nil)

	h := newTestAdminHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?limit=1&min_events=2", nil)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()
	h.GetSeriesCandidates(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items         []json.RawMessage `json:"items"`
		TotalClusters int               `json:"total_clusters"`
		Truncated     bool              `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 2, resp.TotalClusters)
	assert.True(t, resp.Truncated)
}
