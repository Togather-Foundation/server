package scraper

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// SourceConfig defines a scrape source loaded from a YAML config file.
type SourceConfig struct {
	Name string `yaml:"name"            json:"name"`
	// URL is the primary entry-point URL for this source. When URLs is also
	// set, URL is not validated and not used for fetching — GetURLs() returns
	// URLs instead, and URL is stored only as human-readable metadata (e.g. in
	// scraper_run records). See also: ValidateConfig for the url-skip behaviour.
	URL             string         `yaml:"url"               json:"url"`
	URLs            []string       `yaml:"urls,omitempty"    json:"urls,omitempty"`
	Tier            int            `yaml:"tier"              json:"tier"`
	Schedule        string         `yaml:"schedule"          json:"schedule"`
	TrustLevel      int            `yaml:"trust_level"       json:"trust_level"`
	License         string         `yaml:"license"           json:"license"`
	Enabled         bool           `yaml:"enabled"           json:"enabled"`
	EventURLPattern string         `yaml:"event_url_pattern" json:"event_url_pattern"`
	MaxPages        int            `yaml:"max_pages"         json:"max_pages"`
	Notes           string         `yaml:"notes,omitempty"   json:"notes,omitempty"`
	Selectors       SelectorConfig `yaml:"selectors"         json:"selectors"`
	// FollowEventURLs instructs the Tier 0 scraper to fetch each event's detail
	// page to retrieve the full description when the JSON-LD description appears
	// truncated (e.g. Tribe Events WordPress sources always truncate).
	FollowEventURLs bool `yaml:"follow_event_urls" json:"follow_event_urls"`
	// SkipMultiSessionCheck disables the multi-session event heuristic for this
	// source. Use for sources that legitimately emit long-duration single events
	// (e.g., festivals, art installations).
	SkipMultiSessionCheck bool `yaml:"skip_multi_session_check" json:"skip_multi_session_check"`
	// MultiSessionDurationThreshold overrides the default 168h (1 week) duration
	// threshold used by the multi-session heuristic. Use for sources that
	// legitimately publish events longer than 1 week but shorter than 30 days
	// (e.g., festivals spanning multiple weeks). Value is a Go duration string
	// like "720h" (30 days). Zero value means use the default (168h).
	MultiSessionDurationThreshold string `yaml:"multi_session_duration_threshold,omitempty" json:"multi_session_duration_threshold,omitempty"`
	// Headless holds Tier 2 headless-browser options. Ignored for tier 0/1.
	Headless HeadlessConfig `yaml:"headless,omitempty" json:"headless,omitempty"`
	// GraphQL holds Tier 3 GraphQL API options. Ignored for tier 0/1/2.
	GraphQL *GraphQLConfig `yaml:"graphql,omitempty" json:"graphql,omitempty"`
	// REST holds Tier 3 REST JSON feed options. Ignored for tier 0/1/2.
	// Exactly one of GraphQL or REST must be set for tier 3.
	REST *RestConfig `yaml:"rest,omitempty" json:"rest,omitempty"`
	// Sitemap holds sitemap-based URL discovery options. When set, the scraper
	// fetches the sitemap XML and scrapes each matching URL individually using
	// the source's configured tier. Mutually exclusive with urls (but url is
	// kept as human-readable metadata). Ignored for tier 3.
	Sitemap *SitemapConfig `yaml:"sitemap,omitempty" json:"sitemap,omitempty"`
	// Timezone is an IANA timezone name (e.g. "America/Toronto") used when
	// parsing human-readable dates that lack timezone information (common in
	// Tier 1/2 CSS selector scraping). When empty, falls back to the
	// DEFAULT_TIMEZONE environment variable (server-wide setting — each SEL
	// node serves one geographic location). If that is also unset, defaults
	// to "America/Toronto".
	Timezone string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	// LastScrapedAt is populated from the DB when loading configs via the
	// source repository. It is NOT read from YAML. Used by sitemap scraping
	// to filter URLs by lastmod date.
	LastScrapedAt *time.Time `yaml:"-" json:"-"`
}

