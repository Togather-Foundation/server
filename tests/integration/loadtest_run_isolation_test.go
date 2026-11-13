package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/Togather-Foundation/server/internal/loadtest"
	"github.com/Togather-Foundation/server/internal/loadtest/fixture"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type loadtestCreatedEvent struct {
	ID            string
	ULID          string
	AttributionID string
	SourceID      string
	Input         events.EventInput
}

// Bridge the real runner's ownership-response parser to the real API and then
// clean its preflight and sustained fixtures with the same capability.
func TestLoadtestRunIsolation_RealRunnerOwnershipPreflight(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "real-loadtest-runner")
	run := newLoadtestManifest(t, env)
	stats, err := loadtest.NewLoadTester(env.Server.URL).WithAPIKeys([]string{key}).WithFixtureRun(run).
		RunCustom(env.Context, loadtest.ProfileConfig{RequestsPerSecond: 30, Duration: 300 * time.Millisecond, ReadWriteRatio: 0})
	require.NoError(t, err)
	summary := stats.Summary()
	require.True(t, summary.Passed)
	require.EqualValues(t, 1, summary.Endpoints["ownership_create"].StatusCodes[http.StatusCreated])
	require.EqualValues(t, 1, summary.Endpoints["ownership_verify"].StatusCodes[http.StatusOK])
	writes := summary.Endpoints["create_event"].StatusCodes[http.StatusCreated]
	require.Positive(t, writes, "sustained fixture writes must start after ownership proof")
	repo := postgres.NewEventRepository(env.Pool, zerolog.Nop())
	preview, err := repo.CleanupLoadtestRun(env.Context, run, false)
	require.NoError(t, err)
	require.EqualValues(t, writes+1, preview.EventCount, "cleanup includes both ownership probe and sustained writes")
	applied, err := repo.CleanupLoadtestRun(env.Context, run, true)
	require.NoError(t, err)
	require.ElementsMatch(t, preview.EventIDs, applied.EventIDs)
	require.Zero(t, countLoadtestRows(t, env, "events"))
}

