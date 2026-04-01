package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// ----------------------------------------------------------------------------
// Fakes / stubs
// ----------------------------------------------------------------------------

// fakeScraperQueries is a test double for the postgres.Queries subset used by
// AdminScraperHandler. Only the methods exercised in this file are implemented.
type fakeScraperQueries struct {
	// ListScraperSourcesWithLatestRun
	sourcesRows []postgres.ListScraperSourcesWithLatestRunRow
	sourcesErr  error

	// ListScraperRunsBySource
	runsRows []postgres.ScraperRun
	runsErr  error

	// GetScraperSourceByName (used by SetSourceEnabled)
	getSourceRow postgres.GetScraperSourceByNameRow
	getSourceErr error

	// UpsertScraperSource (used by SetSourceEnabled)
	upsertRow postgres.UpsertScraperSourceRow
	upsertErr error

	// GetScraperConfig / SetScraperConfig
	configRow    postgres.ScraperConfig
	configGetErr error
	configSetErr error

	countRunningScraperRunsValue int64
	countRunningScraperRunsErr   error

	// GetLatestScraperRunBySource
	latestRunRow postgres.ScraperRun
	latestRunErr error

	// GetLastSuccessfulRunBySource
	lastSuccessRow postgres.ScraperRun
	lastSuccessErr error

	// ListRecentScraperRunsFiltered
	filteredRunsRows []postgres.ScraperRun
	filteredRunsErr  error
}

// fakeRiverInserter is a test double for scraperJobInserter.
type fakeRiverInserter struct {
	err         error
	insertedArg jobs.ScrapeOrchestratorArgs
	called      bool
}

func (f *fakeRiverInserter) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.called = true
	if argsTyped, ok := args.(jobs.ScrapeOrchestratorArgs); ok {
		f.insertedArg = argsTyped
	}
	if f.err != nil {
		return nil, f.err
	}
	return &rivertype.JobInsertResult{
		Job: &rivertype.JobRow{ID: 12345},
	}, nil
}

func (f *fakeScraperQueries) ListScraperSourcesWithLatestRun(_ context.Context, _ pgtype.Bool) ([]postgres.ListScraperSourcesWithLatestRunRow, error) {
	return f.sourcesRows, f.sourcesErr
}

func (f *fakeScraperQueries) ListScraperRunsBySource(_ context.Context, _ postgres.ListScraperRunsBySourceParams) ([]postgres.ScraperRun, error) {
	return f.runsRows, f.runsErr
}

func (f *fakeScraperQueries) GetScraperSourceByName(_ context.Context, _ string) (postgres.GetScraperSourceByNameRow, error) {
	return f.getSourceRow, f.getSourceErr
}

func (f *fakeScraperQueries) UpsertScraperSource(_ context.Context, _ postgres.UpsertScraperSourceParams) (postgres.UpsertScraperSourceRow, error) {
	return f.upsertRow, f.upsertErr
}

func (f *fakeScraperQueries) GetScraperConfig(_ context.Context) (postgres.ScraperConfig, error) {
	return f.configRow, f.configGetErr
}

func (f *fakeScraperQueries) SetScraperConfig(_ context.Context, _ postgres.SetScraperConfigParams) error {
	return f.configSetErr
}

func (f *fakeScraperQueries) CountRunningScraperRuns(_ context.Context) (int64, error) {
	if f.countRunningScraperRunsErr != nil {
		return 0, f.countRunningScraperRunsErr
	}
	return f.countRunningScraperRunsValue, nil
}

func (f *fakeScraperQueries) GetLatestScraperRunBySource(_ context.Context, _ string) (postgres.ScraperRun, error) {
	return f.latestRunRow, f.latestRunErr
}

func (f *fakeScraperQueries) GetLastSuccessfulRunBySource(_ context.Context, _ string) (postgres.ScraperRun, error) {
	return f.lastSuccessRow, f.lastSuccessErr
}

func (f *fakeScraperQueries) ListRecentScraperRunsFiltered(_ context.Context, _ postgres.ListRecentScraperRunsFilteredParams) ([]postgres.ScraperRun, error) {
	return f.filteredRunsRows, f.filteredRunsErr
}

