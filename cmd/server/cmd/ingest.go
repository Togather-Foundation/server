package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/spf13/cobra"
)

var (
	ingestAPIKey    string
	ingestServerURL string
	ingestTimeout   int
	ingestWatch     bool
)

// ingestCmd represents the ingest command
var ingestCmd = &cobra.Command{
	Use:   "ingest <file.json>",
	Short: "Ingest events from a JSON file",
	Long: `Ingest events from a JSON file using the batch ingestion API.

The JSON file should contain an object with an "events" array:
{
  "events": [
    {
      "@type": "Event",
      "name": "Concert",
      "startDate": "2026-02-15T20:00:00Z",
      ...
    }
  ]
}

Authentication:
  Set API_KEY environment variable or use --key flag.

Examples:
  # Ingest with API key from environment
  export API_KEY="your-key-here"
  server ingest events.json

  # Ingest with explicit API key
  server ingest events.json --key "your-key-here"

  # Ingest and watch batch status
  server ingest events.json --watch

  # Use custom server URL
  server ingest events.json --server http://production:8080`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		return ingestEvents(filePath)
	},
}

func init() {
	rootCmd.AddCommand(ingestCmd)

	// Flags
	ingestCmd.Flags().StringVar(&ingestAPIKey, "key", "", "API key for authentication (or use API_KEY env var)")
	ingestCmd.Flags().StringVar(&ingestServerURL, "server", "http://localhost:8080", "SEL server URL")
	ingestCmd.Flags().IntVar(&ingestTimeout, "timeout", 30, "request timeout in seconds")
	ingestCmd.Flags().BoolVar(&ingestWatch, "watch", false, "watch batch processing status")
}

func ingestEvents(filePath string) error {
	// Load .env file if it exists (so API_KEY can be read from it)
	config.LoadEnvFile(".env")
	config.LoadEnvFile("deploy/docker/.env")

	// Read API key from flag or environment
	apiKey := ingestAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("API key required: set API_KEY environment variable or use --key flag")
	}

	// Read JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Validate JSON structure
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	events, ok := payload["events"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid JSON structure: expected {\"events\": [...]} format")
	}

	fmt.Printf("📦 Ingesting %d event(s) from %s\n", len(events), filePath)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(ingestTimeout) * time.Second,
	}

	// Build request
	url := fmt.Sprintf("%s/api/v1/events:batch", ingestServerURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	// Send request
	fmt.Printf("🚀 Sending to %s\n", url)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Handle non-success status codes
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Error: HTTP %d\n", resp.StatusCode)
		fmt.Printf("Response: %s\n", string(body))
		return fmt.Errorf("ingestion failed with status %d", resp.StatusCode)
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("⚠️  Warning: Could not parse response as JSON\n")
		fmt.Printf("Response: %s\n", string(body))
		return nil
	}

	// Pretty print response
	fmt.Printf("✓ Batch accepted!\n\n")

	if batchID, ok := result["batch_id"].(string); ok {
		fmt.Printf("Batch ID:    %s\n", batchID)
	}
	if jobID, ok := result["job_id"].(float64); ok {
		fmt.Printf("Job ID:      %.0f\n", jobID)
	}
	if status, ok := result["status"].(string); ok {
		fmt.Printf("Status:      %s\n", status)
	}
	if submitted, ok := result["submitted"].(float64); ok {
		fmt.Printf("Submitted:   %.0f event(s)\n", submitted)
	}
	if statusURL, ok := result["status_url"].(string); ok {
		fmt.Printf("Status URL:  %s\n", statusURL)
	}

	// Watch batch processing if requested
	if ingestWatch {
		if batchID, ok := result["batch_id"].(string); ok {
			fmt.Printf("\n🔍 Watching batch processing...\n")
			return watchBatchStatus(client, batchID)
		}
	}

	return nil
}

func watchBatchStatus(client *http.Client, batchID string) error {
	statusURL := fmt.Sprintf("%s/api/v1/batch-status/%s", ingestServerURL, batchID)

	// Poll every 2 seconds for up to 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("⏱️  Timeout waiting for batch completion\n")
			return nil
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
			if err != nil {
				return fmt.Errorf("create status request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("check status: %w", err)
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("read status response: %w", err)
			}

			// 404 means still processing
			if resp.StatusCode == http.StatusNotFound {
				fmt.Printf(".")
				continue
			}

			if resp.StatusCode == http.StatusOK {
				var status map[string]interface{}
				if err := json.Unmarshal(body, &status); err == nil {
					fmt.Printf("\n✓ Batch completed!\n\n")
					prettyJSON, _ := json.MarshalIndent(status, "", "  ")
					fmt.Printf("%s\n", prettyJSON)
					return nil
				}
			}

			fmt.Printf("\n⚠️  Unexpected status code: %d\n", resp.StatusCode)
			fmt.Printf("Response: %s\n", string(body))
			return nil
		}
	}
}
