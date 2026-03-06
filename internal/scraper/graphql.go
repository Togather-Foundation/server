package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog"
)

// GraphQLExtractor fetches events from a GraphQL API endpoint.
type GraphQLExtractor struct {
	logger zerolog.Logger
}

// NewGraphQLExtractor constructs a GraphQLExtractor.
func NewGraphQLExtractor(logger zerolog.Logger) *GraphQLExtractor {
	return &GraphQLExtractor{logger: logger}
}

// Extract executes the GraphQL query defined in source.GraphQL,
// maps each returned event object to a RawEvent using the field names, and
// returns the slice. URLTemplate (if non-empty) is rendered per-event using
// the raw event map as template data.
//
// Timeout precedence: the effective HTTP timeout is the larger of the
// caller-supplied client.Timeout and cfg.TimeoutMs. This allows a source
// config to extend the global timeout for unusually slow GraphQL endpoints
// without ever tightening it below what the caller already provides.
// If cfg.TimeoutMs is zero the caller's timeout is used unchanged.
func (e *GraphQLExtractor) Extract(
	ctx context.Context,
	source SourceConfig,
	client *http.Client,
) ([]RawEvent, error) {
	cfg := source.GraphQL
	if cfg == nil {
		return nil, fmt.Errorf("graphql config is nil for source %q", source.Name)
	}

	// Apply config timeout when it exceeds the caller-supplied timeout
	// (see godoc for precedence rationale).
	if cfg.TimeoutMs > 0 {
		if cfgTimeout := time.Duration(cfg.TimeoutMs) * time.Millisecond; cfgTimeout > client.Timeout {
			client = &http.Client{
				Timeout:   cfgTimeout,
				Transport: client.Transport,
			}
		}
	}

	// Build request body.
	body, err := json.Marshal(map[string]string{"query": cfg.Query})
	if err != nil {
		return nil, fmt.Errorf("graphql: marshalling query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("graphql: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", scraperUserAgent)
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql: unexpected status %d from %s", resp.StatusCode, cfg.Endpoint)
	}

	// Decode: {"data": {"<eventField>": [...]}}
	// Limit the response body to 10 MiB to prevent memory exhaustion from
	// misbehaving or hostile endpoints (consistent with jsonld.go).
	var envelope struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("graphql: decoding response: %w", err)
	}
	// Conservative: treat any errors as a failure even if partial data is present (GraphQL spec allows both).
	if len(envelope.Errors) > 0 {
		msgs := make([]string, len(envelope.Errors))
		for i, e := range envelope.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("graphql: API errors: %s", strings.Join(msgs, "; "))
	}

	rawField, ok := envelope.Data[cfg.EventField]
	if !ok {
		return nil, fmt.Errorf("graphql: event field %q not found in response data", cfg.EventField)
	}

	var items []map[string]any
	if err := json.Unmarshal(rawField, &items); err != nil {
		return nil, fmt.Errorf("graphql: decoding event array: %w", err)
	}

	// Parse URL template once (if provided).
	var urlTmpl *template.Template
	if cfg.URLTemplate != "" {
		urlTmpl, err = template.New("url").Option("missingkey=error").Parse(cfg.URLTemplate)
		if err != nil {
			return nil, fmt.Errorf("graphql: parsing url_template: %w", err)
		}
	}

	events := make([]RawEvent, 0, len(items))
	for _, item := range items {
		raw := mapToRawEvent(item, urlTmpl, e.logger)
		events = append(events, raw)
	}

	e.logger.Debug().
		Str("source", source.Name).
		Str("endpoint", cfg.Endpoint).
		Int("events", len(events)).
		Msg("graphql: extracted events")

	return events, nil
}

// mapToRawEvent maps a GraphQL event object (map[string]any) to a RawEvent.
// Field names follow the DatoCMS schema used by Tranzac but are generic enough
// for any source using the same conventions.
//
// urlTmpl, if non-nil, is rendered with the raw item map as data to produce the
// event's canonical URL. The template string comes from operator-supplied YAML
// config (not user input), so text/template (not html/template) is appropriate.
func mapToRawEvent(item map[string]any, urlTmpl *template.Template, logger zerolog.Logger) RawEvent {
	raw := RawEvent{}

	if v, ok := item["title"].(string); ok {
		raw.Name = v
	}
	if v, ok := item["startDate"].(string); ok {
		raw.StartDate = v
	}
	if v, ok := item["endDate"].(string); ok {
		raw.EndDate = v
	}
	if v, ok := item["description"].(string); ok {
		raw.Description = v
	}

	// photo: { url: "..." }
	if photo, ok := item["photo"].(map[string]any); ok {
		if u, ok := photo["url"].(string); ok {
			raw.Image = u
		}
	}

	// rooms: [{ name: "..." }] — use first room as location
	if rooms, ok := item["rooms"].([]any); ok && len(rooms) > 0 {
		if room, ok := rooms[0].(map[string]any); ok {
			if name, ok := room["name"].(string); ok {
				raw.Location = name
			}
		}
	}

	// Construct event URL from template (e.g. "https://tranzac.org/events/{{.slug}}").
	// The template is executed unconditionally so sources can use any field (not
	// just "slug") as the URL key. Template execution errors are logged at debug
	// level rather than failing the whole event.
	if urlTmpl != nil {
		var buf bytes.Buffer
		if err := urlTmpl.Execute(&buf, item); err != nil {
			// missingkey=error: a missing template variable returns an error here.
			// Clear the URL so each event gets a unique content-based ID instead
			// of a malformed URL shared by all events with the missing field.
			logger.Debug().Err(err).Msg("graphql: url_template execution failed — clearing URL")
		} else if buf.Len() > 0 {
			raw.URL = buf.String()
		}
	}

	return raw
}
