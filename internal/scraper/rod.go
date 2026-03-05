package scraper

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

const (
	rodDefaultTimeout     = 30 * time.Second
	rodDefaultWaitTimeout = 10 * time.Second
	rodDefaultRateLimit   = time.Second
	rodUserAgent          = scraperUserAgent // reuse from jsonld.go
)

// ErrHeadlessDisabled is returned when SCRAPER_HEADLESS_ENABLED is false.
var ErrHeadlessDisabled = fmt.Errorf("headless scraping is disabled (set SCRAPER_HEADLESS_ENABLED=true)")

// RodExtractor performs Tier 2 JS-rendered scraping using go-rod (Chromium).
type RodExtractor struct {
	logger      zerolog.Logger
	maxConc     int           // max concurrent browser sessions
	sem         chan struct{} // semaphore
	chromePath  string        // override SCRAPER_CHROME_PATH; "" = download-on-demand
	headlessEnv bool          // mirrors SCRAPER_HEADLESS_ENABLED env var
}

// NewRodExtractor returns a RodExtractor with the given max concurrency.
// chromePath overrides the Chromium binary path; "" = Rod download-on-demand.
// headlessEnabled must be true for ScrapeWithBrowser to proceed; when false,
// all calls return ErrHeadlessDisabled.
func NewRodExtractor(logger zerolog.Logger, maxConc int, chromePath string, headlessEnabled bool) *RodExtractor {
	if maxConc <= 0 {
		maxConc = 2
	}
	return &RodExtractor{
		logger:      logger,
		maxConc:     maxConc,
		sem:         make(chan struct{}, maxConc),
		chromePath:  chromePath,
		headlessEnv: headlessEnabled,
	}
}

// RodExtractorFromEnv reads SCRAPER_HEADLESS_ENABLED and SCRAPER_CHROME_PATH
// env vars and constructs a default RodExtractor (maxConc=2).
func RodExtractorFromEnv(logger zerolog.Logger) *RodExtractor {
	headlessEnabled := os.Getenv("SCRAPER_HEADLESS_ENABLED") == "true"
	chromePath := os.Getenv("SCRAPER_CHROME_PATH")
	return NewRodExtractor(logger, 2, chromePath, headlessEnabled)
}

// ScrapeWithBrowser renders config.URL in a headless Chromium browser,
// waits for config.Headless.WaitSelector (defaulting to "body"), extracts
// events using config.Selectors (Colly-compatible CSS selectors applied
// against the rendered DOM), and handles JS-pagination via PaginationBtn.
//
// Returns RawEvents suitable for the existing NormalizeRawEvent pipeline.
func (e *RodExtractor) ScrapeWithBrowser(ctx context.Context, config SourceConfig) ([]RawEvent, error) {
	if !e.headlessEnv {
		return nil, ErrHeadlessDisabled
	}

	// Robots.txt check — reuse existing helper.
	// Also checked internally by RodExtractor; early check here provides clearer UX.
	robotsClient := robotsClientFrom(&http.Client{Timeout: fetchTimeout})
	allowed, robotsErr := RobotsAllowed(ctx, config.URL, scraperUserAgent, robotsClient)
	if robotsErr != nil {
		e.logger.Warn().Err(robotsErr).Str("url", config.URL).Msg("rod: robots.txt check failed, proceeding as allowed")
	} else if !allowed {
		return nil, fmt.Errorf("rod: scraping disallowed by robots.txt for %q", config.URL)
	}

	// Acquire semaphore.
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return e.scrapePages(ctx, config)
}

