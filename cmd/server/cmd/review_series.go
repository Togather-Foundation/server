package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainevents "github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/spf13/cobra"
)

// Series-candidate query bounds mirrored by the admin endpoint.
const (
	seriesMinDays         = 1
	seriesMaxDays         = 365
	seriesMinLookbackDays = 0
	seriesMaxLookbackDays = 90
	seriesMinEvents       = 2
	seriesMaxEvents       = 50
	seriesMinLimit        = 1
	seriesMaxLimit        = 200
	seriesMinOffset       = 0
	seriesMaxOffset       = 10000
	// seriesDescriptionDisplayLimit caps each printed description line; the API
	// carries up to 600 runes and the full text lives on the event.
	seriesDescriptionDisplayLimit = 200
)

type SeriesCandidateVenue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SeriesCandidateMember struct {
	ULID            string `json:"ulid"`
	Name            string `json:"name"`
	StartDate       string `json:"start_date"`
	OccurrenceCount int    `json:"occurrence_count"`
	LifecycleState  string `json:"lifecycle_state"`
}

type SeriesCandidateItem struct {
	SuggestedCanonicalULID   string                  `json:"suggested_canonical_ulid"`
	Name                     string                  `json:"name"`
	Venue                    SeriesCandidateVenue    `json:"venue"`
	DayOfWeek                int                     `json:"day_of_week"`
	LocalStartTime           string                  `json:"local_start_time"`
	EventCount               int                     `json:"event_count"`
	OccurrenceCount          int                     `json:"occurrence_count"`
	FirstDate                string                  `json:"first_date"`
	LastDate                 string                  `json:"last_date"`
	Members                  []SeriesCandidateMember `json:"members"`
	DistinctDescriptions     []string                `json:"distinct_descriptions"`
	DescribedMemberCount     int                     `json:"described_member_count"`
	DescriptionsTruncated    bool                    `json:"descriptions_truncated"`
	DistinctDescriptionCount int                     `json:"distinct_description_count"`
	ContentVaries            bool                    `json:"content_varies"`
}

type SeriesCandidatesResponse struct {
	Items         []SeriesCandidateItem `json:"items"`
	TotalClusters int                   `json:"total_clusters"`
	Truncated     bool                  `json:"truncated"`
}

var (
	reviewSeriesCmd = &cobra.Command{
		Use:   "series",
		Short: "Find weekly programmes scraped as separate events",
		Long: `Find conservative weekly-series candidates — the same programme at the
same venue, same weekday, same start time (±30 min), on a 7-day cadence, but
published as N separate events.

Sources that suppress cross_week_series_companion publish these without
creating review rows, so they never appear in the queue. Use this command to
discover them, then consolidate each cluster into one event with N occurrences:

  server review series --days 90
  server review series --source tpl-events --limit 20
  server review series --offset 20 --limit 20
  server review consolidate <canonical> <member-ulids...> --transfer-occurrences --dry-run
  server review consolidate <canonical> <member-ulids...> --transfer-occurrences

Inspect the member list for every cluster: the detector cannot tell different
age groups, audiences, drop-in vs registered sessions, or different programmes
apart. Decide each cluster — consolidate it, or keep it separate and state the
reason.

Description differences are evidence, not a veto. Consolidate when the only
difference is logistics or boilerplate: capacity notices ("N spots remaining",
"this event is full"), room or location suffixes, registration phrasing,
restated dates, punctuation, casing, or whitespace. Keep separate when the text
carries per-occurrence content — a named activity, film, guest, or topic per
date — or when the titles form a progression (Part 1/2, Session 1/3/4, I/II/III,
Beginners/Advanced), because consolidation keeps only the canonical description
and occurrences cannot carry per-date text. When the printed evidence is thin,
fetch each member event for its description and URL before deciding.

The detector is deliberately conservative and covers all published events, not
only suppressed sources. --source scopes the scan to one provenance source
(a source name or UUID) and only sees events attributed to that source; a cluster
that spans sources is narrowed to the requested source's events. --offset pages
through the deterministic cluster order. --json emits the same data for scripting.`,
		Args: cobra.NoArgs,
		RunE: runReviewSeries,
	}

	seriesDays         int
	seriesLookbackDays int
	seriesMinEventFlag int
	seriesVenue        string
	seriesSource       string
	seriesOffset       int
	seriesLimit        int
)

