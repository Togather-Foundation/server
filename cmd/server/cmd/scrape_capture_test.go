package cmd

import (
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/scraper"
)

func TestFormatNetworkSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		requests     []scraper.NetworkRequest
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:     "empty request list",
			requests: []scraper.NetworkRequest{},
			wantContains: []string{
				"--- Network Activity ---",
				"API Calls (XHR/Fetch + JSON): none",
			},
			wantAbsent: []string{
				"Other Requests",
			},
		},
		{
			name: "single API call shows URL status and content type",
			requests: []scraper.NetworkRequest{
				{
					URL:          "https://api.example.com/events",
					Method:       "GET",
					ResourceType: "XHR",
					Status:       200,
					ContentType:  "application/json",
					IsAPI:        true,
				},
			},
			wantContains: []string{
				"API Calls (XHR/Fetch + JSON):",
				"https://api.example.com/events",
				"→ 200",
				"application/json",
				"GET",
			},
			wantAbsent: []string{
				"API Calls (XHR/Fetch + JSON): none",
			},
		},
		{
			name: "mixed content types - API calls separated from others",
			requests: []scraper.NetworkRequest{
				{
					URL:          "https://api.example.com/data",
					Method:       "POST",
					ResourceType: "Fetch",
					Status:       201,
					ContentType:  "application/json",
					IsAPI:        true,
				},
				{
					URL:          "https://example.com/style.css",
					Method:       "GET",
					ResourceType: "Stylesheet",
					Status:       200,
					ContentType:  "text/css",
					IsAPI:        false,
				},
				{
					URL:          "https://example.com/app.js",
					Method:       "GET",
					ResourceType: "Script",
					Status:       200,
					ContentType:  "application/javascript",
					IsAPI:        false,
				},
			},
			wantContains: []string{
				"API Calls (XHR/Fetch + JSON):",
				"https://api.example.com/data",
				"Other Requests",
				"Stylesheet: 1",
				"Script: 1",
			},
		},
		{
			name: "requests with empty content type do not panic",
			requests: []scraper.NetworkRequest{
				{
					URL:          "https://example.com/resource",
					Method:       "GET",
					ResourceType: "XHR",
					Status:       200,
					ContentType:  "",
					IsAPI:        false,
				},
			},
			wantContains: []string{
				"--- Network Activity ---",
			},
		},
		{
			name: "varied status codes appear in output",
			requests: []scraper.NetworkRequest{
				{
					URL:          "https://api.example.com/ok",
					Method:       "GET",
					ResourceType: "Fetch",
					Status:       200,
					ContentType:  "application/json",
					IsAPI:        true,
				},
				{
					URL:          "https://api.example.com/redirect",
					Method:       "GET",
					ResourceType: "Fetch",
					Status:       301,
					ContentType:  "application/json",
					IsAPI:        true,
				},
				{
					URL:          "https://api.example.com/notfound",
					Method:       "GET",
					ResourceType: "XHR",
					Status:       404,
					ContentType:  "application/json",
					IsAPI:        true,
				},
				{
					URL:          "https://api.example.com/error",
					Method:       "POST",
					ResourceType: "XHR",
					Status:       500,
					ContentType:  "application/json",
					IsAPI:        true,
				},
			},
			wantContains: []string{
				"→ 200",
				"→ 301",
				"→ 404",
				"→ 500",
			},
		},
		{
			name: "multiple resource types grouped in other requests",
			requests: []scraper.NetworkRequest{
				{URL: "a.png", ResourceType: "Image", IsAPI: false},
				{URL: "b.png", ResourceType: "Image", IsAPI: false},
				{URL: "c.png", ResourceType: "Image", IsAPI: false},
				{URL: "a.js", ResourceType: "Script", IsAPI: false},
				{URL: "a.css", ResourceType: "Stylesheet", IsAPI: false},
			},
			wantContains: []string{
				"API Calls (XHR/Fetch + JSON): none",
				"Other Requests (5 total):",
				"Image: 3",
				"Script: 1",
				"Stylesheet: 1",
			},
		},
		{
			name: "request with zero status omits status arrow",
			requests: []scraper.NetworkRequest{
				{
					URL:          "https://api.example.com/pending",
					Method:       "GET",
					ResourceType: "Fetch",
					Status:       0,
					ContentType:  "application/json",
					IsAPI:        true,
				},
			},
			wantContains: []string{
				"https://api.example.com/pending",
			},
			wantAbsent: []string{
				"→ 0",
			},
		},
		{
			name: "size and timing annotations appear when present",
			requests: []scraper.NetworkRequest{
				{
					URL:          "https://api.example.com/big",
					Method:       "GET",
					ResourceType: "Fetch",
					Status:       200,
					ContentType:  "application/json",
					BodySize:     2048,
					TimingMs:     150,
					IsAPI:        true,
				},
			},
			wantContains: []string{
				"2.0 KB",
				"150ms",
			},
		},
		{
			name: "empty resource type displayed as Unknown label",
			requests: []scraper.NetworkRequest{
				{URL: "x", ResourceType: "", IsAPI: false},
			},
			wantContains: []string{
				"Other Requests (1 total):",
				"Unknown:",
			},
		},
		{
			name: "API calls sorted by URL for deterministic output",
			requests: []scraper.NetworkRequest{
				{URL: "https://z.example.com/api", Method: "GET", ResourceType: "XHR", ContentType: "application/json", IsAPI: true},
				{URL: "https://a.example.com/api", Method: "GET", ResourceType: "XHR", ContentType: "application/json", IsAPI: true},
				{URL: "https://m.example.com/api", Method: "GET", ResourceType: "XHR", ContentType: "application/json", IsAPI: true},
			},
			wantContains: []string{
				"https://a.example.com/api",
				"https://m.example.com/api",
				"https://z.example.com/api",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatNetworkSummary(tt.requests)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q\ngot:\n%s", want, got)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("expected output NOT to contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

// TestFormatNetworkSummary_SortOrder verifies that API calls are always emitted
// in URL-sorted order regardless of input order.
func TestFormatNetworkSummary_SortOrder(t *testing.T) {
	t.Parallel()
	requests := []scraper.NetworkRequest{
		{URL: "https://z.example.com/", Method: "GET", ResourceType: "XHR", ContentType: "application/json", IsAPI: true},
		{URL: "https://a.example.com/", Method: "GET", ResourceType: "XHR", ContentType: "application/json", IsAPI: true},
	}

	got := formatNetworkSummary(requests)

	idxA := strings.Index(got, "https://a.example.com/")
	idxZ := strings.Index(got, "https://z.example.com/")

	if idxA == -1 || idxZ == -1 {
		t.Fatalf("expected both URLs in output, got:\n%s", got)
	}
	if idxA > idxZ {
		t.Errorf("expected a.example.com to appear before z.example.com in output\ngot:\n%s", got)
	}
}
