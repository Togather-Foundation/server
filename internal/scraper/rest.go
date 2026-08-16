package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

// Extract fetches the REST JSON feed defined in source.REST,
// follows pagination via the next_field URL up to source.MaxPages pages
// (0 = no limit), maps each item to a RawEvent using field_map, and returns
// the combined slice.
//
// Timeout precedence (mirrors graphql.go): the effective HTTP timeout is the
// larger of the caller-supplied client.Timeout and cfg.TimeoutMs. This allows
// a source config to extend the global timeout for slow endpoints without ever
// tightening it below what the caller already provides.
func (e *RestExtractor) Extract(
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
	// Limit redirects to prevent abuse via redirect chains. Unlike jsonld.go
	// which blocks all redirects (SSRF hardening for arbitrary web pages),
	// REST API endpoints may legitimately redirect (e.g. Showpass 301).
	localClient := safeClient(client, limitRedirects(maxRESTRedirects))
	if cfg.TimeoutMs > 0 {
		if cfgTimeout := time.Duration(cfg.TimeoutMs) * time.Millisecond; cfgTimeout > localClient.Timeout {
			localClient.Timeout = cfgTimeout
		}
	}
	client = localClient

	if source.TLSFingerprint != "" {
		merged := mergeHeaders(ChromeHeaders(), cfg.Headers)
		cfgCopy := *cfg
		cfgCopy.Headers = merged
		cfg = &cfgCopy
	}

	// Parse URL template once (if provided).
	var urlTmpl *template.Template
	if cfg.URLTemplate != "" {
		var err error
		urlTmpl, err = template.New("url").Option("missingkey=error").Parse(cfg.URLTemplate)
		if err != nil {
			return nil, fmt.Errorf("rest: parsing url_template: %w", err)
		}
	}

	// Pre-parse any templated field_map values once (not per event).
	resolver, err := newFieldMapResolver(cfg.FieldMap)
	if err != nil {
		return nil, fmt.Errorf("rest: %w", err)
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

		pageEvents, next, err := e.fetchPage(ctx, cfg, client, nextURL, urlTmpl, resolver)
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
	resolver *fieldMapResolver,
) ([]RawEvent, string, error) {
	method := cfg.Method
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if method == http.MethodPost {
		bodyReader = strings.NewReader(cfg.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, pageURL, bodyReader)
	if err != nil {
		return nil, "", fmt.Errorf("rest: creating request for %s: %w", pageURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ScraperUserAgent)
	if method == http.MethodPost {
		contentType := cfg.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("rest: request failed for %s: %w", pageURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read body with 10 MiB limit to prevent memory exhaustion
	// (consistent with graphql.go and jsonld.go).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("rest: reading response from %s: %w", pageURL, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("rest: unexpected status %d from %s", resp.StatusCode, pageURL)
	}

	var items []map[string]any
	var nextURL string

	if cfg.ResultsField == "." {
		// Bare array mode: the entire response body is the results array.
		// No pagination support — bare arrays have no envelope to carry next URLs.
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, "", fmt.Errorf("rest: decoding bare array from %s: %w", pageURL, err)
		}
	} else {
		// Object mode: response is a JSON object with named fields.
		var page map[string]json.RawMessage
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, "", fmt.Errorf("rest: decoding response from %s: %w", pageURL, err)
		}

		// Extract results array.
		rawResults, ok := page[cfg.ResultsField]
		if !ok {
			// Missing results field is treated as empty (not an error — some APIs
			// omit the key entirely on an empty final page).
			return nil, "", nil
		}

		if cfg.Flatten {
			// Nested flattening: results_field resolves to an object whose leaves
			// are arrays of event objects (e.g. Leap's
			// events_by_month.<month>.dates.<day>[]). Walk depth-first collecting
			// every array-of-objects leaf in sorted-key order.
			var raw any
			if err := json.Unmarshal(rawResults, &raw); err != nil {
				return nil, "", fmt.Errorf("rest: decoding %q from %s: %w", cfg.ResultsField, pageURL, err)
			}
			items = flattenResults(raw, e.logger)
		} else {
			if err := json.Unmarshal(rawResults, &items); err != nil {
				return nil, "", fmt.Errorf("rest: decoding %q array from %s: %w", cfg.ResultsField, pageURL, err)
			}
		}

		// Determine next page URL (only meaningful for object responses).
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
	}

	// Map items to RawEvents.
	events := make([]RawEvent, 0, len(items))
	for _, item := range items {
		raw := mapRESTItemToRawEvent(item, resolver, urlTmpl, e.logger)
		events = append(events, raw)
	}

	return events, nextURL, nil
}

// mergeHeaders creates a new map with base entries, then overwrites with
// override entries. Source-specific headers take precedence over defaults.
func mergeHeaders(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
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

// fieldMapResolver resolves RawEvent fields from a source item using the
// operator-supplied field_map. It pre-parses any templated field_map values
// (values containing "{{") once, so templates are not recompiled per event.
// Plain (non-templated) values are resolved via resolveNestedString; templated
// values are rendered as Go text/template against the item map, allowing one
// target field to combine multiple source keys (e.g.
// start_date: "{{.start_date}}T{{.start_time}}").
type fieldMapResolver struct {
	fieldMap  map[string]string
	templates map[string]*template.Template
}

// newFieldMapResolver builds a fieldMapResolver from fieldMap, pre-parsing any
// templated values. It returns an error if a templated value fails to parse.
// When fieldMap is nil/empty the resolver behaves as an identity mapping
// (RawEvent Go field names are used directly as JSON keys).
func newFieldMapResolver(fieldMap map[string]string) (*fieldMapResolver, error) {
	r := &fieldMapResolver{fieldMap: fieldMap}
	for k, v := range fieldMap {
		if !strings.Contains(v, "{{") {
			continue
		}
		t, err := template.New("fieldmap:" + k).Option("missingkey=error").Parse(v)
		if err != nil {
			return nil, fmt.Errorf("parsing field_map template for %q: %w", k, err)
		}
		if r.templates == nil {
			r.templates = make(map[string]*template.Template)
		}
		r.templates[k] = t
	}
	return r, nil
}

// resolve returns the string value for the logical field key (e.g. "name").
// When the resolver's fieldMap is empty, identityKey (the RawEvent Go struct
// field name) is used directly. A templated value is rendered against item; a
// plain value is resolved as a nested dot-separated path.
func (r *fieldMapResolver) resolve(item map[string]any, key, identityKey string, logger zerolog.Logger) string {
	var srcKey string
	if len(r.fieldMap) > 0 {
		mapped, ok := r.fieldMap[key]
		if !ok {
			// key not in field_map — skip.
			return ""
		}
		srcKey = mapped
	} else {
		// Identity mapping: use the Go struct field name.
		srcKey = identityKey
	}

	if tmpl, ok := r.templates[key]; ok {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, item); err != nil {
			// missingkey=error: a missing template variable returns an error here.
			// Leave the field empty rather than emitting "<no value>" or failing
			// the whole event.
			logger.Debug().Err(err).Str("field", key).Msg("field_map template execution failed — leaving field empty")
			return ""
		}
		return buf.String()
	}
	return resolveNestedString(item, srcKey)
}

// flattenResults walks a decoded JSON value depth-first and collects every
// array-of-objects leaf, concatenating them in stable order. Map keys are
// iterated in sorted order so the flattened result is deterministic regardless
// of JSON object key ordering in the source. A leaf array whose elements are
// all objects is treated as a terminal event list (its elements are appended
// directly); arrays containing non-objects are recursed into. If the subtree
// contains scalar values alongside arrays, a warning is logged (the scalars are
// dropped — they carry no event data).
func flattenResults(v any, logger zerolog.Logger) []map[string]any {
	var out []map[string]any
	var sawScalar bool

	var walk func(any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(n[k])
			}
		case []any:
			// An array whose elements are all objects is an event-list leaf.
			allObjects := true
			for _, el := range n {
				if _, ok := el.(map[string]any); !ok {
					allObjects = false
					break
				}
			}
			if allObjects {
				for _, el := range n {
					out = append(out, el.(map[string]any))
				}
				return
			}
			for _, el := range n {
				walk(el)
			}
		default:
			sawScalar = true
		}
	}
	walk(v)

	if sawScalar {
		logger.Warn().Msg("rest: flatten: results object contained scalar values alongside arrays — scalars ignored")
	}
	return out
}

// mapRESTItemToRawEvent maps a REST JSON item (map[string]any) to a RawEvent
// using the operator-supplied field_map (via the pre-parsed resolver). When
// resolver has an empty fieldMap the RawEvent Go field names are used directly
// as JSON keys (identity mapping using the exact Go struct field names: Name,
// StartDate, EndDate, URL, Image, Location, Description).
func mapRESTItemToRawEvent(item map[string]any, resolver *fieldMapResolver, urlTmpl *template.Template, logger zerolog.Logger) RawEvent {
	raw := RawEvent{
		Name:        resolver.resolve(item, "name", "Name", logger),
		StartDate:   resolver.resolve(item, "start_date", "StartDate", logger),
		EndDate:     resolver.resolve(item, "end_date", "EndDate", logger),
		Location:    resolver.resolve(item, "location", "Location", logger),
		Description: resolver.resolve(item, "description", "Description", logger),
		Image:       resolver.resolve(item, "image", "Image", logger),
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
		raw.URL = resolver.resolve(item, "url", "URL", logger)
	}

	return raw
}
