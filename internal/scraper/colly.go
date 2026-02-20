package scraper

import (
	"context"
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

// ScrapeWithSelectors fetches config.URL and all linked pages (up to
// config.MaxPages), applying the CSS selectors in config.Selectors to collect
// RawEvents. It respects robots.txt (Colly default) and applies per-domain
// rate limiting. If ctx is cancelled before scraping completes, the function
// returns whatever events were collected up to that point.
func (e *CollyExtractor) ScrapeWithSelectors(ctx context.Context, config SourceConfig) ([]RawEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Extract the allowed domain from the source URL.
	allowedDomain, err := extractDomain(config.URL)
	if err != nil {
		return nil, err
	}

	var (
		mu        sync.Mutex
		results   []RawEvent
		pagesSeen int
	)

	maxPages := config.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}

	c := colly.NewCollector(
		colly.UserAgent(e.userAgent),
		colly.AllowedDomains(allowedDomain),
		// robots.txt is respected by default in Colly; do NOT use IgnoreRobotsTxt.
	)

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

		raw := RawEvent{}

		if config.Selectors.Name != "" {
			raw.Name = strings.TrimSpace(h.ChildText(config.Selectors.Name))
		}

		if config.Selectors.StartDate != "" {
			raw.StartDate = extractDateFromElement(h, config.Selectors.StartDate)
		}

		if config.Selectors.EndDate != "" {
			raw.EndDate = extractDateFromElement(h, config.Selectors.EndDate)
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
			return results, nil
		}
		return nil, err
	}

	// Wait for all async callbacks to complete.
	c.Wait()

	return results, nil
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