// launchBrowser builds a Rod launcher, launches the browser, and returns the
// connected *rod.Browser with ctx bound (so cancellation is respected) and a
// cleanup func that must be deferred by the caller. The browser's context is
// set to ctx so all page operations inherit the same deadline/cancellation.
//
// Design note — one browser process per scrape call (no persistent pool):
// Each ScrapeWithBrowser / RenderHTML call launches a fresh Chromium process
// and closes it when done. This is intentional: it provides strong isolation
// between scrape operations (no shared cookies, cache, or session state), avoids
// the need for browser-level cleanup or crash-recovery logic, and keeps memory
// bounded (a crashed browser never affects subsequent calls). The concurrency
// semaphore (maxConc) limits simultaneous launches, so the startup cost (~300ms)
// is acceptable for background scraping where throughput matters more than
// per-call latency. If cold-start latency becomes a bottleneck (e.g. interactive
// use, high-frequency scheduled scrapes), replace with a long-lived browser pool
// and open a new page per call instead.
func (e *RodExtractor) launchBrowser(ctx context.Context) (*rod.Browser, func(), error) {
	l := launcher.New().
		Headless(true).
		Set("no-sandbox", "").
		Set("disable-dev-shm-usage", "")

	if e.chromePath != "" {
		l = l.Bin(e.chromePath)
	}

	// Use a temp dir for user data to avoid conflicts between concurrent runs.
	tmpDir, err := os.MkdirTemp("", "rod-userdata-*")
	if err != nil {
		// non-fatal: proceed without a dedicated user-data dir
		e.logger.Debug().Err(err).Msg("rod: failed to create temp user-data dir, proceeding without it")
	} else {
		l = l.UserDataDir(tmpDir)
	}

	u, launchErr := l.Launch()
	if launchErr != nil {
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
		return nil, nil, fmt.Errorf("rod: failed to launch browser: %w", launchErr)
	}

	browser := rod.New().ControlURL(u)
	if connectErr := browser.Connect(); connectErr != nil {
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
		return nil, nil, fmt.Errorf("rod: failed to connect to browser: %w", connectErr)
	}

	// Bind the caller's context so cancellations and timeouts are respected.
	browser = browser.Context(ctx)

	cleanup := func() {
		_ = browser.Close()
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	}

	return browser, cleanup, nil
}

// scrapePages performs the actual browser-based scraping across potentially
// multiple pages. Called after the semaphore is acquired.
func (e *RodExtractor) scrapePages(ctx context.Context, config SourceConfig) ([]RawEvent, error) {
	// Apply a hard timeout for the entire scrape operation.
	ctx, cancel := context.WithTimeout(ctx, rodDefaultTimeout)
	defer cancel()

	browser, cleanup, err := e.launchBrowser(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Resolve settings with defaults.
	waitSelector := config.Headless.WaitSelector
	if waitSelector == "" {
		waitSelector = "body"
	}

	waitTimeout := rodDefaultWaitTimeout
	if config.Headless.WaitTimeoutMs > 0 {
		waitTimeout = time.Duration(config.Headless.WaitTimeoutMs) * time.Millisecond
	}

	rateLimit := rodDefaultRateLimit
	if config.Headless.RateLimitMs > 0 {
		rateLimit = time.Duration(config.Headless.RateLimitMs) * time.Millisecond
	}

	maxPages := config.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}

	pageURL := config.URL
	var events []RawEvent

	for pageNum := 0; pageNum < maxPages; pageNum++ {
		// Rate limiting: delay between page loads (skip on first page).
		if pageNum > 0 {
			select {
			case <-ctx.Done():
				return events, nil
			case <-time.After(rateLimit):
			}
		}

		pageEvents, nextURL, pageErr := e.scrapeSinglePage(ctx, browser, config, pageURL, waitSelector, waitTimeout)
		if pageErr != nil {
			e.logger.Warn().
				Err(pageErr).
				Str("source", config.Name).
				Str("url", pageURL).
				Int("page", pageNum+1).
				Msg("rod: error scraping page, stopping pagination")
			// Propagate the error so callers can distinguish failures from zero-result scrapes.
			return events, pageErr
		}

		events = append(events, pageEvents...)
		e.logger.Debug().
			Str("source", config.Name).
			Str("url", pageURL).
			Int("page", pageNum+1).
			Int("events", len(pageEvents)).
			Msg("rod: scraped page")

		// Pagination: only continue if we have a pagination button config and a next URL.
		if nextURL == "" {
			break
		}
		pageURL = nextURL
	}

	return events, nil
}