// SitemapConfig holds sitemap-based URL discovery options. When set on a
// SourceConfig, the scraper fetches the sitemap XML, filters URLs by
// FilterPattern (and optionally ExcludePattern), and scrapes each matching
// URL individually using the source's configured tier (0, 1, or 2). This
// replaces the static url/urls fields as the URL source for the scrape run.
type SitemapConfig struct {
	// URL is the sitemap XML URL to fetch (e.g. https://example.com/sitemap.xml).
	URL string `yaml:"url" json:"url"`
	// FilterPattern is a Go regular expression matched against each URL in the
	// sitemap. Only URLs matching this pattern are scraped. Required.
	// Example: "/events/.+" to match event detail pages.
	FilterPattern string `yaml:"filter_pattern" json:"filter_pattern"`
	// ExcludePattern is an optional Go regular expression. URLs matching this
	// pattern are excluded even if they match FilterPattern. Useful for large
	// sitemaps where it's easier to exclude non-event pages (e.g. artist bios,
	// about pages) than to enumerate all event URL patterns.
	// Example: "/(artist|about|terms|contact)" to exclude non-event pages.
	ExcludePattern string `yaml:"exclude_pattern,omitempty" json:"exclude_pattern,omitempty"`
	// MaxURLs caps the number of URLs scraped per run. 0 means use the default (200).
	// This is a safety net to prevent runaway scrapes on large sitemaps.
	MaxURLs int `yaml:"max_urls" json:"max_urls"`
	// RateLimitMs is the delay in milliseconds between fetching individual detail
	// pages. 0 does NOT mean no delay — it means use the default (500 ms).
	// Set to 1 for minimal delay. A warning is emitted at load time when 0 is
	// set explicitly, to avoid silent misconfigurations.
	RateLimitMs int `yaml:"rate_limit_ms" json:"rate_limit_ms"`
}

// SelectorConfig holds CSS selectors used for Tier 1 (Colly) and Tier 2
// (Rod headless) scraping. All fields map to their YAML and JSONB column
// names via the yaml/json struct tags.
type SelectorConfig struct {
	EventList   string `yaml:"event_list" json:"event_list"`
	Name        string `yaml:"name" json:"name"`
	StartDate   string `yaml:"start_date" json:"start_date"`
	EndDate     string `yaml:"end_date" json:"end_date"`
	Location    string `yaml:"location" json:"location"`
	Description string `yaml:"description" json:"description"`
	URL         string `yaml:"url" json:"url"`
	Image       string `yaml:"image" json:"image"`
	Pagination  string `yaml:"pagination" json:"pagination"`
	// DateSelectors is a list of CSS selectors that each extract a text
	// fragment containing part of the event date/time information (e.g.
	// one selector for the date, another for the time). The scraper
	// extracts text from each selector and passes all fragments to the
	// smart date assembler, which infers start and end datetimes.
	//
	// When set, DateSelectors takes priority over StartDate/EndDate for
	// date extraction. StartDate/EndDate are still used as fallback if
	// DateSelectors produces no result.
	//
	// Example:
	//   date_selectors:
	//     - ".first [class^='time-container-']"     # "Thu 5th March"
	//     - "[style*='display: flex'] [class^='time-container-']"  # "9:30 PM"
	DateSelectors []string `yaml:"date_selectors,omitempty" json:"date_selectors,omitempty"`
}

