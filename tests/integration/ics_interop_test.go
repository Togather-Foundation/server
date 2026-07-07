package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
				Now:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
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

// TestICSIngestEventSchedule is the Phase 4 end-to-end integration test:
// parse interop-recurrence-exdate.ics → submit each occurrence via the single-event
// POST API → verify JSON-LD responses contain correct eventSchedule on both the
// single-event and list endpoints, EXDATE is excluded, non-recurring events have
// no eventSchedule, and re-ingesting the same fixture upserts (no duplicates).
func TestICSIngestEventSchedule(t *testing.T) {
	env := setupTestEnv(t)
	apiKey := insertAPIKey(t, env, "ics-ingest-test")

	// ── 1. Parse & map the ICS fixture ────────────────────────────────────────
	fixturePath := filepath.Join(projectRoot(t), "tests", "testdata", "ics", "interop-recurrence-exdate.ics")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "read ICS fixture")

	cal, err := ical.Parse(data)
	require.NoError(t, err, "parse ICS fixture")

	opts := ical.MapperOptions{
		SourceURL:  "https://test.example.com/recurrence-exdate.ics",
		SourceName: "ics-ingest-test",
		TrustLevel: 5,
		License:    "CC0-1.0",
		Timezone:   "America/Toronto",
		Now:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	inputs, warnings, err := ical.MapToEventInputs(context.Background(), cal, opts)
	require.NoError(t, err, "MapToEventInputs")
	require.Empty(t, warnings, "unexpected mapper warnings: %v", warnings)

	// Fixture: RRULE FREQ=WEEKLY;BYDAY=MO,WE with one EXDATE on the first Monday.
	// Within the default 90-day horizon from 2026-06-01: July 8 (Wed), July 13 (Mon),
	// July 15 (Wed), etc. survive. The first Monday (July 6) is excluded by EXDATE.
	require.GreaterOrEqual(t, len(inputs), 2, "expected at least 2 non-excluded occurrences from fixture")

	// ── 2. Verify EXDATE occurrence is absent from mapped inputs ───────────────
	// The EXDATE in the fixture is 2026-07-06T19:00:00 America/Toronto (first Monday).
	exdateWant := "2026-07-06"
	for _, inp := range inputs {
		require.NotContains(t, inp.StartDate, exdateWant,
			"EXDATE occurrence %s must not be present in mapped inputs", exdateWant)
	}

	// ── 3. Submit each occurrence via POST /api/v1/events ─────────────────────
	var createdULIDs []string
	for i, inp := range inputs {
		body, merr := json.Marshal(inp)
		require.NoError(t, merr, "marshal EventInput[%d]", i)

		req, rerr := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, rerr)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Accept", "application/ld+json")

		resp, doErr := env.Server.Client().Do(req)
		require.NoError(t, doErr)
		t.Cleanup(func() { _ = resp.Body.Close() })

		if resp.StatusCode != http.StatusCreated {
			raw, _ := io.ReadAll(resp.Body)
			require.Failf(t, "unexpected status", "input[%d] status=%d body=%s", i, resp.StatusCode, raw)
		}

		var created map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
		uid := eventIDFromPayload(created)
		require.NotEmpty(t, uid, "expected @id ULID in created event[%d]", i)
		createdULIDs = append(createdULIDs, uid)
	}
	require.Len(t, createdULIDs, len(inputs))

	// ── 4. Single-event endpoint: verify eventSchedule fields ─────────────────
	t.Run("single-event-eventSchedule", func(t *testing.T) {
		for i, eventULID := range createdULIDs {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "application/ld+json")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, http.StatusOK, resp.StatusCode, "event[%d] GET status", i)

			var payload map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

			// JSON-LD structure
			require.Equal(t, "Event", payload["@type"], "event[%d] @type", i)
			require.Contains(t, payload, "@context", "event[%d] must have @context", i)

			// eventSchedule presence and fields
			schedule, ok := payload["eventSchedule"]
			require.True(t, ok, "event[%d] must have eventSchedule", i)
			sched := schedule.(map[string]any)

			require.Equal(t, "Schedule", sched["@type"], "event[%d] eventSchedule @type", i)
			require.Equal(t, "P1W", sched["repeatFrequency"], "event[%d] repeatFrequency", i)
			require.Equal(t, "America/Toronto", sched["scheduleTimezone"], "event[%d] scheduleTimezone", i)

			startDate, hasSD := sched["startDate"]
			require.True(t, hasSD, "event[%d] eventSchedule.startDate must be present", i)
			require.NotEmpty(t, startDate, "event[%d] eventSchedule.startDate must not be empty", i)

			// byDay: must contain Monday and Wednesday (order-independent)
			byDayRaw, hasByDay := sched["byDay"]
			require.True(t, hasByDay, "event[%d] eventSchedule.byDay must be present", i)
			byDaySlice, ok := byDayRaw.([]any)
			require.True(t, ok, "event[%d] eventSchedule.byDay must be an array", i)
			var byDayStrs []string
			for _, d := range byDaySlice {
				byDayStrs = append(byDayStrs, fmt.Sprint(d))
			}
			require.Contains(t, byDayStrs, "Monday", "event[%d] byDay must include Monday", i)
			require.Contains(t, byDayStrs, "Wednesday", "event[%d] byDay must include Wednesday", i)
		}
	})

	// ── 5. List endpoint: verify eventSchedule present on recurring events ─────
	t.Run("list-endpoint-eventSchedule", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
		require.NoError(t, err)
		req.Header.Set("Accept", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var listPayload struct {
			Items []map[string]any `json:"items"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listPayload))
		require.GreaterOrEqual(t, len(listPayload.Items), len(inputs),
			"list must contain at least the %d ingested occurrences", len(inputs))

		// All recurring events from the fixture must have eventSchedule.
		createdSet := make(map[string]bool, len(createdULIDs))
		for _, u := range createdULIDs {
			createdSet[u] = true
		}
		for _, item := range listPayload.Items {
			uid := eventIDFromPayload(item)
			if !createdSet[uid] {
				continue
			}
			schedule, ok := item["eventSchedule"]
			require.True(t, ok, "list item %s must have eventSchedule", uid)
			sched := schedule.(map[string]any)
			require.Equal(t, "P1W", sched["repeatFrequency"], "list item %s repeatFrequency", uid)
			require.NotEmpty(t, sched["startDate"], "list item %s eventSchedule.startDate", uid)
		}
	})

	// ── 6. Non-recurring event must NOT have eventSchedule ────────────────────
	t.Run("non-recurring-no-eventSchedule", func(t *testing.T) {
		org := insertOrganization(t, env, "Non-recurring Org")
		place := insertPlace(t, env, "Non-recurring Venue", "Toronto")
		nonRecurULID := insertEventWithOccurrence(t, env, "One-off Concert", org.ID, place.ID, "music", "published", nil, time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC))

		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+nonRecurULID, nil)
		require.NoError(t, err)
		req.Header.Set("Accept", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var payload map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		_, hasSchedule := payload["eventSchedule"]
		require.False(t, hasSchedule, "non-recurring event must not have eventSchedule")
	})

	// ── 7. Upsert idempotency: re-submit same inputs, expect no new events ─────
	t.Run("upsert-idempotency", func(t *testing.T) {
		// Re-submit all inputs — they carry the same Source.EventID so they
		// must upsert (return 200 OK) rather than create (201 Created).
		for i, inp := range inputs {
			body, merr := json.Marshal(inp)
			require.NoError(t, merr, "marshal EventInput[%d] for re-ingest", i)

			req, rerr := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
			require.NoError(t, rerr)
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("Content-Type", "application/ld+json")
			req.Header.Set("Accept", "application/ld+json")

			resp, doErr := env.Server.Client().Do(req)
			require.NoError(t, doErr)
			t.Cleanup(func() { _ = resp.Body.Close() })

			// 409 Conflict = recognised duplicate (upserted existing); 201 = new (wrong — would mean duplicate created)
			require.Equal(t, http.StatusConflict, resp.StatusCode,
				"re-ingesting input[%d] must return 409 (duplicate/upsert), not 201 (new duplicate)", i)

			var upserted map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&upserted))
			require.Equal(t, createdULIDs[i], eventIDFromPayload(upserted),
				"upserted event[%d] must have same ULID as original", i)
		}

		// Confirm the DB still has exactly len(inputs) events for this source.
		sourceEventID := inputs[0].Source.EventID
		// source_id suffix is the composite UID:startDate; the source name prefix is stable
		_ = sourceEventID // we verify via count below

		var count int
		err := env.Pool.QueryRow(env.Context,
			`SELECT COUNT(*) FROM events e
			  JOIN event_sources es ON es.event_id = e.id
			  JOIN sources s ON s.id = es.source_id
			 WHERE s.name = $1`,
			"ics-ingest-test",
		).Scan(&count)
		require.NoError(t, err, "count events for source")
		require.Equal(t, len(inputs), count, "DB must have exactly %d events after re-ingest (no duplicates)", len(inputs))
	})
}

func insertRecurringEventWithSeries(t *testing.T, env *testEnv, name, orgID, venueID string) string {
	t.Helper()

	rrule := "FREQ=WEEKLY;BYDAY=MO,WE"
	tzid := "America/Toronto"
	tomorrow := time.Now().In(time.UTC).Add(24 * time.Hour)
	seriesStart := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
	seriesEnd := seriesStart.AddDate(0, 2, 0)
	exdate := tomorrow.In(mustLoadLocation(t, tzid))

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

	startTime := tomorrow
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