// scrapeSinglePage opens a page, navigates to pageURL, waits for waitSelector,
// extracts events using CSS selectors, and optionally clicks a pagination button.
// Returns the events found, the next page URL (if pagination clicked), and any error.
func (e *RodExtractor) scrapeSinglePage(
	ctx context.Context,
	browser *rod.Browser,
	config SourceConfig,
	pageURL string,
	waitSelector string,
	waitTimeout time.Duration,
) (events []RawEvent, nextURL string, retErr error) {
	// Validate the navigation URL to prevent SSRF via non-http(s) schemes.
	if err := validateNavigationURL(pageURL); err != nil {
		return nil, "", err
	}

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, "", fmt.Errorf("rod: failed to open new page: %w", err)
	}
	defer func() {
		if closeErr := page.Close(); closeErr != nil {
			e.logger.Debug().Err(closeErr).Msg("rod: page close error")
		}
	}()

	// Capture screenshot on failure for debugging.
	defer func() {
		if retErr != nil {
			e.captureScreenshot(page, config.Name)
		}
	}()

	// Set page timeout.
	page = page.Timeout(rodDefaultTimeout)

	// Override the browser's default user-agent with the scraper UA.
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: rodUserAgent,
	}); err != nil {
		e.logger.Warn().Err(err).Str("source", config.Name).Msg("rod: failed to set user agent")
	}

	// Apply extra headers if configured.
	if len(config.Headless.Headers) > 0 {
		headers := make([]string, 0, len(config.Headless.Headers)*2)
		for k, v := range config.Headless.Headers {
			headers = append(headers, k, v)
		}
		if _, err := page.SetExtraHeaders(headers); err != nil {
			e.logger.Warn().Err(err).Str("source", config.Name).Msg("rod: failed to set extra headers")
		}
	}

	// Navigate to the page.
	if err := page.Navigate(pageURL); err != nil {
		return nil, "", fmt.Errorf("rod: navigate to %q: %w", pageURL, err)
	}

	// Wait for the target selector to appear.
	waitErr := page.Timeout(waitTimeout).WaitElementsMoreThan(waitSelector, 0)
	if waitErr != nil {
		// Soft failure: log and attempt extraction anyway (selector may still be absent
		// for sites that progressively render content).
		e.logger.Warn().
			Err(waitErr).
			Str("source", config.Name).
			Str("selector", waitSelector).
			Msg("rod: wait selector timed out, attempting extraction anyway")
	}

	// Extract rendered HTML.
	html, err := page.HTML()
	if err != nil {
		return nil, "", fmt.Errorf("rod: getting HTML from %q: %w", pageURL, err)
	}

	// Extract events from rendered HTML using goquery + selectors.
	pageEvents, extractErr := extractEventsFromHTML(html, config, pageURL)
	if extractErr != nil {
		e.logger.Warn().Err(extractErr).Str("source", config.Name).Msg("rod: event extraction error")
	}

	// Handle JS pagination: click a "next page" button if configured.
	if config.Headless.PaginationBtn != "" {
		btnEl, findErr := page.Timeout(3 * time.Second).Element(config.Headless.PaginationBtn)
		if findErr == nil && btnEl != nil {
			// Try to read href before clicking (for <a> elements).
			href, _ := btnEl.Attribute("href")

			if clickErr := btnEl.Click(proto.InputMouseButtonLeft, 1); clickErr != nil {
				e.logger.Debug().Err(clickErr).Str("source", config.Name).Msg("rod: pagination button click failed")
			} else {
				// Resolve the next URL.
				if href != nil && *href != "" && !strings.HasPrefix(*href, "javascript:") {
					nextURL = resolveURL(pageURL, *href)
				} else {
					// Button click may trigger navigation — get current URL.
					if curURL, urlErr := page.Eval(`() => window.location.href`); urlErr == nil {
						candidate := curURL.Value.String()
						if candidate != pageURL {
							nextURL = candidate
						}
					}
				}
			}
		}
	}

	return pageEvents, nextURL, nil
}

// captureScreenshot saves a PNG screenshot to the OS temp dir for debugging.
func (e *RodExtractor) captureScreenshot(page *rod.Page, sourceName string) {
	ts := time.Now().Unix()
	path := fmt.Sprintf("%s/rod-screenshot-%s-%d.png", os.TempDir(), sanitizeName(sourceName), ts)
	if data, err := page.Screenshot(false, nil); err == nil {
		if writeErr := os.WriteFile(path, data, 0o644); writeErr == nil {
			e.logger.Info().Str("path", path).Msg("rod: failure screenshot saved")
		}
	}
}

