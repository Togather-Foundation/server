package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// scraperQueriesIface is the subset of postgres.Queries used by AdminScraperHandler.
type scraperQueriesIface interface {
	ListScraperSourcesWithLatestRun(ctx context.Context, enabled pgtype.Bool) ([]postgres.ListScraperSourcesWithLatestRunRow, error)
	ListScraperRunsBySource(ctx context.Context, arg postgres.ListScraperRunsBySourceParams) ([]postgres.ScraperRun, error)
	GetScraperSourceByName(ctx context.Context, name string) (postgres.GetScraperSourceByNameRow, error)
	UpsertScraperSource(ctx context.Context, arg postgres.UpsertScraperSourceParams) (postgres.UpsertScraperSourceRow, error)
	GetScraperConfig(ctx context.Context) (postgres.ScraperConfig, error)
	SetScraperConfig(ctx context.Context, arg postgres.SetScraperConfigParams) error
	CountRunningScraperRuns(ctx context.Context) (int64, error)
	GetLatestScraperRunBySource(ctx context.Context, sourceName string) (postgres.ScraperRun, error)
	GetLastSuccessfulRunBySource(ctx context.Context, sourceName string) (postgres.ScraperRun, error)
	ListRecentScraperRunsFiltered(ctx context.Context, arg postgres.ListRecentScraperRunsFilteredParams) ([]postgres.ScraperRun, error)
}

// scraperJobInserter is the River client method subset used by AdminScraperHandler.
type scraperJobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// AdminScraperHandler handles admin scraper source management and run history.
type AdminScraperHandler struct {
	Queries           scraperQueriesIface
	Logger            zerolog.Logger
	Env               string
	RiverClient       scraperJobInserter
	OrchestratorReady bool // false if orchestrator dependencies not wired (nil ConfigQueries/SourcesReader)
}

// scraperSourceResponse is the JSON representation of a scraper source.
type scraperSourceResponse struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	Tier                int32      `json:"tier"`
	Schedule            string     `json:"schedule"`
	License             string     `json:"license"`
	Enabled             bool       `json:"enabled"`
	LastRunStatus       string     `json:"last_run_status,omitempty"`
	LastRunStartedAt    *time.Time `json:"last_run_started_at,omitempty"`
	LastRunCompletedAt  *time.Time `json:"last_run_completed_at,omitempty"`
	LastRunEventsFound  int32      `json:"last_run_events_found"`
	LastRunEventsNew    int32      `json:"last_run_events_new"`
	LastRunEventsDup    int32      `json:"last_run_events_dup"`
	LastRunEventsFailed int32      `json:"last_run_events_failed"`
	LastRunErrorMessage string     `json:"last_run_error_message,omitempty"`
}

