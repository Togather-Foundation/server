package scraper

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/rs/zerolog"
)

// defaultSitemapMaxURLs is the default cap on URLs scraped per sitemap run.
const defaultSitemapMaxURLs = 200

// maxSitemapMaxURLs is the upper bound above which a warning is emitted during
// config validation. Values above this threshold are not rejected but may
// overwhelm the target site.
const maxSitemapMaxURLs = 10000

// defaultSitemapRateLimitMs is the default delay between detail page fetches.
const defaultSitemapRateLimitMs = 500

// maxSitemapIndexDepth is the maximum recursion depth for sitemap index files.
const maxSitemapIndexDepth = 2

// SitemapEntry represents a single URL entry from a sitemap XML file.
type SitemapEntry struct {
	URL     string
	LastMod *time.Time
}

// xmlURLSet represents a <urlset> sitemap XML document.
type xmlURLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []xmlURL `xml:"url"`
}

// xmlURL represents a <url> element inside a sitemap.
type xmlURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

// xmlSitemapIndex represents a <sitemapindex> document.
type xmlSitemapIndex struct {
	XMLName  xml.Name     `xml:"sitemapindex"`
	Sitemaps []xmlSitemap `xml:"sitemap"`
}

// xmlSitemap represents a <sitemap> element inside a sitemap index.
type xmlSitemap struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

// FetchSitemap fetches a sitemap XML URL and returns all URL entries found.
// It handles both regular sitemaps (<urlset>) and sitemap index files
// (<sitemapindex>), recursing into child sitemaps up to maxSitemapIndexDepth.
func FetchSitemap(ctx context.Context, sitemapURL string, client *http.Client, logger zerolog.Logger) ([]SitemapEntry, error) {
	return fetchSitemapRecursive(ctx, sitemapURL, client, logger, 0)
}

func fetchSitemapRecursive(ctx context.Context, sitemapURL string, client *http.Client, logger zerolog.Logger, depth int) ([]SitemapEntry, error) {
	if depth > maxSitemapIndexDepth {
		return nil, fmt.Errorf("sitemap index recursion depth exceeded (%d)", maxSitemapIndexDepth)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create sitemap request: %w", err)
	}
	req.Header.Set("User-Agent", "Togather-SEL-Scraper/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch sitemap %s: %w", sitemapURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch sitemap %s: HTTP %d", sitemapURL, resp.StatusCode)
	}

	// Read body (cap at 10 MB to prevent abuse).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read sitemap body: %w", err)
	}

	// Try parsing as sitemap index first.
	var index xmlSitemapIndex
	if xml.Unmarshal(body, &index) == nil && len(index.Sitemaps) > 0 {
		var entries []SitemapEntry
		var childErrors []error
		for _, sm := range index.Sitemaps {
			children, childErr := fetchSitemapRecursive(ctx, sm.Loc, client, logger, depth+1)
			if childErr != nil {
				childErrors = append(childErrors, fmt.Errorf("child sitemap %s: %w", sm.Loc, childErr))
				logger.Debug().Str("url", sm.Loc).Err(childErr).Msg("child sitemap fetch failed, continuing with partial results")
				continue
			}
			entries = append(entries, children...)
		}
		// Return partial results with a combined error if all children failed
		// and we have no entries at all.
		if len(childErrors) > 0 && len(entries) == 0 {
			return nil, errors.Join(childErrors...)
		}
		// When we have some entries (or no errors), return them with nil error —
		// partial success is acceptable and callers proceed with what we have.
		return entries, nil
	}

	// Parse as regular urlset.
	var urlset xmlURLSet
	if err := xml.Unmarshal(body, &urlset); err != nil {
		return nil, fmt.Errorf("parse sitemap XML from %s: %w", sitemapURL, err)
	}

	entries := make([]SitemapEntry, 0, len(urlset.URLs))
	for _, u := range urlset.URLs {
		entry := SitemapEntry{URL: u.Loc}
		if u.LastMod != "" {
			if t, parseErr := parseLastMod(u.LastMod); parseErr == nil {
				entry.LastMod = &t
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseLastMod parses a sitemap <lastmod> value. The sitemap spec allows
// W3C Datetime formats: YYYY, YYYY-MM, YYYY-MM-DD, or full RFC 3339.
func parseLastMod(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01",
		"2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised lastmod format: %q", s)
}

// FilterSitemapEntries filters entries by a compiled include regex pattern,
// an optional exclude regex pattern, and optionally by lastmod freshness
// (entries modified after the cutoff). When cutoff is nil, no time-based
// filtering is applied. When exclude is nil, no exclusion filtering is applied.
func FilterSitemapEntries(entries []SitemapEntry, pattern *regexp.Regexp, exclude *regexp.Regexp, cutoff *time.Time) []SitemapEntry {
	var filtered []SitemapEntry
	for _, e := range entries {
		if !pattern.MatchString(e.URL) {
			continue
		}
		if exclude != nil && exclude.MatchString(e.URL) {
			continue
		}
		// Time-based filtering: skip entries with lastmod <= cutoff.
		// Entries without lastmod are always included (we can't know if they're fresh).
		if cutoff != nil && e.LastMod != nil && !e.LastMod.After(*cutoff) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}
