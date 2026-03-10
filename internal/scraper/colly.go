package scraper

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"github.com/rs/zerolog"
)

// CollyExtractor performs Tier 1 CSS-selector-based scraping using Colly.
type CollyExtractor struct {
	userAgent string
	rateLimit time.Duration
	transport http.RoundTripper // optional; nil = Colly default
	logger    zerolog.Logger
}

// NewCollyExtractor returns a CollyExtractor with the standard SEL User-Agent
// and a 1-second per-domain rate limit.
func NewCollyExtractor(logger zerolog.Logger) *CollyExtractor {
	return &CollyExtractor{
		userAgent: "Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)",
		rateLimit: time.Second,
		logger:    logger,
	}
}

// SetTransport sets a custom http.RoundTripper on the extractor. Call before
// ScrapeWithSelectors. A nil transport uses Colly's default.
func (e *CollyExtractor) SetTransport(t http.RoundTripper) {
	e.transport = t
}

// SetRateLimit overrides the per-domain request delay used by the Colly
// collector. Call before ScrapeWithSelectors. A zero or negative value is
// ignored (keeps the default 1-second delay).
func (e *CollyExtractor) SetRateLimit(d time.Duration) {
	if d > 0 {
		e.rateLimit = d
	}
}

// ScrapeWithSelectors fetches config.URL and all linked pages (up to
// config.MaxPages), applying the CSS selectors in config.Selectors to collect
// RawEvents. When DateSelectors are configured, it populates always-indexed
// DateParts on each RawEvent and captures DateSelectorProbes from the first
// event container for diagnostic visibility. It respects robots.txt (Colly
// default) and applies per-domain rate limiting. If ctx is cancelled before
// scraping completes, the function returns whatever events were collected up
// to that point.
func (e *CollyExtractor) ScrapeWithSelectors(ctx context.Context, config SourceConfig) ([]RawEvent, []DateSelectorProbe, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Extract the allowed domain from the source URL and also allow
	// the www/non-www variant so that redirects between them are not blocked.
	allowedDomain, err := extractDomain(config.URL)
	if err != nil {
		return nil, nil, err
	}
	allowedDomains := wwwVariants(allowedDomain)

	var (
		mu          sync.Mutex
		results     []RawEvent
		firstProbes []DateSelectorProbe
		pagesSeen   int
		eventIndex  int // tracks how many event containers we've processed
	)

	maxPages := config.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}

	c := colly.NewCollector(
		colly.UserAgent(e.userAgent),
		colly.AllowedDomains(allowedDomains...),
		// robots.txt is respected by default in Colly; do NOT use IgnoreRobotsTxt.
	)

	// Apply custom transport if set (e.g. CachingTransport for dev).
	if e.transport != nil {
		c.WithTransport(e.transport)
	}

	// Per-domain rate limiting.
	if err := c.Limit(&colly.LimitRule{
		DomainGlob: "*",
		Delay:      e.rateLimit,
	}); err != nil {
		e.logger.Warn().Err(err).Msg("colly: failed to set rate limit rule")
	}

	// OnHTML: extract events from each matching event card.
	c.OnHTML(config.Selectors.EventList, func(h *colly.HTMLElement) {
		// Check context cancellation inside the callback.
		if ctx.Err() != nil {
			return
		}

		mu.Lock()
		eventIndex++
		isFirst := eventIndex == 1
		mu.Unlock()

		// Extract static fields (Name, Location, Description, URL, Image)
		// that appear once per event container.
		staticName := ""
		if config.Selectors.Name != "" {
			staticName = extractTextOrAttr(h, config.Selectors.Name)
		}

		staticLocation := ""
		if config.Selectors.Location != "" {
			staticLocation = extractTextOrAttr(h, config.Selectors.Location)
		}

		staticDescription := ""
		if config.Selectors.Description != "" {
			staticDescription = extractTextOrAttr(h, config.Selectors.Description)
		}

		staticURL := ""
		if config.Selectors.URL != "" {
			href := h.ChildAttr(config.Selectors.URL, "href")
			if href != "" {
				staticURL = h.Request.AbsoluteURL(href)
			}
		}

		staticImage := ""
		if config.Selectors.Image != "" {
			src := h.ChildAttr(config.Selectors.Image, "src")
			if src != "" {
				staticImage = h.Request.AbsoluteURL(src)
			}
		}

		// Skip events with no name.
		if staticName == "" {
			return
		}

		// Date extraction: prefer date_selectors (smart assembler) over
		// start_date/end_date (single-selector legacy path).
		if len(config.Selectors.DateSelectors) > 0 {
			// Try multi-row extraction first.
			dateRows := extractDateSelectorPartsPerRow(h, config.Selectors.DateSelectors)

			// Capture probes from the first event for diagnostics.
			var probes []DateSelectorProbe
			if isFirst {
				for i, sel := range config.Selectors.DateSelectors {
					var matched bool
					var text string
					if len(dateRows) > 0 && i < len(dateRows[0]) {
						text = dateRows[0][i]
						matched = text != ""
					} else {
						// Fallback single-row detection for probe reporting.
						text = strings.TrimSpace(h.ChildText(sel))
						matched = text != ""
					}
					probes = append(probes, DateSelectorProbe{
						Selector: sel,
						Matched:  matched,
						Text:     text,
					})
				}
				if len(probes) > 0 {
					mu.Lock()
					firstProbes = probes
					mu.Unlock()
				}
			}

			// Emit one RawEvent per row (multi-row case) or one RawEvent with combined DateParts (single-row case).
			if len(dateRows) > 1 {
				// Multi-row case: emit one RawEvent per row.
				for _, row := range dateRows {
					raw := RawEvent{
						Name:        staticName,
						Location:    staticLocation,
						Description: staticDescription,
						URL:         staticURL,
						Image:       staticImage,
						DateParts:   row, // Each row's parts
					}
					mu.Lock()
					results = append(results, raw)
					mu.Unlock()
				}
			} else if len(dateRows) == 1 {
				// Single-row case: emit one RawEvent.
				raw := RawEvent{
					Name:        staticName,
					Location:    staticLocation,
					Description: staticDescription,
					URL:         staticURL,
					Image:       staticImage,
					DateParts:   dateRows[0],
				}
				mu.Lock()
				results = append(results, raw)
				mu.Unlock()
			} else {
				// No rows matched; try fallback single-text-per-selector approach (backward compat).
				raw := RawEvent{
					Name:        staticName,
					Location:    staticLocation,
					Description: staticDescription,
					URL:         staticURL,
					Image:       staticImage,
				}
				for _, sel := range config.Selectors.DateSelectors {
					text := strings.TrimSpace(h.ChildText(sel))
					raw.DateParts = append(raw.DateParts, text)
				}
				mu.Lock()
				results = append(results, raw)
				mu.Unlock()
			}
		} else {
			// Legacy start_date/end_date selectors.
			raw := RawEvent{
				Name:        staticName,
				Location:    staticLocation,
				Description: staticDescription,
				URL:         staticURL,
				Image:       staticImage,
			}

			if config.Selectors.StartDate != "" {
				raw.StartDate = extractDateFromElement(h, config.Selectors.StartDate)
			}

			if config.Selectors.EndDate != "" {
				raw.EndDate = extractDateFromElement(h, config.Selectors.EndDate)
			}

			mu.Lock()
			results = append(results, raw)
			mu.Unlock()
		}
	})

	// OnHTML: follow pagination links if configured.
	if config.Selectors.Pagination != "" {
		c.OnHTML(config.Selectors.Pagination, func(h *colly.HTMLElement) {
			if ctx.Err() != nil {
				return
			}

			mu.Lock()
			current := pagesSeen
			mu.Unlock()

			if current >= maxPages {
				return
			}

			href := h.Attr("href")
			if href == "" {
				href = h.ChildAttr("a", "href")
			}
			if href == "" {
				return
			}

			nextURL := h.Request.AbsoluteURL(href)
			if nextURL == "" {
				return
			}

			if err := c.Visit(nextURL); err != nil {
				e.logger.Warn().Err(err).Str("url", nextURL).Msg("colly: failed to queue pagination URL")
			}
		})
	}

	// Track pages visited.
	c.OnRequest(func(r *colly.Request) {
		mu.Lock()
		pagesSeen++
		reachedMax := pagesSeen > maxPages
		mu.Unlock()

		if reachedMax {
			r.Abort()
			return
		}

		e.logger.Debug().
			Str("url", r.URL.String()).
			Int("page", pagesSeen).
			Msg("colly: visiting page")
	})

	// OnError: log and continue.
	c.OnError(func(r *colly.Response, err error) {
		if ctx.Err() != nil {
			return
		}
		e.logger.Warn().
			Str("url", r.Request.URL.String()).
			Int("status", r.StatusCode).
			Err(err).
			Msg("colly: request error")
	})

	// Start crawl — c.Visit is synchronous with async callbacks.
	if err := c.Visit(config.URL); err != nil {
		// If context was cancelled, don't surface the visit error.
		if ctx.Err() != nil {
			return results, firstProbes, nil
		}
		return nil, nil, err
	}

	// Wait for all async callbacks to complete.
	c.Wait()

	return results, firstProbes, nil
}