// Exercise the public ingestion and provenance routes rather than assuming that
// either the key's source or the submitted source became the stored attribution.
func TestLoadtestRunIsolation_ConcurrentRunsAndCleanup(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "loadtest-isolation")
	fallbackID := insertSource(t, env, "unrelated key fallback", "api", "CC0", 5)
	_, err := env.Pool.Exec(env.Context, `UPDATE api_keys SET source_id=$1`, fallbackID)
	require.NoError(t, err)
	a := newLoadtestManifest(t, env)
	b := newLoadtestManifest(t, env)
	require.NotEqual(t, a.RunID, b.RunID)
	require.NotEqual(t, a.SourceURL, b.SourceURL)

	// Concurrent requests use independent run IDs and per-run event IDs. Decode
	// errors in this goroutine are returned to the test's own goroutine.
	inputs := make([]events.EventInput, 4)
	for i, run := range []fixture.Manifest{a, b, a, b} {
		inputs[i], err = run.Event(uint64(i/2 + 1))
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	ulids := make([]string, len(inputs))
	errs := make([]error, len(inputs))
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ulids[i], errs[i] = postLoadtestInput(env, key, inputs[i])
		}(i)
	}
	wg.Wait()
	created := make([]loadtestCreatedEvent, len(inputs))
	for i := range inputs {
		require.NoError(t, errs[i])
		created[i] = lookupLoadtestEvent(t, env, ulids[i], inputs[i])
		require.NotEqual(t, fallbackID, created[i].SourceID,
			"explicit source URL takes precedence over the API-key fallback")
		assertLoadtestPublicAttribution(t, env, created[i])
	}
	require.Equal(t, created[0].SourceID, created[2].SourceID)
	require.Equal(t, created[1].SourceID, created[3].SourceID)
	require.NotEqual(t, created[0].SourceID, created[1].SourceID)
	require.NotEqual(t, created[0].Input.Source.EventID, created[2].Input.Source.EventID)
	for _, table := range []string{"places", "organizations"} {
		require.Zero(t, countLoadtestRows(t, env, table), "virtual fixtures must not create shared entities")
	}

	// Virtual fixture creation must not leave jobs which will retry against a
	// missing event after cleanup. A real physical event still gets enrichment.
	for _, event := range created {
		var jobs int
		require.NoError(t, env.Pool.QueryRow(env.Context,
			`SELECT count(*) FROM river_job WHERE args->>'event_id'=$1`, event.ULID).Scan(&jobs))
		require.Zero(t, jobs, "virtual fixtures must not enqueue event geocoding")
	}
	real := createLoadtestPhysicalControl(t, env, key)
	var geocodeJobs int
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT count(*) FROM river_job WHERE kind='geocode_event' AND args->>'event_id'=$1`, real.ULID).Scan(&geocodeJobs))
	require.Equal(t, 1, geocodeJobs, "physical events retain geocoding")
	placeCount, orgCount := countLoadtestRows(t, env, "places"), countLoadtestRows(t, env, "organizations")

	// Source rows can acquire config/operator metadata without giving cleanup
	// permission to delete it. This slice retains every source row.
	_, err = env.Pool.Exec(env.Context,
		`UPDATE sources SET notes='operator note',config='{"keep":true}'::jsonb WHERE id=$1`, created[0].SourceID)
	require.NoError(t, err)
	config, err := postgres.NewScraperSourceRepository(env.Pool).Upsert(env.Context, scraper.UpsertParams{
		Name: "protected fixture registry", URL: a.SourceURL, Schedule: "manual", TrustLevel: 5,
		License: "CC0-1.0", MaxPages: 1, Enabled: false,
	})
	require.NoError(t, err)

	repo := postgres.NewEventRepository(env.Pool, zerolog.Nop())
	before := countLoadtestRows(t, env, "events")
	preview, err := repo.CleanupLoadtestRun(env.Context, a, false)
	require.NoError(t, err)
	require.False(t, preview.Applied)
	require.Equal(t, a.RunID, preview.RunID)
	require.Equal(t, 2, preview.EventCount)
	require.ElementsMatch(t, []string{created[0].ID, created[2].ID}, preview.EventIDs)
	require.ElementsMatch(t, []string{created[0].ULID, created[2].ULID}, preview.EventULIDs)
	require.ElementsMatch(t, []string{created[0].AttributionID, created[2].AttributionID}, preview.AttributionIDs)
	require.Equal(t, []string{created[0].SourceID}, preview.SourceIDs)
	require.EqualValues(t, 2, preview.ChangeCount)
	require.Equal(t, before, countLoadtestRows(t, env, "events"), "preview must not mutate")

	applied, err := repo.CleanupLoadtestRun(env.Context, a, true)
	require.NoError(t, err)
	require.True(t, applied.Applied)
	require.ElementsMatch(t, preview.EventIDs, applied.EventIDs)
	require.Equal(t, before-2, countLoadtestRows(t, env, "events"))
	for _, table := range []string{"events", "event_occurrences", "event_sources", "field_provenance", "event_review_queue", "event_changes"} {
		var remaining int
		column := "event_id"
		if table == "events" {
			column = "id"
		}
		require.NoError(t, env.Pool.QueryRow(env.Context,
			fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s=ANY($1::uuid[])`, table, column), preview.EventIDs).Scan(&remaining))
		require.Zero(t, remaining, table+" must not retain owned fixture rows")
	}
	for _, control := range []loadtestCreatedEvent{created[1], created[3], real} {
		var exists bool
		require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT EXISTS(SELECT 1 FROM events WHERE id=$1)`, control.ID).Scan(&exists))
		require.True(t, exists, "other runs and real events must survive")
	}
	require.Equal(t, placeCount, countLoadtestRows(t, env, "places"))
	require.Equal(t, orgCount, countLoadtestRows(t, env, "organizations"))
	var notes string
	var sourceConfig []byte
	require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT notes,config FROM sources WHERE id=$1`, created[0].SourceID).Scan(&notes, &sourceConfig))
	require.Equal(t, "operator note", notes)
	require.JSONEq(t, `{"keep":true}`, string(sourceConfig))
	storedConfig, err := postgres.NewScraperSourceRepository(env.Pool).GetByName(env.Context, config.Name)
	require.NoError(t, err)
	require.Equal(t, config.ID, storedConfig.ID)
	second, err := repo.CleanupLoadtestRun(env.Context, a, true)
	require.NoError(t, err)
	require.Zero(t, second.EventCount, "second cleanup is a no-op")
	require.Empty(t, second.EventIDs)
}

