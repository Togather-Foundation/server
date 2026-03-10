package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

// ── FetchSitemap ──────────────────────────────────────────────────────

func TestFetchSitemap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantLen     int
		wantURLs    []string
		wantLastMod []bool // true = entry has LastMod
		wantErr     bool
	}{
		{
			name: "valid urlset no lastmod",
			handler: serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/event/1</loc></url>
  <url><loc>https://example.com/event/2</loc></url>
</urlset>`),
			wantLen:     2,
			wantURLs:    []string{"https://example.com/event/1", "https://example.com/event/2"},
			wantLastMod: []bool{false, false},
		},
		{
			name: "valid urlset with lastmod RFC3339",
			handler: serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/event/1</loc>
    <lastmod>2024-06-15T10:00:00Z</lastmod>
  </url>
  <url>
    <loc>https://example.com/event/2</loc>
    <lastmod>2024-07-01T00:00:00Z</lastmod>
  </url>
</urlset>`),
			wantLen:     2,
			wantURLs:    []string{"https://example.com/event/1", "https://example.com/event/2"},
			wantLastMod: []bool{true, true},
		},
		{
			name: "valid urlset mixed lastmod formats",
			handler: serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc><lastmod>2024-06-15T10:00:00Z</lastmod></url>
  <url><loc>https://example.com/b</loc><lastmod>2024-06-15</lastmod></url>
  <url><loc>https://example.com/c</loc><lastmod>2024-06</lastmod></url>
  <url><loc>https://example.com/d</loc><lastmod>2024</lastmod></url>
  <url><loc>https://example.com/e</loc></url>
</urlset>`),
			wantLen:     5,
			wantURLs:    []string{"https://example.com/a", "https://example.com/b", "https://example.com/c", "https://example.com/d", "https://example.com/e"},
			wantLastMod: []bool{true, true, true, true, false},
		},
		{
			name: "empty sitemap",
			handler: serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`),
			wantLen: 0,
		},
		{
			name: "HTTP 404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			},
			wantErr: true,
		},
		{
			name: "invalid XML",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				_, _ = fmt.Fprint(w, "<not valid xml <<>>")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			entries, err := FetchSitemap(t.Context(), srv.URL+"/sitemap.xml", srv.Client())

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(entries) != tc.wantLen {
				t.Fatalf("got %d entries, want %d", len(entries), tc.wantLen)
			}

			for i, url := range tc.wantURLs {
				if entries[i].URL != url {
					t.Errorf("entries[%d].URL = %q, want %q", i, entries[i].URL, url)
				}
			}

			for i, hasLastMod := range tc.wantLastMod {
				got := entries[i].LastMod != nil
				if got != hasLastMod {
					t.Errorf("entries[%d].LastMod != nil = %v, want %v", i, got, hasLastMod)
				}
			}
		})
	}
}

