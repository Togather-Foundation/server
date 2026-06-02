package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/api/apitypes"
	"github.com/spf13/cobra"
)

var (
	scrapeFailuresCmd = &cobra.Command{
		Use:   "failures",
		Short: "Show failing scraper sources",
		Long: `Show diagnostics for scraper sources, with options for per-source deep dives,
table overview, or JSON output.

Examples:
  server scrape failures                    # table of all failing sources
  server scrape failures --source mysource  # deep dive into one source
  server scrape failures --json             # JSON output for all sources
  server scrape failures --json --source mysource  # JSON for one source
  server scrape failures --status error     # filter by status
  server scrape failures --limit 20         # limit results`,
		RunE: runScrapeFailures,
	}

	failuresSource string
	failuresLimit  int
	failuresStatus string
	failuresJSON   bool
)

func init() {
	scrapeCmd.AddCommand(scrapeFailuresCmd)
	scrapeFailuresCmd.Flags().StringVar(&failuresSource, "source", "", "Source name for per-source diagnostics")
	scrapeFailuresCmd.Flags().IntVar(&failuresLimit, "limit", 0, "Maximum sources to return (0 = all)")
	scrapeFailuresCmd.Flags().StringVar(&failuresStatus, "status", "", "Filter by run status (e.g. running, completed, failed)")
	scrapeFailuresCmd.Flags().BoolVar(&failuresJSON, "json", false, "Output as JSON")
}

func runScrapeFailures(cmd *cobra.Command, args []string) error {
	serverURL, authKey, err := parseServerConfig(scrapeServerURL, scrapeAPIKey)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	out := cmd.OutOrStdout()

	if failuresJSON && failuresSource != "" {
		return fetchAndPrintSourceJSON(client, serverURL, authKey, failuresSource, out)
	}
	if failuresJSON {
		return fetchAndPrintAllJSON(client, serverURL, authKey, failuresStatus, failuresLimit, out)
	}
	if failuresSource != "" {
		return fetchAndPrintSourceDeepDive(client, serverURL, authKey, failuresSource, out)
	}
	return fetchAndPrintTable(client, serverURL, authKey, failuresStatus, failuresLimit, out)
}

func fetchAndPrintSourceJSON(client *http.Client, serverURL, authKey, source string, out io.Writer) error {
	u := fmt.Sprintf("%s/api/v1/admin/scraper/sources/%s/diagnostics", serverURL, url.PathEscape(source))
	body, err := doGET(client, u, authKey)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, string(body))
	return nil
}

func fetchAndPrintAllJSON(client *http.Client, serverURL, authKey, status string, limit int, out io.Writer) error {
	u := fmt.Sprintf("%s/api/v1/admin/scraper/diagnostics", serverURL)
	params := []string{}
	if status != "" {
		params = append(params, fmt.Sprintf("status=%s", url.QueryEscape(status)))
	}
	if limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	}
	if len(params) > 0 {
		u += "?" + strings.Join(params, "&")
	}
	body, err := doGET(client, u, authKey)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, string(body))
	return nil
}

func fetchAndPrintSourceDeepDive(client *http.Client, serverURL, authKey, source string, out io.Writer) error {
	u := fmt.Sprintf("%s/api/v1/admin/scraper/sources/%s/diagnostics", serverURL, url.PathEscape(source))
	body, err := doGET(client, u, authKey)
	if err != nil {
		return err
	}
	var resp apitypes.DiagnosticsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	printDeepDive(out, &resp)
	return nil
}

func fetchAndPrintTable(client *http.Client, serverURL, authKey, status string, limit int, out io.Writer) error {
	u := fmt.Sprintf("%s/api/v1/admin/scraper/diagnostics", serverURL)
	params := []string{}
	if status != "" {
		params = append(params, fmt.Sprintf("status=%s", url.QueryEscape(status)))
	}
	if limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	}
	if len(params) > 0 {
		u += "?" + strings.Join(params, "&")
	}
	body, err := doGET(client, u, authKey)
	if err != nil {
		return err
	}
	var resp apitypes.AllDiagnosticsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	printTable(out, &resp)
	return nil
}

func doGET(client *http.Client, url, authKey string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if authKey != "" {
		req.Header.Set("Authorization", "Bearer "+authKey)
	}

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
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return body, nil
}

func printDeepDive(out io.Writer, diag *apitypes.DiagnosticsResponse) {
	_, _ = fmt.Fprintf(out, "Source: %s\n", diag.SourceName)

	var sourceURL string
	var tier int32
	if diag.LatestRun != nil {
		sourceURL = diag.LatestRun.SourceURL
		tier = diag.LatestRun.Tier
	} else if diag.LastSuccessfulRun != nil {
		sourceURL = diag.LastSuccessfulRun.SourceURL
		tier = diag.LastSuccessfulRun.Tier
	}
	_, _ = fmt.Fprintf(out, "URL: %s\n", sourceURL)
	_, _ = fmt.Fprintf(out, "Tier: %d\n", tier)
	_, _ = fmt.Fprintln(out)

	if diag.LatestRun == nil {
		_, _ = fmt.Fprintln(out, "No runs found.")
	} else {
		printRunDetail(out, "Latest run", diag.LatestRun)
	}

	if diag.LastSuccessfulRun != nil {
		printRunDetail(out, "Last successful run", diag.LastSuccessfulRun)
	}
}

func printRunDetail(out io.Writer, label string, run *apitypes.ScraperRunResponse) {
	startedAt := "N/A"
	if run.StartedAt != nil {
		startedAt = run.StartedAt.UTC().Format(time.RFC3339)
	}
	_, _ = fmt.Fprintf(out, "%s (%s UTC) \u2014 %s\n", label, startedAt, run.Status)
	if run.ErrorMessage != "" {
		_, _ = fmt.Fprintf(out, "  Error: %s\n", run.ErrorMessage)
	}
	_, _ = fmt.Fprintf(out, "  Events found: %d | new: %d | dup: %d | failed: %d\n",
		run.EventsFound, run.EventsNew, run.EventsDup, run.EventsFailed)

	if len(run.EventFailures) > 0 {
		_, _ = fmt.Fprintln(out, "  Event failures:")
		for _, ef := range run.EventFailures {
			_, _ = fmt.Fprintf(out, "    [%d] %s\n", ef.Index, ef.Message)
		}
	}

}

func printTable(out io.Writer, resp *apitypes.AllDiagnosticsResponse) {
	if len(resp.Items) == 0 {
		_, _ = fmt.Fprintln(out, "No failed sources found.")
		return
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SOURCE\tTIER\tSTATUS\tEVENTS\tFAILED\tERROR")

	statusCounts := map[string]int{}
	for _, item := range resp.Items {
		statusCounts[item.Status]++

		tier := fmt.Sprintf("%d", item.Tier)
		eventsFound := fmt.Sprintf("%d", item.EventsFound)
		eventsFailed := fmt.Sprintf("%d", item.EventsFailed)
		errMsg := truncateError(item.ErrorMessage, 60)

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			item.SourceName, tier, item.Status, eventsFound, eventsFailed, errMsg)
	}

	_ = w.Flush()

	_, _ = fmt.Fprintln(out, "---")
	for status, count := range statusCounts {
		_, _ = fmt.Fprintf(out, "%d sources %s\n", count, status)
	}
}

func truncateError(msg string, maxLen int) string {
	if maxLen < 4 || len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
