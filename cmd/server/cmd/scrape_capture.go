package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
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
	scrapeCaptureNetwork      bool
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

Use --network to capture all HTTP requests/responses made during rendering.
This shows XHR/fetch API calls the page makes, helping diagnose why pages
render empty. With --format json, outputs the full network request list as JSON.

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
  server scrape capture --source-file configs/sources/lula-lounge.yaml --output /tmp/iframe.html

  # Show network activity (XHR/fetch API calls) during rendering
  server scrape capture https://example.com/events --network

  # Show network activity as JSON (for machine consumption)
  server scrape capture https://example.com/events --network --format json`,
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
		case "html", "inspect", "json":
			// valid
		default:
			return fmt.Errorf("invalid --format %q: must be html, inspect, or json", scrapeCaptureFormat)
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
		var networkRequests []scraper.NetworkRequest
		var err error

		if scrapeCaptureSourceFile != "" {
			// Source-config mode: use full headless config including iframe extraction.
			html, captureURL, networkRequests, err = captureWithSourceConfig(ctx, ext, logger, args)
		} else {
			// URL mode: simple render with wait selector.
			captureURL = args[0]
			html, networkRequests, err = captureWithURL(ctx, ext, logger, captureURL)
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

		return outputCapture(logger, captureURL, html, networkRequests)
	},
}

// captureWithURL renders a URL with the simple wait-selector approach (no source config).
// When scrapeCaptureNetwork is true, uses RenderHTMLWithNetwork to capture network activity.
func captureWithURL(ctx context.Context, ext *scraper.RodExtractor, logger zerolog.Logger, rawURL string) (string, []scraper.NetworkRequest, error) {
	logger.Info().
		Str("url", rawURL).
		Str("wait_selector", scrapeCaptureWaitSelector).
		Int("wait_timeout_ms", scrapeCaptureWaitTimeout).
		Bool("network", scrapeCaptureNetwork).
		Msg("capture: rendering page via headless browser")

	if scrapeCaptureNetwork {
		html, requests, err := ext.RenderHTMLWithNetwork(ctx, rawURL, scrapeCaptureWaitSelector, scrapeCaptureWaitTimeout)
		if err != nil {
			return "", nil, fmt.Errorf("capture: render: %w", err)
		}
		return html, requests, nil
	}

	html, err := ext.RenderHTML(ctx, rawURL, scrapeCaptureWaitSelector, scrapeCaptureWaitTimeout)
	if err != nil {
		return "", nil, fmt.Errorf("capture: render: %w", err)
	}
	return html, nil, nil
}

// captureWithSourceConfig loads a source YAML and renders using the full headless
// config (iframe extraction, network-idle, headers, stealth). Returns the HTML
// that extractEventsFromHTML would receive — the exact DOM for selector debugging.
// When scrapeCaptureNetwork is true, also captures network activity.
func captureWithSourceConfig(ctx context.Context, ext *scraper.RodExtractor, logger zerolog.Logger, args []string) (string, string, []scraper.NetworkRequest, error) {
	config, err := scraper.LoadSourceConfig(scrapeCaptureSourceFile)
	if err != nil {
		return "", "", nil, fmt.Errorf("capture: load source config: %w", err)
	}

	// Allow URL override from positional arg.
	if len(args) > 0 && args[0] != "" {
		config.URL = args[0]
	}

	logger.Info().
		Str("source", config.Name).
		Str("url", config.URL).
		Bool("iframe", config.Headless.Iframe != nil).
		Bool("network", scrapeCaptureNetwork).
		Msg("capture: rendering with source config (full headless pipeline)")

	if scrapeCaptureNetwork {
		html, requests, renderErr := ext.RenderHTMLWithConfigAndNetwork(ctx, config)
		if renderErr != nil {
			return "", config.URL, nil, fmt.Errorf("capture: render with config: %w", renderErr)
		}
		return html, config.URL, requests, nil
	}

	html, renderErr := ext.RenderHTMLWithConfig(ctx, config)
	if renderErr != nil {
		return "", config.URL, nil, fmt.Errorf("capture: render with config: %w", renderErr)
	}
	return html, config.URL, nil, nil
}