// extractDomain parses rawURL and returns just the hostname (no port).
func extractDomain(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname() // strips port if present
	return host, nil
}

// wwwVariants returns both the www and non-www forms of a domain so that
// Colly's AllowedDomains check doesn't block redirects between them.
// For example, "www.example.com" → ["www.example.com", "example.com"]
// and "example.com" → ["example.com", "www.example.com"].
func wwwVariants(domain string) []string {
	if strings.HasPrefix(domain, "www.") {
		return []string{domain, strings.TrimPrefix(domain, "www.")}
	}
	return []string{domain, "www." + domain}
}

// extractDateSelectorPartsPerRow extracts text from multiple date selectors,
// returning a 2D array where rows[rowIndex][selectorIndex] is the text extracted
// from that selector's rowIndex-th match. This handles multi-row date tables where
// each row contains a date occurrence.
//
// Example: if selector ".dmTable td:nth-child(1)" matches ["June 1", "June 2"] and
// selector ".dmTable td:nth-child(2)" matches ["7:30 PM", "7:30 PM"], the result is:
//
//	[[June 1, 7:30 PM], [June 2, 7:30 PM]]
func extractDateSelectorPartsPerRow(h *colly.HTMLElement, dateSelectors []string) [][]string {
	if len(dateSelectors) == 0 {
		return nil
	}

	// Collect all matches for each selector.
	selectorMatches := make([][]string, len(dateSelectors))
	maxRows := 0
	for i, sel := range dateSelectors {
		var matches []string
		h.DOM.Find(sel).Each(func(_ int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			matches = append(matches, text)
		})
		selectorMatches[i] = matches
		if len(matches) > maxRows {
			maxRows = len(matches)
		}
	}

	// If no matches at all, return empty.
	if maxRows == 0 {
		return nil
	}

	// Build rows: rows[rowIndex][selectorIndex].
	// For selectors with fewer matches than maxRows, pad with empty strings.
	rows := make([][]string, maxRows)
	for rowIdx := 0; rowIdx < maxRows; rowIdx++ {
		rows[rowIdx] = make([]string, len(dateSelectors))
		for selIdx, matches := range selectorMatches {
			if rowIdx < len(matches) {
				rows[rowIdx][selIdx] = matches[rowIdx]
			}
			// else: zero value "" is already set
		}
	}

	return rows
}

