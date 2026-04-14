package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/ical"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

type interopIngestCase struct {
	Name              string
	FixturePath       string
	ExpectedMinEvents int
	ExpectWarnings    bool
	ExpectRecurrence  bool
	MatrixRow         string
}

func TestICSInteropIngest(t *testing.T) {
	fixtureDir := filepath.Join(projectRoot(t), "tests", "testdata", "ics")

	cases := []interopIngestCase{
		{
			Name:              "outlook-vtimezone",
			FixturePath:       "interop-outlook-vtimezone.ics",
			ExpectedMinEvents: 2,
			ExpectWarnings:    false,
			MatrixRow:         "Strict ICS parser target",
		},
		{
			Name:              "tribe-ical",
			FixturePath:       "interop-tribe-ical.ics",
			ExpectedMinEvents: 3,
			ExpectWarnings:    false,
			MatrixRow:         "Community-calendar-style consumer",
		},
		{
			Name:              "google-basic",
			FixturePath:       "interop-google-basic.ics",
			ExpectedMinEvents: 2,
			ExpectWarnings:    false,
			MatrixRow:         "Google Calendar smoke target",
		},
		{
			Name:              "meetup",
			FixturePath:       "interop-meetup.ics",
			ExpectedMinEvents: 2,
			ExpectWarnings:    false,
			MatrixRow:         "Community-calendar-style consumer",
		},
		{
			Name:              "tockify",
			FixturePath:       "interop-tockify.ics",
			ExpectedMinEvents: 2,
			ExpectWarnings:    false,
			MatrixRow:         "Community-calendar-style consumer",
		},
		{
			Name:              "recurrence-exdate",
			FixturePath:       "interop-recurrence-exdate.ics",
			ExpectedMinEvents: 1,
			ExpectRecurrence:  true,
			MatrixRow:         "Strict ICS parser target",
		},
		{
			Name:              "mixed-malformed",
			FixturePath:       "interop-mixed-malformed.ics",
			ExpectedMinEvents: 2,
			ExpectWarnings:    true,
			MatrixRow:         "Community-calendar-style consumer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(fixtureDir, tc.FixturePath))
			require.NoError(t, err, "read fixture %s", tc.FixturePath)

			cal, err := ical.Parse(data)
			require.NoError(t, err, "Parse fixture %s", tc.FixturePath)

			opts := ical.MapperOptions{
				SourceURL:  "https://test.example.com/feed.ics",
				SourceName: "Test",
				TrustLevel: 1,
				License:    "CC0-1.0",
			}

			results, warnings, err := ical.MapToEventInputs(context.Background(), cal, opts)
			require.NoError(t, err, "MapToEventInputs for %s", tc.FixturePath)

			// Matrix: reference row for traceability
			t.Logf("Matrix: %s", tc.MatrixRow)

			require.GreaterOrEqual(t, len(results), tc.ExpectedMinEvents,
				"expected at least %d events from %s, got %d", tc.ExpectedMinEvents, tc.FixturePath, len(results))

			allWarnings := append([]string{}, cal.Warnings...)
			allWarnings = append(allWarnings, warnings...)
			if tc.ExpectWarnings {
				require.NotEmpty(t, allWarnings, "expected warnings for %s", tc.FixturePath)
			} else {
				require.Empty(t, allWarnings,
					"unexpected warnings from %s: %v", tc.FixturePath, allWarnings)
			}

			if tc.ExpectRecurrence {
				require.NotEmpty(t, cal.Events, "fixture %s must have at least one parsed event", tc.FixturePath)

				// Fixture: FREQ=WEEKLY;BYDAY=MO,WE starting 2026-07-06 with 90-day default horizon.
				// The EXDATE removes 2026-07-06 (Monday); at least one occurrence (Wednesday 2026-07-08)
				// must remain within the window.
				require.GreaterOrEqual(t, len(results), 1,
					"recurrence fixture %s should expand to at least one non-excluded occurrence", tc.FixturePath)

				// Verify the EXDATE (2026-07-06T19:00:00 America/Toronto) is excluded:
				// that specific datetime must NOT appear in the mapped results.
				exdateParsed := cal.Events[0].ExDates
				require.NotEmpty(t, exdateParsed,
					"EXDATE should be parsed from %s", tc.FixturePath)

				excludedDate := exdateParsed[0]
				for _, ev := range results {
					if ev.StartDate != "" {
						require.NotEqual(t, excludedDate.Format(time.RFC3339), ev.StartDate,
							"excluded date %s must not appear in mapped results from %s",
							excludedDate.Format(time.RFC3339), tc.FixturePath)
					}
				}
			}
		})
	}
}

type exportExpectation struct {
	Name              string
	IncludeRRule      bool
	RequireRRULE      bool
	ForbidRRULE       bool
	RequireEXDATE     bool
	EXDATENoTrailingZ bool
	MatrixRow         string
}

