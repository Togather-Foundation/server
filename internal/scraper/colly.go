package scraper

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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

		raw := RawEvent{}

		if config.Selectors.Name != "" {
			raw.Name = strings.TrimSpace(h.ChildText(config.Selectors.Name))
		}

		// Date extraction: prefer date_selectors (smart assembler) over
		// start_date/end_date (single-selector legacy path).
		if len(config.Selectors.DateSelectors) > 0 {
			var probes []DateSelectorProbe
			for _, sel := range config.Selectors.DateSelectors {
				text := strings.TrimSpace(h.ChildText(sel))
				matched := text != ""
				// Always append to DateParts (empty string for misses) so that
				// index i reliably corresponds to DateSelectors[i].
				// assembleDateTimeParts already skips empty strings.
				raw.DateParts = append(raw.DateParts, text)
				if isFirst {
					probes = append(probes, DateSelectorProbe{
						Selector: sel,
						Matched:  matched,
						Text:     text,
					})
				}
			}
			if isFirst && len(probes) > 0 {
				mu.Lock()
				firstProbes = probes
				mu.Unlock()
			}
		} else {
			if config.Selectors.StartDate != "" {
				raw.StartDate = extractDateFromElement(h, config.Selectors.StartDate)
			}

			if config.Selectors.EndDate != "" {
				raw.EndDate = extractDateFromElement(h, config.Selectors.EndDate)
			}
		}

		if config.Selectors.Location != "" {
			raw.Location = strings.TrimSpace(h.ChildText(config.Selectors.Location))
		}

		if config.Selectors.Description != "" {
			raw.Description = strings.TrimSpace(h.ChildText(config.Selectors.Description))
		}

		if config.Selectors.URL != "" {
			href := h.ChildAttr(config.Selectors.URL, "href")
			if href != "" {
				raw.URL = h.Request.AbsoluteURL(href)
			}
		}

		if config.Selectors.Image != "" {
			src := h.ChildAttr(config.Selectors.Image, "src")
			if src != "" {
				raw.Image = h.Request.AbsoluteURL(src)
			}
		}

		// Only collect events with a non-empty name.
		if raw.Name == "" {
			return
		}

		mu.Lock()
		results = append(results, raw)
		mu.Unlock()
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