// HeadlessConfig holds Tier 2 headless-browser-specific options.
type HeadlessConfig struct {
	// WaitSelector is a CSS selector to wait for before extracting events.
	// Required for tier 2. Defaults to "body" if empty after validation.
	WaitSelector string `yaml:"wait_selector" json:"wait_selector"`
	// WaitTimeoutMs is the maximum time (ms) to wait for WaitSelector.
	// 0 means use the RodExtractor default (10 000 ms).
	WaitTimeoutMs int `yaml:"wait_timeout_ms" json:"wait_timeout_ms"`
	// WaitNetworkIdle instructs the scraper to additionally wait for all
	// in-flight network requests to settle (500 ms idle window) after
	// WaitSelector resolves. Useful for pages whose content is loaded via
	// async XHR/fetch requests that fire after the initial DOM is ready (e.g.
	// third-party event widget embeds). Adds up to WaitTimeoutMs of extra wait.
	WaitNetworkIdle bool `yaml:"wait_network_idle" json:"wait_network_idle"`
	// Undetected launches the browser page with stealth evasions
	// (patches navigator.webdriver, fake plugins, etc.) to reduce the chance
	// of bot-detection by sites that check for headless Chrome fingerprints.
	Undetected bool `yaml:"undetected" json:"undetected"`
	// PaginationBtn is a CSS selector for a "load more" / "next" button to click
	// for JS-rendered pagination. Empty means no pagination.
	PaginationBtn string `yaml:"pagination_button" json:"pagination_button"`
	// Headers are extra HTTP headers injected into every browser request.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// RateLimitMs overrides the per-domain delay between page loads (ms).
	// 0 means use the RodExtractor default.
	RateLimitMs int `yaml:"rate_limit_ms" json:"rate_limit_ms"`
	// Iframe configures extraction from a cross-origin iframe. When set,
	// the scraper navigates into the matched iframe's execution context
	// and extracts HTML from the frame instead of the parent page.
	Iframe *IframeConfig `yaml:"iframe,omitempty" json:"iframe,omitempty"`
	// Intercept configures network request interception to capture JSON API
	// responses made by JS widgets on the page. When set, the scraper installs
	// a request router before navigation that captures responses matching
	// URLPattern, then extracts events from the captured JSON using ResultsPath
	// and FieldMap. Intercepted events are merged with any DOM-extracted events.
	Intercept *InterceptConfig `yaml:"intercept,omitempty" json:"intercept,omitempty"`
}

// IframeConfig holds options for extracting content from a cross-origin
// iframe inside a headless-rendered page. When set, the scraper enters
// the iframe's execution context via Rod's CDP frame navigation and
// extracts HTML from the frame instead of the parent page. CSS selectors
// in SourceConfig.Selectors then apply to the iframe DOM.
type IframeConfig struct {
	// Selector is a CSS selector that matches the target <iframe> element
	// in the parent page (e.g. "iframe[title='Ticket Spot']").
	Selector string `yaml:"selector" json:"selector"`
	// WaitSelector is a CSS selector to wait for inside the iframe before
	// extracting its HTML (e.g. ".events-container").
	WaitSelector string `yaml:"wait_selector" json:"wait_selector"`
	// WaitTimeoutMs is the maximum time (ms) to wait for WaitSelector
	// inside the iframe. 0 means use the default (10 000 ms).
	WaitTimeoutMs int `yaml:"wait_timeout_ms" json:"wait_timeout_ms"`
}

// InterceptConfig configures network request interception for headless scraping.
// When set on HeadlessConfig.Intercept, the scraper installs a request router
// before page navigation that captures JSON API responses matching URLPattern.
// Captured responses are parsed using ResultsPath and mapped to RawEvents via
// FieldMap (same dot-notation as RestConfig.FieldMap). Useful for JS widgets
// that fetch events from a backend API (AWS CloudSearch, Algolia, custom REST)
// rather than rendering them directly in the DOM.
type InterceptConfig struct {
	// URLPattern is a Go regular expression matched against the full request URL.
	// Required. Only responses whose URL matches this pattern are captured.
	// Example: "cloudsearch|algolia" or "api/events".
	URLPattern string `yaml:"url_pattern" json:"url_pattern"`
	// ResponseFormat is the format of the captured response. Only "json" is
	// currently supported. Defaults to "json" when empty.
	ResponseFormat string `yaml:"response_format" json:"response_format"`
	// ResultsPath is a dot-notation path into the JSON response object that
	// resolves to the array of event items. Required.
	// Example: "hits.hit" resolves response["hits"]["hit"].
	ResultsPath string `yaml:"results_path" json:"results_path"`
	// CacheEndpoint, when true, logs the intercepted request URL at Info level.
	// Useful for discovering the exact API endpoint so it can be moved to a
	// Tier 3 REST config.
	CacheEndpoint bool `yaml:"cache_endpoint" json:"cache_endpoint"`
	// FieldMap maps RawEvent field names to JSON response field names using
	// dot-notation (same as RestConfig.FieldMap). Supported keys: name,
	// start_date, end_date, url, image, location, description.
	FieldMap map[string]string `yaml:"field_map,omitempty" json:"field_map,omitempty"`
}