func TestLoadtestRunIsolation_RefusesAmbiguousOwnership(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *testEnv, loadtestCreatedEvent, loadtestCreatedEvent, string)
	}{
		{"shared attribution", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `INSERT INTO event_sources(event_id,source_id,source_url,payload,payload_hash) VALUES($1,$2,'https://foreign.example.org/feed','{}','foreign')`, owned.ID, foreign)
			require.NoError(t, err)
		}},
		{"foreign field provenance", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `INSERT INTO field_provenance(event_id,field_path,value_hash,source_id,confidence) VALUES($1,'name','foreign',$2,1)`, owned.ID, foreign)
			require.NoError(t, err)
		}},
		{"same-source field provenance", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `INSERT INTO field_provenance(event_id,field_path,value_hash,source_id,confidence) VALUES($1,'name','operator-edit',$2,1)`, owned.ID, owned.SourceID)
			require.NoError(t, err)
		}},
		{"operator update with null actor", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `UPDATE events SET description='operator correction' WHERE id=$1`, owned.ID)
			require.NoError(t, err)
		}},
		{"wrong source UUID", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `UPDATE event_sources SET source_id=$2 WHERE event_id=$1`, owned.ID, foreign)
			require.NoError(t, err)
		}},
		{"altered signed payload", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `UPDATE event_sources SET payload=jsonb_set(payload,'{description}','"not the signed fixture"') WHERE event_id=$1`, owned.ID)
			require.NoError(t, err)
		}},
		{"external review reference", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `INSERT INTO event_review_queue(event_id,original_payload,normalized_payload,warnings,event_start_time,duplicate_of_event_id) VALUES($1,'{}','{}','[]',$2,$3)`, real.ID, testhelpers.FutureTime(7), owned.ID)
			require.NoError(t, err)
		}},
		{"idempotency reference", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `INSERT INTO idempotency_keys(key,request_hash,event_id,event_ulid) VALUES('protected-request','fixture',$1,$2)`, owned.ID, owned.ULID)
			require.NoError(t, err)
		}},
		{"operator reviewed fixture", func(t *testing.T, env *testEnv, owned, real loadtestCreatedEvent, foreign string) {
			_, err := env.Pool.Exec(env.Context, `INSERT INTO event_review_queue(event_id,original_payload,normalized_payload,warnings,event_start_time,status,reviewed_by,reviewed_at) VALUES($1,'{}','{}','[]',$2,'approved','operator',now())`, owned.ID, testhelpers.FutureTime(7))
			require.NoError(t, err)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestEnv(t)
			key := insertAPIKey(t, env, "guarded-loadtest")
			run := newLoadtestManifest(t, env)
			owned := createLoadtestFixture(t, env, key, run, 1)
			// A second clean fixture proves refusal is all-or-nothing rather than
			// silently deleting the safe subset and leaving ambiguous records.
			clean := createLoadtestFixture(t, env, key, run, 2)
			real := createLoadtestPhysicalControl(t, env, key)
			foreign := insertSource(t, env, "foreign attribution", "api", "CC0", 5)
			tt.mutate(t, env, owned, real, foreign)
			before := loadtestDatabaseCounts(t, env)
			for _, apply := range []bool{false, true} {
				_, err := postgres.NewEventRepository(env.Pool, zerolog.Nop()).CleanupLoadtestRun(env.Context, run, apply)
				require.Error(t, err, "ambiguous ownership must refuse preview and deletion")
				require.Equal(t, before, loadtestDatabaseCounts(t, env), "refusal must preserve every related row")
			}
			var survives bool
			require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT EXISTS(SELECT 1 FROM events WHERE id=$1)`, clean.ID).Scan(&survives))
			require.True(t, survives, "safe subset must survive a refused run")
		})
	}
}

func TestLoadtestRunIsolation_RollsBackRelatedDeletion(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "rollback-loadtest")
	run := newLoadtestManifest(t, env)
	owned := createLoadtestFixture(t, env, key, run, 1)
	_, err := env.Pool.Exec(env.Context, `
CREATE FUNCTION integration_refuse_loadtest_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'loadtest rollback proof';
END;
$$;
CREATE TRIGGER integration_refuse_loadtest_delete BEFORE DELETE ON events
FOR EACH ROW EXECUTE FUNCTION integration_refuse_loadtest_delete();`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := env.Pool.Exec(env.Context, `DROP TRIGGER IF EXISTS integration_refuse_loadtest_delete ON events; DROP FUNCTION IF EXISTS integration_refuse_loadtest_delete();`)
		require.NoError(t, cleanupErr)
	})
	before := loadtestDatabaseCounts(t, env)
	_, err = postgres.NewEventRepository(env.Pool, zerolog.Nop()).CleanupLoadtestRun(env.Context, run, true)
	require.ErrorContains(t, err, "loadtest rollback proof")
	require.Equal(t, before, loadtestDatabaseCounts(t, env), "failure after deleting owned changes must roll back every mutation")
	for _, table := range []string{"events", "event_sources", "event_changes"} {
		column := "event_id"
		if table == "events" {
			column = "id"
		}
		var remaining int
		require.NoError(t, env.Pool.QueryRow(env.Context,
			fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s=$1`, table, column), owned.ID).Scan(&remaining))
		require.Equal(t, 1, remaining, "owned event, attribution and create history must survive rollback")
	}
}

