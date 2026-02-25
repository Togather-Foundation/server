package scraper

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// SourceConfig defines a scrape source loaded from a YAML config file.
type SourceConfig struct {
	Name            string         `yaml:"name"              json:"name"`
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
}

// HeadlessConfig holds Tier 2 headless-browser-specific options.
type HeadlessConfig struct {
	// WaitSelector is a CSS selector to wait for before extracting events.
	// Required for tier 2. Defaults to "body" if empty after validation.
	WaitSelector string `yaml:"wait_selector" json:"wait_selector"`
	// WaitTimeoutMs is the maximum time (ms) to wait for WaitSelector.
	// 0 means use the RodExtractor default (10 000 ms).
	WaitTimeoutMs int `yaml:"wait_timeout_ms" json:"wait_timeout_ms"`
	// PaginationBtn is a CSS selector for a "load more" / "next" button to click
	// for JS-rendered pagination. Empty means no pagination.
	PaginationBtn string `yaml:"pagination_button" json:"pagination_button"`
	// Headers are extra HTTP headers injected into every browser request.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// RateLimitMs overrides the per-domain delay between page loads (ms).
	// 0 means use the RodExtractor default.
	RateLimitMs int `yaml:"rate_limit_ms" json:"rate_limit_ms"`
}

// GraphQLConfig holds Tier 3 GraphQL API options.
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

// ValidateConfig validates a SourceConfig and returns an error describing all
// problems found, or nil if the config is valid.
func ValidateConfig(cfg SourceConfig) error {
	var errs []string

	if strings.TrimSpace(cfg.Name) == "" {
		errs = append(errs, "name: required")
	}

	hasURL := strings.TrimSpace(cfg.URL) != ""
	hasURLs := len(cfg.URLs) > 0

	if !hasURL && !hasURLs {
		errs = append(errs, "url: required (set url or urls)")
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

	if cfg.Tier < 0 || cfg.Tier > 3 {
		errs = append(errs, fmt.Sprintf("tier: must be 0, 1, 2, or 3, got %d", cfg.Tier))
	}

	if cfg.TrustLevel != 0 && (cfg.TrustLevel < 1 || cfg.TrustLevel > 10) {
		errs = append(errs, fmt.Sprintf("trust_level: must be 1-10, got %d", cfg.TrustLevel))
	}

	if cfg.Tier == 1 && strings.TrimSpace(cfg.Selectors.EventList) == "" {
		errs = append(errs, "selectors.event_list: required for tier 1")
	}

	if cfg.Tier == 2 && strings.TrimSpace(cfg.Headless.WaitSelector) == "" &&
		strings.TrimSpace(cfg.Selectors.EventList) == "" {
		errs = append(errs, "tier 2 requires either headless.wait_selector or selectors.event_list")
	}

	if cfg.Tier == 3 {
		if cfg.GraphQL == nil {
			errs = append(errs, "tier 3 requires a graphql config block")
		} else {
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
				if _, err := template.New("url_template").Parse(t); err != nil {
					errs = append(errs, fmt.Sprintf("graphql.url_template: invalid Go template: %v", err))
				}
			}
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

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
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

		if err := ValidateConfig(cfg); err != nil {
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

	return cfg, nil
}
