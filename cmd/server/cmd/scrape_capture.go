package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var (
	scrapeCaptureOutput       string
	scrapeCaptureScreenshot   string
	scrapeCaptureWaitSelector string
	scrapeCaptureWaitTimeout  int
	scrapeCaptureFormat       string
	scrapeCaptureSourceFile   string
)

// scrapeCaptureCmd renders a URL via headless browser and dumps the HTML or an
// inspect-style DOM analysis. Designed to feed the generate-selectors workflow
// for JS-rendered pages where static Inspect/Tier 1 scraping is insufficient.
var scrapeCaptureCmd = &cobra.Command{
	Use:   "capture [URL]",
	Short: "Render a URL via headless browser and dump the HTML or DOM analysis",
	Long: `Launch a headless Chromium browser, navigate to the given URL, wait for a
CSS selector to appear, then write the fully rendered HTML to stdout (or a file).

Use --format inspect to run a DOM analysis on the rendered HTML — this is the
same analysis as 'server scrape inspect' but applied to the JS-rendered page.

Use --source-file to load a YAML source config and use the full headless
configuration (including iframe extraction, network-idle waits, extra headers,
and stealth mode). The URL argument is optional when --source-file is set — the
source URL is taken from the config. This is the primary tool for debugging
selector issues on iframe-embedded content: the returned HTML is exactly what
the scraper's event extractor would receive.

Requires SCRAPER_HEADLESS_ENABLED=true and Chromium to be available (set
SCRAPER_CHROME_PATH if Chromium is not in the default location).

Examples:
  # Dump rendered HTML to stdout
  server scrape capture https://example.com/events

  # Save HTML to file
  server scrape capture https://example.com/events --output rendered.html

  # Save screenshot as well
  server scrape capture https://example.com/events --screenshot screenshot.png

  # Wait for a specific element before capturing
  server scrape capture https://example.com/events --wait-selector ".event-list"

  # Run DOM inspection on the rendered page
  server scrape capture https://example.com/events --format inspect

  # Capture using full source config (iframe extraction, network idle, etc.)
  server scrape capture --source-file configs/sources/lula-lounge.yaml --format inspect

  # Capture iframe HTML and save to file for selector debugging
  server scrape capture --source-file configs/sources/lula-lounge.yaml --output /tmp/iframe.html`,
	Args: func(cmd *cobra.Command, args []string) error {
		if scrapeCaptureSourceFile != "" {
			if len(args) > 1 {
				return fmt.Errorf("accepts at most 1 arg when --source-file is set, received %d", len(args))
			}
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("accepts 1 arg (URL), received %d; or use --source-file", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate --format.
		switch scrapeCaptureFormat {
		case "html", "inspect":
			// valid
		default:
			return fmt.Errorf("invalid --format %q: must be html or inspect", scrapeCaptureFormat)
		}

		// Load env (.env, deploy/docker/.env).
		_, _, _ = loadScrapeConfig() // side-effect: loads env files

		// Check headless is enabled before attempting to construct the extractor.
		// Also checked internally by RodExtractor; early check here provides clearer UX.
		if os.Getenv("SCRAPER_HEADLESS_ENABLED") != "true" {
			fmt.Fprintln(os.Stderr, "Error: headless scraping is disabled.")
			fmt.Fprintln(os.Stderr, "Set SCRAPER_HEADLESS_ENABLED=true and ensure Chromium is available.")
			fmt.Fprintln(os.Stderr, "Optionally set SCRAPER_CHROME_PATH to specify the Chromium binary.")
			return errors.New("headless scraping disabled (SCRAPER_HEADLESS_ENABLED != true)")
		}

		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		ext := scraper.RodExtractorFromEnv(logger)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		var html string
		var captureURL string // for inspect output
		var err error

		if scrapeCaptureSourceFile != "" {
			// Source-config mode: use full headless config including iframe extraction.
			html, captureURL, err = captureWithSourceConfig(ctx, ext, logger, args)
		} else {
			// URL mode: simple render with wait selector.
			captureURL = args[0]
			html, err = captureWithURL(ctx, ext, logger, captureURL)
		}
		if err != nil {
			return err
		}

		// Save screenshot if requested.
		if scrapeCaptureScreenshot != "" {
			fmt.Fprintf(os.Stderr, "Note: --screenshot is not yet supported for successful renders "+
				"(screenshots are captured automatically on failure). "+
				"Use a browser DevTools Protocol client for on-success screenshots.\n")
		}

		return outputCapture(logger, captureURL, html)
	},
}

// captureWithURL renders a URL with the simple wait-selector approach (no source config).
func captureWithURL(ctx context.Context, ext *scraper.RodExtractor, logger zerolog.Logger, rawURL string) (string, error) {
	logger.Info().
		Str("url", rawURL).
		Str("wait_selector", scrapeCaptureWaitSelector).
		Int("wait_timeout_ms", scrapeCaptureWaitTimeout).
		Msg("capture: rendering page via headless browser")

	html, err := ext.RenderHTML(ctx, rawURL, scrapeCaptureWaitSelector, scrapeCaptureWaitTimeout)
	if err != nil {
		return "", fmt.Errorf("capture: render: %w", err)
	}
	return html, nil
}

// captureWithSourceConfig loads a source YAML and renders using the full headless
// config (iframe extraction, network-idle, headers, stealth). Returns the HTML
// that extractEventsFromHTML would receive — the exact DOM for selector debugging.
func captureWithSourceConfig(ctx context.Context, ext *scraper.RodExtractor, logger zerolog.Logger, args []string) (string, string, error) {
	config, err := scraper.LoadSourceConfig(scrapeCaptureSourceFile)
	if err != nil {
		return "", "", fmt.Errorf("capture: load source config: %w", err)
	}

	// Allow URL override from positional arg.
	if len(args) > 0 && args[0] != "" {
		config.URL = args[0]
	}

	logger.Info().
		Str("source", config.Name).
		Str("url", config.URL).
		Bool("iframe", config.Headless.Iframe != nil).
		Msg("capture: rendering with source config (full headless pipeline)")

	html, renderErr := ext.RenderHTMLWithConfig(ctx, config)
	if renderErr != nil {
		return "", config.URL, fmt.Errorf("capture: render with config: %w", renderErr)
	}
	return html, config.URL, nil
}

// outputCapture writes the captured HTML (or inspect analysis) to stdout or file.
func outputCapture(logger zerolog.Logger, captureURL, html string) error {
	switch scrapeCaptureFormat {
	case "html":
		if scrapeCaptureOutput != "" {
			if writeErr := os.WriteFile(scrapeCaptureOutput, []byte(html), 0o644); writeErr != nil {
				return fmt.Errorf("capture: write output file: %w", writeErr)
			}
			logger.Info().Str("path", scrapeCaptureOutput).Int("bytes", len(html)).Msg("capture: HTML written to file")
		} else {
			fmt.Print(html)
		}

	case "inspect":
		result, inspErr := scraper.InspectHTML(captureURL, html)
		if inspErr != nil {
			return fmt.Errorf("capture: inspect: %w", inspErr)
		}
		output := scraper.FormatInspectResultSafe(result)
		if scrapeCaptureOutput != "" {
			if writeErr := os.WriteFile(scrapeCaptureOutput, []byte(output), 0o644); writeErr != nil {
				return fmt.Errorf("capture: write output file: %w", writeErr)
			}
			logger.Info().Str("path", scrapeCaptureOutput).Msg("capture: inspect result written to file")
		} else {
			fmt.Print(output)
		}
	}

	return nil
}

func init() {
	scrapeCmd.AddCommand(scrapeCaptureCmd)

	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureOutput, "output", "", "write output to file instead of stdout")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureScreenshot, "screenshot", "", "save a PNG screenshot to file (see note: on-success screenshots not yet supported)")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureWaitSelector, "wait-selector", "body", "CSS selector to wait for before capturing (default: body)")
	scrapeCaptureCmd.Flags().IntVar(&scrapeCaptureWaitTimeout, "wait-timeout", 10000, "max wait time in milliseconds for --wait-selector")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureFormat, "format", "html", "output format: html (default) or inspect (DOM analysis)")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureSourceFile, "source-file", "", "load a YAML source config for full headless pipeline (iframe extraction, network idle, etc.)")
}