func TestICSInteropExport(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Interop Test Org")
	place := insertPlace(t, env, "Interop Test Venue", "Toronto")
	eventULID := insertRecurringEventWithSeries(t, env, "Weekly Workshop", org.ID, place.ID)

	cases := []exportExpectation{
		{
			Name:         "IncludeRRule-false",
			IncludeRRule: false,
			ForbidRRULE:  true,
			MatrixRow:    "Strict ICS parser target",
		},
		{
			Name:              "IncludeRRule-true",
			IncludeRRule:      true,
			RequireRRULE:      true,
			RequireEXDATE:     true,
			EXDATENoTrailingZ: true,
			MatrixRow:         "Strict ICS parser target",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			url := env.Server.URL + "/api/v1/events.ics"
			if tc.IncludeRRule {
				url += "?include_rrule=true"
			}

			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "text/calendar; charset=utf-8", resp.Header.Get("Content-Type"))

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Matrix: reference row for traceability
			t.Logf("Matrix: %s", tc.MatrixRow)

			if tc.ForbidRRULE {
				require.False(t, bytes.Contains(body, []byte("RRULE:")),
					"RRULE must not appear when IncludeRRule=false")
				require.False(t, bytes.Contains(body, []byte("EXDATE")),
					"EXDATE must not appear when IncludeRRule=false")
			}

			if tc.RequireRRULE {
				require.True(t, bytes.Contains(body, []byte("RRULE:")),
					"RRULE must appear when IncludeRRule=true")
			}

			if tc.RequireEXDATE {
				require.True(t, bytes.Contains(body, []byte("EXDATE")),
					"EXDATE must appear when IncludeRRule=true and exdates are set")
			}

			if tc.EXDATENoTrailingZ {
				// RFC 5545 §3.3.5: When TZID is set, EXDATE values must be local time (no trailing Z).
				// This is a regression guard — assert unconditionally that EXDATE is present.
				exdateIdx := bytes.Index(body, []byte("EXDATE"))
				require.GreaterOrEqual(t, exdateIdx, 0,
					"EXDATENoTrailingZ=true but EXDATE not found in export body for %s", tc.Name)
				lineEnd := bytes.IndexByte(body[exdateIdx:], '\n')
				exdateLine := body[exdateIdx:]
				if lineEnd >= 0 {
					exdateLine = body[exdateIdx : exdateIdx+lineEnd]
				}
				require.Contains(t, string(exdateLine), "TZID=America/Toronto",
					"EXDATE must include TZID when timezone is set")
				require.NotContains(t, string(exdateLine), "T190000Z",
					"EXDATE with TZID must not have trailing Z (RFC 5545 §3.3.5)")
			}
		})
	}

	t.Run("eventSchedule-jsonld", func(t *testing.T) {
		// Matrix: Community-calendar-style consumer
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
		require.NoError(t, err)
		req.Header.Set("Accept", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var payload map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

		schedule, ok := payload["eventSchedule"]
		require.True(t, ok, "eventSchedule must be present for recurring event with series")
		scheduleMap, ok := schedule.(map[string]any)
		require.True(t, ok, "eventSchedule must be a JSON object")

		_, hasStartDate := scheduleMap["startDate"]
		require.True(t, hasStartDate, "eventSchedule must include startDate")

		_, hasEndDate := scheduleMap["endDate"]
		require.True(t, hasEndDate, "eventSchedule must include endDate")

		t.Logf("Matrix: Community-calendar-style consumer")
	})
}

func insertRecurringEventWithSeries(t *testing.T, env *testEnv, name, orgID, venueID string) string {
	t.Helper()

	rrule := "FREQ=WEEKLY;BYDAY=MO,WE"
	tzid := "America/Toronto"
	seriesStart := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	seriesEnd := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	exdate := time.Date(2026, 7, 6, 19, 0, 0, 0, mustLoadLocation(t, tzid))

	var seriesID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO event_series (name, series_start_date, series_end_date, schedule_timezone, rrule, exdates)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		name+" Series",
		seriesStart.Format("2006-01-02"),
		seriesEnd.Format("2006-01-02"),
		tzid,
		rrule,
		[]time.Time{exdate},
	).Scan(&seriesID)
	require.NoError(t, err, "insert event_series")

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, organizer_id, primary_venue_id, event_domain, lifecycle_state, series_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::uuid)
		 RETURNING id`,
		eventULID, name, orgID, venueID, "arts", "published", seriesID,
	).Scan(&eventID)
	require.NoError(t, err, "insert event linked to series")

	startTime := time.Date(2026, 7, 6, 19, 0, 0, 0, time.UTC)
	endTime := startTime.Add(2 * time.Hour)
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
		 VALUES ($1, $2, $3, $4)`,
		eventID, startTime, endTime, venueID,
	)
	require.NoError(t, err, "insert event_occurrence")

	return eventULID
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err, "load timezone %s", name)
	return loc
}
