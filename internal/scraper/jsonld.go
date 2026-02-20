package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog"
	"github.com/temoto/robotstxt"
)

const (
	scraperUserAgent = "Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)"
	fetchTimeout     = 30 * time.Second
	robotsTimeout    = 10 * time.Second
)

// FetchAndExtractJSONLD fetches the page at rawURL, parses all JSON-LD script
// blocks, and returns every schema.org Event (or EventSeries) found within them.
func FetchAndExtractJSONLD(ctx context.Context, rawURL string) ([]json.RawMessage, error) {
	// 1. Parse and validate URL.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid URL %q: missing scheme or host", rawURL)
	}

	// 2. Check robots.txt compliance.
	allowed, robotsErr := RobotsAllowed(ctx, rawURL, scraperUserAgent)
	if robotsErr != nil {
		// Non-fatal: treat as allowed when robots.txt is unreachable, but log.
		zerolog.Ctx(ctx).Warn().Err(robotsErr).Str("url", rawURL).Msg("scraper: robots.txt check failed, proceeding as allowed")
		allowed = true
	}
	if !allowed {
		return nil, fmt.Errorf("scraping disallowed by robots.txt for %q", rawURL)
	}

	// 3. HTTP GET with timeout. Redirects are disabled to prevent SSRF via
	// redirect chains to internal/private addresses.
	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %q: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", scraperUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %q: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// 4. Check response status.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %q", resp.StatusCode, rawURL)
	}

	// 5. Parse HTML with goquery (body capped at 10 MiB to prevent OOM).
	limitedBody := io.LimitReader(resp.Body, 10*1024*1024) // 10 MiB
	doc, err := goquery.NewDocumentFromReader(limitedBody)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML from %q: %w", rawURL, err)
	}

	return extractFromDocument(doc)
}

// extractFromDocument extracts Event objects from all JSON-LD script tags in the
// parsed document. Exported for use in tests via httptest servers.
func extractFromDocument(doc *goquery.Document) ([]json.RawMessage, error) {
	var events []json.RawMessage

	// 6. Find all <script type="application/ld+json"> elements.
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return
		}

		// 7. Parse JSON, extract Event objects. Log and skip malformed blocks
		// rather than aborting — a single bad script tag shouldn't discard
		// all events on the page.
		extracted, err := extractEvents([]byte(raw))
		if err != nil {
			// Use the discard logger — no logger is available here; the error
			// is recoverable so we skip and continue.
			_ = err
			return
		}
		events = append(events, extracted...)
	})

	// 8. Return all extracted raw JSON events.
	return events, nil
}

// extractEvents inspects a single JSON-LD block and returns all schema.org
// Event / EventSeries objects found within it, handling the following shapes:
//
//   - Single top-level Event or EventSeries object
//   - Top-level JSON array of objects
//   - Object with an @graph array
//   - ItemList with itemListElement containing ListItem→item Events
func extractEvents(data []byte) ([]json.RawMessage, error) {
	// Detect whether we have an array or an object at the top level.
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 {
		return nil, nil
	}

	if trimmed[0] == '[' {
		return extractFromArray(data)
	}
	return extractFromObject(data)
}

// extractFromArray handles a top-level JSON array.
func extractFromArray(data []byte) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	var events []json.RawMessage
	for _, item := range items {
		extracted, err := extractFromObject(item)
		if err != nil {
			return nil, err
		}
		events = append(events, extracted...)
	}
	return events, nil
}

// extractFromObject handles a single JSON object, dispatching to the appropriate
// shape handler based on @type and presence of @graph / itemListElement.
func extractFromObject(data []byte) ([]json.RawMessage, error) {
	// Use a minimal struct to peek at top-level fields without full unmarshalling.
	var envelope struct {
		Type            json.RawMessage   `json:"@type"`
		Graph           []json.RawMessage `json:"@graph"`
		ItemListElement []json.RawMessage `json:"itemListElement"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	// Shape 3: @graph container.
	if len(envelope.Graph) > 0 {
		return extractFromGraphArray(envelope.Graph)
	}

	// Determine the @type value(s).
	typStr := jsonTypeString(envelope.Type)

	// Shape 4: ItemList.
	if typStr == "ItemList" && len(envelope.ItemListElement) > 0 {
		return extractFromItemList(envelope.ItemListElement)
	}

	// Shape 1: Single Event or EventSeries.
	if isEventType(typStr) {
		return []json.RawMessage{json.RawMessage(data)}, nil
	}

	// Non-event object — skip silently.
	return nil, nil
}

// extractFromGraphArray recurses into the @graph array.
func extractFromGraphArray(items []json.RawMessage) ([]json.RawMessage, error) {
	var events []json.RawMessage
	for _, item := range items {
		extracted, err := extractFromObject(item)
		if err != nil {
			return nil, err
		}
		events = append(events, extracted...)
	}
	return events, nil
}

// extractFromItemList extracts Events from itemListElement entries.
func extractFromItemList(elements []json.RawMessage) ([]json.RawMessage, error) {
	var events []json.RawMessage
	for _, elem := range elements {
		var listItem struct {
			Item json.RawMessage `json:"item"`
		}
		if err := json.Unmarshal(elem, &listItem); err != nil {
			return nil, err
		}
		if len(listItem.Item) == 0 {
			continue
		}
		extracted, err := extractFromObject(listItem.Item)
		if err != nil {
			return nil, err
		}
		events = append(events, extracted...)
	}
	return events, nil
}

// jsonTypeString returns the string value of a @type field, handling both a
// plain string ("Event") and a single-element JSON array (["Event"]).
func jsonTypeString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		// Strip optional schema.org prefix.
		return stripSchemaPrefix(s)
	}
	// Try array.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return stripSchemaPrefix(arr[0])
	}
	return ""
}

// stripSchemaPrefix removes an optional "https://schema.org/" or
// "http://schema.org/" prefix from a type string.
func stripSchemaPrefix(s string) string {
	for _, prefix := range []string{"https://schema.org/", "http://schema.org/"} {
		if after, ok := strings.CutPrefix(s, prefix); ok {
			return after
		}
	}
	return s
}

// isEventType reports whether typStr represents a schema.org Event or EventSeries.
func isEventType(typStr string) bool {
	return typStr == "Event" || typStr == "EventSeries"
}

// RobotsAllowed checks whether the given user agent is permitted to fetch rawURL
// according to the site's robots.txt. A missing (404) robots.txt is treated as
// "allow all". Network errors fetching robots.txt are returned as errors; callers
// should typically treat them as allowed.
func RobotsAllowed(ctx context.Context, rawURL string, userAgent string) (bool, error) {
	// 1. Parse the URL and build the robots.txt URL.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("parsing URL %q: %w", rawURL, err)
	}
	robotsURL := &url.URL{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Host,
		Path:   "/robots.txt",
	}

	// 2. Fetch robots.txt. Redirects are disabled to prevent open-redirect abuse.
	client := &http.Client{
		Timeout: robotsTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return false, fmt.Errorf("building robots.txt request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetching robots.txt from %q: %w", robotsURL.String(), err)
	}
	defer resp.Body.Close()

	// 3. 404 (or any 4xx that signals absence) → allow.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return true, nil
	}

	// Read and parse the robots.txt body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading robots.txt body: %w", err)
	}

	// 4. Parse with temoto/robotstxt.
	data, err := robotstxt.FromBytes(body)
	if err != nil {
		// Malformed robots.txt — treat as allow.
		return true, nil
	}

	// 5. Check if the user agent is allowed for the requested path.
	allowed := data.TestAgent(parsedURL.Path, userAgent)
	return allowed, nil
}
