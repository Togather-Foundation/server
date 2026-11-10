package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// Exercise extraction through submission so the event-page URL, rather than
// the listing URL, is demonstrably the value matched by source exclusions.
func TestSourceExclusionsGraphQL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, title, slug, urlPattern, titlePattern string
		excluded                                    bool
	}{
		{"url match", "Room Hold", "private-booking-2030", "/events/private-booking-", "^Private Booking$", true},
		{"title match", "Private Booking", "room-hold", "/events/private-booking-", "^Private Booking$", true},
		{"neither", "Public Concert", "public-concert", "/events/private-booking-", "^Private Booking$", false},
		{"both", "Private Booking", "private-booking-2030", "/events/private-booking-", "^Private Booking$", true},
		{"opt in only", "Private Booking", "private-booking-2030", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
			feed := newGraphQLServer(t, tranzacResponse([]map[string]any{
				{"title": tc.title, "slug": tc.slug, "startDate": future},
				{"title": "Allowed Sibling", "slug": "allowed-sibling", "startDate": future},
			}))
			var mu sync.Mutex
			var posted []events.EventInput
			ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/events:batch", r.URL.Path)
				var payload struct {
					Events []events.EventInput `json:"events"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode ingest: %v", err)
					w.WriteHeader(400)
					return
				}
				mu.Lock()
				posted = append(posted, payload.Events...)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"batch_id": "test", "events_created": len(payload.Events)})
			}))
			defer ingest.Close()
			source := newGraphQLSource(t, feed.URL, "", "https://tranzac.org/events/{{.slug}}")
			source.ExcludeURLPattern = tc.urlPattern
			source.ExcludeTitlePattern = tc.titlePattern
			var logs bytes.Buffer
			scraper := NewScraper(NewIngestClient(ingest.URL, "key"), nil, zerolog.New(&logs))
			result, err := scraper.scrapeTier3(t.Context(), source, ScrapeOptions{})
			require.NoError(t, err)
			require.NoError(t, result.Error)
			mu.Lock()
			defer mu.Unlock()
			want := 2
			if tc.excluded {
				want = 1
			}
			require.Len(t, posted, want)
			require.Equal(t, "Allowed Sibling", posted[len(posted)-1].Name)
			require.Equal(t, "https://tranzac.org/events/allowed-sibling", posted[len(posted)-1].URL)
			for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
				var entry map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &entry))
				for _, rule := range []struct{ key, pattern, sample string }{
					{"url_pattern", tc.urlPattern, "https://tranzac.org/events/" + tc.slug},
					{"title_pattern", tc.titlePattern, tc.title},
				} {
					if _, ok := entry[rule.key]; !ok {
						continue
					}
					require.Equal(t, rule.pattern, entry[rule.key])
					count := float64(0)
					if rule.key == "url_pattern" && strings.Contains(tc.slug, "private-booking-") || rule.key == "title_pattern" && tc.title == "Private Booking" {
						count = 1
					}
					require.Equal(t, count, entry["excluded"])
					if count > 0 {
						require.Equal(t, rule.sample, entry["sample"])
					}
				}
			}
			if tc.excluded {
				require.NotContains(t, result.QualityWarnings, tc.title)
				require.Contains(t, logs.String(), "excluded")
				if strings.Contains(tc.slug, "private-booking-") {
					require.Contains(t, logs.String(), "https://tranzac.org/events/"+tc.slug)
				} else {
					require.Contains(t, logs.String(), tc.title)
				}
			}
		})
	}
}

func TestValidateSourceExclusionPatterns(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, urlPattern, titlePattern, wantError string }{
		{"unset", "", "", ""},
		{"valid", "/private-booking-", "(?i)^private booking$", ""},
		{"invalid URL", "[", "", "exclude_url_pattern"},
		{"invalid title", "", "[", "exclude_title_pattern"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := SourceConfig{Name: "exclusions", URL: "https://example.com/events", Tier: 0, ExcludeURLPattern: tc.urlPattern, ExcludeTitlePattern: tc.titlePattern}
			err := ValidateConfig(source)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.True(t, strings.Contains(err.Error(), tc.wantError), "%v", err)
			}
		})
	}
}

// Midnight-only room holds must not contribute to source quality warnings once
// excluded; the public sibling remains eligible for ingestion.
func TestSourceExclusionsBeforeQualityWarnings(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(48 * time.Hour).UTC()
	midnight := time.Date(future.Year(), future.Month(), future.Day(), 0, 0, 0, 0, time.UTC)
	raw := make([]map[string]any, 0, 10)
	for i := 0; i < 9; i++ {
		raw = append(raw, map[string]any{"title": "Private Booking", "slug": fmt.Sprintf("private-booking-%d", i), "startDate": midnight.Format(time.RFC3339)})
	}
	raw = append(raw, map[string]any{"title": "Allowed Sibling", "slug": "allowed-sibling", "startDate": midnight.Add(19 * time.Hour).Format(time.RFC3339)})
	feed := newGraphQLServer(t, tranzacResponse(raw))
	source := newGraphQLSource(t, feed.URL, "", "https://tranzac.org/events/{{.slug}}")
	source.ExcludeURLPattern = "/events/private-booking-"
	scraper := NewScraper(NewIngestClient("http://unused", "key"), nil, zerolog.Nop())
	result, err := scraper.scrapeTier3(t.Context(), source, ScrapeOptions{DryRun: true})
	require.NoError(t, err)
	require.NoError(t, result.Error)
	require.Len(t, result.DryRunEvents, 1)
	require.Equal(t, "Allowed Sibling", result.DryRunEvents[0].Name)
	for _, warning := range result.QualityWarnings {
		require.NotContains(t, warning, WarnAllMidnight)
	}
}

func TestSourceExclusionsICSBeforeParseWarnings(t *testing.T) {
	t.Parallel()
	body := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:private-no-date\r\nSUMMARY:Private Booking\r\nURL:https://example.com/private-booking-one\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:private-invalid-url\r\nSUMMARY:Private Booking\r\nDTSTART:20301110T190000Z\r\nURL:javascript:invalid\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:public-sibling\r\nSUMMARY:Allowed Sibling\r\nDTSTART:20301110T190000Z\r\nURL:https://example.com/public\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(body))
	}))
	defer feed.Close()
	source := SourceConfig{Name: "ics-exclusions", URL: feed.URL, ExtractionMethod: "ics", License: "CC0-1.0", ExcludeTitlePattern: "^Private Booking$"}
	scraper := NewScraper(NewIngestClient("http://unused", "key"), nil, zerolog.Nop())
	result, err := scraper.scrapeICS(t.Context(), source, ScrapeOptions{DryRun: true})
	require.NoError(t, err)
	require.NoError(t, result.Error)
	require.Len(t, result.DryRunEvents, 1)
	require.Equal(t, "Allowed Sibling", result.DryRunEvents[0].Name)
	require.Empty(t, result.QualityWarnings)
}
