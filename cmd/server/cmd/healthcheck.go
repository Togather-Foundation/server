package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/deployment"
	"github.com/spf13/cobra"
)

var (
	// healthcheckCmd represents the healthcheck command
	healthcheckCmd = &cobra.Command{
		Use:   "healthcheck",
		Short: "Check if the server is healthy",
		Long: `Performs a health check by calling the /health endpoint.

This command supports multiple modes:
  1. Basic check (default): Check localhost server
  2. Slot check: Check specific blue/green deployment slot
  3. Deployment check: Check deployment based on deployment-state.json
  4. Watch mode: Continuously monitor health status

Exit codes:
  0 - Server is healthy
  1 - Server is unhealthy or unreachable
  2 - Invalid response from server

Examples:
  # Basic health check (Docker HEALTHCHECK compatible)
  server healthcheck

  # Check specific deployment slot
  server healthcheck --slot blue
  server healthcheck --slot green

  # Check deployment from state file
  server healthcheck --deployment production

  # Watch mode with custom interval
  server healthcheck --watch --interval 10s

  # Retry with backoff
  server healthcheck --retries 5 --retry-delay 3s

  # Different output formats
  server healthcheck --format json
  server healthcheck --format table
  server healthcheck --format simple`,
		RunE: runHealthcheck,
	}

	// Flags
	healthcheckTimeout    int
	healthcheckURL        string
	healthcheckSlot       string
	healthcheckDeployment string
	healthcheckWatch      bool
	healthcheckInterval   time.Duration
	healthcheckMaxChecks  int
	healthcheckRetries    int
	healthcheckRetryDelay time.Duration
	healthcheckFormat     string
)

func init() {
	healthcheckCmd.Flags().IntVar(&healthcheckTimeout, "timeout", 5, "timeout in seconds for each health check")
	healthcheckCmd.Flags().StringVar(&healthcheckURL, "url", "", "health check URL (overrides slot/deployment detection)")
	healthcheckCmd.Flags().StringVar(&healthcheckSlot, "slot", "", "deployment slot to check (blue or green)")
	healthcheckCmd.Flags().StringVar(&healthcheckDeployment, "deployment", "", "check deployment from deployment-state.json")
	healthcheckCmd.Flags().BoolVar(&healthcheckWatch, "watch", false, "continuously monitor health status")
	healthcheckCmd.Flags().DurationVar(&healthcheckInterval, "interval", 5*time.Second, "interval between checks in watch mode")
	healthcheckCmd.Flags().IntVar(&healthcheckMaxChecks, "max-checks", 0, "maximum number of checks in watch mode (0=unlimited)")
	healthcheckCmd.Flags().IntVar(&healthcheckRetries, "retries", 3, "number of retry attempts on failure")
	healthcheckCmd.Flags().DurationVar(&healthcheckRetryDelay, "retry-delay", 2*time.Second, "delay between retry attempts")
	healthcheckCmd.Flags().StringVar(&healthcheckFormat, "format", "simple", "output format: simple, json, or table")
}

// HealthResponse matches the response from internal/api/handlers/health.go
type HealthResponse struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version,omitempty"`
	GitCommit string                 `json:"git_commit,omitempty"`
	Slot      string                 `json:"slot,omitempty"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	LatencyMs int64                  `json:"latency_ms,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthCheckResult represents the result of a health check operation
