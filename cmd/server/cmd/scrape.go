package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var (
	scrapeServerURL string
	scrapeAPIKey    string
	scrapeDryRun    bool
	scrapeLimit     int
	scrapeSourceDir string

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

// scrapeCmd is the root command group for scraper subcommands.
var scrapeCmd = &cobra.Command{
	Use:   "scrape",
	Short: "Scrape events from configured sources",
	Long: `Scrape events from web sources and ingest them into the SEL server.

Supports scraping a single URL, a named source, or all enabled sources from the
source config directory.

Examples:
  # Scrape a single URL
  server scrape url https://example.com/events

  # List configured sources
  server scrape list

  # Scrape a named source (dry-run)
  server scrape source toronto-symphony-orch --dry-run

  # Scrape all enabled sources
  server scrape all

  # Discover CSS selectors for a page
  server scrape inspect https://example.com/events

  # Test CSS selectors against a live URL
  server scrape test https://example.com/events --event-list ".event-card" --name "h2"`,
}

// scrapeURLCmd scrapes a single URL.
var scrapeURLCmd = &cobra.Command{
	Use:   "url <URL>",
	Short: "Scrape events from a single URL",
	Long: `Fetch and extract JSON-LD events from the given URL, then ingest them.

Examples:
  server scrape url https://tso.ca/concerts
  server scrape url https://example.com/events --dry-run
  server scrape url https://example.com/events --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]

		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

		serverURL, apiKey, err := loadScrapeConfig()
		if err != nil {
			return err
		}

		s, cleanup, err := newScraperWithDB(serverURL, apiKey, logger)
		if err != nil {
			return err
		}
		defer cleanup()

		opts := scraper.ScrapeOptions{
			DryRun: scrapeDryRun,
			Limit:  scrapeLimit,
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		result, err := s.ScrapeURL(ctx, rawURL, opts)
		if err != nil {
			return fmt.Errorf("scrape url: %w", err)
		}

		printSingleResult(result)

		if result.Error != nil {
			return result.Error
		}
		return nil
	},
}

// scrapeListCmd lists all configured sources.
var scrapeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured scrape sources",
	Long: `List all source configurations found in the sources directory.

Examples:
  server scrape list
  server scrape list --sources configs/sources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := scrapeSourceDir

		configs, err := scraper.LoadSourceConfigs(dir)
		if err != nil {
			// Still print what we have, but note validation errors
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		if len(configs) == 0 {
			fmt.Printf("No source configs found in %s\n", dir)
			return nil
		}

		// Print table header
		fmt.Printf("%-30s %-44s %-4s %-7s %s\n", "NAME", "URL", "TIER", "ENABLED", "SCHEDULE")
		for _, cfg := range configs {
			u := cfg.URL
			if len(u) > 44 {
				u = u[:41] + "..."
			}
			fmt.Printf("%-30s %-44s %-4d %-7v %s\n",
				cfg.Name, u, cfg.Tier, cfg.Enabled, cfg.Schedule,
			)
		}

		return nil
	},
}

// scrapeSourceCmd scrapes a named configured source.
var scrapeSourceCmd = &cobra.Command{
	Use:   "source <name>",
	Short: "Scrape events from a named configured source",
	Long: `Load the named source from the sources directory and scrape it.

Examples:
  server scrape source toronto-symphony-orch
  server scrape source toronto-symphony-orch --dry-run
  server scrape source toronto-symphony-orch --limit 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceName := args[0]

		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

		serverURL, apiKey, err := loadScrapeConfig()
		if err != nil {
			return err
		}

		s, cleanup, err := newScraperWithDB(serverURL, apiKey, logger)
		if err != nil {
			return err
		}
		defer cleanup()

		opts := scraper.ScrapeOptions{
			DryRun:     scrapeDryRun,
			Limit:      scrapeLimit,
			SourcesDir: scrapeSourceDir,
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		result, err := s.ScrapeSource(ctx, sourceName, opts)
		if err != nil {
			return fmt.Errorf("scrape source: %w", err)
		}

		printSingleResult(result)

		if result.Error != nil {
			return result.Error
		}
		return nil
	},
}

// scrapeAllCmd scrapes all enabled configured sources.
var scrapeAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Scrape all enabled configured sources",
	Long: `Load all enabled sources from the sources directory and scrape each one.

Per-source errors are reported in the table but do not abort the run.
Exits with a non-zero status if any source encountered an error.

Examples:
  server scrape all
  server scrape all --dry-run
  server scrape all --limit 10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

		serverURL, apiKey, err := loadScrapeConfig()
		if err != nil {
			return err
		}

		s, cleanup, err := newScraperWithDB(serverURL, apiKey, logger)
		if err != nil {
			return err
		}
		defer cleanup()

		opts := scraper.ScrapeOptions{
			DryRun:     scrapeDryRun,
			Limit:      scrapeLimit,
			SourcesDir: scrapeSourceDir,
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		results, err := s.ScrapeAll(ctx, opts)
		if err != nil {
			return fmt.Errorf("scrape all: %w", err)
		}

		return printAllResults(results)
	},
}

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

func init() {
	rootCmd.AddCommand(scrapeCmd)

	// Subcommands
	scrapeCmd.AddCommand(scrapeURLCmd)
	scrapeCmd.AddCommand(scrapeListCmd)
	scrapeCmd.AddCommand(scrapeSourceCmd)
	scrapeCmd.AddCommand(scrapeAllCmd)
	scrapeCmd.AddCommand(scrapeInspectCmd)
	scrapeCmd.AddCommand(scrapeTestCmd)

	// Persistent flags available to all scrape subcommands
	scrapeCmd.PersistentFlags().StringVar(&scrapeServerURL, "server", "", "SEL server base URL (default: SEL_SERVER_URL or http://localhost:8080)")
	scrapeCmd.PersistentFlags().StringVar(&scrapeAPIKey, "key", "", "API key for ingest (default: SEL_API_KEY or SEL_INGEST_KEY env var)")
	scrapeCmd.PersistentFlags().BoolVar(&scrapeDryRun, "dry-run", false, "display extracted events without submitting")
	scrapeCmd.PersistentFlags().IntVar(&scrapeLimit, "limit", 0, "max events per source (0 = no limit)")
	scrapeCmd.PersistentFlags().StringVar(&scrapeSourceDir, "sources", "configs/sources", "path to sources directory")

	// Flags for `scrape test`
	scrapeTestCmd.Flags().StringVar(&scrapeTestSelectorFile, "config", "", "path to a YAML source config file to load selectors from")
	scrapeTestCmd.Flags().StringVar(&scrapeTestEventList, "event-list", "", "CSS selector for the event container element (required)")
	scrapeTestCmd.Flags().StringVar(&scrapeTestName, "name", "", "CSS selector for the event name/title")
	scrapeTestCmd.Flags().StringVar(&scrapeTestStartDate, "start-date", "", "CSS selector for the event start date")
	scrapeTestCmd.Flags().StringVar(&scrapeTestEndDate, "end-date", "", "CSS selector for the event end date")
	scrapeTestCmd.Flags().StringVar(&scrapeTestLocation, "location", "", "CSS selector for the event location")
	scrapeTestCmd.Flags().StringVar(&scrapeTestDescription, "description", "", "CSS selector for the event description")
	scrapeTestCmd.Flags().StringVar(&scrapeTestURL, "url", "", "CSS selector for the event URL link element")
	scrapeTestCmd.Flags().StringVar(&scrapeTestImage, "image", "", "CSS selector for the event image element")
	scrapeTestCmd.Flags().StringVar(&scrapeTestPagination, "pagination", "", "CSS selector for the pagination next-page link")
}

// loadScrapeConfig loads environment files and resolves server URL and API key
// from flags or environment variables.
func loadScrapeConfig() (serverURL, apiKey string, err error) {
	config.LoadEnvFile(".env")
	config.LoadEnvFile("deploy/docker/.env")

	serverURL = scrapeServerURL
	if serverURL == "" {
		serverURL = os.Getenv("SEL_SERVER_URL")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	apiKey = scrapeAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("SEL_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("SEL_INGEST_KEY")
	}

	return serverURL, apiKey, nil
}

// newScraperWithDB builds a Scraper and optionally wires in a DB connection for
// scraper_runs tracking. If DATABASE_URL is not set, tracking is skipped
// (best-effort). The returned cleanup function must be called when done.
func newScraperWithDB(serverURL, apiKey string, logger zerolog.Logger) (*scraper.Scraper, func(), error) {
	client := scraper.NewIngestClient(serverURL, apiKey)

	dbURL := getDatabaseURL()
	if dbURL == "" {
		logger.Warn().Msg("scraper: DATABASE_URL not set — scraper_runs tracking disabled")
		s := scraper.NewScraper(client, nil, logger)
		return s, func() {}, nil
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		logger.Warn().Err(err).Msg("scraper: failed to connect to DB — scraper_runs tracking disabled")
		s := scraper.NewScraper(client, nil, logger)
		return s, func() {}, nil
	}

	queries := postgres.New(pool)
	s := scraper.NewScraper(client, queries, logger)
	return s, pool.Close, nil
}

// printSingleResult prints a summary for a single scrape run. In dry-run mode
// the extracted events (available via EventsFound/EventsSubmitted counts) are
// reported; the event payloads themselves are not returned by the scraper so
// we print the counts only.
func printSingleResult(r scraper.ScrapeResult) {
	if r.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", r.Error)
		return
	}

	if scrapeDryRun {
		if r.EventsFound == 0 {
			fmt.Println("No events found")
			return
		}
		// For dry-run mode we emit a compact JSON summary of the counts.
		summary := map[string]any{
			"dry_run":   true,
			"source":    r.SourceName,
			"url":       r.SourceURL,
			"found":     r.EventsFound,
			"submitted": r.EventsSubmitted,
		}
		out, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(out))
		return
	}

	fmt.Printf("Source: %-30s  Found: %d  New: %d  Duplicate: %d  Failed: %d\n",
		r.SourceName, r.EventsFound, r.EventsCreated, r.EventsDuplicate, r.EventsFailed,
	)
}

