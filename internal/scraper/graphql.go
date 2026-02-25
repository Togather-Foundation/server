package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog"
)

const graphqlDefaultTimeoutMs = 30_000

// GraphQLExtractor fetches events from a GraphQL API endpoint.
type GraphQLExtractor struct {
	logger zerolog.Logger
}

// NewGraphQLExtractor constructs a GraphQLExtractor.
func NewGraphQLExtractor(logger zerolog.Logger) *GraphQLExtractor {
	return &GraphQLExtractor{logger: logger}
}

// FetchAndExtractGraphQL executes the GraphQL query defined in source.GraphQL,
// maps each returned event object to a RawEvent using the field names, and
// returns the slice. URLTemplate (if non-empty) is rendered per-event using
// the raw event map as template data.
func (e *GraphQLExtractor) FetchAndExtractGraphQL(
	ctx context.Context,
	source SourceConfig,
	client *http.Client,
) ([]RawEvent, error) {
	cfg := source.GraphQL
	if cfg == nil {
		return nil, fmt.Errorf("graphql config is nil for source %q", source.Name)
	}

	// Apply timeout from config.
	if cfg.TimeoutMs > 0 && client.Timeout == 0 {
		client = &http.Client{
			Timeout:   time.Duration(cfg.TimeoutMs) * time.Millisecond,
			Transport: client.Transport,
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
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql: unexpected status %d from %s", resp.StatusCode, cfg.Endpoint)
	}

	// Decode: {"data": {"<eventField>": [...]}}
	var envelope struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("graphql: decoding response: %w", err)
	}
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
		urlTmpl, err = template.New("url").Parse(cfg.URLTemplate)
		if err != nil {
			return nil, fmt.Errorf("graphql: parsing url_template: %w", err)
		}
	}

	events := make([]RawEvent, 0, len(items))
	for _, item := range items {
		raw := mapToRawEvent(item, urlTmpl)
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
func mapToRawEvent(item map[string]any, urlTmpl *template.Template) RawEvent {
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

	// Construct event URL from template (e.g. "https://tranzac.org/events/{{.slug}}")
	if urlTmpl != nil {
		if slug, ok := item["slug"].(string); ok && slug != "" {
			var buf bytes.Buffer
			if err := urlTmpl.Execute(&buf, item); err == nil {
				raw.URL = buf.String()
			}
		}
	}

	return raw
}
