package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/scraper"
)

func TestScrapeTestOutputJSON(t *testing.T) {
	t.Parallel()

	longDescription := strings.Repeat("A", 200)

	tests := []struct {
		name          string
		events        []scraper.RawEvent
		jsonFlag      bool
		wantContains  []string
		wantAbsent    []string
		wantValidJSON bool
		wantFullDesc  bool
	}{
		{
			name: "json mode with events outputs valid JSON",
			events: []scraper.RawEvent{
				{
					Name:        "Test Event",
					StartDate:   "2026-05-01T19:00:00Z",
					EndDate:     "2026-05-01T22:00:00Z",
					Location:    "Test Venue",
					Description: longDescription,
					URL:         "https://example.com/event",
					Image:       "https://example.com/image.jpg",
				},
			},
			jsonFlag:      true,
			wantValidJSON: true,
			wantFullDesc:  true,
		},
		{
			name:     "json mode empty output is empty array",
			events:   []scraper.RawEvent{},
			jsonFlag: true,
			wantContains: []string{
				"[]",
			},
			wantValidJSON: false,
		},
		{
			name: "text mode truncates description at 120 chars",
			events: []scraper.RawEvent{
				{
					Name:        "Test Event",
					StartDate:   "2026-05-01T19:00:00Z",
					Description: longDescription,
				},
			},
			jsonFlag: false,
			wantContains: []string{
				"Description: AAAAAAAAAAAAAAAAAAAAAAAAAAA",
				"…",
			},
			wantAbsent: []string{
				longDescription,
			},
		},
		{
			name:     "text mode shows no events message",
			events:   []scraper.RawEvent{},
			jsonFlag: false,
			wantContains: []string{
				"No events extracted.",
			},
		},
		{
			name: "json mode with multiple events",
			events: []scraper.RawEvent{
				{
					Name:      "Event One",
					StartDate: "2026-05-01T19:00:00Z",
				},
				{
					Name:      "Event Two",
					StartDate: "2026-05-02T20:00:00Z",
				},
			},
			jsonFlag:      true,
			wantValidJSON: true,
			wantContains: []string{
				"Event One",
				"Event Two",
			},
		},
		{
			name: "json mode includes all fields",
			events: []scraper.RawEvent{
				{
					Name:        "Full Event",
					StartDate:   "2026-05-01T19:00:00Z",
					EndDate:     "2026-05-01T22:00:00Z",
					Location:    "Test Venue",
					Description: "Description text",
					URL:         "https://example.com/event",
					Image:       "https://example.com/image.jpg",
					DateParts:   []string{"May 1", "7:00 PM"},
				},
			},
			jsonFlag:      true,
			wantValidJSON: true,
			wantContains: []string{
				"Full Event",
				"Test Venue",
				"https://example.com/event",
				"Description text",
				"May 1",
				"7:00 PM",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scrapeTestJSON = tt.jsonFlag

			output, err := captureScrapeTestOutput(tt.events)
			scrapeTestJSON = false

			if err != nil {
				t.Fatalf("printScrapeTestOutput error: %v", err)
			}

			if tt.wantValidJSON {
				var v interface{}
				if err := json.Unmarshal([]byte(output), &v); err != nil {
					t.Errorf("expected valid JSON, got error: %v\noutput: %s", err, output)
				}

				if tt.wantFullDesc && len(tt.events) > 0 {
					if !strings.Contains(output, longDescription) {
						t.Errorf("expected full description in JSON output, got: %s", output)
					}
				}
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("expected output to contain %q\ngot:\n%s", want, output)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("expected output NOT to contain %q\ngot:\n%s", absent, output)
				}
			}
		})
	}
}

func captureScrapeTestOutput(events []scraper.RawEvent) (string, error) {
	var buf bytes.Buffer
	err := formatScrapeTestOutput(&buf, events, scrapeTestJSON)
	return buf.String(), err
}

func TestScrapeTestOutputTruncation(t *testing.T) {
	t.Parallel()

	events := []scraper.RawEvent{
		{
			Name:        "Test Event",
			StartDate:   "2026-05-01T19:00:00Z",
			Description: strings.Repeat("B", 150),
		},
	}

	output, err := captureScrapeTestOutput(events)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if strings.Contains(output, strings.Repeat("B", 150)) {
		t.Error("text mode should truncate description but it didn't")
	}

	if !strings.Contains(output, "…") {
		t.Error("text mode should show ellipsis for truncated description")
	}

	expectedTruncated := strings.Repeat("B", 120) + "…"
	if !strings.Contains(output, expectedTruncated) {
		t.Errorf("expected truncated description %q in output", expectedTruncated)
	}
}

func TestScrapeTestOutputJSONFullDescription(t *testing.T) {
	t.Parallel()

	longDesc := strings.Repeat("C", 200)
	events := []scraper.RawEvent{
		{
			Name:        "Test Event",
			StartDate:   "2026-05-01T19:00:00Z",
			Description: longDesc,
		},
	}

	scrapeTestJSON = true
	output, err := captureScrapeTestOutput(events)
	scrapeTestJSON = false

	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(output, longDesc) {
		t.Error("json mode should include full description but it was truncated")
	}

	var parsed []scraper.RawEvent
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}

	if len(parsed) != 1 || parsed[0].Description != longDesc {
		t.Errorf("json output should preserve full description")
	}
}