// outputCapture writes the captured HTML (or inspect analysis) to stdout or file,
// and when network requests are provided (--network flag), appends a summary.
func outputCapture(logger zerolog.Logger, captureURL string, html string, requests []scraper.NetworkRequest) error {
	// When --format json and --network, output just the network requests as JSON.
	if scrapeCaptureFormat == "json" && scrapeCaptureNetwork {
		if requests == nil {
			requests = []scraper.NetworkRequest{}
		}
		data, jsonErr := json.MarshalIndent(requests, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf("capture: marshal network requests: %w", jsonErr)
		}
		if scrapeCaptureOutput != "" {
			if writeErr := os.WriteFile(scrapeCaptureOutput, data, 0o644); writeErr != nil {
				return fmt.Errorf("capture: write output file: %w", writeErr)
			}
			logger.Info().Str("path", scrapeCaptureOutput).Int("requests", len(requests)).Msg("capture: network requests written to file")
		} else {
			fmt.Println(string(data))
		}
		return nil
	}

	switch scrapeCaptureFormat {
	case "html", "json": // json without --network falls back to html output
		output := html
		if scrapeCaptureNetwork && len(requests) > 0 {
			output += "\n" + formatNetworkSummary(requests)
		}
		if scrapeCaptureOutput != "" {
			if writeErr := os.WriteFile(scrapeCaptureOutput, []byte(output), 0o644); writeErr != nil {
				return fmt.Errorf("capture: write output file: %w", writeErr)
			}
			logger.Info().Str("path", scrapeCaptureOutput).Int("bytes", len(html)).Msg("capture: HTML written to file")
			if scrapeCaptureNetwork {
				fmt.Print(formatNetworkSummary(requests))
			}
		} else {
			fmt.Print(output)
		}

	case "inspect":
		result, inspErr := scraper.InspectHTML(captureURL, html)
		if inspErr != nil {
			return fmt.Errorf("capture: inspect: %w", inspErr)
		}
		inspOutput := scraper.FormatInspectResultSafe(result)
		if scrapeCaptureNetwork && len(requests) > 0 {
			inspOutput += "\n" + formatNetworkSummary(requests)
		}
		if scrapeCaptureOutput != "" {
			if writeErr := os.WriteFile(scrapeCaptureOutput, []byte(inspOutput), 0o644); writeErr != nil {
				return fmt.Errorf("capture: write output file: %w", writeErr)
			}
			logger.Info().Str("path", scrapeCaptureOutput).Msg("capture: inspect result written to file")
			if scrapeCaptureNetwork {
				fmt.Print(formatNetworkSummary(requests))
			}
		} else {
			fmt.Print(inspOutput)
		}
	}

	return nil
}

// formatNetworkSummary formats a human-readable summary of captured network requests.
// API calls (XHR/Fetch + JSON) are listed prominently; other request types are summarised.
func formatNetworkSummary(requests []scraper.NetworkRequest) string {
	var sb strings.Builder
	sb.WriteString("\n--- Network Activity ---\n")

	// Separate API calls from other requests.
	var apiCalls []scraper.NetworkRequest
	other := make(map[string]int) // resource_type → count
	for _, r := range requests {
		if r.IsAPI {
			apiCalls = append(apiCalls, r)
		} else {
			other[r.ResourceType]++
		}
	}

	// Sort API calls by URL for deterministic output.
	sort.Slice(apiCalls, func(i, j int) bool {
		return apiCalls[i].URL < apiCalls[j].URL
	})

	if len(apiCalls) > 0 {
		sb.WriteString("API Calls (XHR/Fetch + JSON):\n")
		for _, r := range apiCalls {
			size := ""
			if r.BodySize > 0 {
				size = fmt.Sprintf(" (%.1f KB", float64(r.BodySize)/1024)
				if r.TimingMs > 0 {
					size += fmt.Sprintf(", %dms", r.TimingMs)
				}
				size += ")"
			} else if r.TimingMs > 0 {
				size = fmt.Sprintf(" (%dms)", r.TimingMs)
			}
			status := ""
			if r.Status > 0 {
				status = fmt.Sprintf(" → %d", r.Status)
			}
			sb.WriteString(fmt.Sprintf("  %-4s %s%s %s%s\n",
				r.Method, r.URL, status, r.ContentType, size))
		}
	} else {
		sb.WriteString("API Calls (XHR/Fetch + JSON): none\n")
	}

	otherTotal := 0
	for _, c := range other {
		otherTotal += c
	}
	if otherTotal > 0 {
		// Sort type names for deterministic output.
		types := make([]string, 0, len(other))
		for t := range other {
			types = append(types, t)
		}
		sort.Strings(types)

		parts := make([]string, 0, len(types))
		for _, t := range types {
			if t == "" {
				t = "Unknown"
			}
			parts = append(parts, fmt.Sprintf("%s: %d", t, other[t]))
		}
		sb.WriteString(fmt.Sprintf("\nOther Requests (%d total):\n  %s\n", otherTotal, strings.Join(parts, ", ")))
	}

	return sb.String()
}

func init() {
	scrapeCmd.AddCommand(scrapeCaptureCmd)

	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureOutput, "output", "", "write output to file instead of stdout")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureScreenshot, "screenshot", "", "save a PNG screenshot to file (see note: on-success screenshots not yet supported)")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureWaitSelector, "wait-selector", "body", "CSS selector to wait for before capturing (default: body)")
	scrapeCaptureCmd.Flags().IntVar(&scrapeCaptureWaitTimeout, "wait-timeout", 10000, "max wait time in milliseconds for --wait-selector")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureFormat, "format", "html", "output format: html (default), inspect (DOM analysis), or json (network requests as JSON, requires --network)")
	scrapeCaptureCmd.Flags().StringVar(&scrapeCaptureSourceFile, "source-file", "", "load a YAML source config for full headless pipeline (iframe extraction, network idle, etc.)")
	scrapeCaptureCmd.Flags().BoolVar(&scrapeCaptureNetwork, "network", false, "capture and display network activity (XHR/fetch API calls made during rendering)")
}