// eventFailureResponse is a single per-event ingest failure within a scraper run.
type eventFailureResponse struct {
	Index   int    `json:"index"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

// scraperRunResponse is the JSON representation of a single scraper run.
type scraperRunResponse struct {
	ID            int64                  `json:"id"`
	SourceName    string                 `json:"source_name"`
	SourceURL     string                 `json:"source_url"`
	Tier          int32                  `json:"tier"`
	Status        string                 `json:"status"`
	StartedAt     *time.Time             `json:"started_at,omitempty"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	EventsFound   int32                  `json:"events_found"`
	EventsNew     int32                  `json:"events_new"`
	EventsDup     int32                  `json:"events_dup"`
	EventsFailed  int32                  `json:"events_failed"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	EventFailures []eventFailureResponse `json:"event_failures,omitempty"`
}

// toScraperSourceResponse converts a ListScraperSourcesWithLatestRunRow to a scraperSourceResponse.
func toScraperSourceResponse(row postgres.ListScraperSourcesWithLatestRunRow) scraperSourceResponse {
	resp := scraperSourceResponse{
		ID:                  row.ID,
		Name:                row.Name,
		URL:                 row.Url,
		Tier:                row.Tier,
		Schedule:            row.Schedule,
		License:             row.License,
		Enabled:             row.Enabled,
		LastRunStatus:       row.LastRunStatus,
		LastRunEventsFound:  row.LastRunEventsFound,
		LastRunEventsNew:    row.LastRunEventsNew,
		LastRunEventsDup:    row.LastRunEventsDup,
		LastRunEventsFailed: row.LastRunEventsFailed,
	}
	if row.LastRunStartedAt.Valid {
		t := row.LastRunStartedAt.Time
		resp.LastRunStartedAt = &t
	}
	if row.LastRunCompletedAt.Valid {
		t := row.LastRunCompletedAt.Time
		resp.LastRunCompletedAt = &t
	}
	if row.LastRunErrorMessage.Valid {
		resp.LastRunErrorMessage = row.LastRunErrorMessage.String
	}
	return resp
}

// scraperSourceFromDB converts a postgres.UpsertScraperSourceRow to a scraperSourceResponse.
func scraperSourceFromDB(s postgres.UpsertScraperSourceRow) scraperSourceResponse {
	return scraperSourceResponse{
		ID:       s.ID,
		Name:     s.Name,
		URL:      s.Url,
		Tier:     s.Tier,
		Schedule: s.Schedule,
		License:  s.License,
		Enabled:  s.Enabled,
	}
}

// toScraperRunResponse converts a postgres.ScraperRun to a scraperRunResponse.
func toScraperRunResponse(run postgres.ScraperRun) scraperRunResponse {
	resp := scraperRunResponse{
		ID:           run.ID,
		SourceName:   run.SourceName,
		SourceURL:    run.SourceUrl,
		Tier:         run.Tier,
		Status:       run.Status,
		EventsFound:  run.EventsFound,
		EventsNew:    run.EventsNew,
		EventsDup:    run.EventsDup,
		EventsFailed: run.EventsFailed,
	}
	if run.StartedAt.Valid {
		t := run.StartedAt.Time
		resp.StartedAt = &t
	}
	if run.CompletedAt.Valid {
		t := run.CompletedAt.Time
		resp.CompletedAt = &t
	}
	if run.ErrorMessage.Valid {
		resp.ErrorMessage = run.ErrorMessage.String
	}
	// Unmarshal per-event failure details from the metadata JSONB column.
	if len(run.Metadata) > 0 {
		var meta struct {
			EventFailures []eventFailureResponse `json:"event_failures"`
		}
		if err := json.Unmarshal(run.Metadata, &meta); err == nil && len(meta.EventFailures) > 0 {
			resp.EventFailures = meta.EventFailures
		}
	}
	return resp
}

// ListSources handles GET /api/v1/admin/scraper/sources.
// Returns all scraper sources with their latest run stats.
func (h *AdminScraperHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	// pgtype.Bool{Valid: false} sends NULL to the query, which the WHERE clause
	// treats as "no filter" — returning all sources regardless of enabled state.
	allSources := pgtype.Bool{Valid: false}
	rows, err := h.Queries.ListScraperSourcesWithLatestRun(r.Context(), allSources)
	if err != nil {
		h.Logger.Error().Err(err).Msg("admin scraper: list sources")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list scraper sources", fmt.Errorf("list scraper sources: %w", err), h.Env)
		return
	}

	items := make([]scraperSourceResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toScraperSourceResponse(row))
	}

	writeJSON(w, http.StatusOK, struct {
		Items []scraperSourceResponse `json:"items"`
	}{Items: items}, "application/json")
}

// ListSourceRuns handles GET /api/v1/admin/scraper/sources/{name}/runs.
// Returns recent scraper runs for the given source.
func (h *AdminScraperHandler) ListSourceRuns(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing source name", nil, h.Env)
		return
	}

	runs, err := h.Queries.ListScraperRunsBySource(r.Context(), postgres.ListScraperRunsBySourceParams{
		SourceName: name,
		Limit:      50,
	})
	if err != nil {
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: list source runs")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list scraper runs", fmt.Errorf("list scraper runs source=%s: %w", name, err), h.Env)
		return
	}

	items := make([]scraperRunResponse, 0, len(runs))
	for _, run := range runs {
		items = append(items, toScraperRunResponse(run))
	}

	writeJSON(w, http.StatusOK, struct {
		Items []scraperRunResponse `json:"items"`
	}{Items: items}, "application/json")
}

// TriggerScrape handles POST /api/v1/admin/scraper/sources/{name}/trigger.
// Enqueues a River ScrapeSourceJob for the named source and returns 202 immediately.
// The scrape runs inside the existing ScrapeSourceWorker — same path as scheduled scrapes.
func (h *AdminScraperHandler) TriggerScrape(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing source name", nil, h.Env)
		return
	}

	// Verify the source exists and is enabled — callers get a 404 for typos/invalid
	// names and a 409 for disabled sources rather than a silent background failure.
	src, err := h.Queries.GetScraperSourceByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Scraper source not found", nil, h.Env)
			return
		}
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: lookup source for trigger")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to look up scraper source", fmt.Errorf("get scraper source name=%s: %w", name, err), h.Env)
		return
	}
	if !src.Enabled {
		problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Scraper source is disabled", nil, h.Env)
		return
	}

	// Return 503 if River is not available on this node.
	if h.RiverClient == nil {
		problem.Write(w, r, http.StatusServiceUnavailable, "https://sel.events/problems/not-available", "Job queue not available on this node", nil, h.Env)
		return
	}

	if _, err := h.RiverClient.Insert(r.Context(), jobs.ScrapeSourceArgs{SourceName: name}, nil); err != nil {
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: failed to enqueue trigger job")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to enqueue scrape job", fmt.Errorf("insert scrape job name=%s: %w", name, err), h.Env)
		return
	}

	writeJSON(w, http.StatusAccepted, struct {
		SourceName string `json:"source_name"`
		Status     string `json:"status"`
	}{SourceName: name, Status: "triggered"}, "application/json")
}

// SetSourceEnabled handles PATCH /api/v1/admin/scraper/sources/{name}.
// Enables or disables a scraper source.
func (h *AdminScraperHandler) SetSourceEnabled(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing source name", nil, h.Env)
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	existing, err := h.Queries.GetScraperSourceByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Scraper source not found", nil, h.Env)
			return
		}
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: get source for enable toggle")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to look up scraper source", fmt.Errorf("get scraper source name=%s: %w", name, err), h.Env)
		return
	}

	updated, err := h.Queries.UpsertScraperSource(r.Context(), postgres.UpsertScraperSourceParams{
		Name:                          existing.Name,
		Url:                           existing.Url,
		Urls:                          existing.Urls,
		Tier:                          existing.Tier,
		Schedule:                      existing.Schedule,
		TrustLevel:                    existing.TrustLevel,
		License:                       existing.License,
		Enabled:                       body.Enabled,
		MaxPages:                      existing.MaxPages,
		Selectors:                     existing.Selectors,
		Notes:                         existing.Notes,
		EventUrlPattern:               existing.EventUrlPattern,
		SkipMultiSessionCheck:         existing.SkipMultiSessionCheck,
		MultiSessionDurationThreshold: existing.MultiSessionDurationThreshold,
		FollowEventUrls:               existing.FollowEventUrls,
		Timezone:                      existing.Timezone,
		LastScrapedAt:                 existing.LastScrapedAt,
		HeadlessWaitSelector:          existing.HeadlessWaitSelector,
		HeadlessWaitTimeoutMs:         existing.HeadlessWaitTimeoutMs,
		HeadlessPaginationBtn:         existing.HeadlessPaginationBtn,
		HeadlessHeaders:               existing.HeadlessHeaders,
		HeadlessRateLimitMs:           existing.HeadlessRateLimitMs,
		HeadlessWaitNetworkIdle:       existing.HeadlessWaitNetworkIdle,
		HeadlessUndetected:            existing.HeadlessUndetected,
		HeadlessIframe:                existing.HeadlessIframe,
		HeadlessIntercept:             existing.HeadlessIntercept,
		GraphqlConfig:                 existing.GraphqlConfig,
		RestConfig:                    existing.RestConfig,
		SitemapConfig:                 existing.SitemapConfig,
	})
	if err != nil {
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: set source enabled")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to update scraper source", fmt.Errorf("upsert scraper source name=%s: %w", name, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, scraperSourceFromDB(updated), "application/json")
}

// scraperConfigResponse is the JSON representation of the global scraper config.
type scraperConfigResponse struct {
	AutoScrape            bool  `json:"auto_scrape"`
	MaxConcurrentSources  int32 `json:"max_concurrent_sources"`
	RequestTimeoutSeconds int32 `json:"request_timeout_seconds"`
	RetryMaxAttempts      int32 `json:"retry_max_attempts"`
	MaxBatchSize          int32 `json:"max_batch_size"`
	RateLimitMs           int32 `json:"rate_limit_ms"`
}

// GetConfig handles GET /api/v1/admin/scraper/config.
// Returns the current global scraper configuration.
func (h *AdminScraperHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Queries.GetScraperConfig(r.Context())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No config row yet — return sensible defaults.
			writeJSON(w, http.StatusOK, scraperConfigResponse{
				AutoScrape:            true,
				MaxConcurrentSources:  3,
				RequestTimeoutSeconds: 30,
				RetryMaxAttempts:      3,
				MaxBatchSize:          100,
				RateLimitMs:           0,
			}, "application/json")
			return
		}
		h.Logger.Error().Err(err).Msg("admin scraper: get config")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to read scraper config", fmt.Errorf("get scraper config: %w", err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, scraperConfigResponse{
		AutoScrape:            cfg.AutoScrape,
		MaxConcurrentSources:  cfg.MaxConcurrentSources,
		RequestTimeoutSeconds: cfg.RequestTimeoutSeconds,
		RetryMaxAttempts:      cfg.RetryMaxAttempts,
		MaxBatchSize:          cfg.MaxBatchSize,
		RateLimitMs:           cfg.RateLimitMs,
	}, "application/json")
}

// PatchConfig handles PATCH /api/v1/admin/scraper/config.
// Accepts a partial JSON body; only provided fields are applied over the current config.
func (h *AdminScraperHandler) PatchConfig(w http.ResponseWriter, r *http.Request) {
	// Read current config as baseline.
	current, err := h.Queries.GetScraperConfig(r.Context())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No config row yet — seed defaults as the baseline.
			current = postgres.ScraperConfig{
				AutoScrape:            true,
				MaxConcurrentSources:  3,
				RequestTimeoutSeconds: 30,
				RetryMaxAttempts:      3,
				MaxBatchSize:          100,
				RateLimitMs:           0,
			}
		} else {
			h.Logger.Error().Err(err).Msg("admin scraper: patch config — read baseline")
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to read scraper config", fmt.Errorf("get scraper config: %w", err), h.Env)
			return
		}
	}

	// Decode the patch body (all fields optional).
	var patch struct {
		AutoScrape            *bool  `json:"auto_scrape"`
		MaxConcurrentSources  *int32 `json:"max_concurrent_sources"`
		RequestTimeoutSeconds *int32 `json:"request_timeout_seconds"`
		RetryMaxAttempts      *int32 `json:"retry_max_attempts"`
		MaxBatchSize          *int32 `json:"max_batch_size"`
		RateLimitMs           *int32 `json:"rate_limit_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	// Validate numeric fields: reject zero or negative values.
	if patch.MaxConcurrentSources != nil && *patch.MaxConcurrentSources <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "max_concurrent_sources must be greater than 0", nil, h.Env)
		return
	}
	if patch.RequestTimeoutSeconds != nil && *patch.RequestTimeoutSeconds <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "request_timeout_seconds must be greater than 0", nil, h.Env)
		return
	}
	if patch.RetryMaxAttempts != nil && *patch.RetryMaxAttempts <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "retry_max_attempts must be greater than 0", nil, h.Env)
		return
	}
	if patch.MaxBatchSize != nil && *patch.MaxBatchSize <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "max_batch_size must be greater than 0", nil, h.Env)
		return
	}
	if patch.RateLimitMs != nil && *patch.RateLimitMs < 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "rate_limit_ms must be 0 or greater", nil, h.Env)
		return
	}

	// Apply patch fields over the baseline.
	params := postgres.SetScraperConfigParams{
		AutoScrape:            current.AutoScrape,
		MaxConcurrentSources:  current.MaxConcurrentSources,
		RequestTimeoutSeconds: current.RequestTimeoutSeconds,
		RetryMaxAttempts:      current.RetryMaxAttempts,
		MaxBatchSize:          current.MaxBatchSize,
		RateLimitMs:           current.RateLimitMs,
	}
	if patch.AutoScrape != nil {
		params.AutoScrape = *patch.AutoScrape
	}
	if patch.MaxConcurrentSources != nil {
		params.MaxConcurrentSources = *patch.MaxConcurrentSources
	}
	if patch.RequestTimeoutSeconds != nil {
		params.RequestTimeoutSeconds = *patch.RequestTimeoutSeconds
	}
	if patch.RetryMaxAttempts != nil {
		params.RetryMaxAttempts = *patch.RetryMaxAttempts
	}
	if patch.MaxBatchSize != nil {
		params.MaxBatchSize = *patch.MaxBatchSize
	}
	if patch.RateLimitMs != nil {
		params.RateLimitMs = *patch.RateLimitMs
	}

	if err := h.Queries.SetScraperConfig(r.Context(), params); err != nil {
		h.Logger.Error().Err(err).Msg("admin scraper: patch config — write")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to update scraper config", fmt.Errorf("set scraper config: %w", err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, scraperConfigResponse{
		AutoScrape:            params.AutoScrape,
		MaxConcurrentSources:  params.MaxConcurrentSources,
		RequestTimeoutSeconds: params.RequestTimeoutSeconds,
		RetryMaxAttempts:      params.RetryMaxAttempts,
		MaxBatchSize:          params.MaxBatchSize,
		RateLimitMs:           params.RateLimitMs,
	}, "application/json")
}

