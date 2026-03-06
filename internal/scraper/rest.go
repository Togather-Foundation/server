package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog"
)

// RestExtractor fetches events from a REST JSON feed endpoint.
type RestExtractor struct {
	logger zerolog.Logger
}

// maxRESTRedirects is the maximum number of HTTP redirects the REST scraper
// will follow per request. This matches Go's default limit but makes it
// explicit for auditability. Unlike jsonld.go which blocks all redirects
// (SSRF hardening), REST endpoints may legitimately redirect (e.g. Showpass
// returns 301 for canonical URLs).
const maxRESTRedirects = 10

// NewRestExtractor constructs a RestExtractor.
func NewRestExtractor(logger zerolog.Logger) *RestExtractor {
	return &RestExtractor{logger: logger}
}

// FetchAndExtractREST fetches the REST JSON feed defined in source.REST,
// follows pagination via the next_field URL up to source.MaxPages pages
// (0 = no limit), maps each item to a RawEvent using field_map, and returns
// the combined slice.
//
// Timeout precedence (mirrors graphql.go): the effective HTTP timeout is the
// larger of the caller-supplied client.Timeout and cfg.TimeoutMs. This allows
// a source config to extend the global timeout for slow endpoints without ever
// tightening it below what the caller already provides.
func (e *RestExtractor) FetchAndExtractREST(
	ctx context.Context,
	source SourceConfig,
	client *http.Client,
) ([]RawEvent, error) {
	cfg := source.REST
	if cfg == nil {
		return nil, fmt.Errorf("rest: config is nil for source %q", source.Name)
	}

	// Create a local client copy to avoid mutating the caller's client.
	// Apply config timeout when it exceeds the caller-supplied timeout.
	localClient := &http.Client{
		Transport: client.Transport,
		Timeout:   client.Timeout,
	}
	if cfg.TimeoutMs > 0 {
		if cfgTimeout := time.Duration(cfg.TimeoutMs) * time.Millisecond; cfgTimeout > localClient.Timeout {
			localClient.Timeout = cfgTimeout
		}
	}
	// Limit redirects to prevent abuse via redirect chains. Unlike jsonld.go
	// which blocks all redirects (SSRF hardening for arbitrary web pages),
	// REST API endpoints may legitimately redirect (e.g. Showpass 301).
	localClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRESTRedirects {
			return http.ErrUseLastResponse
		}
		return nil
	}
	client = localClient

	// Parse URL template once (if provided).
	var urlTmpl *template.Template
	if cfg.URLTemplate != "" {
		var err error
		urlTmpl, err = template.New("url").Option("missingkey=error").Parse(cfg.URLTemplate)
		if err != nil {
			return nil, fmt.Errorf("rest: parsing url_template: %w", err)
		}
	}

	var allEvents []RawEvent
	nextURL := cfg.Endpoint
	page := 0

	for nextURL != "" {
		// Check max_pages limit (0 = no limit).
		if source.MaxPages > 0 && page >= source.MaxPages {
			break
		}
		page++

		pageEvents, next, err := e.fetchPage(ctx, cfg, client, nextURL, urlTmpl)
		if err != nil {
			return nil, err
		}

		e.logger.Debug().
			Str("source", source.Name).
			Str("url", nextURL).
			Int("page", page).
			Int("events", len(pageEvents)).
			Msg("rest: extracted events from page")

		allEvents = append(allEvents, pageEvents...)
		nextURL = next
	}

	e.logger.Debug().
		Str("source", source.Name).
		Str("endpoint", cfg.Endpoint).
		Int("pages", page).
		Int("total_events", len(allEvents)).
		Msg("rest: extraction complete")

	return allEvents, nil
}

