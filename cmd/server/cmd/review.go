package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type Warning struct {
	Field   string         `json:"field"`
	Message string         `json:"message"`
	Code    string         `json:"code"`
	Details map[string]any `json:"details,omitempty"`
}

type ReviewQueueItem struct {
	ID                 int        `json:"id"`
	EventID            string     `json:"eventId"`
	Status             string     `json:"status"`
	Warnings           []Warning  `json:"warnings"`
	CreatedAt          time.Time  `json:"createdAt"`
	ReviewedBy         *string    `json:"reviewedBy,omitempty"`
	ReviewedAt         *time.Time `json:"reviewedAt,omitempty"`
	RejectionReason    *string    `json:"rejectionReason,omitempty"`
	DuplicateOfEventID *string    `json:"duplicateOfEventUlid,omitempty"`
	SourceID           *string    `json:"sourceId,omitempty"`
	EventName          string     `json:"eventName,omitempty"`
	EventStartTime     *time.Time `json:"eventStartTime,omitempty"`
	EventEndTime       *time.Time `json:"eventEndTime,omitempty"`
	OccurrenceCount    int        `json:"occurrenceCount,omitempty"`
}

type ReviewQueueListResponse struct {
	Items      []ReviewQueueItem `json:"items"`
	NextCursor string            `json:"next_cursor"`
	Total      int64             `json:"total,omitempty"`
}

type ChangeDetail struct {
	Field     string `json:"field"`
	Original  any    `json:"original"`
	Corrected any    `json:"corrected"`
	Reason    string `json:"reason"`
}

type OccurrenceDetail struct {
	ID            string     `json:"id"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty"`
	Timezone      string     `json:"timezone"`
	DoorTime      *time.Time `json:"doorTime,omitempty"`
	VenueULID     *string    `json:"venueUlid,omitempty"`
	VirtualURL    *string    `json:"virtualUrl,omitempty"`
	TicketURL     string     `json:"ticketUrl,omitempty"`
	PriceMin      *float64   `json:"priceMin,omitempty"`
	PriceMax      *float64   `json:"priceMax,omitempty"`
	PriceCurrency string     `json:"priceCurrency,omitempty"`
	Availability  string     `json:"availability,omitempty"`
}

type RelatedEventDetail struct {
	ULID               string             `json:"ulid"`
	Name               string             `json:"name,omitempty"`
	Description        string             `json:"description,omitempty"`
	URL                string             `json:"url,omitempty"`
	ImageURL           string             `json:"imageUrl,omitempty"`
	VenueName          string             `json:"venueName,omitempty"`
	VenueULID          string             `json:"venueUlid,omitempty"`
	VenueStreetAddress string             `json:"venueStreetAddress,omitempty"`
	VenueCity          string             `json:"venueCity,omitempty"`
	VenueRegion        string             `json:"venueRegion,omitempty"`
	VenuePostalCode    string             `json:"venuePostalCode,omitempty"`
	OrganizerName      string             `json:"organizerName,omitempty"`
	OrganizerURL       string             `json:"organizerUrl,omitempty"`
	Occurrences        []OccurrenceDetail `json:"occurrences,omitempty"`
	Similarity         *float64           `json:"similarity,omitempty"`
}

type ReviewQueueDetail struct {
	ReviewQueueItem
	Original      map[string]any       `json:"original"`
	Normalized    map[string]any       `json:"normalized"`
	Changes       []ChangeDetail       `json:"changes"`
	ReviewNotes   *string              `json:"reviewNotes,omitempty"`
	Occurrences   []OccurrenceDetail   `json:"occurrences,omitempty"`
	RelatedEvents []RelatedEventDetail `json:"relatedEvents,omitempty"`
}