// printAllResults prints a table of per-source results and a totals row.
// Returns an error if any source had a failure.
func printAllResults(results []scraper.ScrapeResult) error {
	if len(results) == 0 {
		fmt.Println("No sources scraped.")
		return nil
	}

	var totalFound, totalNew, totalDup, totalFailed int
	anyError := false

	// Header
	fmt.Printf("%-30s %-6s %-4s %-4s %-6s  %s\n",
		"SOURCE", "FOUND", "NEW", "DUP", "FAILED", "STATUS",
	)

	for _, r := range results {
		status := "ok"
		if r.Error != nil {
			status = fmt.Sprintf("error: %v", r.Error)
			anyError = true
		}
		fmt.Printf("%-30s %-6d %-4d %-4d %-6d  %s\n",
			r.SourceName, r.EventsFound, r.EventsCreated, r.EventsDuplicate, r.EventsFailed, status,
		)
		totalFound += r.EventsFound
		totalNew += r.EventsCreated
		totalDup += r.EventsDuplicate
		totalFailed += r.EventsFailed
	}

	// Totals row
	fmt.Printf("---\n")
	fmt.Printf("%-30s %-6d %-4d %-4d %-6d\n",
		"TOTAL", totalFound, totalNew, totalDup, totalFailed,
	)

	if anyError {
		return fmt.Errorf("one or more sources failed")
	}
	return nil
}