// GraphQLConfig holds Tier 3 GraphQL API options.
//
// Note on URL fields: the url/urls fields on SourceConfig are NOT used for
// fetching by Tier 3 — only Endpoint is used to make the GraphQL request.
// source.URL is persisted as human-readable metadata in scraper_run records
// (SourceURL column) so operators can identify the source in dashboards and
// logs, but it is never passed to the HTTP client.
type GraphQLConfig struct {
	// Endpoint is the GraphQL API URL (e.g. https://graphql.datocms.com/).
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	// Token is the Bearer auth token. Optional — omit for public APIs that
	// don't require authentication.
	//
	// Only commit public/read-only tokens (e.g. DatoCMS public API tokens
	// that are embedded in the site's JavaScript). Never commit tokens with
	// write access or private data access. If a token must stay out of version
	// control, use an env-var reference: set token to the empty string and
	// pass the value via environment variable in the deploy config instead.
	Token string `yaml:"token" json:"token"`
	// Query is the full GraphQL query string.
	Query string `yaml:"query" json:"query"`
	// EventField is the top-level key in data{} that holds the events array
	// (e.g. "allEvents").
	EventField string `yaml:"event_field" json:"event_field"`
	// TimeoutMs is the HTTP request timeout in milliseconds. 0 = use default (30s).
	TimeoutMs int `yaml:"timeout_ms" json:"timeout_ms"`
	// URLTemplate is an optional Go text/template string used to construct the
	// canonical URL for each event. The template receives the raw event map as
	// its data (e.g. "https://tranzac.org/events/{{.slug}}").
	// If empty, no URL is constructed from the API response.
	URLTemplate string `yaml:"url_template" json:"url_template"`
	// FieldMap maps RawEvent field names (keys) to source GraphQL response
	// field names (values). Supported keys: name, start_date, end_date, url,
	// image, location, description. Values support dot-notation for nested
	// fields (e.g. "photo.url" to extract a nested image URL).
	//
	// When FieldMap is nil or empty, the legacy DatoCMS-convention mapping is
	// used: title→Name, startDate→StartDate, endDate→EndDate,
	// description→Description, photo.url→Image, rooms[0].name→Location.
	// Set FieldMap to opt into explicit mapping for non-DatoCMS GraphQL APIs.
	FieldMap map[string]string `yaml:"field_map,omitempty" json:"field_map,omitempty"`
}

// RestConfig holds Tier 3 REST JSON feed options.
//
// Note on URL fields: the url/urls fields on SourceConfig are NOT used for
// fetching by Tier 3 REST — only Endpoint is used to make the HTTP request.
// source.URL is persisted as human-readable metadata in scraper_run records
// (SourceURL column) so operators can identify the source in dashboards and
// logs, but it is never passed to the HTTP client.
type RestConfig struct {
	// Endpoint is the first page URL for the REST API (e.g.
	// https://www.showpass.com/api/public/events/?venue=17330).
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	// ResultsField is the key in the JSON response that holds the array of
	// event objects. Use "." when the response is a bare JSON array (e.g.
	// [{...}, {...}]) with no envelope object. When ".", pagination is not
	// supported (bare arrays have no envelope to carry a next-page URL).
	// An empty string will fail to find any key (treated as empty page).
	ResultsField string `yaml:"results_field" json:"results_field"`
	// NextField is the key in the JSON response that holds the URL of the
	// next page (null or absent = no more pages). Defaults to "next" when
	// empty.
	NextField string `yaml:"next_field" json:"next_field"`
	// URLTemplate is an optional Go text/template string used to construct the
	// canonical URL for each event. The template receives the raw item map as
	// its data (e.g. "https://www.showpass.com/{{.slug}}").
	// If empty, no URL is constructed from the field_map url key.
	URLTemplate string `yaml:"url_template" json:"url_template"`
	// TimeoutMs is the HTTP request timeout in milliseconds. 0 = use default.
	TimeoutMs int `yaml:"timeout_ms" json:"timeout_ms"`
	// Headers are extra HTTP headers sent with every request.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// FieldMap maps RawEvent field names (keys) to source JSON field names
	// (values). Supported keys: name, start_date, end_date, url, image,
	// location, description. When empty, field names are used directly as-is
	// (identity mapping using the RawEvent Go field names).
	FieldMap map[string]string `yaml:"field_map,omitempty" json:"field_map,omitempty"`
}