func TestLoadtestRunIsolation_InFlightForeignWriter(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "concurrent-loadtest")
	run := newLoadtestManifest(t, env)
	owned := createLoadtestFixture(t, env, key, run, 1)
	foreign := insertSource(t, env, "in-flight foreign source", "api", "CC0", 5)
	writer, err := env.Pool.Begin(env.Context)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback(context.Background()) }()
	var writerPID int
	require.NoError(t, writer.QueryRow(env.Context, `SELECT pg_backend_pid()`).Scan(&writerPID))
	_, err = writer.Exec(env.Context, `INSERT INTO event_sources(event_id,source_id,source_url,payload,payload_hash) VALUES($1,$2,'https://concurrent.example.org/feed','{}','in-flight')`, owned.ID, foreign)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(env.Context, 10*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, cleanupErr := postgres.NewEventRepository(env.Pool, zerolog.Nop()).CleanupLoadtestRun(ctx, run, true)
		result <- cleanupErr
	}()
	// Observe an actual PostgreSQL lock wait rather than relying on a sleep or
	// hoping the goroutines happened to interleave around validation.
	require.Eventually(t, func() bool {
		var blocked int
		if err := env.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid))`, writerPID).Scan(&blocked); err != nil {
			return false
		}
		return blocked > 0
	}, 5*time.Second, 10*time.Millisecond, "cleanup must wait for the in-flight attribution writer")
	require.NoError(t, writer.Commit(env.Context))
	require.Error(t, <-result, "committed foreign attribution must be seen before any deletion")
	var eventsRemaining, attributionsRemaining int
	require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT count(*) FROM events WHERE id=$1`, owned.ID).Scan(&eventsRemaining))
	require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT count(*) FROM event_sources WHERE event_id=$1`, owned.ID).Scan(&attributionsRemaining))
	require.Equal(t, 1, eventsRemaining)
	require.Equal(t, 2, attributionsRemaining)
}

func newLoadtestManifest(t *testing.T, env *testEnv) fixture.Manifest {
	t.Helper()
	run, err := fixture.New(env.Server.URL, time.Now())
	require.NoError(t, err)
	return run
}

func postLoadtestInput(env *testEnv, key string, input events.EventInput) (string, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")
	resp, err := env.Server.Client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create fixture: status %d: %v", resp.StatusCode, payload)
	}
	if ulid := eventIDFromPayload(payload); ulid != "" {
		return ulid, nil
	}
	return "", fmt.Errorf("create fixture returned no event identity")
}

func createLoadtestFixture(t *testing.T, env *testEnv, key string, run fixture.Manifest, seq uint64) loadtestCreatedEvent {
	t.Helper()
	input, err := run.Event(seq)
	require.NoError(t, err)
	ulid, err := postLoadtestInput(env, key, input)
	require.NoError(t, err)
	return lookupLoadtestEvent(t, env, ulid, input)
}

func lookupLoadtestEvent(t *testing.T, env *testEnv, ulid string, input events.EventInput) loadtestCreatedEvent {
	t.Helper()
	created := loadtestCreatedEvent{ULID: ulid, Input: input}
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT e.id::text,es.id::text,es.source_id::text FROM events e JOIN event_sources es ON es.event_id=e.id WHERE e.ulid=$1`, ulid).
		Scan(&created.ID, &created.AttributionID, &created.SourceID))
	return created
}

