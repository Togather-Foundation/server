package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// IngestClient submits event batches to a SEL server via the batch ingest API.
type IngestClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

// IngestResult holds the parsed response from a batch ingest submission.
type IngestResult struct {
	BatchID         string        `json:"batch_id"`
	EventsCreated   int           `json:"events_created"`
	EventsDuplicate int           `json:"events_duplicate"`
	EventsFailed    int           `json:"events_failed"`
	Errors          []IngestError `json:"errors,omitempty"`
}

// IngestError describes a per-event error within a batch.
type IngestError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// NewIngestClient constructs an IngestClient targeting baseURL with the given
// API key. A 30-second HTTP timeout and a descriptive User-Agent are set by
// default.
func NewIngestClient(baseURL, apiKey string) *IngestClient {
	return &IngestClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgent: "Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)",
	}
}

// SubmitBatch marshals events as {"events":[...]} and POSTs them to
// {baseURL}/api/v1/events:batch. It returns an IngestResult populated from the
// JSON response.
func (c *IngestClient) SubmitBatch(ctx context.Context, evts []events.EventInput) (IngestResult, error) {
	payload, err := json.Marshal(map[string]any{"events": evts})
	if err != nil {
		return IngestResult{}, fmt.Errorf("marshal batch: %w", err)
	}

	url := c.baseURL + "/api/v1/events:batch"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return IngestResult{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return IngestResult{}, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return IngestResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		snippet := bodySnippet(body)
		return IngestResult{}, fmt.Errorf("rate limited (HTTP 429): %s", snippet)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := bodySnippet(body)
		return IngestResult{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, snippet)
	}

	var result IngestResult
	if err := json.Unmarshal(body, &result); err != nil {
		return IngestResult{}, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

// SubmitBatchDryRun returns a synthetic IngestResult without making any HTTP
// call. EventsCreated is set to len(evts) and BatchID is "dry-run".
func (c *IngestClient) SubmitBatchDryRun(_ context.Context, evts []events.EventInput) (IngestResult, error) {
	return IngestResult{
		BatchID:       "dry-run",
		EventsCreated: len(evts),
	}, nil
}

// bodySnippet returns up to 200 characters of body as a string.
func bodySnippet(body []byte) string {
	if len(body) > 200 {
		return string(body[:200])
	}
	return string(body)
}