// ----------------------------------------------------------------------------
// Helper to build a handler under test
// ----------------------------------------------------------------------------
// Helper to build a handler under test
// ----------------------------------------------------------------------------

func newTestScraperHandler(q scraperQueriesIface) *AdminScraperHandler {
	return &AdminScraperHandler{
		Queries: q,
		Logger:  zerolog.Nop(),
		Env:     "test",
	}
}

// nowTs returns a valid pgtype.Timestamptz for use in test fixtures.
func nowTs() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_ListSources
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_ListSources(t *testing.T) {
	t.Parallel()

	makeRow := func(name string) postgres.ListScraperSourcesWithLatestRunRow {
		return postgres.ListScraperSourcesWithLatestRunRow{
			ID:                  1,
			Name:                name,
			Url:                 "https://example.com",
			Tier:                1,
			Enabled:             true,
			Schedule:            "daily",
			License:             "CC0",
			LastRunStatus:       "completed",
			LastRunStartedAt:    nowTs(),
			LastRunCompletedAt:  nowTs(),
			LastRunEventsFound:  10,
			LastRunEventsNew:    5,
			LastRunEventsDup:    3,
			LastRunEventsFailed: 2,
		}
	}

	tests := []struct {
		name         string
		rows         []postgres.ListScraperSourcesWithLatestRunRow
		dbErr        error
		wantStatus   int
		wantItemsLen int
	}{
		{
			name:         "returns sources with items",
			rows:         []postgres.ListScraperSourcesWithLatestRunRow{makeRow("source-a"), makeRow("source-b")},
			wantStatus:   http.StatusOK,
			wantItemsLen: 2,
		},
		{
			name:         "returns empty list when no sources",
			rows:         []postgres.ListScraperSourcesWithLatestRunRow{},
			wantStatus:   http.StatusOK,
			wantItemsLen: 0,
		},
		{
			name:       "returns 500 on db error",
			dbErr:      errStubNotImplemented,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				sourcesRows: tc.rows,
				sourcesErr:  tc.dbErr,
			}
			h := newTestScraperHandler(q)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/sources", nil)
			w := httptest.NewRecorder()
			h.ListSources(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body struct {
					Items []scraperSourceResponse `json:"items"`
				}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Len(t, body.Items, tc.wantItemsLen)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_ListSourceRuns
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_ListSourceRuns(t *testing.T) {
	t.Parallel()

	makeRun := func(status string) postgres.ScraperRun {
		return postgres.ScraperRun{
			ID:          1,
			SourceName:  "test-source",
			SourceUrl:   "https://example.com",
			Tier:        1,
			StartedAt:   nowTs(),
			CompletedAt: nowTs(),
			Status:      status,
			EventsFound: 10,
			EventsNew:   5,
		}
	}

	tests := []struct {
		name         string
		sourceName   string
		rows         []postgres.ScraperRun
		dbErr        error
		wantStatus   int
		wantItemsLen int
	}{
		{
			name:         "returns runs for a known source",
			sourceName:   "test-source",
			rows:         []postgres.ScraperRun{makeRun("completed"), makeRun("failed")},
			wantStatus:   http.StatusOK,
			wantItemsLen: 2,
		},
		{
			name:         "returns empty list for unknown source (no rows)",
			sourceName:   "unknown-source",
			rows:         []postgres.ScraperRun{},
			wantStatus:   http.StatusOK,
			wantItemsLen: 0,
		},
		{
			name:       "returns 400 for missing name param",
			sourceName: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 500 on db error",
			sourceName: "test-source",
			dbErr:      errStubNotImplemented,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				runsRows: tc.rows,
				runsErr:  tc.dbErr,
			}
			h := newTestScraperHandler(q)

			var path string
			if tc.sourceName != "" {
				path = "/api/v1/admin/scraper/sources/" + tc.sourceName + "/runs"
			} else {
				path = "/api/v1/admin/scraper/sources//runs"
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)

			// Inject path value via ServeMux pattern matching or manual injection
			if tc.sourceName != "" {
				req.SetPathValue("name", tc.sourceName)
			}

			w := httptest.NewRecorder()
			h.ListSourceRuns(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body struct {
					Items []scraperRunResponse `json:"items"`
				}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Len(t, body.Items, tc.wantItemsLen)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_TriggerScrape
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_TriggerScrape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sourceName   string
		getSourceRow postgres.GetScraperSourceByNameRow
		getSourceErr error
		wantStatus   int
	}{
		{
			name:       "returns 400 when name is missing",
			sourceName: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:         "returns 503 when RiverClient is nil (not configured)",
			sourceName:   "my-source",
			getSourceRow: postgres.GetScraperSourceByNameRow{Enabled: true},
			wantStatus:   http.StatusServiceUnavailable,
		},
		{
			name:         "returns 404 when source does not exist",
			sourceName:   "unknown-source",
			getSourceErr: pgx.ErrNoRows,
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "returns 409 when source is disabled",
			sourceName:   "disabled-source",
			getSourceRow: postgres.GetScraperSourceByNameRow{Enabled: false},
			wantStatus:   http.StatusConflict,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				getSourceRow: tc.getSourceRow,
				getSourceErr: tc.getSourceErr,
			}
			// RiverClient is always nil in these tests — hits 503 after enabled check.
			h := newTestScraperHandler(q)

			var path string
			if tc.sourceName != "" {
				path = "/api/v1/admin/scraper/sources/" + tc.sourceName + "/trigger"
			} else {
				path = "/api/v1/admin/scraper/sources//trigger"
			}
			req := httptest.NewRequest(http.MethodPost, path, nil)
			if tc.sourceName != "" {
				req.SetPathValue("name", tc.sourceName)
			}

			w := httptest.NewRecorder()
			h.TriggerScrape(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_TriggerScrape_WithRiver
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_TriggerScrape_WithRiver(t *testing.T) {
	t.Parallel()

	t.Run("returns 202 and enqueues River job on success", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{}
		q := &fakeScraperQueries{getSourceRow: postgres.GetScraperSourceByNameRow{Enabled: true}}
		h := &AdminScraperHandler{
			Queries:     q,
			Logger:      zerolog.Nop(),
			Env:         "test",
			RiverClient: inserter,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/sources/my-source/trigger", nil)
		req.SetPathValue("name", "my-source")
		w := httptest.NewRecorder()
		h.TriggerScrape(w, req)

		assert.Equal(t, http.StatusAccepted, w.Result().StatusCode)
		require.True(t, inserter.called, "Insert should have been called")
	})

	t.Run("returns 500 when River Insert fails", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{err: errStubNotImplemented}
		q := &fakeScraperQueries{getSourceRow: postgres.GetScraperSourceByNameRow{Enabled: true}}
		h := &AdminScraperHandler{
			Queries:     q,
			Logger:      zerolog.Nop(),
			Env:         "test",
			RiverClient: inserter,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/sources/my-source/trigger", nil)
		req.SetPathValue("name", "my-source")
		w := httptest.NewRecorder()
		h.TriggerScrape(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
	})
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_SetSourceEnabled
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_SetSourceEnabled(t *testing.T) {
	t.Parallel()

	existingSource := postgres.GetScraperSourceByNameRow{
		ID:       1,
		Name:     "my-source",
		Url:      "https://example.com",
		Tier:     1,
		Schedule: "daily",
		License:  "CC0",
		Enabled:  false,
	}

	tests := []struct {
		name         string
		sourceName   string
		body         any
		getSourceRow postgres.GetScraperSourceByNameRow
		getSourceErr error
		upsertRow    postgres.UpsertScraperSourceRow
		upsertErr    error
		wantStatus   int
		wantEnabled  bool
	}{
		{
			name:         "enables a disabled source",
			sourceName:   "my-source",
			body:         map[string]any{"enabled": true},
			getSourceRow: existingSource,
			upsertRow:    postgres.UpsertScraperSourceRow{ID: 1, Name: "my-source", Enabled: true, Url: "https://example.com", License: "CC0", Schedule: "daily", Tier: 1},
			wantStatus:   http.StatusOK,
			wantEnabled:  true,
		},
		{
			name:         "disables an enabled source",
			sourceName:   "my-source",
			body:         map[string]any{"enabled": false},
			getSourceRow: postgres.GetScraperSourceByNameRow{ID: 1, Name: "my-source", Enabled: true, Url: "https://example.com", License: "CC0", Schedule: "daily", Tier: 1},
			upsertRow:    postgres.UpsertScraperSourceRow{ID: 1, Name: "my-source", Enabled: false, Url: "https://example.com", License: "CC0", Schedule: "daily", Tier: 1},
			wantStatus:   http.StatusOK,
			wantEnabled:  false,
		},
		{
			name:       "returns 400 for invalid JSON body",
			sourceName: "my-source",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 400 for missing source name",
			sourceName: "",
			body:       map[string]any{"enabled": true},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:         "returns 404 when source not found",
			sourceName:   "missing-source",
			body:         map[string]any{"enabled": true},
			getSourceErr: pgx.ErrNoRows,
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "returns 500 on get source db error",
			sourceName:   "my-source",
			body:         map[string]any{"enabled": true},
			getSourceErr: errStubNotImplemented,
			wantStatus:   http.StatusInternalServerError,
		},
		{
			name:         "returns 500 on upsert db error",
			sourceName:   "my-source",
			body:         map[string]any{"enabled": true},
			getSourceRow: existingSource,
			upsertErr:    errStubNotImplemented,
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				getSourceRow: tc.getSourceRow,
				getSourceErr: tc.getSourceErr,
				upsertRow:    tc.upsertRow,
				upsertErr:    tc.upsertErr,
			}
			h := newTestScraperHandler(q)

			var bodyBytes []byte
			var err error
			if s, ok := tc.body.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, err = json.Marshal(tc.body)
				require.NoError(t, err)
			}

			var path string
			if tc.sourceName != "" {
				path = "/api/v1/admin/scraper/sources/" + tc.sourceName
			} else {
				path = "/api/v1/admin/scraper/sources/"
			}
			req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			if tc.sourceName != "" {
				req.SetPathValue("name", tc.sourceName)
			}

			w := httptest.NewRecorder()
			h.SetSourceEnabled(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body scraperSourceResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Equal(t, tc.wantEnabled, body.Enabled)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_TriggerAllScrape
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_TriggerAllScrape(t *testing.T) {
	t.Parallel()

	t.Run("returns 503 when RiverClient is nil", func(t *testing.T) {
		t.Parallel()

		q := &fakeScraperQueries{}
		h := &AdminScraperHandler{
			Queries:           q,
			Logger:            zerolog.Nop(),
			Env:               "test",
			RiverClient:       nil,
			OrchestratorReady: true,
		}

		bodyBytes, err := json.Marshal(map[string]any{"respect_auto_scrape": false})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.TriggerAllScrape(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	})

	t.Run("returns 503 when orchestrator not ready (nil dependencies)", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{}
		q := &fakeScraperQueries{}
		h := &AdminScraperHandler{
			Queries:           q,
			Logger:            zerolog.Nop(),
			Env:               "test",
			RiverClient:       inserter,
			OrchestratorReady: false,
		}

		bodyBytes, err := json.Marshal(map[string]any{"respect_auto_scrape": false})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.TriggerAllScrape(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	})

	tests := []struct {
		name           string
		body           any
		configRow      postgres.ScraperConfig
		configGetErr   error
		runningCount   int64
		wantStatus     int
		wantStatusText string
	}{
		{
			name:           "returns 400 for invalid JSON body",
			body:           "not-json",
			wantStatus:     http.StatusBadRequest,
			wantStatusText: "",
		},
		{
			name: "respect_auto_scrape false triggers even when config is disabled",
			body: map[string]any{"respect_auto_scrape": false},
			configRow: postgres.ScraperConfig{
				AutoScrape: false,
			},
			runningCount:   0,
			wantStatus:     http.StatusAccepted,
			wantStatusText: "triggered",
		},
		{
			name: "returns skipped when respect_auto_scrape true and auto_scrape disabled",
			body: map[string]any{"respect_auto_scrape": true},
			configRow: postgres.ScraperConfig{
				AutoScrape: false,
			},
			runningCount:   0,
			wantStatus:     http.StatusOK,
			wantStatusText: "skipped",
		},
		{
			name:           "defaults respect_auto_scrape to true",
			body:           map[string]any{},
			configRow:      postgres.ScraperConfig{AutoScrape: false},
			runningCount:   0,
			wantStatus:     http.StatusOK,
			wantStatusText: "skipped",
		},
		{
			name:           "defaults skip_up_to_date to true",
			body:           map[string]any{"respect_auto_scrape": false},
			configRow:      postgres.ScraperConfig{AutoScrape: true},
			runningCount:   0,
			wantStatus:     http.StatusAccepted,
			wantStatusText: "triggered",
		},
		{
			name:           "returns conflict when run already active",
			body:           map[string]any{"respect_auto_scrape": false},
			configRow:      postgres.ScraperConfig{AutoScrape: true},
			runningCount:   2,
			wantStatus:     http.StatusConflict,
			wantStatusText: "already_running",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inserter := &fakeRiverInserter{}
			q := &fakeScraperQueries{
				configRow:                    tc.configRow,
				configGetErr:                 tc.configGetErr,
				countRunningScraperRunsValue: tc.runningCount,
			}
			h := &AdminScraperHandler{
				Queries:           q,
				Logger:            zerolog.Nop(),
				Env:               "test",
				RiverClient:       inserter,
				OrchestratorReady: true,
			}

			var bodyBytes []byte
			var err error
			if s, ok := tc.body.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, err = json.Marshal(tc.body)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.TriggerAllScrape(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatusText != "" && tc.wantStatus != http.StatusServiceUnavailable {
				var body struct {
					Status string `json:"status"`
				}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Equal(t, tc.wantStatusText, body.Status)
			}
		})
	}
}

func TestAdminScraperHandler_TriggerAllScrape_WithRiver(t *testing.T) {
	t.Parallel()

	t.Run("returns 202 and enqueues orchestrator job on success", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{}
		q := &fakeScraperQueries{
			configRow: postgres.ScraperConfig{AutoScrape: true},
		}
		h := &AdminScraperHandler{
			Queries:           q,
			Logger:            zerolog.Nop(),
			Env:               "test",
			RiverClient:       inserter,
			OrchestratorReady: true,
		}

		bodyBytes, err := json.Marshal(map[string]any{"respect_auto_scrape": false})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.TriggerAllScrape(w, req)

		assert.Equal(t, http.StatusAccepted, w.Result().StatusCode)
		require.True(t, inserter.called, "Insert should have been called")
	})

	t.Run("returns 409 when running source exists and does not enqueue", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{}
		q := &fakeScraperQueries{
			configRow:                    postgres.ScraperConfig{AutoScrape: true},
			countRunningScraperRunsValue: 1,
		}
		h := &AdminScraperHandler{
			Queries:           q,
			Logger:            zerolog.Nop(),
			Env:               "test",
			RiverClient:       inserter,
			OrchestratorReady: true,
		}

		bodyBytes, err := json.Marshal(map[string]any{"respect_auto_scrape": false})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.TriggerAllScrape(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.False(t, inserter.called, "Insert should not be called when a run is active")

		var body triggerAllResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "already_running", body.Status)
		assert.EqualValues(t, 1, body.RunningSources)
	})

	t.Run("empty request body uses defaults and does not 400", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{}
		q := &fakeScraperQueries{
			configRow: postgres.ScraperConfig{AutoScrape: true},
		}
		h := &AdminScraperHandler{
			Queries:           q,
			Logger:            zerolog.Nop(),
			Env:               "test",
			RiverClient:       inserter,
			OrchestratorReady: true,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.TriggerAllScrape(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode, "empty body should use defaults (respect_auto_scrape=true, skip_up_to_date=true) and not 400")

		require.True(t, inserter.called, "Insert should have been called with default args")
		assert.True(t, inserter.insertedArg.RespectAutoScrape, "default RespectAutoScrape should be true")
		assert.True(t, inserter.insertedArg.SkipUpToDate, "default SkipUpToDate should be true")
	})

	t.Run("returns 500 when River Insert fails", func(t *testing.T) {
		t.Parallel()

		inserter := &fakeRiverInserter{err: errStubNotImplemented}
		q := &fakeScraperQueries{
			configRow: postgres.ScraperConfig{AutoScrape: true},
		}
		h := &AdminScraperHandler{
			Queries:           q,
			Logger:            zerolog.Nop(),
			Env:               "test",
			RiverClient:       inserter,
			OrchestratorReady: true,
		}

		bodyBytes, err := json.Marshal(map[string]any{"respect_auto_scrape": false})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scraper/trigger-all", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.TriggerAllScrape(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
	})
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_GetConfig
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_GetConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		configRow         postgres.ScraperConfig
		configGetErr      error
		wantStatus        int
		wantAutoScrape    bool
		wantMaxConcurrent int32
	}{
		{
			name: "returns config from database",
			configRow: postgres.ScraperConfig{
				AutoScrape:            false,
				MaxConcurrentSources:  5,
				RequestTimeoutSeconds: 60,
				RetryMaxAttempts:      5,
				MaxBatchSize:          200,
				RateLimitMs:           500,
			},
			wantStatus:        http.StatusOK,
			wantAutoScrape:    false,
			wantMaxConcurrent: 5,
		},
		{
			name:              "returns defaults when no config in DB",
			configGetErr:      pgx.ErrNoRows,
			wantStatus:        http.StatusOK,
			wantAutoScrape:    true,
			wantMaxConcurrent: 3,
		},
		{
			name:         "returns 500 on DB error",
			configGetErr: errors.New("db connection error"),
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeScraperQueries{
				configRow:    tc.configRow,
				configGetErr: tc.configGetErr,
			}
			h := newTestScraperHandler(q)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/config", nil)
			w := httptest.NewRecorder()
			h.GetConfig(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body scraperConfigResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Equal(t, tc.wantAutoScrape, body.AutoScrape)
				assert.Equal(t, tc.wantMaxConcurrent, body.MaxConcurrentSources)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_PatchConfig
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_PatchConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		existingConfig    postgres.ScraperConfig
		patchBody         map[string]any
		wantStatus        int
		wantAutoScrape    *bool
		wantMaxConcurrent *int32
		wantErrContains   string
	}{
		{
			name: "patches auto_scrape successfully",
			existingConfig: postgres.ScraperConfig{
				AutoScrape:            true,
				MaxConcurrentSources:  3,
				RequestTimeoutSeconds: 30,
				RetryMaxAttempts:      3,
				MaxBatchSize:          100,
				RateLimitMs:           0,
			},
			patchBody:      map[string]any{"auto_scrape": false},
			wantStatus:     http.StatusOK,
			wantAutoScrape: boolPtr(false),
		},
		{
			name: "patches max_concurrent_sources successfully",
			existingConfig: postgres.ScraperConfig{
				AutoScrape:           true,
				MaxConcurrentSources: 3,
			},
			patchBody:         map[string]any{"max_concurrent_sources": 10},
			wantStatus:        http.StatusOK,
			wantMaxConcurrent: int32Ptr(10),
		},
		{
			name:            "rejects zero max_concurrent_sources",
			patchBody:       map[string]any{"max_concurrent_sources": 0},
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "max_concurrent_sources must be greater than 0",
		},
		{
			name:            "rejects negative rate_limit_ms",
			patchBody:       map[string]any{"rate_limit_ms": -1},
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "rate_limit_ms must be 0 or greater",
		},
		{
			name:           "seeds defaults when no config exists",
			patchBody:      map[string]any{"auto_scrape": false},
			wantStatus:     http.StatusOK,
			wantAutoScrape: boolPtr(false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeScraperQueries{
				configRow:    tc.existingConfig,
				configGetErr: nil,
			}
			h := newTestScraperHandler(q)

			bodyBytes, err := json.Marshal(tc.patchBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/scraper/config", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.PatchConfig(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body scraperConfigResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				if tc.wantAutoScrape != nil {
					assert.Equal(t, *tc.wantAutoScrape, body.AutoScrape)
				}
				if tc.wantMaxConcurrent != nil {
					assert.Equal(t, *tc.wantMaxConcurrent, body.MaxConcurrentSources)
				}
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func int32Ptr(i int32) *int32 {
	return &i
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_GetSourceDiagnostics
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_GetSourceDiagnostics(t *testing.T) {
	t.Parallel()

	makeRun := func(status string, id int64) postgres.ScraperRun {
		return postgres.ScraperRun{
			ID:           id,
			SourceName:   "test-source",
			SourceUrl:    "https://example.com",
			Tier:         1,
			StartedAt:    nowTs(),
			CompletedAt:  nowTs(),
			Status:       status,
			EventsFound:  10,
			EventsNew:    5,
			EventsDup:    3,
			EventsFailed: 2,
		}
	}

	tests := []struct {
		name            string
		sourceName      string
		latestRunRow    postgres.ScraperRun
		latestRunErr    error
		lastSuccessRow  postgres.ScraperRun
		lastSuccessErr  error
		runsRows        []postgres.ScraperRun
		wantStatus      int
		wantLatest      bool
		wantLastSuccess bool
		wantRecentLen   int
	}{
		{
			name:            "returns latest run only when completed",
			sourceName:      "test-source",
			latestRunRow:    makeRun("completed", 1),
			runsRows:        []postgres.ScraperRun{makeRun("completed", 1)},
			wantStatus:      http.StatusOK,
			wantLatest:      true,
			wantLastSuccess: false,
			wantRecentLen:   1,
		},
		{
			name:            "returns latest and last success when latest failed",
			sourceName:      "test-source",
			latestRunRow:    makeRun("failed", 2),
			lastSuccessRow:  makeRun("completed", 1),
			runsRows:        []postgres.ScraperRun{makeRun("failed", 2), makeRun("completed", 1)},
			wantStatus:      http.StatusOK,
			wantLatest:      true,
			wantLastSuccess: true,
			wantRecentLen:   2,
		},
		{
			name:            "returns null last_success when no successful runs exist",
			sourceName:      "test-source",
			latestRunRow:    makeRun("failed", 1),
			lastSuccessErr:  pgx.ErrNoRows,
			runsRows:        []postgres.ScraperRun{makeRun("failed", 1)},
			wantStatus:      http.StatusOK,
			wantLatest:      true,
			wantLastSuccess: false,
			wantRecentLen:   1,
		},
		{
			name:          "returns empty when no runs exist",
			sourceName:    "test-source",
			latestRunErr:  pgx.ErrNoRows,
			wantStatus:    http.StatusOK,
			wantLatest:    false,
			wantRecentLen: 0,
		},
		{
			name:         "returns 500 on db error",
			sourceName:   "test-source",
			latestRunErr: errStubNotImplemented,
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				latestRunRow:   tc.latestRunRow,
				latestRunErr:   tc.latestRunErr,
				lastSuccessRow: tc.lastSuccessRow,
				lastSuccessErr: tc.lastSuccessErr,
				runsRows:       tc.runsRows,
			}
			h := newTestScraperHandler(q)

			path := "/api/v1/admin/scraper/sources/" + tc.sourceName + "/diagnostics"
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.SetPathValue("name", tc.sourceName)
			w := httptest.NewRecorder()
			h.GetSourceDiagnostics(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body diagnosticsResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Equal(t, tc.sourceName, body.SourceName)
				if tc.wantLatest {
					assert.NotNil(t, body.LatestRun)
				} else {
					assert.Nil(t, body.LatestRun)
				}
				if tc.wantLastSuccess {
					assert.NotNil(t, body.LastSuccessfulRun)
				} else {
					assert.Nil(t, body.LastSuccessfulRun)
				}
				assert.Len(t, body.RecentRuns, tc.wantRecentLen)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestAdminScraperHandler_GetAllDiagnostics
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_GetAllDiagnostics(t *testing.T) {
	t.Parallel()

	makeRun := func(source, status string) postgres.ScraperRun {
		return postgres.ScraperRun{
			ID:           1,
			SourceName:   source,
			SourceUrl:    "https://example.com",
			Tier:         1,
			StartedAt:    nowTs(),
			CompletedAt:  nowTs(),
			Status:       status,
			EventsFound:  10,
			EventsNew:    5,
			EventsDup:    3,
			EventsFailed: 2,
		}
	}

	tests := []struct {
		name         string
		filteredRows []postgres.ScraperRun
		filteredErr  error
		query        string
		wantStatus   int
		wantItemsLen int
	}{
		{
			name:         "returns recent runs across all sources",
			filteredRows: []postgres.ScraperRun{makeRun("source-a", "completed"), makeRun("source-b", "failed")},
			query:        "",
			wantStatus:   http.StatusOK,
			wantItemsLen: 2,
		},
		{
			name:         "returns empty list when no runs",
			filteredRows: []postgres.ScraperRun{},
			query:        "",
			wantStatus:   http.StatusOK,
			wantItemsLen: 0,
		},
		{
			name:        "returns 500 on db error",
			filteredErr: errStubNotImplemented,
			query:       "",
			wantStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				filteredRunsRows: tc.filteredRows,
				filteredRunsErr:  tc.filteredErr,
			}
			h := newTestScraperHandler(q)

			path := "/api/v1/admin/scraper/diagnostics"
			if tc.query != "" {
				path += "?" + tc.query
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			h.GetAllDiagnostics(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				var body allDiagnosticsResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Len(t, body.Items, tc.wantItemsLen)
				assert.Equal(t, tc.wantItemsLen, body.Total)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// toScraperRunResponse — metadata / event_failures tests (srv-b5pdv)
// ---------------------------------------------------------------------------

func TestToScraperRunResponse_NilMetadata(t *testing.T) {
	run := postgres.ScraperRun{
		ID:         1,
		SourceName: "test-source",
		Status:     "completed",
		Metadata:   nil,
	}
	resp := toScraperRunResponse(run)
	if resp.EventFailures != nil {
		t.Errorf("expected nil EventFailures for nil metadata, got %v", resp.EventFailures)
	}
}

func TestToScraperRunResponse_ValidMetadata(t *testing.T) {
	metaJSON := []byte(`{"event_failures":[{"index":1,"url":"https://example.org/e/1","message":"bad date"},{"index":3,"url":"","message":"missing name"}]}`)
	run := postgres.ScraperRun{
		ID:         2,
		SourceName: "test-source",
		Status:     "completed",
		Metadata:   metaJSON,
	}
	resp := toScraperRunResponse(run)
	if len(resp.EventFailures) != 2 {
		t.Fatalf("len(EventFailures) = %d, want 2", len(resp.EventFailures))
	}
	if resp.EventFailures[0].Index != 1 || resp.EventFailures[0].URL != "https://example.org/e/1" || resp.EventFailures[0].Message != "bad date" {
		t.Errorf("EventFailures[0] = %+v", resp.EventFailures[0])
	}
	if resp.EventFailures[1].Index != 3 || resp.EventFailures[1].URL != "" || resp.EventFailures[1].Message != "missing name" {
		t.Errorf("EventFailures[1] = %+v", resp.EventFailures[1])
	}
}

func TestToScraperRunResponse_MalformedMetadata(t *testing.T) {
	run := postgres.ScraperRun{
		ID:         3,
		SourceName: "test-source",
		Status:     "completed",
		Metadata:   []byte(`not valid json {`),
	}
	// Must not panic; malformed metadata silently produces no failures.
	resp := toScraperRunResponse(run)
	if resp.EventFailures != nil {
		t.Errorf("expected nil EventFailures for malformed metadata, got %v", resp.EventFailures)
	}
}

func TestToScraperRunResponse_EmptyEventFailuresArray(t *testing.T) {
	metaJSON := []byte(`{"event_failures":[]}`)
	run := postgres.ScraperRun{
		ID:         4,
		SourceName: "test-source",
		Status:     "completed",
		Metadata:   metaJSON,
	}
	resp := toScraperRunResponse(run)
	// An empty array in metadata should not populate EventFailures (omitempty).
	if resp.EventFailures != nil {
		t.Errorf("expected nil EventFailures for empty array, got %v", resp.EventFailures)
	}
}
