package cmd

import (
	"context"
	"os"

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

func init() {
	rootCmd.AddCommand(scrapeCmd)

	// Subcommands
	scrapeCmd.AddCommand(scrapeURLCmd)
	scrapeCmd.AddCommand(scrapeListCmd)
	scrapeCmd.AddCommand(scrapeSourceCmd)
	scrapeCmd.AddCommand(scrapeAllCmd)
	scrapeCmd.AddCommand(scrapeInspectCmd)
	scrapeCmd.AddCommand(scrapeTestCmd)
	scrapeCmd.AddCommand(scrapeSyncCmd)
	scrapeCmd.AddCommand(scrapeExportCmd)

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
// scraper_runs tracking and DB-backed source configs. If DATABASE_URL is not
// set, both features are skipped (best-effort). The returned cleanup function
// must be called when done.
func newScraperWithDB(serverURL, apiKey string, logger zerolog.Logger) (*scraper.Scraper, func(), error) {
	client := scraper.NewIngestClient(serverURL, apiKey)

	dbURL := getDatabaseURL()
	if dbURL == "" {
		logger.Warn().Msg("scraper: DATABASE_URL not set — scraper_runs tracking and DB source configs disabled")
		s := scraper.NewScraper(client, nil, logger)
		return s, func() {}, nil
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		logger.Warn().Err(err).Msg("scraper: failed to connect to DB — scraper_runs tracking and DB source configs disabled")
		s := scraper.NewScraper(client, nil, logger)
		return s, func() {}, nil
	}

	queries := postgres.New(pool)
	sourceRepo := postgres.NewScraperSourceRepository(pool)
	s := scraper.NewScraperWithSourceRepo(client, queries, sourceRepo, logger)
	return s, pool.Close, nil
}
