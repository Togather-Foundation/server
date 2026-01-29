package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	// healthcheckCmd represents the healthcheck command
	healthcheckCmd = &cobra.Command{
		Use:   "healthcheck",
		Short: "Check if the server is healthy",
		Long: `Performs a health check by calling the /health endpoint.
		
This command is used by Docker HEALTHCHECK to monitor container health.
It exits with code 0 if the server is healthy, non-zero otherwise.

Exit codes:
  0 - Server is healthy
  1 - Server is unhealthy or unreachable
  2 - Invalid response from server`,
		RunE: runHealthcheck,
	}

	// Flags
	healthcheckTimeout int
	healthcheckURL     string
)

func init() {
	healthcheckCmd.Flags().IntVar(&healthcheckTimeout, "timeout", 5, "timeout in seconds")
	healthcheckCmd.Flags().StringVar(&healthcheckURL, "url", "", "health check URL (default: http://localhost:{SERVER_PORT}/health)")
}

// HealthResponse matches the response from internal/api/handlers/health.go
type HealthResponse struct {
	Status string                 `json:"status"`
	Checks map[string]interface{} `json:"checks,omitempty"`
}

func runHealthcheck(cmd *cobra.Command, args []string) error {
	// Determine health check URL
	url := healthcheckURL
	if url == "" {
		// Default to localhost with SERVER_PORT from environment
		port := os.Getenv("SERVER_PORT")
		if port == "" {
			port = "8080"
		}
		url = fmt.Sprintf("http://localhost:%s/health", port)
	}

	// Create HTTP client with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(healthcheckTimeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
		return err
	}

	// Perform the health check
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", closeErr)
		}
	}()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Health check returned status %d\n", resp.StatusCode)
		os.Exit(1)
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}

	// Parse response body
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing health check response: %v\n", err)
		os.Exit(2)
		return err
	}

	// Check overall status
	if healthResp.Status != "healthy" {
		fmt.Fprintf(os.Stderr, "Server status: %s\n", healthResp.Status)
		os.Exit(1)
		return fmt.Errorf("unhealthy: status=%s", healthResp.Status)
	}

	// Success - server is healthy
	return nil
}
