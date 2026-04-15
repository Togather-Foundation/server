package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Togather-Foundation/server/internal/scraper"
)

var (
	fixtureExtractionMethod string
	fixtureTier             int
	fixtureTrustLevel       int
	fixtureSourceName       string
)

var scrapeFixtureCmd = &cobra.Command{
	Use:   "test-fixture <fixture-path>",
	Short: "Serve a local fixture file and scrape it in one shot",
	Long: `Start an ephemeral HTTP server, serve the given fixture file, generate
a matching source config, and run the scrape pipeline — all in one command.

The extraction method and tier are auto-detected from the file extension:
  .ics → extraction_method: ics, tier: 0
  .html → tier: 1 (CSS selectors must be provided via --selectors flags)
  .json / .jsonld → tier: 0 (JSON-LD auto-detection)

Override auto-detection with --extraction-method and --tier flags.

Examples:
  # Scrape an ICS fixture (auto-detected)
  server scrape test-fixture tests/testdata/ics/interop-recurrence-exdate.ics

  # Dry-run to inspect without ingesting
  server scrape test-fixture tests/testdata/ics/interop-recurrence-exdate.ics --dry-run

  # Override extraction method
  server scrape test-fixture tests/testdata/ics/interop-recurrence-exdate.ics --extraction-method ics --tier 3

  # JSON-LD fixture
  server scrape test-fixture /tmp/events.jsonld`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fixturePath := args[0]

		if _, err := os.Stat(fixturePath); err != nil {
			return fmt.Errorf("fixture file: %w", err)
		}

		absPath, err := filepath.Abs(fixturePath)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(absPath))
		extractionMethod := fixtureExtractionMethod
		tier := fixtureTier

		if extractionMethod == "" {
			switch ext {
			case ".ics":
				extractionMethod = "ics"
			case ".html", ".htm":
				extractionMethod = ""
			default:
				extractionMethod = ""
			}
		}

		if tier == -1 {
			switch ext {
			case ".ics":
				tier = 0
			case ".html", ".htm":
				tier = 1
			default:
				tier = 0
			}
		}

		name := fixtureSourceName
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
		}

		dir := filepath.Dir(absPath)
		filename := filepath.Base(absPath)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("start listener: %w", err)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			cleanPath := strings.TrimPrefix(r.URL.Path, "/")
			servePath := filepath.Join(dir, cleanPath)
			if _, statErr := os.Stat(servePath); statErr != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, servePath)
		})

		httpServer := &http.Server{Handler: mux}
		go func() { _ = httpServer.Serve(listener) }()

		baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
		fixtureURL := baseURL + "/" + filename

		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		logger.Info().Str("url", fixtureURL).Str("fixture", absPath).Msg("fixture server started")

		defer func() {
			_ = httpServer.Shutdown(context.Background())
			logger.Info().Msg("fixture server stopped")
		}()

		cfg := scraper.SourceConfig{
			Name:             name,
			URL:              fixtureURL,
			Tier:             tier,
			TrustLevel:       fixtureTrustLevel,
			Enabled:          true,
			ExtractionMethod: extractionMethod,
			Schedule:         "manual",
			MaxPages:         1,
		}

		cfgBytes, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal source config: %w", err)
		}

		tmpDir, err := os.MkdirTemp("", "togather-fixture-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		cfgPath := filepath.Join(tmpDir, "source.yaml")
		if err := os.WriteFile(cfgPath, cfgBytes, 0644); err != nil {
			return fmt.Errorf("write source config: %w", err)
		}

		logger.Info().Str("config", cfgPath).Str("extraction_method", extractionMethod).Int("tier", tier).Msg("generated source config")

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
			SourceFile:       cfgPath,
			Transport:        buildScrapeTransport(cmd, logger),
			HeadlessOverride: scrapeHeadless,
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		result, err := s.ScrapeSource(ctx, name, opts)
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