var (
	reviewServerURL string
	reviewAPIKey    string
	reviewTokenFlag string
	reviewJSON      bool
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Manage event review queue",
	Long: `Manage the event review queue. Subcommands cover listing, inspecting,
approving, rejecting, fixing, batch-operating, merging, consolidating, and
getting aggregate statistics for review queue items.

Examples:
  server review queue                      # list pending items
  server review queue --status approved    # list approved items
  server review check 42                   # deep inspect review entry 42
  server review edit 42 --description "..." # edit event fields before approving
  server review approve 42                 # approve review entry 42
  server review reject 42 --reason "spam"  # reject review entry 42
  server review fix 42 --notes "corrected" # fix and approve review entry 42
  server review batch --name "Astro" --action approve --dry-run
  server review stats                      # aggregate queue statistics
  server review merge evt_001 evt_002      # merge evt_002 into evt_001
  server review consolidate evt_001 evt_002 evt_003

═══ Common Agent Workflows ═══

── Spotting patterns ──

  # Group by event name to find duplicate/repeat scrapes
  server review queue --group-by name

  # Group by source to find bad scrape runs
  server review queue --group-by source

  # Full stats: warning breakdown, top name groups, age buckets
  server review stats

── Cleaning up bad scrape runs ──

  # 1. Identify the bad source — look for many items from same source
  server review queue --group-by source

  # 2. Inspect one item from that source
  server review check 301

  # 3. Dry-run the batch rejection to preview
  server review batch --source "meetup-scraper" --action reject --reason "bad scrape run" --dry-run

  # 4. Execute (dry-run off, --action explicitly set)
  server review batch --source "meetup-scraper" --action reject --reason "bad scrape run 2025-06-08"

── Handling cross-week series companions (same event, different dates) ──

  # 1. Group by name to find repeated event names
  server review queue --group-by name

  # 2. Inspect one companion to understand its data and find the primary
  server review check 41
  #
  #   Look at the "Related Events" section — the primary series event
  #   is listed there. The companion's warnings may also reference the
  #   primary by ULID (e.g. "companion to primary series 01ABC...").

  # 3. Inspect the primary series event to see what it already has
  server review check <primary-id>

  # 4. Merge companion into primary, copying its occurrence (date) and any missing fields
  server review merge <primary-ulid> <companion-ulid> \
    --transfer-occurrences \
    --name "Weekly Event Name" \
    --description "Fixed up description"

  # 5. Or batch-merge all companions into the primary series
  server review batch --name "Comedy Bar: After Dark" \
    --action merge-into-primary --primary-id <primary-ulid> --dry-run

  #    (review the preview, then remove --dry-run to execute)

  NOTE: When using --name/--source/--warning filters, the queue command
  automatically fetches all items (up to 1000) to ensure client-side
  filters see the complete dataset. Use --limit to override.

── Approving good-but-flagged events ──

  # If scraped events are correct but flagged (e.g. cross-week series,
  # missing_description when the event legitimately has no description):

  # 1. Inspect to confirm it looks good
  server review check 42

  # 2. Optionally edit fields before approving (description, name, image, URL, domain)
  server review edit 42 --description "Fixed up description" --name "Better Event Name"
  server review edit 42 --description "..." --dry-run    # preview first

  # 3. Approve a single item
  server review approve 42 --notes "Correct weekly event, minor description missing"

  # 4. Or batch-approve all items matching a name
  server review batch --name "Tranzac Open Stage" --action approve --dry-run

── Fixing date corrections ──

  # Event has reversed start/end dates — fix and publish in one step
  server review fix 42 --start-date 2025-07-15T20:00:00Z --end-date 2025-07-15T22:00:00Z

  # Batch-fix items with the same date correction (e.g. all shifted by one day)
  server review batch --name "Weekly Workshop" --action fix \
    --start-date 2025-07-15T09:00:00Z --dry-run

── Cleaning up duplicates ──

  # 1. Find repeated names
  server review queue --group-by name

  # 2. Consolidate multiple duplicates into one canonical
  server review consolidate <canonical-ulid> <dup1> <dup2> <dup3>

  # 3. With occurrences transfer and field patching
  server review consolidate <canonical-ulid> <dup1> <dup2> \
    --transfer-occurrences --description "Merged description"

── Full pipeline: review a new scrape run ──

  # 1. Get an overview
  server review stats

  # 2. Check for obvious junk (many from one source, many from one name)
  server review queue --group-by source
  server review queue --group-by name

  # 3. For same-name groups that are legitimate weekly events:
  #    find the primary ULID, merge companions
  server review batch --name "Event Name" --action merge-into-primary \
    --primary-id <primary-ulid> --dry-run
  server review batch --name "Event Name" --action merge-into-primary \
    --primary-id <primary-ulid>

  # 4. For bad scrapes: reject the whole batch
  server review batch --source "<bad-source>" --action reject \
    --reason "bad scrape run" --dry-run
  server review batch --source "<bad-source>" --action reject \
    --reason "bad scrape run"

  # 5. For everything else: approve single items or small batches
  server review approve 42 --notes "looks good"
  server review batch --name "Small Event Series" --action approve --dry-run
  server review batch --name "Small Event Series" --action approve

═══ Safety Notes ═══

  • --dry-run is implied for batch when --action is not explicitly set.
  • Batch chunks large sets automatically (REVIEW_BATCH_MAX_SIZE, default 100).
  • Batch adds a configurable inter-request delay (REVIEW_BATCH_DELAY_MS, default 50ms).
  • Batch stops immediately on 401/403 auth errors.
  • merge and consolidate are atomic — all operations happen in one transaction.
  • Use --json on any command for structured output.`,
}