// GetURLs returns the list of entry-point URLs for this source.
// When URLs is non-empty it is returned directly and URL is ignored (URLs
// takes precedence). When URLs is empty and URL is set, URL is wrapped in a
// single-element slice for backwards compatibility. Returns nil when neither
// field is set (should not happen after validation).
func (c SourceConfig) GetURLs() []string {
	if len(c.URLs) > 0 {
		return c.URLs
	}
	if c.URL != "" {
		return []string{c.URL}
	}
	return nil
}

// GetTimezone returns the effective IANA timezone name for this source.
// Priority: per-source Timezone field → DEFAULT_TIMEZONE env var →
// "America/Toronto" fallback. Each SEL node runs for one geographic
// location, so the env var covers all sources on that node.
func (c SourceConfig) GetTimezone() string {
	if c.Timezone != "" {
		return c.Timezone
	}
	if tz := os.Getenv("DEFAULT_TIMEZONE"); tz != "" {
		return tz
	}
	return "America/Toronto"
}

// DefaultSourceConfig returns a SourceConfig with sensible defaults applied.
func DefaultSourceConfig() SourceConfig {
	return SourceConfig{
		Enabled:    true,
		Tier:       0,
		TrustLevel: 5,
		MaxPages:   10,
		Schedule:   "manual",
	}
}

// knownFieldMapKeys is the set of valid keys for FieldMap (used by both
// RestConfig.FieldMap and GraphQLConfig.FieldMap). Only these 7 keys are
// consumed by the event-mapping functions; any other key
// is silently ignored at runtime, so operators should be warned of typos.
var knownFieldMapKeys = map[string]struct{}{
	"name":        {},
	"start_date":  {},
	"end_date":    {},
	"url":         {},
	"image":       {},
	"location":    {},
	"description": {},
}

// ValidateConfig validates a SourceConfig and returns an error describing all
// problems found, or nil if the config is valid.
//
// URL-skip behaviour: when the URLs field is non-empty the URL field is not
// validated and not used for fetching (GetURLs returns URLs). A malformed URL
// field will not block an otherwise valid config in that case. This is
// intentional — URL is treated as human-readable metadata when URLs is set.
// Callers relying on URL for display or logging should be aware that it may
// not be a valid http/https URL when URLs is also present. See SourceConfig.URL
// godoc for details.
//
// To also retrieve non-fatal warnings (e.g. unrecognised FieldMap keys), use
// ValidateConfigWithWarnings instead.
func ValidateConfig(cfg SourceConfig) error {
	_, err := ValidateConfigWithWarnings(cfg)
	return err
}