// extractEventsFromHTML parses an HTML string with goquery and applies the
// CSS selectors from config to collect RawEvents. pageURL is used to resolve
// relative URLs (href/src attributes). Returns an error if config.Selectors.EventList
// is empty — callers should ensure the config is validated before calling.
func extractEventsFromHTML(html string, config SourceConfig, pageURL string) ([]RawEvent, error) {
	if config.Selectors.EventList == "" {
		return nil, fmt.Errorf("rod: extractEventsFromHTML: selectors.event_list is required but empty (source %q)", config.Name)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("rod: parsing HTML: %w", err)
	}

	var events []RawEvent

	doc.Find(config.Selectors.EventList).Each(func(_ int, s *goquery.Selection) {
		raw := RawEvent{}

		if config.Selectors.Name != "" {
			raw.Name = strings.TrimSpace(s.Find(config.Selectors.Name).First().Text())
		}

		if config.Selectors.StartDate != "" {
			raw.StartDate = extractDateFromSelection(s, config.Selectors.StartDate)
		}

		if config.Selectors.EndDate != "" {
			raw.EndDate = extractDateFromSelection(s, config.Selectors.EndDate)
		}

		if config.Selectors.Location != "" {
			raw.Location = strings.TrimSpace(s.Find(config.Selectors.Location).First().Text())
		}

		if config.Selectors.Description != "" {
			raw.Description = strings.TrimSpace(s.Find(config.Selectors.Description).First().Text())
		}

		if config.Selectors.URL != "" {
			href, exists := s.Find(config.Selectors.URL).First().Attr("href")
			if exists && href != "" {
				raw.URL = resolveURL(pageURL, href)
			}
		}

		if config.Selectors.Image != "" {
			src, exists := s.Find(config.Selectors.Image).First().Attr("src")
			if exists && src != "" {
				raw.Image = resolveURL(pageURL, src)
			}
		}

		// Only collect events with a non-empty name.
		if raw.Name == "" {
			return
		}

		events = append(events, raw)
	})

	return events, nil
}

// extractDateFromSelection finds a child element matching selector in the goquery
// selection and returns the value of its datetime attribute, falling back to an
// ISO 8601 date extracted from its id attribute (e.g. id="date-2026-03-04"),
// then falling back to its text content.
func extractDateFromSelection(s *goquery.Selection, selector string) string {
	el := s.Find(selector).First()
	if el.Length() == 0 {
		return ""
	}
	// Prefer datetime attribute (standard HTML5 time element).
	if dt, exists := el.Attr("datetime"); exists && dt != "" {
		return strings.TrimSpace(dt)
	}
	// Some sites encode the date in the element's id attribute with a well-known
	// prefix, e.g. id="date-2026-03-04". Extract the ISO date portion.
	if id, exists := el.Attr("id"); exists && strings.HasPrefix(id, "date-") {
		candidate := strings.TrimPrefix(id, "date-")
		// Validate it looks like YYYY-MM-DD before returning.
		if len(candidate) == 10 && candidate[4] == '-' && candidate[7] == '-' {
			return candidate
		}
	}
	return strings.TrimSpace(el.Text())
}

// resolveURL resolves href/src relative to baseURL. Returns href unchanged if
// parsing fails or href is already absolute.
func resolveURL(baseURL, href string) string {
	if href == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// sanitizeName replaces characters unsafe for filenames with underscores.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// blockedCIDRs is the set of private/loopback/link-local ranges blocked for
// headless navigation URLs to prevent SSRF (srv-tbg24). Mirrors the list in
// internal/jobs/validate_submissions.go (newSSRFBlockingTransport).
var blockedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC 1918 private
		"172.16.0.0/12",  // RFC 1918 private
		"192.168.0.0/16", // RFC 1918 private
		"169.254.0.0/16", // link-local / cloud metadata
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("rod: invalid blocked CIDR %q: %v", cidr, err))
		}
		nets = append(nets, ipNet)
	}
	return nets
}()