// fetchPage fetches a single page from pageURL and returns the events plus
// the URL of the next page (empty string = no more pages).
func (e *RestExtractor) fetchPage(
	ctx context.Context,
	cfg *RestConfig,
	client *http.Client,
	pageURL string,
	urlTmpl *template.Template,
) ([]RawEvent, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("rest: creating request for %s: %w", pageURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", scraperUserAgent)
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("rest: request failed for %s: %w", pageURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("rest: unexpected status %d from %s", resp.StatusCode, pageURL)
	}

	// Decode page JSON. Limit body to 10 MiB to prevent memory exhaustion
	// (consistent with graphql.go and jsonld.go).
	var page map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("rest: decoding response from %s: %w", pageURL, err)
	}

	// Extract results array.
	rawResults, ok := page[cfg.ResultsField]
	if !ok {
		// Missing results field is treated as empty (not an error — some APIs
		// omit the key entirely on an empty final page).
		return nil, "", nil
	}

	var items []map[string]any
	if err := json.Unmarshal(rawResults, &items); err != nil {
		return nil, "", fmt.Errorf("rest: decoding %q array from %s: %w", cfg.ResultsField, pageURL, err)
	}

	// Determine next page URL.
	var nextURL string
	if rawNext, ok := page[cfg.NextField]; ok {
		// next can be a JSON string or null.
		var nextStr string
		if err := json.Unmarshal(rawNext, &nextStr); err == nil && nextStr != "" {
			// SSRF guard: next URL host must match the configured endpoint host.
			nu, parseErr := url.Parse(nextStr)
			epURL, _ := url.Parse(cfg.Endpoint)
			if parseErr != nil || nu.Host != epURL.Host {
				e.logger.Warn().
					Str("next_url", nextStr).
					Str("endpoint_host", epURL.Host).
					Msg("rest: next URL host mismatch — stopping pagination")
				// Return accumulated results up to this point; treat as end of pagination.
			} else {
				nextURL = nextStr
			}
		}
	}

	// Map items to RawEvents.
	events := make([]RawEvent, 0, len(items))
	for _, item := range items {
		raw := mapRESTItemToRawEvent(item, cfg.FieldMap, urlTmpl, e.logger)
		events = append(events, raw)
	}

	return events, nextURL, nil
}

// resolveNestedString traverses item using a dot-separated path and returns
// the leaf value as a string. Returns "" if any segment is missing, a non-map
// intermediate is encountered, or the leaf is not a string.
func resolveNestedString(item map[string]any, path string) string {
	if path == "" {
		return ""
	}
	segments := strings.Split(path, ".")
	current := item
	for _, seg := range segments[:len(segments)-1] {
		next, ok := current[seg]
		if !ok {
			return ""
		}
		current, ok = next.(map[string]any)
		if !ok {
			return ""
		}
	}
	leaf := segments[len(segments)-1]
	v, ok := current[leaf].(string)
	if !ok {
		return ""
	}
	return v
}

// mapRESTItemToRawEvent maps a REST JSON item (map[string]any) to a RawEvent
// using the operator-supplied field_map. When fieldMap is nil the RawEvent Go
// field names are used directly as JSON keys (identity mapping using the exact
// Go struct field names: Name, StartDate, EndDate, URL, Image, Location,
// Description).
func mapRESTItemToRawEvent(item map[string]any, fieldMap map[string]string, urlTmpl *template.Template, logger zerolog.Logger) RawEvent {
	// resolve returns the string value of the JSON field whose name is
	// determined by fieldMap[key] (or the identity mapping when fieldMap is nil).
	// key is the field_map logical key (e.g. "name", "start_date").
	// identityKey is the RawEvent Go struct field name used for identity mapping.
	resolve := func(key, identityKey string) string {
		var srcKey string
		if len(fieldMap) > 0 {
			mapped, ok := fieldMap[key]
			if !ok {
				// key not in field_map — skip.
				return ""
			}
			srcKey = mapped
		} else {
			// Identity mapping: use the Go struct field name.
			srcKey = identityKey
		}
		return resolveNestedString(item, srcKey)
	}

	raw := RawEvent{
		Name:        resolve("name", "Name"),
		StartDate:   resolve("start_date", "StartDate"),
		EndDate:     resolve("end_date", "EndDate"),
		Location:    resolve("location", "Location"),
		Description: resolve("description", "Description"),
		Image:       resolve("image", "Image"),
	}

	// URL: either from field_map/identity or (if a url_template is set) from the
	// rendered template. Template takes precedence when set.
	if urlTmpl != nil {
		var buf bytes.Buffer
		if err := urlTmpl.Execute(&buf, item); err != nil {
			// missingkey=error: a missing template variable returns an error here.
			// Clear the URL so each event gets a unique content-based ID instead
			// of a malformed URL shared by all events with the missing field.
			logger.Debug().Err(err).Msg("rest: url_template execution failed — clearing URL")
		} else if buf.Len() > 0 {
			raw.URL = buf.String()
		}
	} else {
		raw.URL = resolve("url", "URL")
	}

	return raw
}
