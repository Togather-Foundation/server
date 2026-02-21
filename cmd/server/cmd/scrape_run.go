package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

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
		fmt.Printf("[dry-run] source=%-28s  found=%-4d  would-submit=%d\n",
			r.SourceName, r.EventsFound, r.EventsSubmitted)
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