// ValidateConfigWithWarnings validates a SourceConfig and returns an error for
// hard failures and a slice of human-readable warning strings for non-fatal
// issues (e.g. unrecognised keys in RestConfig.FieldMap). Either return value
// may be nil/empty when there are no issues of that severity.
func ValidateConfigWithWarnings(cfg SourceConfig) ([]string, error) {
	var errs []string
	var warnings []string

	if strings.TrimSpace(cfg.Name) == "" {
		errs = append(errs, "name: required")
	}

	hasURL := strings.TrimSpace(cfg.URL) != ""
	hasURLs := len(cfg.URLs) > 0

	hasSitemap := cfg.Sitemap != nil
	if !hasURL && !hasURLs && !hasSitemap {
		errs = append(errs, "url: required (set url, urls, or sitemap)")
	}
	// Validate url only when it will actually be used (i.e. urls is not set).
	// When urls is present, url is ignored by GetURLs(), so a malformed url
	// should not block a valid config.
	if hasURL && !hasURLs {
		u, err := url.Parse(cfg.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Sprintf("url: must be a valid http/https URL, got %q", cfg.URL))
		}
	}
	for i, u := range cfg.URLs {
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			errs = append(errs, fmt.Sprintf("urls[%d]: must be a valid http/https URL, got %q", i, u))
		}
	}

	if hasSitemap && !hasURL {
		warnings = append(warnings, "url: recommended for sitemap sources (used as display/tracking identifier in scraper_run records)")
	}

	if cfg.Tier < 0 || cfg.Tier > 3 {
		errs = append(errs, fmt.Sprintf("tier: must be 0, 1, 2, or 3, got %d", cfg.Tier))
	}

	if cfg.TrustLevel != 0 && (cfg.TrustLevel < 1 || cfg.TrustLevel > 10) {
		errs = append(errs, fmt.Sprintf("trust_level: must be 1-10, got %d", cfg.TrustLevel))
	}

	if cfg.Tier == 1 && strings.TrimSpace(cfg.Selectors.EventList) == "" {
		errs = append(errs, "selectors.event_list: required for tier 1")
	}

	if cfg.Tier == 2 && strings.TrimSpace(cfg.Selectors.EventList) == "" {
		errs = append(errs, "selectors.event_list: required for tier 2")
	}

	if cfg.Headless.Iframe != nil {
		if cfg.Tier != 2 {
			warnings = append(warnings, fmt.Sprintf("iframe config is only used by tier 2 (headless) sources; it will be ignored for tier %d", cfg.Tier))
		}
		if strings.TrimSpace(cfg.Headless.Iframe.Selector) == "" {
			errs = append(errs, "headless.iframe.selector is required when iframe block is set")
		}
		if strings.TrimSpace(cfg.Headless.Iframe.WaitSelector) == "" {
			errs = append(errs, "headless.iframe.wait_selector is required when iframe block is set")
		}
	}

	if cfg.Headless.Intercept != nil {
		if cfg.Tier != 2 {
			warnings = append(warnings, fmt.Sprintf("intercept config is only used by tier 2 (headless) sources; it will be ignored for tier %d", cfg.Tier))
		}
		ic := cfg.Headless.Intercept
		if strings.TrimSpace(ic.URLPattern) == "" {
			errs = append(errs, "headless.intercept.url_pattern: required when intercept block is set")
		} else if _, err := regexp.Compile(ic.URLPattern); err != nil {
			errs = append(errs, fmt.Sprintf("headless.intercept.url_pattern: invalid Go regex: %v", err))
		}
		if ic.ResponseFormat != "" && ic.ResponseFormat != "json" {
			errs = append(errs, fmt.Sprintf("headless.intercept.response_format: only \"json\" is supported, got %q", ic.ResponseFormat))
		}
		if strings.TrimSpace(ic.ResultsPath) == "" {
			errs = append(errs, "headless.intercept.results_path: required when intercept block is set")
		}
		for k := range ic.FieldMap {
			if _, ok := knownFieldMapKeys[k]; !ok {
				warnings = append(warnings, fmt.Sprintf("headless.intercept.field_map: unrecognised key %q (known keys: name, start_date, end_date, url, image, location, description)", k))
			}
		}
	}

	if cfg.Tier == 3 {
		if cfg.GraphQL != nil && cfg.REST != nil {
			errs = append(errs, "tier 3: graphql and rest are mutually exclusive; set exactly one")
		} else if cfg.GraphQL == nil && cfg.REST == nil {
			errs = append(errs, "tier 3 requires a graphql or rest config block")
		} else if cfg.GraphQL != nil {
			if strings.TrimSpace(cfg.GraphQL.Endpoint) == "" {
				errs = append(errs, "graphql.endpoint: required for tier 3")
			} else {
				u, err := url.Parse(cfg.GraphQL.Endpoint)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					errs = append(errs, fmt.Sprintf("graphql.endpoint: must be a valid http/https URL, got %q", cfg.GraphQL.Endpoint))
				}
			}
			if strings.TrimSpace(cfg.GraphQL.Query) == "" {
				errs = append(errs, "graphql.query: required for tier 3")
			}
			if strings.TrimSpace(cfg.GraphQL.EventField) == "" {
				errs = append(errs, "graphql.event_field: required for tier 3")
			}
			if t := strings.TrimSpace(cfg.GraphQL.URLTemplate); t != "" {
				if _, err := template.New("url").Option("missingkey=error").Parse(t); err != nil {
					errs = append(errs, fmt.Sprintf("graphql.url_template: invalid Go template: %v", err))
				}
			}
			for k := range cfg.GraphQL.FieldMap {
				if _, ok := knownFieldMapKeys[k]; !ok {
					warnings = append(warnings, fmt.Sprintf("graphql.field_map: unrecognised key %q (known keys: name, start_date, end_date, url, image, location, description)", k))
				}
			}
		} else { // cfg.REST != nil
			if strings.TrimSpace(cfg.REST.Endpoint) == "" {
				errs = append(errs, "rest.endpoint: required for tier 3")
			} else {
				u, err := url.Parse(cfg.REST.Endpoint)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					errs = append(errs, fmt.Sprintf("rest.endpoint: must be a valid http/https URL, got %q", cfg.REST.Endpoint))
				}
			}
			if t := strings.TrimSpace(cfg.REST.URLTemplate); t != "" {
				if _, err := template.New("url").Option("missingkey=error").Parse(t); err != nil {
					errs = append(errs, fmt.Sprintf("rest.url_template: invalid Go template: %v", err))
				}
			}
		}
	}

	if cfg.Sitemap != nil {
		if len(cfg.URLs) > 0 {
			errs = append(errs, "sitemap: mutually exclusive with urls (set one or the other)")
		}
		if cfg.Tier == 3 {
			errs = append(errs, "sitemap: not supported for tier 3 sources (use graphql/rest config instead)")
		}
		if strings.TrimSpace(cfg.Sitemap.URL) == "" {
			errs = append(errs, "sitemap.url: required when sitemap block is set")
		} else {
			u, err := url.Parse(cfg.Sitemap.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				errs = append(errs, fmt.Sprintf("sitemap.url: must be a valid http/https URL, got %q", cfg.Sitemap.URL))
			}
		}
		if strings.TrimSpace(cfg.Sitemap.FilterPattern) == "" {
			errs = append(errs, "sitemap.filter_pattern: required when sitemap block is set")
		} else if _, err := regexp.Compile(cfg.Sitemap.FilterPattern); err != nil {
			errs = append(errs, fmt.Sprintf("sitemap.filter_pattern: invalid Go regex: %v", err))
		}
		if cfg.Sitemap.ExcludePattern != "" {
			if _, err := regexp.Compile(cfg.Sitemap.ExcludePattern); err != nil {
				errs = append(errs, fmt.Sprintf("sitemap.exclude_pattern: invalid Go regex: %v", err))
			}
		}
		if cfg.Sitemap.MaxURLs < 0 {
			errs = append(errs, fmt.Sprintf("sitemap.max_urls: must be >= 0, got %d", cfg.Sitemap.MaxURLs))
		} else if cfg.Sitemap.MaxURLs > maxSitemapMaxURLs {
			warnings = append(warnings, fmt.Sprintf("sitemap.max_urls: %d is very high; consider a lower value to avoid overwhelming the target site", cfg.Sitemap.MaxURLs))
		}
		if cfg.Sitemap.RateLimitMs < 0 {
			errs = append(errs, fmt.Sprintf("sitemap.rate_limit_ms: must be >= 0, got %d", cfg.Sitemap.RateLimitMs))
		} else if cfg.Sitemap.RateLimitMs == 0 {
			warnings = append(warnings, "sitemap.rate_limit_ms: 0 means use default (500 ms), not no delay; set to 1 for minimal delay")
		}
	}

	if cfg.Schedule != "" {
		switch cfg.Schedule {
		case "daily", "weekly", "manual":
			// valid
		default:
			errs = append(errs, fmt.Sprintf("schedule: must be daily, weekly, or manual, got %q", cfg.Schedule))
		}
	}

	if cfg.MaxPages < 0 {
		errs = append(errs, fmt.Sprintf("max_pages: must be > 0, got %d", cfg.MaxPages))
	}

	// Warn about unrecognised FieldMap keys. These are non-fatal: the scraper
	// silently ignores unknown keys, but a typo (e.g. "strat_date") would
	// produce an empty field with no other indication to the operator.
	if cfg.REST != nil {
		for k := range cfg.REST.FieldMap {
			if _, ok := knownFieldMapKeys[k]; !ok {
				warnings = append(warnings, fmt.Sprintf("rest.field_map: unrecognised key %q (known keys: name, start_date, end_date, url, image, location, description)", k))
			}
		}
	}

	if len(errs) > 0 {
		return warnings, errors.New(strings.Join(errs, "; "))
	}
	return warnings, nil
}

