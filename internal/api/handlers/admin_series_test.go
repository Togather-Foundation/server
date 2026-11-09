package handlers

import (
	"encoding/json"
	"fmt"
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
		{"offset negative", "?offset=-1"},
		{"offset too large", "?offset=10001"},
		{"offset not a number", "?offset=all"},
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
			DistinctDescriptions     []string `json:"distinct_descriptions"`
			DescribedMemberCount     int      `json:"described_member_count"`
			DescriptionsTruncated    bool     `json:"descriptions_truncated"`
			DistinctDescriptionCount int      `json:"distinct_description_count"`
			ContentVaries            bool     `json:"content_varies"`
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
	assert.Equal(t, 0, item.DistinctDescriptionCount, "no description fingerprints yields 0")
	assert.False(t, item.ContentVaries)
	require.NotNil(t, item.DistinctDescriptions, "distinct_descriptions must be [] not null")
	assert.Empty(t, item.DistinctDescriptions)
	assert.Equal(t, 0, item.DescribedMemberCount)
	assert.False(t, item.DescriptionsTruncated)
	assert.Contains(t, rec.Body.String(), `"distinct_descriptions":[]`, "empty evidence must serialize as [] not null")
	assert.Equal(t, 1, resp.TotalClusters)
	assert.False(t, resp.Truncated)

	repo.AssertExpectations(t)
}