type triggerAllRequest struct {
	RespectAutoScrape *bool `json:"respect_auto_scrape"`
	SkipUpToDate      *bool `json:"skip_up_to_date"`
}

type triggerAllResponse struct {
	Status            string `json:"status"`
	RespectAutoScrape bool   `json:"respect_auto_scrape"`
	SkipUpToDate      bool   `json:"skip_up_to_date"`
	OrchestratorJobID int64  `json:"orchestrator_job_id,omitempty"`
	RunningSources    int64  `json:"running_sources,omitempty"`
}

func (h *AdminScraperHandler) TriggerAllScrape(w http.ResponseWriter, r *http.Request) {
	if h.RiverClient == nil || !h.OrchestratorReady {
		problem.Write(w, r, http.StatusServiceUnavailable, "https://sel.events/problems/not-available", "Scraper orchestrator not configured on this node", nil, h.Env)
		return
	}

	var req triggerAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	respectAutoScrape := true
	if req.RespectAutoScrape != nil {
		respectAutoScrape = *req.RespectAutoScrape
	}

	skipUpToDate := true
	if req.SkipUpToDate != nil {
		skipUpToDate = *req.SkipUpToDate
	}

	if respectAutoScrape {
		cfg, err := h.Queries.GetScraperConfig(r.Context())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
			} else {
				h.Logger.Error().Err(err).Msg("admin scraper: trigger-all read config")
				problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to read scraper config", fmt.Errorf("get scraper config: %w", err), h.Env)
				return
			}
		} else if !cfg.AutoScrape {
			writeJSON(w, http.StatusOK, triggerAllResponse{
				Status:            "skipped",
				RespectAutoScrape: respectAutoScrape,
				SkipUpToDate:      skipUpToDate,
			}, "application/json")
			return
		}
	}

	runningSources, err := h.Queries.CountRunningScraperRuns(r.Context())
	if err != nil {
		h.Logger.Error().Err(err).Msg("admin scraper: trigger-all count running runs")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to inspect active scraper runs", fmt.Errorf("count running scraper runs: %w", err), h.Env)
		return
	}
	if runningSources > 0 {
		writeJSON(w, http.StatusConflict, triggerAllResponse{
			Status:            "already_running",
			RespectAutoScrape: respectAutoScrape,
			SkipUpToDate:      skipUpToDate,
			RunningSources:    runningSources,
		}, "application/json")
		return
	}

	insertResult, err := h.RiverClient.Insert(r.Context(), jobs.ScrapeOrchestratorArgs{
		RespectAutoScrape: respectAutoScrape,
		SkipUpToDate:      skipUpToDate,
		SourceNames:       nil,
		CurrentIndex:      0,
	}, &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{
			ByArgs: false,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
			},
		},
	})
	if err != nil {
		h.Logger.Error().Err(err).Msg("admin scraper: trigger-all enqueue")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to enqueue orchestrator job", fmt.Errorf("insert orchestrator job: %w", err), h.Env)
		return
	}

	writeJSON(w, http.StatusAccepted, triggerAllResponse{
		Status:            "triggered",
		RespectAutoScrape: respectAutoScrape,
		SkipUpToDate:      skipUpToDate,
		OrchestratorJobID: insertResult.Job.ID,
		RunningSources:    runningSources,
	}, "application/json")
}

