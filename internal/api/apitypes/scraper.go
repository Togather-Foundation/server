package apitypes

import "time"

type EventFailureResponse struct {
	Index   int    `json:"index"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

type ScraperRunResponse struct {
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
	EventFailures []EventFailureResponse `json:"event_failures,omitempty"`
}

type DiagnosticsResponse struct {
	SourceName        string               `json:"source_name"`
	LatestRun         *ScraperRunResponse  `json:"latest_run"`
	LastSuccessfulRun *ScraperRunResponse  `json:"last_successful_run,omitempty"`
	RecentRuns        []ScraperRunResponse `json:"recent_runs"`
}

type AllDiagnosticsResponse struct {
	Items []ScraperRunResponse `json:"items"`
	Total int                  `json:"total"`
}
