package scraper

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/ical"
	"github.com/rs/zerolog"
)

// ICSExtractor fetches and parses an ICS feed URL.
type ICSExtractor struct {
	client             *http.Client
	maxBodyBytes       int64 // Default: 10 * 1024 * 1024 (10 MB)
	insecureSkipVerify bool  // Skip TLS verification
	logger             zerolog.Logger
}

// defaultICSMaxBodyBytes is the default maximum ICS feed body size.
const defaultICSMaxBodyBytes int64 = 10 * 1024 * 1024 // 10 MB

// NewICSExtractor creates an ICS extractor with the given HTTP client.
// maxBodyBytes defaults to 10 MB if <= 0.
// insecureSkipVerify disables TLS certificate verification (use for dev/testing only).
func NewICSExtractor(client *http.Client, maxBodyBytes int64, insecureSkipVerify bool, logger zerolog.Logger) *ICSExtractor {
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultICSMaxBodyBytes
	}
	return &ICSExtractor{
		client:             client,
		maxBodyBytes:       maxBodyBytes,
		insecureSkipVerify: insecureSkipVerify,
		logger:             logger,
	}
}

// Extract fetches the ICS feed and returns EventInputs.
func (e *ICSExtractor) Extract(ctx context.Context, cfg SourceConfig, icsConfig config.ICSConfig) ([]events.EventInput, []string, error) {
	var allWarnings []string

	// 1. Fetch the ICS feed.
	data, fetchWarnings, err := e.fetchFeed(ctx, cfg.URL, cfg.Headers)
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
		SourceURL:   cfg.URL,
		SourceName:  cfg.Name,
		TrustLevel:  cfg.TrustLevel,
		License:     cfg.License,
		Timezone:    cfg.Timezone,
		CountryCode: "CA",
		Logger:      e.logger,
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

// ICSUserAgent is defined in useragent.go.

// fetchFeed fetches the ICS feed from the given URL with body size limits
// and redirect handling. extraHeaders are merged on top of the default
// headers (User-Agent, Accept); a User-Agent in extraHeaders overrides the default.
func (e *ICSExtractor) fetchFeed(ctx context.Context, feedURL string, extraHeaders map[string]string) ([]byte, []string, error) {
	var warnings []string

	// Create HTTP client with redirect limit (same as Tier 3 REST).
	client := *e.client
	client.CheckRedirect = limitRedirects(10)

	// Apply TLS config if insecure skip verify is enabled.
	if e.insecureSkipVerify {
		transport := client.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		client.Transport = &http.Transport{
			Proxy:               transport.(*http.Transport).Proxy,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			DialContext:         transport.(*http.Transport).DialContext,
			TLSHandshakeTimeout: transport.(*http.Transport).TLSHandshakeTimeout,
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/calendar")
	req.Header.Set("User-Agent", ICSUserAgent)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

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
		// Apply source-specific timeout override if set.
		timeout := fetchTimeout
		if source.RequestTimeoutSeconds > 0 {
			timeout = time.Duration(source.RequestTimeoutSeconds) * time.Second
		}
		opts.TLSFingerprint = source.TLSFingerprint
		httpClient := opts.HTTPClient(timeout)
		maxBody := s.icsConfig.MaxBodyBytes
		if maxBody <= 0 {
			maxBody = defaultICSMaxBodyBytes
		}
		// Override with source-specific max_body_bytes if set.
		if source.MaxBodyBytes > 0 {
			maxBody = source.MaxBodyBytes
		}
		extractor := NewICSExtractor(httpClient, maxBody, source.InsecureSkipVerify, s.logger)

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