// diagnosticsResponse is the JSON response for source diagnostics.
type diagnosticsResponse struct {
	SourceName        string               `json:"source_name"`
	LatestRun         *scraperRunResponse  `json:"latest_run"`
	LastSuccessfulRun *scraperRunResponse  `json:"last_successful_run,omitempty"`
	RecentRuns        []scraperRunResponse `json:"recent_runs"`
}

// GetSourceDiagnostics handles GET /api/v1/admin/scraper/sources/{name}/diagnostics.
// Returns the latest run, last successful run (for comparison when latest failed),
// and a configurable list of recent runs.
func (h *AdminScraperHandler) GetSourceDiagnostics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing source name", nil, h.Env)
		return
	}

	limitParam := r.URL.Query().Get("limit")
	limit := int32(10)
	if limitParam != "" {
		if parsed, err := strconv.ParseInt(limitParam, 10, 32); err == nil && parsed > 0 && parsed <= 100 {
			limit = int32(parsed)
		}
	}

	latest, err := h.Queries.GetLatestScraperRunBySource(r.Context(), name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, diagnosticsResponse{
				SourceName: name,
				RecentRuns: []scraperRunResponse{},
			}, "application/json")
			return
		}
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: get latest run for diagnostics")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get latest run", fmt.Errorf("get latest run source=%s: %w", name, err), h.Env)
		return
	}

	resp := diagnosticsResponse{
		SourceName: name,
		LatestRun:  runPtr(toScraperRunResponse(latest)),
		RecentRuns: make([]scraperRunResponse, 0),
	}

	if latest.Status == "failed" {
		lastSuccess, err := h.Queries.GetLastSuccessfulRunBySource(r.Context(), name)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: get last successful run")
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get last successful run", fmt.Errorf("get last successful run source=%s: %w", name, err), h.Env)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			resp.LastSuccessfulRun = runPtr(toScraperRunResponse(lastSuccess))
		}
	}

	runs, err := h.Queries.ListScraperRunsBySource(r.Context(), postgres.ListScraperRunsBySourceParams{
		SourceName: name,
		Limit:      limit,
	})
	if err != nil {
		h.Logger.Error().Err(err).Str("source", name).Msg("admin scraper: list recent runs for diagnostics")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list recent runs", fmt.Errorf("list recent runs source=%s: %w", name, err), h.Env)
		return
	}
	for _, run := range runs {
		resp.RecentRuns = append(resp.RecentRuns, toScraperRunResponse(run))
	}

	writeJSON(w, http.StatusOK, resp, "application/json")
}