// validateNavigationURL returns an error if rawURL is not a safe http/https URL.
// This prevents SSRF via file://, chrome://, javascript:, or other schemes, as
// well as navigation to private/loopback/link-local IP addresses (srv-tbg24).
func validateNavigationURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("rod: invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("rod: navigation URL must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("rod: navigation URL missing host: %q", rawURL)
	}

	// Resolve hostname and block private/loopback/link-local addresses.
	host := u.Hostname()
	// If the host is already an IP literal (e.g. [::1] or 1.2.3.4), check directly.
	if ip := net.ParseIP(host); ip != nil {
		for _, blocked := range blockedCIDRs {
			if blocked.Contains(ip) {
				return fmt.Errorf("rod: SSRF check: navigation URL %q resolves to blocked address %s", rawURL, ip)
			}
		}
		return nil
	}

	// Resolve hostname to IPs.
	ips, resolveErr := net.LookupHost(host)
	if resolveErr != nil {
		return fmt.Errorf("rod: SSRF check: resolve %q: %w", host, resolveErr)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		for _, blocked := range blockedCIDRs {
			if blocked.Contains(ip) {
				return fmt.Errorf("rod: SSRF check: %s resolves to blocked address %s", host, ip)
			}
		}
	}
	return nil
}

// RenderHTML navigates to rawURL in a headless Chromium browser, waits for
// waitSelector to appear (default "body"), and returns the fully rendered HTML.
// waitTimeoutMs controls the wait deadline (0 = 10 000 ms default).
//
// This is intended for CLI tooling (scrape capture) that needs the rendered
// HTML for further analysis or selector discovery, without extracting events.
func (e *RodExtractor) RenderHTML(ctx context.Context, rawURL, waitSelector string, waitTimeoutMs int) (string, error) {
	if !e.headlessEnv {
		return "", ErrHeadlessDisabled
	}

	// Validate URL to prevent SSRF before launching any browser.
	if err := validateNavigationURL(rawURL); err != nil {
		return "", err
	}

	// Robots.txt check.
	// Also checked internally by RodExtractor; early check here provides clearer UX.
	robotsClient := robotsClientFrom(&http.Client{Timeout: fetchTimeout})
	allowed, robotsErr := RobotsAllowed(ctx, rawURL, scraperUserAgent, robotsClient)
	if robotsErr != nil {
		e.logger.Warn().Err(robotsErr).Str("url", rawURL).Msg("rod: robots.txt check failed, proceeding as allowed")
	} else if !allowed {
		return "", fmt.Errorf("rod: scraping disallowed by robots.txt for %q", rawURL)
	}

	// Acquire semaphore.
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// Apply a hard timeout for the entire render operation.
	ctx, cancel := context.WithTimeout(ctx, rodDefaultTimeout)
	defer cancel()

	browser, cleanup, err := e.launchBrowser(ctx)
	if err != nil {
		return "", err
	}
	defer cleanup()

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return "", fmt.Errorf("rod: failed to open new page: %w", err)
	}
	defer func() {
		if closeErr := page.Close(); closeErr != nil {
			e.logger.Debug().Err(closeErr).Msg("rod: page close error (RenderHTML)")
		}
	}()

	page = page.Timeout(rodDefaultTimeout)

	// Override the browser's default user-agent with the scraper UA.
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: rodUserAgent,
	}); err != nil {
		e.logger.Warn().Err(err).Msg("rod: RenderHTML: failed to set user agent")
	}

	if navErr := page.Navigate(rawURL); navErr != nil {
		return "", fmt.Errorf("rod: navigate to %q: %w", rawURL, navErr)
	}

	if waitSelector == "" {
		waitSelector = "body"
	}
	waitTimeout := rodDefaultWaitTimeout
	if waitTimeoutMs > 0 {
		waitTimeout = time.Duration(waitTimeoutMs) * time.Millisecond
	}

	if waitErr := page.Timeout(waitTimeout).WaitElementsMoreThan(waitSelector, 0); waitErr != nil {
		e.logger.Warn().
			Err(waitErr).
			Str("url", rawURL).
			Str("selector", waitSelector).
			Msg("rod: RenderHTML wait selector timed out, continuing anyway")
	}

	html, err := page.HTML()
	if err != nil {
		// Attempt screenshot for debugging.
		e.captureScreenshot(page, "render-html")
		return "", fmt.Errorf("rod: getting HTML from %q: %w", rawURL, err)
	}

	return html, nil
}