func init() {
	rootCmd.AddCommand(reviewCmd)

	reviewCmd.PersistentFlags().StringVar(&reviewServerURL, "server", "", "Server base URL (env: TOGATHER_BASE_URL, default: http://localhost:8080)")
	reviewCmd.PersistentFlags().StringVar(&reviewAPIKey, "key", "", "Admin API key (env: TOGATHER_ADMIN_API_KEY)")
	reviewCmd.PersistentFlags().StringVar(&reviewTokenFlag, "token", "", "JWT token (skips STS exchange)")
	reviewCmd.PersistentFlags().BoolVar(&reviewJSON, "json", false, "JSON output")

	reviewCmd.AddCommand(reviewQueueCmd)
	reviewCmd.AddCommand(reviewCheckCmd)
	reviewCmd.AddCommand(reviewEditCmd)
	reviewCmd.AddCommand(reviewApproveCmd)
	reviewCmd.AddCommand(reviewRejectCmd)
	reviewCmd.AddCommand(reviewFixCmd)
	reviewCmd.AddCommand(reviewBatchCmd)
	reviewCmd.AddCommand(reviewStatsCmd)
	reviewCmd.AddCommand(reviewMergeCmd)
	reviewCmd.AddCommand(reviewConsolidateCmd)
}

func doPOST(client *http.Client, url string, body io.Reader, authKey string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if authKey != "" {
		req.Header.Set("Authorization", "Bearer "+authKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rbody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401)")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(rbody))
	}

	return rbody, nil
}

func exchangeReviewJWT(serverURL, apiKey string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/auth/token", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode token exchange response: %w", err)
	}

	return result.Token, nil
}

func getReviewJWT() (string, error) {
	if reviewTokenFlag != "" {
		return reviewTokenFlag, nil
	}

	serverURL := resolveReviewServerURL()

	key := reviewAPIKey
	if key == "" {
		key = os.Getenv("TOGATHER_ADMIN_API_KEY")
	}

	if key == "" {
		return "", fmt.Errorf("no API key provided; set --key, --token, or TOGATHER_ADMIN_API_KEY env")
	}

	return exchangeReviewJWT(serverURL, key)
}

func resolveReviewServerURL() string {
	u := reviewServerURL
	if u == "" {
		u = os.Getenv("TOGATHER_BASE_URL")
	}
	if u == "" {
		u = "http://localhost:8080"
	}
	if !strings.Contains(u, "://") && !strings.HasPrefix(u, "localhost") {
		u = "https://" + u
	}
	return u
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	if d.Hours() >= 24*7 {
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
	if d.Hours() >= 24 {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d.Hours() >= 1 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d.Minutes() >= 1 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return "<1m"
}

func formatTimeRange(newest, oldest *time.Time) string {
	if oldest == nil || newest == nil {
		return "-"
	}
	// newest-age–oldest-age (shorter age = more recent)
	return fmt.Sprintf("%s-%s", formatAge(*newest), formatAge(*oldest))
}

func fetchReviewQueue(client *http.Client, serverURL, jwt string, status string, limit int, cursor string) (*ReviewQueueListResponse, error) {
	u := fmt.Sprintf("%s/api/v1/admin/review-queue", serverURL)
	params := []string{}
	if status != "" {
		params = append(params, fmt.Sprintf("status=%s", url.QueryEscape(status)))
	}
	if limit > 0 && limit <= 100 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	} else if limit > 100 {
		params = append(params, "limit=100")
	}
	if cursor != "" {
		params = append(params, fmt.Sprintf("cursor=%s", url.QueryEscape(cursor)))
	}
	if len(params) > 0 {
		u += "?" + strings.Join(params, "&")
	}

	body, err := doGET(client, u, jwt)
	if err != nil {
		return nil, fmt.Errorf("fetch review queue: %w", err)
	}

	var resp ReviewQueueListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse review queue response: %w", err)
	}

	return &resp, nil
}

func fetchAllReviewQueue(client *http.Client, serverURL, jwt string, status string, maxLimit int) ([]ReviewQueueItem, error) {
	var all []ReviewQueueItem
	cursor := ""

	for {
		resp, err := fetchReviewQueue(client, serverURL, jwt, status, 100, cursor)
		if err != nil {
			return nil, err
		}

		all = append(all, resp.Items...)

		if maxLimit > 0 && len(all) >= maxLimit {
			all = all[:maxLimit]
			break
		}

		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return all, nil
}

func fetchReviewDetail(client *http.Client, serverURL, jwt string, id int) (*ReviewQueueDetail, error) {
	u := fmt.Sprintf("%s/api/v1/admin/review-queue/%d", serverURL, id)
	body, err := doGET(client, u, jwt)
	if err != nil {
		return nil, fmt.Errorf("fetch review detail %d: %w", id, err)
	}

	var detail ReviewQueueDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("parse review detail %d: %w", id, err)
	}

	return &detail, nil
}

func headerWarningCodes(warnings []Warning) string {
	seen := map[string]int{}
	for _, w := range warnings {
		seen[w.Code]++
	}
	var parts []string
	for code, count := range seen {
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s (x%d)", code, count))
		} else {
			parts = append(parts, code)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}

func containsWarning(warnings []Warning, code string) bool {
	for _, w := range warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}
