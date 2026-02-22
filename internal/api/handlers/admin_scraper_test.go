package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	getSourceRow postgres.ScraperSource
	getSourceErr error

	// UpsertScraperSource (used by SetSourceEnabled)
	upsertRow postgres.ScraperSource
	upsertErr error
}

func (f *fakeScraperQueries) ListScraperSourcesWithLatestRun(_ context.Context, _ pgtype.Bool) ([]postgres.ListScraperSourcesWithLatestRunRow, error) {
	return f.sourcesRows, f.sourcesErr
}

func (f *fakeScraperQueries) ListScraperRunsBySource(_ context.Context, _ postgres.ListScraperRunsBySourceParams) ([]postgres.ScraperRun, error) {
	return f.runsRows, f.runsErr
}

func (f *fakeScraperQueries) GetScraperSourceByName(_ context.Context, _ string) (postgres.ScraperSource, error) {
	return f.getSourceRow, f.getSourceErr
}

func (f *fakeScraperQueries) UpsertScraperSource(_ context.Context, _ postgres.UpsertScraperSourceParams) (postgres.ScraperSource, error) {
	return f.upsertRow, f.upsertErr
}

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
		getSourceErr error
		wantStatus   int
		wantName     string
	}{
		{
			name:       "returns 400 when name is missing",
			sourceName: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 503 when scraper is nil (not configured)",
			sourceName: "my-source",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:         "returns 404 when source does not exist",
			sourceName:   "unknown-source",
			getSourceErr: pgx.ErrNoRows,
			wantStatus:   http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeScraperQueries{
				getSourceErr: tc.getSourceErr,
			}
			// Scraper is always nil in these tests — TriggerScrape now returns 503 for nil.
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
// TestAdminScraperHandler_SetSourceEnabled
// ----------------------------------------------------------------------------

func TestAdminScraperHandler_SetSourceEnabled(t *testing.T) {
	t.Parallel()

	existingSource := postgres.ScraperSource{
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
		getSourceRow postgres.ScraperSource
		getSourceErr error
		upsertRow    postgres.ScraperSource
		upsertErr    error
		wantStatus   int
		wantEnabled  bool
	}{
		{
			name:         "enables a disabled source",
			sourceName:   "my-source",
			body:         map[string]any{"enabled": true},
			getSourceRow: existingSource,
			upsertRow:    postgres.ScraperSource{ID: 1, Name: "my-source", Enabled: true, Url: "https://example.com", License: "CC0", Schedule: "daily", Tier: 1},
			wantStatus:   http.StatusOK,
			wantEnabled:  true,
		},
		{
			name:         "disables an enabled source",
			sourceName:   "my-source",
			body:         map[string]any{"enabled": false},
			getSourceRow: postgres.ScraperSource{ID: 1, Name: "my-source", Enabled: true, Url: "https://example.com", License: "CC0", Schedule: "daily", Tier: 1},
			upsertRow:    postgres.ScraperSource{ID: 1, Name: "my-source", Enabled: false, Url: "https://example.com", License: "CC0", Schedule: "daily", Tier: 1},
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