func init() {
	reviewSeriesCmd.Flags().IntVar(&seriesDays, "days", domainevents.DefaultSeriesCandidateDays,
		fmt.Sprintf("Look-ahead window in days (%d-%d)", seriesMinDays, seriesMaxDays))
	reviewSeriesCmd.Flags().IntVar(&seriesLookbackDays, "lookback-days", domainevents.DefaultSeriesCandidateLookbackDays,
		fmt.Sprintf("Include recent past occurrences in days (%d-%d)", seriesMinLookbackDays, seriesMaxLookbackDays))
	reviewSeriesCmd.Flags().IntVar(&seriesMinEventFlag, "min-events", domainevents.DefaultSeriesCandidateMinEvents,
		fmt.Sprintf("Minimum distinct events per cluster (%d-%d)", seriesMinEvents, seriesMaxEvents))
	reviewSeriesCmd.Flags().StringVar(&seriesVenue, "venue", "", "Restrict to a place ULID")
	reviewSeriesCmd.Flags().StringVar(&seriesSource, "source", "", "Restrict to a provenance source name or UUID")
	reviewSeriesCmd.Flags().IntVar(&seriesOffset, "offset", 0,
		fmt.Sprintf("Skip this many clusters after sorting (%d-%d)", seriesMinOffset, seriesMaxOffset))
	reviewSeriesCmd.Flags().IntVar(&seriesLimit, "limit", domainevents.DefaultSeriesCandidateLimit,
		fmt.Sprintf("Maximum clusters returned (%d-%d)", seriesMinLimit, seriesMaxLimit))
}

// validateSeriesFlags mirrors the server-side range checks so an out-of-range
// flag fails before any request is made.
func validateSeriesFlags() error {
	if seriesDays < seriesMinDays || seriesDays > seriesMaxDays {
		return fmt.Errorf("--days must be between %d and %d", seriesMinDays, seriesMaxDays)
	}
	if seriesLookbackDays < seriesMinLookbackDays || seriesLookbackDays > seriesMaxLookbackDays {
		return fmt.Errorf("--lookback-days must be between %d and %d", seriesMinLookbackDays, seriesMaxLookbackDays)
	}
	if seriesMinEventFlag < seriesMinEvents || seriesMinEventFlag > seriesMaxEvents {
		return fmt.Errorf("--min-events must be between %d and %d", seriesMinEvents, seriesMaxEvents)
	}
	if seriesOffset < seriesMinOffset || seriesOffset > seriesMaxOffset {
		return fmt.Errorf("--offset must be between %d and %d", seriesMinOffset, seriesMaxOffset)
	}
	if seriesLimit < seriesMinLimit || seriesLimit > seriesMaxLimit {
		return fmt.Errorf("--limit must be between %d and %d", seriesMinLimit, seriesMaxLimit)
	}
	return nil
}

func runReviewSeries(cmd *cobra.Command, args []string) error {
	if err := validateSeriesFlags(); err != nil {
		return err
	}

	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	// A 90-day scan can exceed the shared 30-second default; this command gets
	// its own longer budget without changing newHTTPClient.
	client := &http.Client{Timeout: 120 * time.Second}
	out := cmd.OutOrStdout()

	params := url.Values{}
	params.Set("days", strconv.Itoa(seriesDays))
	params.Set("lookback_days", strconv.Itoa(seriesLookbackDays))
	params.Set("min_events", strconv.Itoa(seriesMinEventFlag))
	params.Set("offset", strconv.Itoa(seriesOffset))
	params.Set("limit", strconv.Itoa(seriesLimit))
	if seriesVenue != "" {
		params.Set("venue_id", seriesVenue)
	}
	if seriesSource != "" {
		params.Set("source", seriesSource)
	}

	u := fmt.Sprintf("%s/api/v1/admin/events/series-candidates?%s", serverURL, params.Encode())
	body, err := getWithProblemDetail(client, u, jwt)
	if err != nil {
		return fmt.Errorf("fetch series candidates: %w", err)
	}

	var resp SeriesCandidatesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse series candidates response: %w", err)
	}

	if reviewJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	printSeriesCandidates(out, resp, seriesOffset)
	return nil
}