func TestGetSeriesCandidates_DescriptionDivergence(t *testing.T) {
	now := time.Now().UTC()
	first := now.AddDate(0, 0, 2).Format(time.DateOnly)
	second := now.AddDate(0, 0, 9).Format(time.DateOnly)
	third := now.AddDate(0, 0, 16).Format(time.DateOnly)

	a := seriesMember("01HEVENT000000000000000001", "Family Storytime", first)
	a.DescriptionFingerprint = "fingerprint-secret-a"
	a.Description = "Weekly storytime for toddlers"
	b := seriesMember("01HEVENT000000000000000002", "Family Storytime", second)
	b.DescriptionFingerprint = "fingerprint-secret-b"
	b.Description = "Weekly storytime for toddlers (full)"
	c := seriesMember("01HEVENT000000000000000003", "Family Storytime", third)
	c.DescriptionFingerprint = "fingerprint-secret-b"
	c.Description = "Weekly storytime for toddlers (full)"

	repo := new(MockRepository)
	repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).Return([]events.SeriesCandidatePair{
		{A: a, B: b},
		{A: b, B: c},
	}, nil)

	h := newTestAdminHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?days=30&min_events=2", nil)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()
	h.GetSeriesCandidates(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []struct {
			DistinctDescriptions     []string `json:"distinct_descriptions"`
			DescribedMemberCount     int      `json:"described_member_count"`
			DescriptionsTruncated    bool     `json:"descriptions_truncated"`
			DistinctDescriptionCount int      `json:"distinct_description_count"`
			ContentVaries            bool     `json:"content_varies"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, 2, resp.Items[0].DistinctDescriptionCount)
	assert.Equal(t, 3, resp.Items[0].DescribedMemberCount)
	assert.False(t, resp.Items[0].DescriptionsTruncated)
	assert.Equal(t, []string{
		"Weekly storytime for toddlers",
		"Weekly storytime for toddlers (full)",
	}, resp.Items[0].DistinctDescriptions)
	assert.True(t, resp.Items[0].ContentVaries)

	body := rec.Body.String()
	assert.NotContains(t, body, "description_fingerprint", "fingerprints must not be serialized")
	assert.NotContains(t, body, "fingerprint-secret", "description fingerprints must not leak")
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

func TestGetSeriesCandidates_Offset(t *testing.T) {
	now := time.Now().UTC()
	first := now.AddDate(0, 0, 2).Format(time.DateOnly)
	second := now.AddDate(0, 0, 9).Format(time.DateOnly)

	newVenueCluster := func(venueULID, venueName, canonical string) events.SeriesCandidatePair {
		a := seriesMember(canonical, "Same Name", first)
		b := seriesMember(canonical+"B", "Same Name", second)
		a.VenueULID, a.VenueName = venueULID, venueName
		b.VenueULID, b.VenueName = venueULID, venueName
		return events.SeriesCandidatePair{A: a, B: b}
	}

	pairs := []events.SeriesCandidatePair{
		newVenueCluster("01HPLACE00000000000000000A", "Alpha Branch", "01HEVENT000000000000000001"),
		newVenueCluster("01HPLACE00000000000000000B", "Beta Branch", "01HEVENT000000000000000002"),
		newVenueCluster("01HPLACE00000000000000000C", "Gamma Branch", "01HEVENT000000000000000003"),
	}

	cases := []struct {
		name      string
		query     string
		wantItems int
		wantTrunc bool
	}{
		{name: "offset zero", query: "?min_events=2&limit=10", wantItems: 3, wantTrunc: false},
		{name: "middle slice", query: "?min_events=2&offset=1&limit=1", wantItems: 1, wantTrunc: true},
		{name: "last page", query: "?min_events=2&offset=2&limit=1", wantItems: 1, wantTrunc: false},
		{name: "past the end", query: "?min_events=2&offset=99&limit=10", wantItems: 0, wantTrunc: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).Return(pairs, nil)

			h := newTestAdminHandler(repo)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates"+tc.query, nil)
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
			assert.Len(t, resp.Items, tc.wantItems)
			assert.Equal(t, 3, resp.TotalClusters, "total_clusters is the full count")
			assert.Equal(t, tc.wantTrunc, resp.Truncated)
			if tc.wantItems == 0 {
				assert.Contains(t, rec.Body.String(), `"items":[]`, "empty pages must serialize as [] not null")
			}
		})
	}
}

func TestGetSeriesCandidates_SourceResolution(t *testing.T) {
	const sourceID = "0f8c5b5e-9d1a-4c1e-8f1a-6f2b7c8d9e0f"

	repo := new(MockRepository)
	repo.On("ResolveSourceIdentifier", mock.Anything, "tpl-events").Return(sourceID, nil)
	var captured events.SeriesCandidatesQuery
	repo.On("ListSeriesCandidatePairs", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			captured = args.Get(1).(events.SeriesCandidatesQuery)
		}).
		Return(nil, nil)

	h := newTestAdminHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?source=tpl-events", nil)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()
	h.GetSeriesCandidates(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, sourceID, captured.SourceID, "resolved source UUID must reach the storage filter")
	repo.AssertExpectations(t)
}

func TestGetSeriesCandidates_SourceErrors(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantSub string
	}{
		{
			name:    "unknown",
			err:     fmt.Errorf("%w: %q looks like a ULID; expected a source name or UUID", events.ErrSourceNotFound, "01ARZ3NDEKTSV4RRFFQ69G5FAW"),
			wantSub: "expected a source name or UUID",
		},
		{
			name:    "ambiguous",
			err:     fmt.Errorf("%w: %q matches 2 sources; use the sources UUID", events.ErrSourceAmbiguous, "duplicated"),
			wantSub: "use the sources UUID",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			repo.On("ResolveSourceIdentifier", mock.Anything, "tpl-events").Return("", tc.err)

			h := newTestAdminHandler(repo)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?source=tpl-events", nil)
			req = withAdminUser(req, "admin@example.com")
			rec := httptest.NewRecorder()
			h.GetSeriesCandidates(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			assert.Contains(t, rec.Body.String(), tc.wantSub)
			repo.AssertNotCalled(t, "ListSeriesCandidatePairs", mock.Anything, mock.Anything)
		})
	}
}

func TestGetSeriesCandidates_SourceULID400(t *testing.T) {
	const ulidSource = "01ARZ3NDEKTSV4RRFFQ69G5FAW"

	repo := new(MockRepository)
	repo.On("ResolveSourceIdentifier", mock.Anything, ulidSource).
		Return("", fmt.Errorf("%w: %q looks like a ULID; expected a source name or UUID", events.ErrSourceNotFound, ulidSource))

	h := newTestAdminHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/events/series-candidates?source="+ulidSource, nil)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()
	h.GetSeriesCandidates(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "looks like a ULID")
	assert.Contains(t, rec.Body.String(), "expected a source name or UUID")
	repo.AssertNotCalled(t, "ListSeriesCandidatePairs", mock.Anything, mock.Anything)
}
