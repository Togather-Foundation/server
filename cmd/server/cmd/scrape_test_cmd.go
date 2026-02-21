package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var (
	// flags for scrape test
	scrapeTestSelectorFile string
	scrapeTestEventList    string
	scrapeTestName         string
	scrapeTestStartDate    string
	scrapeTestEndDate      string
	scrapeTestLocation     string
	scrapeTestDescription  string
	scrapeTestURL          string
	scrapeTestImage        string
	scrapeTestPagination   string
)

// scrapeInspectCmd fetches a URL and prints a DOM structure summary to help
// discover CSS selectors for Tier 1 scraping.
var scrapeInspectCmd = &cobra.Command{
	Use:   "inspect <URL>",
	Short: "Analyse a page's DOM to discover CSS selectors",
	Long: `Fetch a URL and print a summary of its DOM structure:
  - Most frequent CSS classes (top 20)
  - data-* attribute names and counts
  - hrefs containing "event" or "program"
  - Candidate event container elements

Use this to identify selectors before writing a source config.

Examples:
  server scrape inspect https://harbourfrontcentre.com/whats-on/
  server scrape inspect https://example.com/events`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		result, err := scraper.Inspect(ctx, args[0])
		if err != nil {
			return fmt.Errorf("inspect: %w", err)
		}

		fmt.Print(scraper.FormatInspectResult(result))
		return nil
	},
}

// scrapeTestCmd runs a SelectorConfig against a live URL and prints extracted
// RawEvents. Selectors may be provided via flags or a YAML file.
var scrapeTestCmd = &cobra.Command{
	Use:   "test <URL>",
	Short: "Test CSS selectors against a live URL",
	Long: `Run a set of CSS selectors against a live URL using the Tier 1 (Colly)
extractor and print the extracted events. Use this to validate selectors before
enabling a source config.

Selectors can be specified via flags or loaded from a YAML source config file
(--config). Flags take precedence over the config file.

Examples:
  # Test with inline flags
  server scrape test https://harbourfrontcentre.com/whats-on/ \
    --event-list ".wo-event" \
    --name ".date-copy div:nth-child(2)" \
    --start-date ".date-copy div:first-child"

  # Test using an existing source config file
  server scrape test https://harbourfrontcentre.com/whats-on/ \
    --config configs/sources/harbourfront-centre.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

		// Start from an empty config for this URL.
		cfg := scraper.SourceConfig{
			Name:     "test",
			URL:      rawURL,
			Tier:     1,
			MaxPages: 1,
			Enabled:  true,
		}

		// Load base from YAML file if provided.
		if scrapeTestSelectorFile != "" {
			loaded, err := scraper.LoadSourceConfig(scrapeTestSelectorFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg.Selectors = loaded.Selectors
		}

		// Apply flag overrides.
		if scrapeTestEventList != "" {
			cfg.Selectors.EventList = scrapeTestEventList
		}
		if scrapeTestName != "" {
			cfg.Selectors.Name = scrapeTestName
		}
		if scrapeTestStartDate != "" {
			cfg.Selectors.StartDate = scrapeTestStartDate
		}
		if scrapeTestEndDate != "" {
			cfg.Selectors.EndDate = scrapeTestEndDate
		}
		if scrapeTestLocation != "" {
			cfg.Selectors.Location = scrapeTestLocation
		}
		if scrapeTestDescription != "" {
			cfg.Selectors.Description = scrapeTestDescription
		}
		if scrapeTestURL != "" {
			cfg.Selectors.URL = scrapeTestURL
		}
		if scrapeTestImage != "" {
			cfg.Selectors.Image = scrapeTestImage
		}
		if scrapeTestPagination != "" {
			cfg.Selectors.Pagination = scrapeTestPagination
		}

		if cfg.Selectors.EventList == "" {
			return fmt.Errorf("--event-list (or --config with selectors.event_list) is required")
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		extractor := scraper.NewCollyExtractor(logger)
		events, err := extractor.ScrapeWithSelectors(ctx, cfg)
		if err != nil {
			return fmt.Errorf("scrape test: %w", err)
		}

		if len(events) == 0 {
			fmt.Println("No events extracted.")
			return nil
		}

		fmt.Printf("Extracted %d event(s):\n\n", len(events))
		for i, e := range events {
			fmt.Printf("[%d] Name:        %s\n", i+1, e.Name)
			fmt.Printf("    StartDate:   %s\n", e.StartDate)
			fmt.Printf("    EndDate:     %s\n", e.EndDate)
			fmt.Printf("    Location:    %s\n", e.Location)
			fmt.Printf("    URL:         %s\n", e.URL)
			fmt.Printf("    Image:       %s\n", e.Image)
			if e.Description != "" {
				desc := e.Description
				if len(desc) > 120 {
					desc = desc[:120] + "…"
				}
				fmt.Printf("    Description: %s\n", desc)
			}
			fmt.Println()
		}
		return nil
	},
}