// parseSelector parses a selector that may include an attribute specifier.
// Format: "selector::attribute" returns (selector, attribute).
// Format: "selector" returns (selector, "").
// Example: "span.prices::data-event" returns ("span.prices", "data-event")
func parseSelector(sel string) (string, string) {
	if idx := strings.Index(sel, "::"); idx != -1 {
		return sel[:idx], sel[idx+2:]
	}
	return sel, ""
}

// extractTextOrAttr extracts either an attribute value or text content from the
// current element or a child element. If selector contains "::attribute",
// extracts that attribute; otherwise extracts text content.
//
// For self-extraction: if h matches the selector, extract from h itself.
// For child-extraction: if h doesn't match, extract from the first child matching selector.
// This handles cases where the event_list selector IS the container element itself
// (e.g., event_list="span.prices" and name="span.prices::data-event").
func extractTextOrAttr(h *colly.HTMLElement, selector string) string {
	sel, attr := parseSelector(selector)

	// Check if the current element matches the selector.
	// This is a simple check: if selector is a class, test if h has that class.
	// If selector is an element+class, test if h is that element and has the class.
	matches := elementMatches(h, sel)

	if matches {
		// Extract from the current element itself.
		if attr != "" {
			return strings.TrimSpace(h.Attr(attr))
		}
		return strings.TrimSpace(h.Text)
	}

	// Extract from a child element.
	if attr != "" {
		return strings.TrimSpace(h.ChildAttr(sel, attr))
	}
	return strings.TrimSpace(h.ChildText(sel))
}

// elementMatches checks if h matches the given CSS selector in a simple way.
// It handles:
// - ".classname" → h has class "classname"
// - "tag.classname" → h is tag and has class "classname"
// - "tag" → h is tag
// This is NOT a full CSS selector parser, but covers the common cases for event_list selectors.
func elementMatches(h *colly.HTMLElement, selector string) bool {
	// Handle class selectors.
	if strings.HasPrefix(selector, ".") {
		className := strings.TrimPrefix(selector, ".")
		classes := strings.Fields(h.Attr("class"))
		for _, c := range classes {
			if c == className {
				return true
			}
		}
		return false
	}

	// Handle tag.class selectors.
	if strings.Contains(selector, ".") {
		parts := strings.Split(selector, ".")
		tag := parts[0]
		className := strings.Join(parts[1:], ".")

		if h.Name != tag {
			return false
		}

		classes := strings.Fields(h.Attr("class"))
		for _, c := range classes {
			if c == className {
				return true
			}
		}
		return false
	}

	// Handle simple tag selectors.
	return h.Name == selector
}

// extractDateFromElement finds a child element matching selector and returns
// the value of its datetime attribute, falling back to its text content.
func extractDateFromElement(h *colly.HTMLElement, selector string) string {
	// Prefer datetime attribute (standard HTML5 time element).
	dt := h.ChildAttr(selector, "datetime")
	if dt != "" {
		return strings.TrimSpace(dt)
	}
	return strings.TrimSpace(h.ChildText(selector))
}
