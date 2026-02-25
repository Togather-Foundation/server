package scraper

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SourceConfig defines a scrape source loaded from a YAML config file.
type SourceConfig struct {
	Name            string         `yaml:"name"`
	URL             string         `yaml:"url"`
	Tier            int            `yaml:"tier"`
	Schedule        string         `yaml:"schedule"`
	TrustLevel      int            `yaml:"trust_level"`
	License         string         `yaml:"license"`
	Enabled         bool           `yaml:"enabled"`
	EventURLPattern string         `yaml:"event_url_pattern"`
	MaxPages        int            `yaml:"max_pages"`
	Notes           string         `yaml:"notes,omitempty"`
	Selectors       SelectorConfig `yaml:"selectors"`
	// FollowEventURLs instructs the Tier 0 scraper to fetch each event's detail
	// page to retrieve the full description when the JSON-LD description appears
	// truncated (e.g. Tribe Events WordPress sources always truncate).
	FollowEventURLs bool `yaml:"follow_event_urls"`
	// SkipMultiSessionCheck disables the multi-session event heuristic for this
	// source. Use for sources that legitimately emit long-duration single events
	// (e.g., festivals, art installations).
	SkipMultiSessionCheck bool `yaml:"skip_multi_session_check"`
	// MultiSessionDurationThreshold overrides the default 168h (1 week) duration
	// threshold used by the multi-session heuristic. Use for sources that
	// legitimately publish events longer than 1 week but shorter than 30 days
	// (e.g., festivals spanning multiple weeks). Value is a Go duration string
	// like "720h" (30 days). Zero value means use the default (168h).
	MultiSessionDurationThreshold string `yaml:"multi_session_duration_threshold,omitempty"`
	// Headless holds Tier 2 headless-browser options. Ignored for tier 0/1.
	Headless HeadlessConfig `yaml:"headless,omitempty"`
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
	WaitSelector string `yaml:"wait_selector"`
	// WaitTimeoutMs is the maximum time (ms) to wait for WaitSelector.
	// 0 means use the RodExtractor default (10 000 ms).
	WaitTimeoutMs int `yaml:"wait_timeout_ms"`
	// PaginationBtn is a CSS selector for a "load more" / "next" button to click
	// for JS-rendered pagination. Empty means no pagination.
	PaginationBtn string `yaml:"pagination_button"`
	// Headers are extra HTTP headers injected into every browser request.
	Headers map[string]string `yaml:"headers,omitempty"`
	// RateLimitMs overrides the per-domain delay between page loads (ms).
	// 0 means use the RodExtractor default.
	RateLimitMs int `yaml:"rate_limit_ms"`
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

	if strings.TrimSpace(cfg.URL) == "" {
		errs = append(errs, "url: required")
	} else {
		u, err := url.Parse(cfg.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Sprintf("url: must be a valid http/https URL, got %q", cfg.URL))
		}
	}

	if cfg.Tier < 0 || cfg.Tier > 2 {
		errs = append(errs, fmt.Sprintf("tier: must be 0, 1, or 2, got %d", cfg.Tier))
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