func createLoadtestPhysicalControl(t *testing.T, env *testEnv, key string) loadtestCreatedEvent {
	t.Helper()
	input := events.EventInput{
		Name: "Existing real community event", Description: "A real community concert at a physical venue that fixture cleanup must preserve.",
		StartDate: testhelpers.FutureTime(10).Format(time.RFC3339),
		Location:  &events.PlaceInput{Name: "Existing community hall", AddressLocality: "Toronto", AddressRegion: "ON"},
		Organizer: &events.OrganizationInput{Name: "Existing community organization"},
		Source:    &events.SourceInput{URL: "https://community.example.org/calendar", EventID: "real-community-event", Name: "Community calendar"},
		License:   "https://creativecommons.org/publicdomain/zero/1.0/",
	}
	ulid, err := postLoadtestInput(env, key, input)
	require.NoError(t, err)
	return lookupLoadtestEvent(t, env, ulid, input)
}

func assertLoadtestPublicAttribution(t *testing.T, env *testEnv, created loadtestCreatedEvent) {
	t.Helper()
	resp, err := env.Server.Client().Get(env.Server.URL + "/api/v1/events/" + created.ULID + "?include_provenance=true")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var payload struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Sources []struct {
			URL     string `json:"url"`
			EventID string `json:"sel:sourceEventId"`
		} `json:"sources"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, created.Input.Name, payload.Name)
	require.Equal(t, created.Input.URL, payload.URL)
	require.Len(t, payload.Sources, 1)
	require.Equal(t, created.Input.Source.URL, payload.Sources[0].URL)
	require.Equal(t, created.Input.Source.EventID, payload.Sources[0].EventID)
}

// Call sites pass only fixed, test-owned table names; values remain parameters.
func countLoadtestRows(t *testing.T, env *testEnv, table string) int {
	t.Helper()
	var n int
	require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT count(*) FROM `+table).Scan(&n))
	return n
}

func loadtestDatabaseCounts(t *testing.T, env *testEnv) map[string]int {
	t.Helper()
	tables := []string{"events", "event_occurrences", "event_sources", "field_provenance", "event_review_queue", "event_changes", "sources", "api_keys", "scraper_sources", "places", "organizations"}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		counts[table] = countLoadtestRows(t, env, table)
	}
	return counts
}