// getWithProblemDetail is doGET with RFC 7807 title/detail surfaced in the
// error, matching doPOST's behaviour for admin API failures.
func getWithProblemDetail(client *http.Client, requestURL, authKey string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if authKey != "" {
		req.Header.Set("Authorization", "Bearer "+authKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401)")
	}
	if resp.StatusCode >= 400 {
		var pd problemDetail
		if json.Unmarshal(body, &pd) == nil && (pd.Title != "" || pd.Detail != "") {
			return nil, fmt.Errorf("server returned status %d: %s: %s", resp.StatusCode, pd.Title, pd.Detail)
		}
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func printSeriesCandidates(out io.Writer, resp SeriesCandidatesResponse, offset int) {
	if len(resp.Items) == 0 {
		if offset > 0 && resp.TotalClusters > 0 {
			_, _ = fmt.Fprintf(out, "%d series candidate(s) (offset %d is past the end)\n", resp.TotalClusters, offset)
			return
		}
		_, _ = fmt.Fprintln(out, "No series candidates found.")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "The detector covers all published events but is deliberately conservative")
		_, _ = fmt.Fprintln(out, "(same venue, similar name, same weekday, start time within 30 minutes,")
		_, _ = fmt.Fprintln(out, "exact 7-day cadence). Absence is not proof that no series exist.")
		_, _ = fmt.Fprintln(out, "Widen --days/--lookback-days or lower --min-events to look further.")
		return
	}

	if offset > 0 || resp.Truncated {
		start := offset + 1
		end := offset + len(resp.Items)
		_, _ = fmt.Fprintf(out, "%d series candidate(s) (showing %d-%d of %d", resp.TotalClusters, start, end, resp.TotalClusters)
		// Never suggest an offset the client-side validator would reject.
		if end < resp.TotalClusters && end <= seriesMaxOffset {
			_, _ = fmt.Fprintf(out, "; next: --offset %d", end)
		}
		_, _ = fmt.Fprintln(out, ")")
	} else {
		_, _ = fmt.Fprintf(out, "%d series candidate(s)\n", resp.TotalClusters)
	}

	for i, item := range resp.Items {
		// Number clusters globally so page 2 continues the header's numbering.
		_, _ = fmt.Fprintf(out, "\n%d) %s — %s — %s — %d events — %s..%s\n",
			offset+i+1,
			item.Name,
			item.Venue.Name,
			seriesWhen(item),
			item.EventCount,
			item.FirstDate,
			item.LastDate,
		)
		if seriesNamesDiffer(item) {
			_, _ = fmt.Fprintln(out, "   names differ: inspect carefully")
		}
		if item.ContentVaries {
			printSeriesDescriptions(out, item)
		}
		_, _ = fmt.Fprintf(out, "   canonical: %s\n", item.SuggestedCanonicalULID)
		_, _ = fmt.Fprintln(out, "   members:")
		for _, member := range item.Members {
			marker := "  "
			if member.ULID == item.SuggestedCanonicalULID {
				marker = "* "
			}
			_, _ = fmt.Fprintf(out, "     %s%s  %s  %s\n", marker, member.ULID, member.StartDate, member.Name)
		}
		_, _ = fmt.Fprintf(out, "   next: server review consolidate %s %s --transfer-occurrences --dry-run\n",
			item.SuggestedCanonicalULID,
			strings.Join(otherMemberULIDs(item), " "),
		)
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "* = suggested canonical. Decide every cluster: consolidate, or keep separate with a stated reason.")
}

// printSeriesDescriptions renders the content_varies evidence block. The
// descriptions are display evidence only; the full text lives on each event.
func printSeriesDescriptions(out io.Writer, item SeriesCandidateItem) {
	distinct := len(item.DistinctDescriptions)
	if distinct == 0 {
		distinct = item.DistinctDescriptionCount
	}
	_, _ = fmt.Fprintf(out, "   descriptions differ (%d distinct, %d/%d members described): decide, do not skip\n",
		distinct, item.DescribedMemberCount, item.EventCount)
	shortened := false
	for _, description := range item.DistinctDescriptions {
		line, truncated := formatSeriesDescription(description)
		shortened = shortened || truncated
		_, _ = fmt.Fprintf(out, "     - %s\n", line)
	}
	if item.DescribedMemberCount < item.EventCount {
		_, _ = fmt.Fprintf(out, "   undescribed: %d of %d members have no description; fetch their events before deciding\n",
			item.EventCount-item.DescribedMemberCount, item.EventCount)
	}
	if item.DescriptionsTruncated || shortened {
		_, _ = fmt.Fprintf(out, "   [descriptions shortened to %d chars for display; full text on the event]\n", seriesDescriptionDisplayLimit)
	}
}

// formatSeriesDescription flattens whitespace to one line and shortens the text
// to a terminal-friendly length, appending an ellipsis when it is cut. The
// boolean reports whether the display text was shortened.
func formatSeriesDescription(description string) (string, bool) {
	flat := strings.Join(strings.Fields(description), " ")
	runes := []rune(flat)
	if len(runes) > seriesDescriptionDisplayLimit {
		return string(runes[:seriesDescriptionDisplayLimit]) + "…", true
	}
	return flat, false
}

func seriesNamesDiffer(item SeriesCandidateItem) bool {
	if len(item.Members) < 2 {
		return false
	}
	first := strings.TrimSpace(item.Members[0].Name)
	for _, member := range item.Members[1:] {
		if !strings.EqualFold(first, strings.TrimSpace(member.Name)) {
			return true
		}
	}
	return false
}

func seriesWhen(item SeriesCandidateItem) string {
	day := weekdayName(item.DayOfWeek)
	if item.LocalStartTime == "" {
		return day
	}
	return day + " " + item.LocalStartTime
}

func weekdayName(isodow int) string {
	switch isodow {
	case 1:
		return "Mon"
	case 2:
		return "Tue"
	case 3:
		return "Wed"
	case 4:
		return "Thu"
	case 5:
		return "Fri"
	case 6:
		return "Sat"
	case 7:
		return "Sun"
	default:
		return "?"
	}
}

func otherMemberULIDs(item SeriesCandidateItem) []string {
	ulids := make([]string, 0, len(item.Members))
	for _, member := range item.Members {
		if member.ULID == item.SuggestedCanonicalULID {
			continue
		}
		ulids = append(ulids, member.ULID)
	}
	return ulids
}
