package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/ical"
)

// ICSExtractor fetches and parses an ICS feed URL.
type ICSExtractor struct {
	client       *http.Client
	maxBodyBytes int64 // Default: 10 * 1024 * 1024 (10 MB)
}

// defaultICSMaxBodyBytes is the default maximum ICS feed body size.
const defaultICSMaxBodyBytes int64 = 10 * 1024 * 1024 // 10 MB

// NewICSExtractor creates an ICS extractor with the given HTTP client.
// maxBodyBytes defaults to 10 MB if <= 0.
func NewICSExtractor(client *http.Client, maxBodyBytes int64) *ICSExtractor {
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultICSMaxBodyBytes
	}
	return &ICSExtractor{
		client:       client,
		maxBodyBytes: maxBodyBytes,
	}
}

// Extract fetches the ICS feed and returns EventInputs.
func (e *ICSExtractor) Extract(ctx context.Context, cfg SourceConfig, icsConfig config.ICSConfig) ([]events.EventInput, []string, error) {
	var allWarnings []string

	// 1. Fetch the ICS feed.
	data, fetchWarnings, err := e.fetchFeed(ctx, cfg.URL)
	allWarnings = append(allWarnings, fetchWarnings...)
	if err != nil {
		return nil, allWarnings, fmt.Errorf("ics fetch %q: %w", cfg.URL, err)
	}

	// 2. Parse the ICS data.
	cal, err := ical.Parse(data)
	if err != nil {
		return nil, allWarnings, fmt.Errorf("ics parse: %w", err)
	}
	allWarnings = append(allWarnings, cal.Warnings...)

	// 3. Build mapper options from source config.
	opts := ical.MapperOptions{
		SourceURL:  cfg.URL,
		SourceName: cfg.Name,
		TrustLevel: cfg.TrustLevel,
		License:    cfg.License,
		Timezone:   cfg.Timezone,
	}

	if opts.License == "" {
		opts.License = "CC0-1.0"
	}

	// Apply ICS config defaults.
	if icsConfig.HorizonDays > 0 {
		opts.HorizonDays = icsConfig.HorizonDays
	}
	if icsConfig.MaxOccurrences > 0 {
		opts.MaxOccurrences = icsConfig.MaxOccurrences
	}

	// Map default location.
	if cfg.DefaultLocation != nil {
		opts.DefaultLocation = cfg.DefaultLocation.ToPlaceInput()
	}

	// 4. Map parsed events to EventInputs.
	results, mapWarnings, err := ical.MapToEventInputs(ctx, cal, opts)
	if err != nil {
		return nil, append(allWarnings, mapWarnings...), fmt.Errorf("ics map: %w", err)
	}
	allWarnings = append(allWarnings, mapWarnings...)

	return results, allWarnings, nil
}

// fetchFeed fetches the ICS feed from the given URL with body size limits
// and redirect handling.
func (e *ICSExtractor) fetchFeed(ctx context.Context, feedURL string) ([]byte, []string, error) {
	var warnings []string

	// Create HTTP client with redirect limit (same as Tier 3 REST).
	client := *e.client
	client.CheckRedirect = limitRedirects(10)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/calendar")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response body

	// HTTP 204 No Content → empty calendar, not an error.
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Check Content-Type — warn if not text/calendar.
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !isCalendarContentType(ct) {
		warnings = append(warnings,
			fmt.Sprintf("expected text/calendar, got %s — likely error page", ct))
	}

	// Read with body size limit. Read limit+1 bytes to detect overflow.
	limitReader := io.LimitReader(resp.Body, e.maxBodyBytes+1)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, warnings, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > e.maxBodyBytes {
		return nil, warnings, fmt.Errorf("body exceeds %d bytes limit", e.maxBodyBytes)
	}

	return data, warnings, nil
}

// isCalendarContentType checks if the Content-Type is a calendar MIME type.
func isCalendarContentType(ct string) bool {
	// Normalize: truncate and lowercase before checking prefix.
	if len(ct) > 50 {
		ct = ct[:50]
	}
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "text/calendar") || strings.HasPrefix(ct, "application/ics")
}

// scrapeICS handles ICS extraction for a single source.
func (s *Scraper) scrapeICS(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       source.Tier,
		DryRun:     opts.DryRun,
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		httpClient := opts.HTTPClient(fetchTimeout)
		maxBody := s.icsConfig.MaxBodyBytes
		if maxBody <= 0 {
			maxBody = defaultICSMaxBodyBytes
		}
		extractor := NewICSExtractor(httpClient, maxBody)

		eventInputs, warnings, err := extractor.Extract(ctx, source, s.icsConfig)
		if err != nil {
			return 0, nil, warnings, err
		}

		// Apply limit if set.
		if opts.Limit > 0 && len(eventInputs) > opts.Limit {
			eventInputs = eventInputs[:opts.Limit]
		}

		return len(eventInputs), eventInputs, warnings, nil
	}), nil
}