// LoadSourceConfigs reads all *.yaml files from dir (skipping files starting
// with "_"), parses each into a SourceConfig with defaults applied, validates
// each config, and returns the slice of valid configs. If any config is
// invalid an error is returned that includes the file path and field errors.
// A non-existent directory returns an empty slice with no error.
func LoadSourceConfigs(dir string) ([]SourceConfig, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []SourceConfig{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading source config dir %s: %w", dir, err)
	}

	var configs []SourceConfig
	var validationErrors []string
	seen := make(map[string]string) // name → file path of first occurrence

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}
		if filepath.Ext(name) != ".yaml" {
			continue
		}

		filePath := filepath.Join(dir, name)
		cfg, err := loadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", filePath, err)
		}

		warnings, err := ValidateConfigWithWarnings(cfg)
		for _, w := range warnings {
			slog.Warn("source config warning", "file", filePath, "warning", w)
		}
		if err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", filePath, err.Error()))
			continue
		}

		if first, dup := seen[cfg.Name]; dup {
			validationErrors = append(validationErrors,
				fmt.Sprintf("%s: duplicate source name %q (already defined in %s)", filePath, cfg.Name, first))
			continue
		}
		seen[cfg.Name] = filePath
		configs = append(configs, cfg)
	}

	if len(validationErrors) > 0 {
		return configs, fmt.Errorf("invalid source configs:\n  %s", strings.Join(validationErrors, "\n  "))
	}
	return configs, nil
}

