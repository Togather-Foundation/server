package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var scrapeHeadless bool
var scrapeSourceFile string

// scrapeURLCmd scrapes a single URL.
var scrapeURLCmd = &cobra.Command{
	Use:   "url <URL>",
	Short: "Scrape events from a single URL",
	Long: `Fetch and extract JSON-LD events from the given URL, then ingest them.

Use --headless to scrape JS-rendered pages via the Tier 2 headless browser path.
Requires SCRAPER_HEADLESS_ENABLED=true and Chromium to be available.

Examples:
  server scrape url https://tso.ca/concerts
  server scrape url https://example.com/events --dry-run
  server scrape url https://example.com/events --limit 10
  server scrape url https://example.com/events --headless`,
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
			DryRun:           scrapeDryRun,
			Verbose:          scrapeVerbose,
			Limit:            scrapeLimit,
			Transport:        buildScrapeTransport(cmd, logger),
			HeadlessOverride: scrapeHeadless,
		}

		if scrapeHeadless {
			logger.Info().Str("url", rawURL).Msg("scraper: --headless flag set; using Tier 2 headless browser path (WaitSelector=body)")
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
	Long: `List all source configurations. Tries the DB first; falls back to YAML files.

Examples:
  server scrape list
  server scrape list --sources configs/sources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		dir := scrapeSourceDir

		// Try DB first.
		var configs []scraper.SourceConfig
		dbURL := getDatabaseURL()
		if dbURL != "" {
			pool, poolErr := pgxpool.New(ctx, dbURL)
			if poolErr == nil {
				defer pool.Close()
				repo := postgres.NewScraperSourceRepository(pool)
				sources, listErr := repo.List(ctx, nil) // all, not just enabled
				if listErr == nil && len(sources) > 0 {
					for _, src := range sources {
						cfg, convErr := scraper.SourceConfigFromDomain(src)
						if convErr != nil {
							fmt.Fprintf(os.Stderr, "Warning: skipping %q: %v\n", src.Name, convErr)
							continue
						}
						configs = append(configs, cfg)
					}
				}
			}
		}

		// Fall back to YAML if DB yielded nothing.
		if len(configs) == 0 {
			var err error
			configs, err = scraper.LoadSourceConfigs(dir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		}

		if len(configs) == 0 {
			fmt.Printf("No source configs found (DB empty and no YAML files in %s)\n", dir)
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
			if !cfg.Enabled && cfg.Notes != "" {
				fmt.Printf("  # %s\n", cfg.Notes)
			}
		}

		return nil
	},
}

// scrapeSourceCmd scrapes a named configured source.
var scrapeSourceCmd = &cobra.Command{
	Use:   "source [name]",
	Short: "Scrape events from a named configured source",
	Long: `Load the named source from the sources directory and scrape it.

If the source config has tier: 2, headless browser scraping is used automatically
(requires SCRAPER_HEADLESS_ENABLED=true). The --headless flag forces Tier 2 mode
even for sources not configured with tier: 2.

Use --source-file to load a YAML config directly from a file path instead of
the sources directory. This bypasses the DB and directory lookup, and runs the
source regardless of its enabled flag — useful for testing draft or disabled
configs. When --source-file is given the [name] argument is optional (the name
is taken from the file's name field).

Examples:
  server scrape source toronto-symphony-orch
  server scrape source toronto-symphony-orch --dry-run
  server scrape source toronto-symphony-orch --limit 5
  server scrape source mysite --headless
  server scrape source --source-file /tmp/my-draft.yaml --dry-run`,
	Args: func(cmd *cobra.Command, args []string) error {
		if scrapeSourceFile != "" {
			// --source-file: name arg is optional
			if len(args) > 1 {
				return fmt.Errorf("accepts at most 1 arg when --source-file is set, received %d", len(args))
			}
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("accepts 1 arg (source name), received %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceName := ""
		if len(args) > 0 {
			sourceName = args[0]
		}

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
			DryRun:           scrapeDryRun,
			Verbose:          scrapeVerbose,
			Limit:            scrapeLimit,
			SourcesDir:       scrapeSourceDir,
			SourceFile:       scrapeSourceFile,
			Transport:        buildScrapeTransport(cmd, logger),
			HeadlessOverride: scrapeHeadless,
		}

		if scrapeHeadless {
			logger.Info().Str("source", sourceName).Msg("scraper: --headless flag set; will override source tier to 2 if not already")
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
  server scrape all --limit 10
  server scrape all --tier 0
  server scrape all --tier 1`,
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
			TierFilter: scrapeTier,
			Transport:  buildScrapeTransport(cmd, logger),
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

// printSingleResult prints a summary for a single scrape run. In dry-run mode
// the extracted events (available via EventsFound/EventsSubmitted counts) are
// reported; when --verbose is set, individual event details are also printed.
// Quality warnings are always shown when present (regardless of --verbose).
func printSingleResult(r scraper.ScrapeResult) {
	if r.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", r.Error)
		return
	}

	if r.DryRun {
		if r.EventsFound == 0 {
			fmt.Println("No events found")
			return
		}
		fmt.Printf("[dry-run] source=%-28s  found=%-4d  would-submit=%d\n",
			r.SourceName, r.EventsFound, r.EventsSubmitted)

		// Print individual events when --verbose is active.
		if scrapeVerbose && len(r.DryRunEvents) > 0 {
			fmt.Println()
			for i, evt := range r.DryRunEvents {
				fmt.Printf("  %2d. %s\n", i+1, evt.Name)
				if evt.StartDate != "" {
					fmt.Printf("      start: %s\n", evt.StartDate)
				}
				if evt.EndDate != "" {
					fmt.Printf("      end:   %s\n", evt.EndDate)
				}
				if evt.URL != "" {
					fmt.Printf("      url:   %s\n", evt.URL)
				}
				if evt.Location != nil && evt.Location.Name != "" {
					fmt.Printf("      venue: %s\n", evt.Location.Name)
				}
			}
			fmt.Println()
		}

		printQualityWarnings(r)
		return
	}

	fmt.Printf("Source: %-30s  Found: %d  New: %d  Duplicate: %d  Failed: %d\n",
		r.SourceName, r.EventsFound, r.EventsCreated, r.EventsDuplicate, r.EventsFailed,
	)

	printQualityWarnings(r)
}

// printQualityWarnings prints quality warnings from the scrape result.
// Warnings are always shown when present — they indicate potential data
// quality issues that operators should investigate.
func printQualityWarnings(r scraper.ScrapeResult) {
	if len(r.QualityWarnings) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\nQuality warnings (%d):\n", len(r.QualityWarnings))
	for _, w := range r.QualityWarnings {
		fmt.Fprintf(os.Stderr, "  WARNING: %s\n", w)
	}
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

// buildScrapeTransport reads --cache, --refresh, and --cache-dir flags from cmd
// and constructs a CachingTransport if caching is enabled. Returns nil when
// --cache is not set (callers fall back to http.DefaultTransport).
func buildScrapeTransport(cmd *cobra.Command, logger zerolog.Logger) http.RoundTripper {
	cacheEnabled, _ := cmd.Flags().GetBool("cache")
	if !cacheEnabled {
		return nil
	}
	refresh, _ := cmd.Flags().GetBool("refresh")
	cacheDir, _ := cmd.Flags().GetString("cache-dir")
	logger.Info().Str("cache_dir", cacheDir).Bool("refresh", refresh).Msg("scrape cache enabled")
	return scraper.NewCachingTransport(http.DefaultTransport, cacheDir, refresh)
}