func TestFetchSitemap_Index(t *testing.T) {
	t.Parallel()

	// Set up two child sitemap servers.
	child1 := httptest.NewServer(serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/event/1</loc></url>
  <url><loc>https://example.com/event/2</loc></url>
</urlset>`))
	defer child1.Close()

	child2 := httptest.NewServer(serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/event/3</loc></url>
</urlset>`))
	defer child2.Close()

	// Index sitemap referencing both children.
	index := httptest.NewServer(serveXML(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/sitemap1.xml</loc></sitemap>
  <sitemap><loc>%s/sitemap2.xml</loc></sitemap>
</sitemapindex>`, child1.URL, child2.URL)))
	defer index.Close()

	// Use a client that allows cross-server requests.
	client := &http.Client{}
	entries, err := FetchSitemap(t.Context(), index.URL+"/sitemap_index.xml", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	wantURLs := []string{
		"https://example.com/event/1",
		"https://example.com/event/2",
		"https://example.com/event/3",
	}
	for i, want := range wantURLs {
		if entries[i].URL != want {
			t.Errorf("entries[%d].URL = %q, want %q", i, entries[i].URL, want)
		}
	}
}

func TestFetchSitemap_IndexDepthExceeded(t *testing.T) {
	t.Parallel()

	// A chain of sitemap index files: index → level1 → level2 → level3.
	// Level 3 is a plain urlset — it should never be reached.
	leaf := httptest.NewServer(serveXML(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/event/deep</loc></url>
</urlset>`))
	defer leaf.Close()

	level2 := httptest.NewServer(serveXML(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/leaf.xml</loc></sitemap>
</sitemapindex>`, leaf.URL)))
	defer level2.Close()

	level1 := httptest.NewServer(serveXML(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/level2.xml</loc></sitemap>
</sitemapindex>`, level2.URL)))
	defer level1.Close()

	// Root index at depth 0 → level1 at depth 1 → level2 at depth 2 →
	// leaf would be depth 3 (> maxSitemapIndexDepth=2) → error, skipped.
	root := httptest.NewServer(serveXML(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/level1.xml</loc></sitemap>
</sitemapindex>`, level1.URL)))
	defer root.Close()

	client := &http.Client{}
	entries, err := FetchSitemap(t.Context(), root.URL+"/root.xml", client)
	// With error-propagation semantics, when all children in a sitemap index
	// fail (depth exceeded bubbles up), FetchSitemap returns a non-nil error.
	if err == nil {
		t.Error("expected error from depth-exceeded children, got nil")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	// The deep URL must never appear in the results.
	for _, e := range entries {
		if e.URL == "https://example.com/event/deep" {
			t.Errorf("deep URL should not have been reached, got entry: %v", e.URL)
		}
	}
}

// ── FilterSitemapEntries ──────────────────────────────────────────────

func TestFilterSitemapEntries(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)

	entries := []SitemapEntry{
		{URL: "https://example.com/events/summer-festival", LastMod: &t1},
		{URL: "https://example.com/events/autumn-show", LastMod: &t2},
		{URL: "https://example.com/events/winter-gala", LastMod: &t3},
		{URL: "https://example.com/about", LastMod: &t1},
		{URL: "https://example.com/events/no-lastmod"},
	}

	tests := []struct {
		name    string
		pattern string
		cutoff  *time.Time
		wantLen int
		wantURL []string
	}{
		{
			name:    "regex matches event URLs only",
			pattern: `/events/`,
			cutoff:  nil,
			wantLen: 4,
			wantURL: []string{
				"https://example.com/events/summer-festival",
				"https://example.com/events/autumn-show",
				"https://example.com/events/winter-gala",
				"https://example.com/events/no-lastmod",
			},
		},
		{
			name:    "regex no matches",
			pattern: `/concerts/`,
			cutoff:  nil,
			wantLen: 0,
		},
		{
			name:    "cutoff filters out old entries",
			pattern: `/events/`,
			cutoff:  timePtr(time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)),
			// t1 (June) <= cutoff → filtered out; t2 (Aug) > cutoff → kept; t3 (Oct) > cutoff → kept; no-lastmod → kept
			wantLen: 3,
			wantURL: []string{
				"https://example.com/events/autumn-show",
				"https://example.com/events/winter-gala",
				"https://example.com/events/no-lastmod",
			},
		},
		{
			name:    "entries without lastmod always included with cutoff",
			pattern: `no-lastmod`,
			cutoff:  timePtr(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)),
			wantLen: 1,
			wantURL: []string{"https://example.com/events/no-lastmod"},
		},
		{
			name:    "nil cutoff passes all regex matches",
			pattern: `/events/`,
			cutoff:  nil,
			wantLen: 4,
		},
		{
			name:    "empty entries",
			pattern: `/events/`,
			cutoff:  nil,
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := entries
			if tc.name == "empty entries" {
				input = nil
			}

			pattern := regexp.MustCompile(tc.pattern)
			got := FilterSitemapEntries(input, pattern, tc.cutoff)

			if len(got) != tc.wantLen {
				t.Fatalf("got %d entries, want %d; entries: %v", len(got), tc.wantLen, got)
			}

			for i, want := range tc.wantURL {
				if i >= len(got) {
					t.Fatalf("missing entry at index %d, want %q", i, want)
				}
				if got[i].URL != want {
					t.Errorf("entries[%d].URL = %q, want %q", i, got[i].URL, want)
				}
			}
		})
	}
}

// ── parseLastMod ──────────────────────────────────────────────────────

func TestParseLastMod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantY   int
		wantM   time.Month
		wantD   int
	}{
		{
			name:  "RFC3339 with Z",
			input: "2024-06-15T10:30:00Z",
			wantY: 2024, wantM: time.June, wantD: 15,
		},
		{
			name:  "RFC3339 with offset",
			input: "2024-06-15T10:30:00-05:00",
			wantY: 2024, wantM: time.June, wantD: 15,
		},
		{
			name:  "date-only",
			input: "2024-01-15",
			wantY: 2024, wantM: time.January, wantD: 15,
		},
		{
			name:  "year-month",
			input: "2024-01",
			wantY: 2024, wantM: time.January, wantD: 1,
		},
		{
			name:  "year-only",
			input: "2024",
			wantY: 2024, wantM: time.January, wantD: 1,
		},
		{
			name:    "invalid format",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "human readable",
			input:   "June 15 2024",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseLastMod(tc.input)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got time: %v", tc.input, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Year() != tc.wantY {
				t.Errorf("year = %d, want %d", got.Year(), tc.wantY)
			}
			if got.Month() != tc.wantM {
				t.Errorf("month = %v, want %v", got.Month(), tc.wantM)
			}
			if got.Day() != tc.wantD {
				t.Errorf("day = %d, want %d", got.Day(), tc.wantD)
			}
		})
	}
}

// ── SitemapEntryURLs ──────────────────────────────────────────────────

func TestSitemapEntryURLs(t *testing.T) {
	t.Parallel()

	t.Run("extracts URL strings correctly", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		entries := []SitemapEntry{
			{URL: "https://example.com/1", LastMod: &now},
			{URL: "https://example.com/2"},
			{URL: "https://example.com/3", LastMod: &now},
		}

		got := SitemapEntryURLs(entries)

		want := []string{
			"https://example.com/1",
			"https://example.com/2",
			"https://example.com/3",
		}

		if len(got) != len(want) {
			t.Fatalf("got %d URLs, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("URLs[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		t.Parallel()

		got := SitemapEntryURLs(nil)
		if len(got) != 0 {
			t.Fatalf("got %d URLs, want 0", len(got))
		}
	})
}

// ── helpers ───────────────────────────────────────────────────────────

// serveXML returns an http.HandlerFunc that writes the given XML body.
func serveXML(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}
}

// timePtr is a convenience helper for taking the address of a time.Time.
func timePtr(t time.Time) *time.Time {
	return &t
}