type HealthCheckResult struct {
	URL        string          `json:"url"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Response   *HealthResponse `json:"response,omitempty"`
	Error      string          `json:"error,omitempty"`
	LatencyMs  int64           `json:"latency_ms"`
	CheckedAt  time.Time       `json:"checked_at"`
	Slot       string          `json:"slot,omitempty"`
	IsHealthy  bool            `json:"is_healthy"`
	RetryCount int             `json:"retry_count,omitempty"`
}

func runHealthcheck(cmd *cobra.Command, args []string) error {
	// Validate flags
	if healthcheckSlot != "" && healthcheckSlot != "blue" && healthcheckSlot != "green" {
		return fmt.Errorf("invalid slot: %s (must be blue or green)", healthcheckSlot)
	}

	if healthcheckFormat != "simple" && healthcheckFormat != "json" && healthcheckFormat != "table" {
		return fmt.Errorf("invalid format: %s (must be simple, json, or table)", healthcheckFormat)
	}

	// Determine URL(s) to check
	urls, err := determineHealthCheckURLs()
	if err != nil {
		return err
	}

	// Watch mode
	if healthcheckWatch {
		return runWatchMode(urls)
	}

	// Single check mode (with retries)
	results := make([]HealthCheckResult, 0, len(urls))
	allHealthy := true

	for _, url := range urls {
		result := performHealthCheckWithRetries(url)
		results = append(results, result)
		if !result.IsHealthy {
			allHealthy = false
		}
	}

	// Output results
	outputResults(results)

	// Exit with appropriate code
	if !allHealthy {
		os.Exit(1)
	}

	return nil
}

// determineHealthCheckURLs determines which URL(s) to check based on flags
func determineHealthCheckURLs() ([]string, error) {
	// Explicit URL overrides everything
	if healthcheckURL != "" {
		return []string{healthcheckURL}, nil
	}

	// Deployment state check
	if healthcheckDeployment != "" {
		return getDeploymentURLs(healthcheckDeployment)
	}

	// Slot-specific check
	if healthcheckSlot != "" {
		port := getSlotPort(healthcheckSlot)
		url := fmt.Sprintf("http://localhost:%d/health", port)
		return []string{url}, nil
	}

	// Default: check localhost with SERVER_PORT
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	url := fmt.Sprintf("http://localhost:%s/health", port)
	return []string{url}, nil
}

// getDeploymentURLs gets health check URLs based on deployment state
func getDeploymentURLs(environment string) ([]string, error) {
	cfg := deployment.DefaultConfig()
	state, err := deployment.LoadState(cfg.StateFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load deployment state: %w", err)
	}

	if state.CurrentDeployment == nil {
		return nil, fmt.Errorf("no deployment found for environment: %s", environment)
	}

	// Check active slot
	activeSlot := state.GetActiveSlot()
	port := getSlotPort(activeSlot.String())
	url := fmt.Sprintf("http://localhost:%d/health", port)

	return []string{url}, nil
}

// getSlotPort returns the port number for a given slot
func getSlotPort(slot string) int {
	switch slot {
	case "blue":
		return 8081
	case "green":
		return 8082
	default:
		return 8080
	}
}

// performHealthCheckWithRetries performs a health check with retry logic
func performHealthCheckWithRetries(url string) HealthCheckResult {
	var lastResult HealthCheckResult

	for attempt := 0; attempt <= healthcheckRetries; attempt++ {
		lastResult = performHealthCheck(url)
		lastResult.RetryCount = attempt

		if lastResult.IsHealthy {
			return lastResult
		}

		// Don't sleep after last attempt
		if attempt < healthcheckRetries {
			time.Sleep(healthcheckRetryDelay)
		}
	}

	return lastResult
}

// performHealthCheck performs a single health check
func performHealthCheck(url string) HealthCheckResult {
	result := HealthCheckResult{
		URL:       url,
		CheckedAt: time.Now(),
	}

	// Extract slot from URL if possible
	if strings.Contains(url, ":8081") {
		result.Slot = "blue"
	} else if strings.Contains(url, ":8082") {
		result.Slot = "green"
	}

	start := time.Now()

	// Create HTTP client with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(healthcheckTimeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	// Perform the health check
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.LatencyMs = time.Since(start).Milliseconds()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		result.Status = "unhealthy"
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}

	// Parse response body
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		result.Error = fmt.Sprintf("invalid response: %v", err)
		result.StatusCode = 2 // Invalid response
		return result
	}

	result.Response = &healthResp
	result.Status = healthResp.Status
	result.IsHealthy = (healthResp.Status == "healthy")

	return result
}

// runWatchMode runs continuous health monitoring
func runWatchMode(urls []string) error {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("Starting health check monitoring (interval: %s, press Ctrl+C to stop)\n\n", healthcheckInterval)

	checkCount := 0
	ticker := time.NewTicker(healthcheckInterval)
	defer ticker.Stop()

	// Perform initial check immediately
	performAndDisplayWatchCheck(urls, checkCount)
	checkCount++

	for {
		select {
		case <-sigChan:
			fmt.Println("\nStopping health check monitoring...")
			return nil
		case <-ticker.C:
			performAndDisplayWatchCheck(urls, checkCount)
			checkCount++

			if healthcheckMaxChecks > 0 && checkCount >= healthcheckMaxChecks {
				fmt.Printf("\nReached maximum checks (%d), stopping...\n", healthcheckMaxChecks)
				return nil
			}
		}
	}
}

// performAndDisplayWatchCheck performs and displays a single watch check
func performAndDisplayWatchCheck(urls []string, checkCount int) {
	results := make([]HealthCheckResult, 0, len(urls))

	for _, url := range urls {
		result := performHealthCheck(url)
		results = append(results, result)
	}

	// Display results inline
	timestamp := time.Now().Format("15:04:05")
	for _, result := range results {
		status := "✓ HEALTHY"
		if !result.IsHealthy {
			status = "✗ UNHEALTHY"
		}

		slotInfo := ""
		if result.Slot != "" {
			slotInfo = fmt.Sprintf(" [%s]", result.Slot)
		}

		latency := fmt.Sprintf("%dms", result.LatencyMs)

		if result.Error != "" {
			fmt.Printf("[%s] %s%s - ERROR: %s (latency: %s)\n",
				timestamp, status, slotInfo, result.Error, latency)
		} else {
			fmt.Printf("[%s] %s%s - %s (latency: %s)\n",
				timestamp, status, slotInfo, result.Status, latency)
		}
	}
}

// outputResults outputs health check results in the requested format
func outputResults(results []HealthCheckResult) {
	switch healthcheckFormat {
	case "json":
		outputJSON(results)
	case "table":
		outputTable(results)
	case "simple":
		outputSimple(results)
	}
}

// outputJSON outputs results as JSON
func outputJSON(results []HealthCheckResult) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(results)
}

// outputTable outputs results as a formatted table
func outputTable(results []HealthCheckResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLOT\tSTATUS\tDB\tRIVER\tJSONLD\tLATENCY\tURL")
	fmt.Fprintln(w, "----\t------\t--\t-----\t------\t-------\t---")

	for _, result := range results {
		slot := result.Slot
		if slot == "" {
			slot = "-"
		}

		status := result.Status
		if result.Error != "" {
			status = "ERROR"
		}

		dbStatus := "-"
		riverStatus := "-"
		jsonldStatus := "-"

		if result.Response != nil && result.Response.Checks != nil {
			if db, ok := result.Response.Checks["database"]; ok {
				dbStatus = db.Status
			}
			if river, ok := result.Response.Checks["job_queue"]; ok {
				riverStatus = river.Status
			}
			if jsonld, ok := result.Response.Checks["jsonld_contexts"]; ok {
				jsonldStatus = jsonld.Status
			}
		}

		latency := fmt.Sprintf("%dms", result.LatencyMs)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			slot, status, dbStatus, riverStatus, jsonldStatus, latency, result.URL)
	}

	w.Flush()
}

// outputSimple outputs a simple one-line result (for scripts)
func outputSimple(results []HealthCheckResult) {
	for _, result := range results {
		if result.IsHealthy {
			fmt.Println("OK")
		} else if result.Error != "" {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", result.Error)
		} else {
			fmt.Fprintf(os.Stderr, "DEGRADED: %s\n", result.Status)
		}
	}
}
