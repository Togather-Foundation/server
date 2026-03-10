package scraper

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// defaultSitemapMaxURLs is the default cap on URLs scraped per sitemap run.
const defaultSitemapMaxURLs = 200

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
func FetchSitemap(ctx context.Context, sitemapURL string, client *http.Client) ([]SitemapEntry, error) {
	return fetchSitemapRecursive(ctx, sitemapURL, client, 0)
}

func fetchSitemapRecursive(ctx context.Context, sitemapURL string, client *http.Client, depth int) ([]SitemapEntry, error) {
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
	defer resp.Body.Close()

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
		for _, sm := range index.Sitemaps {
			children, childErr := fetchSitemapRecursive(ctx, sm.Loc, client, depth+1)
			if childErr != nil {
				// Log but don't fail the whole sitemap -- partial results are acceptable.
				continue
			}
			entries = append(entries, children...)
		}
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
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z",
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

// FilterSitemapEntries filters entries by a compiled regex pattern and
// optionally by lastmod freshness (entries modified after the cutoff).
// When cutoff is nil, no time-based filtering is applied.
func FilterSitemapEntries(entries []SitemapEntry, pattern *regexp.Regexp, cutoff *time.Time) []SitemapEntry {
	var filtered []SitemapEntry
	for _, e := range entries {
		if !pattern.MatchString(e.URL) {
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

// SitemapEntryURLs extracts just the URL strings from a slice of SitemapEntry.
func SitemapEntryURLs(entries []SitemapEntry) []string {
	urls := make([]string, len(entries))
	for i, e := range entries {
		urls[i] = e.URL
	}
	return urls
}
