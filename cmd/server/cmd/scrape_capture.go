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
)

// scrapeCaptureCmd renders a URL via headless browser and dumps the HTML or an
// inspect-style DOM analysis. Designed to feed the generate-selectors workflow
// for JS-rendered pages where static Inspect/Tier 1 scraping is insufficient.
var scrapeCaptureCmd = &cobra.Command{
	Use:   "capture <URL>",
	Short: "Render a URL via headless browser and dump the HTML or DOM analysis",
	Long: `Launch a headless Chromium browser, navigate to the given URL, wait for a
CSS selector to appear, then write the fully rendered HTML to stdout (or a file).

Use --format inspect to run a DOM analysis on the rendered HTML — this is the
same analysis as 'server scrape inspect' but applied to the JS-rendered page.

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
  server scrape capture https://example.com/events --format inspect`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]

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

		logger.Info().
			Str("url", rawURL).
			Str("wait_selector", scrapeCaptureWaitSelector).
			Int("wait_timeout_ms", scrapeCaptureWaitTimeout).
			Msg("capture: rendering page via headless browser")

		html, err := ext.RenderHTML(ctx, rawURL, scrapeCaptureWaitSelector, scrapeCaptureWaitTimeout)
		if err != nil {
			return fmt.Errorf("capture: render: %w", err)
		}

		// Save screenshot if requested (requires a separate render; screenshot is
		// captured as a side-effect of RenderHTML on error only, so we use the
		// captured HTML to indicate success and skip an extra browser launch).
		// NOTE: taking a screenshot on success would require a second browser launch
		// or exposing a different API. For now we document this limitation.
		if scrapeCaptureScreenshot != "" {
			fmt.Fprintf(os.Stderr, "Note: --screenshot is not yet supported for successful renders "+
				"(screenshots are captured automatically on failure). "+
				"Use a browser DevTools Protocol client for on-success screenshots.\n")
		}

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
			result, inspErr := scraper.InspectHTML(rawURL, html)
			if inspErr != nil {
				return fmt.Errorf("capture: inspect: %w", inspErr)
			}
			output := scraper.FormatInspectResult(result)
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
	},
}

func init() {
	scrapeCmd.AddCommand(scrapeCaptureCmd)

	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureOutput, "output", "", "write output to file instead of stdout")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureScreenshot, "screenshot", "", "save a PNG screenshot to file (see note: on-success screenshots not yet supported)")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureWaitSelector, "wait-selector", "body", "CSS selector to wait for before capturing (default: body)")
	scrapeCaptureCmd.Flags().IntVar(&scrapeCaptureWaitTimeout, "wait-timeout", 10000, "max wait time in milliseconds for --wait-selector")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureFormat, "format", "html", "output format: html (default) or inspect (DOM analysis)")
}