// allDiagnosticsResponse is the JSON response for cross-source diagnostics.
type allDiagnosticsResponse struct {
	Items []scraperRunResponse `json:"items"`
	Total int                  `json:"total"`
}

// GetAllDiagnostics handles GET /api/v1/admin/scraper/diagnostics.
// Returns recent scraper runs across all sources with optional filters.
func (h *AdminScraperHandler) GetAllDiagnostics(w http.ResponseWriter, r *http.Request) {
	limitParam := r.URL.Query().Get("limit")
	limit := int32(20)
	if limitParam != "" {
		if parsed, err := strconv.ParseInt(limitParam, 10, 32); err == nil && parsed > 0 && parsed <= 100 {
			limit = int32(parsed)
		}
	}

	statusFilter := r.URL.Query().Get("status")
	sourceFilter := r.URL.Query().Get("source_name")

	params := postgres.ListRecentScraperRunsFilteredParams{
		Limit: limit,
	}
	if statusFilter != "" {
		params.StatusFilter = pgtype.Text{String: statusFilter, Valid: true}
	}
	if sourceFilter != "" {
		params.SourceFilter = pgtype.Text{String: sourceFilter, Valid: true}
	}

	runs, err := h.Queries.ListRecentScraperRunsFiltered(r.Context(), params)
	if err != nil {
		h.Logger.Error().Err(err).Msg("admin scraper: list filtered runs")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list runs", fmt.Errorf("list filtered runs: %w", err), h.Env)
		return
	}

	items := make([]scraperRunResponse, 0, len(runs))
	for _, run := range runs {
		items = append(items, toScraperRunResponse(run))
	}

	writeJSON(w, http.StatusOK, allDiagnosticsResponse{
		Items: items,
		Total: len(items),
	}, "application/json")
}

func runPtr(r scraperRunResponse) *scraperRunResponse {
	return &r
}
