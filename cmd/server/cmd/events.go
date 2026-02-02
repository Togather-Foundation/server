package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	eventsLimit     int
	eventsServerURL string
	eventsFormat    string
	eventsVerbose   bool
)

// eventsCmd represents the events command
var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Query events from the SEL server",
	Long: `Query and list events from the SEL server.

This command queries the public events API and displays results in a readable format.
No authentication required for public read access.

Examples:
  # List recent events (default: 10)
  server events

  # List 20 events
  server events --limit 20

  # Show verbose output with full details
  server events --verbose

  # Query custom server
  server events --server http://localhost:8080

  # Output raw JSON
  server events --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEventsQuery()
	},
}

func init() {
	rootCmd.AddCommand(eventsCmd)

	eventsCmd.Flags().IntVarP(&eventsLimit, "limit", "n", 10, "number of events to retrieve")
	eventsCmd.Flags().StringVar(&eventsServerURL, "server", "http://localhost:8080", "SEL server URL")
	eventsCmd.Flags().StringVar(&eventsFormat, "format", "table", "output format (table, json)")
	eventsCmd.Flags().BoolVarP(&eventsVerbose, "verbose", "v", false, "show detailed event information")
}

func runEventsQuery() error {
	// Build request URL
	url := fmt.Sprintf("%s/api/v1/events?limit=%d", eventsServerURL, eventsLimit)

	// Make request
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error %d: %s", resp.StatusCode, string(body))
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Output based on format
	if eventsFormat == "json" {
		// Pretty-print JSON
		output, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format JSON: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	// Table format
	items, ok := result["items"].([]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format: missing items array")
	}

	if len(items) == 0 {
		fmt.Println("No events found.")
		return nil
	}

	fmt.Printf("Found %d event(s):\n\n", len(items))

	for i, item := range items {
		event, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		fmt.Printf("%d. %s\n", i+1, getString(event, "name"))

		if eventsVerbose {
			// Detailed view
			fmt.Printf("   ID:          %s\n", extractID(event))
			fmt.Printf("   Start:       %s\n", formatDate(getString(event, "startDate")))
			if endDate := getString(event, "endDate"); endDate != "" {
				fmt.Printf("   End:         %s\n", formatDate(endDate))
			}
			if location := getLocation(event); location != "" {
				fmt.Printf("   Location:    %s\n", location)
			}
			if organizer := getString(event, "organizer"); organizer != "" {
				fmt.Printf("   Organizer:   %s\n", organizer)
			}
			if desc := getString(event, "description"); desc != "" {
				// Truncate long descriptions
				if len(desc) > 100 {
					desc = desc[:97] + "..."
				}
				fmt.Printf("   Description: %s\n", desc)
			}
			fmt.Println()
		} else {
			// Compact view
			location := getLocation(event)
			startDate := formatDate(getString(event, "startDate"))

			parts := []string{}
			if startDate != "" {
				parts = append(parts, startDate)
			}
			if location != "" {
				parts = append(parts, location)
			}

			if len(parts) > 0 {
				fmt.Printf("   %s\n", strings.Join(parts, " • "))
			}
		}
	}

	// Show pagination info
	if nextCursor, ok := result["next_cursor"].(string); ok && nextCursor != "" {
		fmt.Println("\nMore events available. Use --limit to fetch more.")
	}

	return nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func extractID(event map[string]interface{}) string {
	// Try @id first (JSON-LD)
	if id := getString(event, "@id"); id != "" {
		// Extract ULID from URL
		parts := strings.Split(id, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return id
	}
	// Try id field
	return getString(event, "id")
}

func getLocation(event map[string]interface{}) string {
	loc, ok := event["location"].(map[string]interface{})
	if !ok {
		return ""
	}

	name := getString(loc, "name")
	locality := getString(loc, "addressLocality")

	if name != "" && locality != "" {
		return fmt.Sprintf("%s, %s", name, locality)
	} else if name != "" {
		return name
	} else if locality != "" {
		return locality
	}

	return ""
}

func formatDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	// Try parsing as RFC3339
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return dateStr // Return as-is if can't parse
	}

	// Format as "Jan 2, 2006 3:04 PM"
	return t.Format("Jan 2, 2006 3:04 PM")
}