// LoadSourceConfig reads a single YAML source config file, applies defaults,
// and validates it. It is the public counterpart of the internal loadFile,
// intended for use by CLI commands that accept an explicit config path.
func LoadSourceConfig(path string) (SourceConfig, error) {
	cfg, err := loadFile(path)
	if err != nil {
		return SourceConfig{}, fmt.Errorf("loading %s: %w", path, err)
	}
	if err := ValidateConfig(cfg); err != nil {
		return SourceConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// loadFile reads a single YAML source config file and applies defaults.
func loadFile(path string) (SourceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceConfig{}, err
	}

	// Start from defaults so zero-value booleans and ints are set properly.
	cfg := DefaultSourceConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return SourceConfig{}, fmt.Errorf("parsing YAML: %w", err)
	}

	// Apply conditional defaults that depend on parsed values.
	if cfg.TrustLevel == 0 {
		cfg.TrustLevel = 5
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = 10
	}
	if cfg.REST != nil {
		if cfg.REST.ResultsField == "" {
			cfg.REST.ResultsField = "results"
		}
		if cfg.REST.NextField == "" {
			cfg.REST.NextField = "next"
		}
	}
	if cfg.Headless.Iframe != nil && cfg.Headless.Iframe.WaitTimeoutMs == 0 {
		cfg.Headless.Iframe.WaitTimeoutMs = 10000
	}

	return cfg, nil
}
