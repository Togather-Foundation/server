package scraper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeYAML writes content to a file named fname inside dir.
func writeYAML(t *testing.T, dir, fname, content string) string {
	t.Helper()
	path := filepath.Join(dir, fname)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// --------------------------------------------------------------------------
// ValidateConfig
// --------------------------------------------------------------------------

func TestValidateConfig(t *testing.T) {
	validTier0 := SourceConfig{
		Name:       "Test Source",
		URL:        "https://example.com/events",
		Tier:       0,
		TrustLevel: 5,
		MaxPages:   10,
		Schedule:   "daily",
		Enabled:    true,
	}

	validTier1 := SourceConfig{
		Name:       "Selector Source",
		URL:        "https://example.com/events",
		Tier:       1,
		TrustLevel: 7,
		MaxPages:   5,
		Schedule:   "weekly",
		Enabled:    true,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
		},
	}

	tests := []struct {
		name    string
		cfg     SourceConfig
		wantErr string // empty means no error expected; substring match
	}{
		{
			name: "valid tier 0 config",
			cfg:  validTier0,
		},
		{
			name: "valid tier 1 config with selectors",
			cfg:  validTier1,
		},
		{
			name:    "missing name",
			cfg:     func() SourceConfig { c := validTier0; c.Name = ""; return c }(),
			wantErr: "name: required",
		},
		{
			name:    "empty name whitespace",
			cfg:     func() SourceConfig { c := validTier0; c.Name = "   "; return c }(),
			wantErr: "name: required",
		},
		{
			name:    "missing URL",
			cfg:     func() SourceConfig { c := validTier0; c.URL = ""; return c }(),
			wantErr: "url: required",
		},
		{
			name:    "invalid URL scheme ftp",
			cfg:     func() SourceConfig { c := validTier0; c.URL = "ftp://example.com"; return c }(),
			wantErr: "url: must be a valid http/https URL",
		},
		{
			name:    "invalid URL not parseable",
			cfg:     func() SourceConfig { c := validTier0; c.URL = "not a url"; return c }(),
			wantErr: "url: must be a valid http/https URL",
		},
		{
			name: "valid tier 2",
			cfg: func() SourceConfig {
				c := validTier0
				c.Tier = 2
				c.Headless.WaitSelector = "body"
				c.Selectors.EventList = ".event-card"
				return c
			}(),
			wantErr: "",
		},
		{
			name: "valid tier 2 with wait_selector but no event_list — should fail (srv-wgb5p)",
			cfg: SourceConfig{
				Name:       "Headless Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Headless: HeadlessConfig{
					WaitSelector: ".events",
				},
			},
			wantErr: "selectors.event_list: required for tier 2",
		},
		{
			name: "valid tier 2 with selectors.event_list",
			cfg: SourceConfig{
				Name:       "Headless Selector Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Selectors: SelectorConfig{
					EventList: ".events",
				},
			},
			wantErr: "",
		},
		{
			name: "invalid tier 2 missing event_list",
			cfg: SourceConfig{
				Name:       "Bad Headless Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
			},
			wantErr: "selectors.event_list: required for tier 2",
		},
		{
			name:    "invalid tier negative",
			cfg:     func() SourceConfig { c := validTier0; c.Tier = -1; return c }(),
			wantErr: "tier: must be 0, 1, 2, or 3",
		},
		{
			name:    "invalid tier 4",
			cfg:     func() SourceConfig { c := validTier0; c.Tier = 4; return c }(),
			wantErr: "tier: must be 0, 1, 2, or 3",
		},
		{
			name:    "invalid trust_level 11",
			cfg:     func() SourceConfig { c := validTier0; c.TrustLevel = 11; return c }(),
			wantErr: "trust_level: must be 1-10",
		},
		{
			name:    "invalid trust_level 0 is allowed (default)",
			cfg:     func() SourceConfig { c := validTier0; c.TrustLevel = 0; return c }(),
			wantErr: "", // 0 means use default, not an error at validation time
		},
		{
			name:    "tier 1 without selectors",
			cfg:     func() SourceConfig { c := validTier0; c.Tier = 1; return c }(),
			wantErr: "selectors.event_list: required for tier 1",
		},
		{
			name:    "invalid schedule",
			cfg:     func() SourceConfig { c := validTier0; c.Schedule = "hourly"; return c }(),
			wantErr: "schedule: must be daily, weekly, or manual",
		},
		{
			name: "empty schedule is allowed",
			cfg:  func() SourceConfig { c := validTier0; c.Schedule = ""; return c }(),
		},
		{
			name:    "negative max_pages",
			cfg:     func() SourceConfig { c := validTier0; c.MaxPages = -1; return c }(),
			wantErr: "max_pages: must be > 0",
		},
		{
			name: "zero max_pages is allowed (default applied before validation)",
			cfg:  func() SourceConfig { c := validTier0; c.MaxPages = 0; return c }(),
		},
		// Multi-URL (srv-71948)
		{
			name: "valid with urls only (no url field)",
			cfg: SourceConfig{
				Name:       "Multi URL Source",
				URLs:       []string{"https://example.com/events", "https://example.com/workshops"},
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
		},
		{
			name: "valid with both url and urls",
			cfg: SourceConfig{
				Name:       "Both Fields Source",
				URL:        "https://example.com/events",
				URLs:       []string{"https://example.com/workshops"},
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
		},
		{
			name:    "neither url nor urls",
			cfg:     func() SourceConfig { c := validTier0; c.URL = ""; return c }(),
			wantErr: "url: required",
		},
		{
			name: "invalid url in urls slice",
			cfg: SourceConfig{
				Name:       "Bad URLs Source",
				URLs:       []string{"https://example.com/events", "not-a-url"},
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
			wantErr: "urls[1]:",
		},
		// srv-wrjl4: when urls is set, the url field validation is skipped so
		// an invalid url alone should not block an otherwise valid config.
		// srv-d5b70: no warning is asserted here because the warning about this
		// url-skip behaviour lives in the ValidateConfig godoc (not in its return
		// value) — adding a []string warnings return would be a breaking API change.
		{
			name: "url invalid but urls is set — url field skipped",
			cfg: SourceConfig{
				Name:       "Skip URL Validation",
				URL:        "not-a-url",
				URLs:       []string{"https://example.com/events"},
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
			wantErr: "",
		},
		// Tier 3 / GraphQL (srv-wz0h7)
		{
			name: "valid tier 3 with graphql config",
			cfg: SourceConfig{
				Name:       "GraphQL Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Endpoint:   "https://graphql.example.com/",
					Query:      "{ allEvents { title startDate } }",
					EventField: "allEvents",
				},
			},
		},
		{
			name: "tier 3 missing graphql block",
			cfg: SourceConfig{
				Name:       "No GraphQL Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
			wantErr: "tier 3 requires a graphql config block",
		},
		{
			name: "tier 3 graphql missing endpoint",
			cfg: SourceConfig{
				Name:       "No Endpoint Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Query:      "{ allEvents { title } }",
					EventField: "allEvents",
				},
			},
			wantErr: "graphql.endpoint: required",
		},
		{
			name: "tier 3 graphql missing query",
			cfg: SourceConfig{
				Name:       "No Query Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Endpoint:   "https://graphql.example.com/",
					EventField: "allEvents",
				},
			},
			wantErr: "graphql.query: required",
		},
		{
			name: "tier 3 graphql missing event_field",
			cfg: SourceConfig{
				Name:       "No EventField Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Endpoint: "https://graphql.example.com/",
					Query:    "{ allEvents { title } }",
				},
			},
			wantErr: "graphql.event_field: required",
		},
		{
			name: "tier 3 graphql invalid url_template",
			cfg: SourceConfig{
				Name:       "Bad Template Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Endpoint:    "https://graphql.example.com/",
					Query:       "{ allEvents { title } }",
					EventField:  "allEvents",
					URLTemplate: "https://example.com/events/{{.slug", // unclosed action
				},
			},
			wantErr: "graphql.url_template: invalid Go template",
		},
		{
			name: "tier 3 graphql valid url_template",
			cfg: SourceConfig{
				Name:       "Good Template Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Endpoint:    "https://graphql.example.com/",
					Query:       "{ allEvents { title } }",
					EventField:  "allEvents",
					URLTemplate: "https://example.com/events/{{.slug}}",
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// --------------------------------------------------------------------------
// DefaultSourceConfig
// --------------------------------------------------------------------------

func TestDefaultSourceConfig(t *testing.T) {
	cfg := DefaultSourceConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 0, cfg.Tier)
	assert.Equal(t, 5, cfg.TrustLevel)
	assert.Equal(t, 10, cfg.MaxPages)
	assert.Equal(t, "manual", cfg.Schedule)
}

// --------------------------------------------------------------------------
// LoadSourceConfigs
// --------------------------------------------------------------------------

const validTier0YAML = `
name: "Toronto Symphony Orchestra"
url: "https://www.tso.ca/events"
tier: 0
schedule: "daily"
trust_level: 8
license: "CC0-1.0"
enabled: true
max_pages: 5
`

const validTier1YAML = `
name: "Colly Source"
url: "https://example.com/events"
tier: 1
schedule: "weekly"
trust_level: 6
enabled: true
selectors:
  event_list: "div.event-card"
  name: "h2.title"
  start_date: "time[datetime]"
  url: "a.event-link"
`

const missingNameYAML = `
url: "https://example.com/events"
tier: 0
`

const invalidURLYAML = `
name: "Bad URL Source"
url: "not-a-url"
tier: 0
`

const tier1NoSelectorsYAML = `
name: "Missing Selectors"
url: "https://example.com/events"
tier: 1
`

func TestLoadSourceConfigs_NonExistentDir(t *testing.T) {
	configs, err := LoadSourceConfigs("/tmp/does-not-exist-ever-xyzzy")
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestLoadSourceConfigs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestLoadSourceConfigs_ValidTier0(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "tso.yaml", validTier0YAML)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	assert.Equal(t, "Toronto Symphony Orchestra", cfg.Name)
	assert.Equal(t, "https://www.tso.ca/events", cfg.URL)
	assert.Equal(t, 0, cfg.Tier)
	assert.Equal(t, "daily", cfg.Schedule)
	assert.Equal(t, 8, cfg.TrustLevel)
	assert.Equal(t, "CC0-1.0", cfg.License)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 5, cfg.MaxPages)
}

func TestLoadSourceConfigs_ValidTier1(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "colly.yaml", validTier1YAML)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	assert.Equal(t, "Colly Source", cfg.Name)
	assert.Equal(t, 1, cfg.Tier)
	assert.Equal(t, "div.event-card", cfg.Selectors.EventList)
	assert.Equal(t, "h2.title", cfg.Selectors.Name)
	assert.Equal(t, "time[datetime]", cfg.Selectors.StartDate)
	assert.Equal(t, "a.event-link", cfg.Selectors.URL)
}

func TestLoadSourceConfigs_SkipsUnderscoreFiles(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "tso.yaml", validTier0YAML)
	writeYAML(t, dir, "_draft.yaml", missingNameYAML) // invalid but should be skipped

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "Toronto Symphony Orchestra", configs[0].Name)
}

func TestLoadSourceConfigs_SkipsNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "tso.yaml", validTier0YAML)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# sources"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0o644))

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
}

func TestLoadSourceConfigs_InvalidMissingName(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "bad.yaml", missingNameYAML)

	configs, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "name: required")
	assert.Empty(t, configs)
}

func TestLoadSourceConfigs_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "bad.yaml", invalidURLYAML)

	_, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "url:")
}

func TestLoadSourceConfigs_Tier1NoSelectors(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "tier1.yaml", tier1NoSelectorsYAML)

	configs, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "selectors.event_list")
	assert.Empty(t, configs)
}

func TestLoadSourceConfigs_MultipleFiles_InvalidCausesError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "valid.yaml", validTier0YAML)
	invalidPath := writeYAML(t, dir, "invalid.yaml", missingNameYAML)

	_, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	// Error message must include the file path of the invalid file.
	assert.Contains(t, err.Error(), invalidPath)
}

func TestLoadSourceConfigs_DefaultsApplied(t *testing.T) {
	// A minimal valid config without optional fields.
	minimalYAML := `
name: "Minimal Source"
url: "https://example.com"
tier: 0
`
	dir := t.TempDir()
	writeYAML(t, dir, "minimal.yaml", minimalYAML)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	// Defaults should be applied.
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 5, cfg.TrustLevel)
	assert.Equal(t, 10, cfg.MaxPages)
	assert.Equal(t, "manual", cfg.Schedule)
}

func TestLoadSourceConfigs_EnabledDefaultTrue(t *testing.T) {
	// enabled: not specified in YAML — should default to true.
	yamlContent := `
name: "No Enabled Field"
url: "https://example.com"
tier: 0
`
	dir := t.TempDir()
	writeYAML(t, dir, "source.yaml", yamlContent)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.True(t, configs[0].Enabled)
}

func TestLoadSourceConfigs_ExplicitlyDisabled(t *testing.T) {
	yamlContent := `
name: "Disabled Source"
url: "https://example.com"
tier: 0
enabled: false
`
	dir := t.TempDir()
	writeYAML(t, dir, "source.yaml", yamlContent)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.False(t, configs[0].Enabled)
}

func TestLoadSourceConfigs_TrustLevelZeroGetsDefault(t *testing.T) {
	yamlContent := `
name: "No Trust"
url: "https://example.com"
tier: 0
trust_level: 0
`
	dir := t.TempDir()
	writeYAML(t, dir, "source.yaml", yamlContent)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, 5, configs[0].TrustLevel)
}

func TestLoadSourceConfigs_InvalidTrustLevel(t *testing.T) {
	yamlContent := `
name: "Bad Trust"
url: "https://example.com"
tier: 0
trust_level: 11
`
	dir := t.TempDir()
	writeYAML(t, dir, "bad.yaml", yamlContent)

	_, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trust_level")
}

func TestLoadSourceConfigs_SubdirIgnored(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))
	// valid yaml inside subdir — should be ignored
	writeYAML(t, subDir, "sub.yaml", validTier0YAML)
	writeYAML(t, dir, "top.yaml", validTier0YAML)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
}

// TestLoadSourceConfigs_InvalidURLScheme ensures ftp:// URLs fail.
func TestLoadSourceConfigs_InvalidURLScheme(t *testing.T) {
	yamlContent := `
name: "FTP Source"
url: "ftp://example.com/events"
tier: 0
`
	dir := t.TempDir()
	path := writeYAML(t, dir, "ftp.yaml", yamlContent)

	_, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.True(t,
		strings.Contains(err.Error(), "url:"),
		"expected url error, got: %s", err.Error(),
	)
}

func TestLoadSourceConfigs_DuplicateNameError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "a.yaml", validTier0YAML)
	writeYAML(t, dir, "b.yaml", validTier0YAML) // same name: "Toronto Symphony Orchestra"

	_, err := LoadSourceConfigs(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate source name")
	assert.Contains(t, err.Error(), "Toronto Symphony Orchestra")
}

func TestLoadSourceConfigs_MultiSessionDurationThreshold(t *testing.T) {
	t.Parallel()

	t.Run("parses valid duration string from YAML", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Festival Source"
url: "https://example.com/events"
tier: 0
multi_session_duration_threshold: "720h"
`
		dir := t.TempDir()
		writeYAML(t, dir, "festival.yaml", yamlContent)

		configs, err := LoadSourceConfigs(dir)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, "720h", configs[0].MultiSessionDurationThreshold)
	})

	t.Run("empty threshold is valid (uses default)", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Normal Source"
url: "https://example.com/events"
tier: 0
`
		dir := t.TempDir()
		writeYAML(t, dir, "normal.yaml", yamlContent)

		configs, err := LoadSourceConfigs(dir)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, "", configs[0].MultiSessionDurationThreshold)
	})

	t.Run("threshold with skip_multi_session_check false still parses", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Custom Threshold Source"
url: "https://example.com/events"
tier: 0
skip_multi_session_check: false
multi_session_duration_threshold: "336h"
`
		dir := t.TempDir()
		writeYAML(t, dir, "custom.yaml", yamlContent)

		configs, err := LoadSourceConfigs(dir)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, "336h", configs[0].MultiSessionDurationThreshold)
		assert.False(t, configs[0].SkipMultiSessionCheck)
	})
}

func TestParseMultiSessionThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:  "empty string returns zero (use default)",
			input: "",
			want:  0,
		},
		{
			name:  "valid 720h (30 days)",
			input: "720h",
			want:  720 * time.Hour,
		},
		{
			name:  "valid 336h (14 days)",
			input: "336h",
			want:  336 * time.Hour,
		},
		{
			name:  "valid 168h (1 week)",
			input: "168h",
			want:  168 * time.Hour,
		},
		{
			name:  "valid with minutes",
			input: "720h30m",
			want:  720*time.Hour + 30*time.Minute,
		},
		{
			name:    "invalid string returns error",
			input:   "30d",
			wantErr: true,
		},
		{
			name:    "invalid non-duration string returns error",
			input:   "thirty days",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMultiSessionThreshold(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// SourceConfigFromDomain — JSON round-trip (srv-r7nov)
// --------------------------------------------------------------------------

// TestSourceConfigFromDomain_JSONRoundTrip verifies that SelectorConfig
// marshals to snake_case JSON keys and that SourceConfigFromDomain correctly
// reconstructs it from a domain.Source whose Selectors field contains the
// snake_case JSONB payload written by the DB.
//
// This is a regression test for the silent failure fixed in srv-2db1q: without
// json: struct tags, json.Marshal produced PascalCase keys ("EventList"), but
// the DB stored snake_case keys ("event_list"), so Unmarshal silently produced
// a zero-valued SelectorConfig on every read.
func TestSourceConfigFromDomain_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selectors SelectorConfig
	}{
		{
			name: "all fields populated",
			selectors: SelectorConfig{
				EventList:   "div.event-card",
				Name:        "h2.title",
				StartDate:   "time[datetime]",
				EndDate:     "time.end",
				Location:    "span.venue",
				Description: "p.summary",
				URL:         "a.event-link",
				Image:       "img.thumb",
				Pagination:  "a.next-page",
			},
		},
		{
			name: "partial fields (Tier 2 typical)",
			selectors: SelectorConfig{
				EventList: ".eventon_list_event",
				Name:      ".evcal_event_title",
				StartDate: ".evcal_desc2",
				URL:       ".evcal_list_a a",
			},
		},
		{
			name:      "empty (Tier 0 — no selectors)",
			selectors: SelectorConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Marshal the SelectorConfig to JSON (simulates what scrape_sync writes to DB).
			raw, err := json.Marshal(tt.selectors)
			require.NoError(t, err)

			// Verify keys are snake_case, not PascalCase.
			if tt.selectors.EventList != "" {
				assert.Contains(t, string(raw), `"event_list"`,
					"json.Marshal must produce snake_case key event_list")
				assert.NotContains(t, string(raw), `"EventList"`,
					"json.Marshal must not produce PascalCase key EventList")
			}
			if tt.selectors.StartDate != "" {
				assert.Contains(t, string(raw), `"start_date"`)
				assert.NotContains(t, string(raw), `"StartDate"`)
			}

			// Build a domain Source with the marshalled JSONB payload (simulates a DB read).
			src := domainScraper.Source{
				Name:       "test-source",
				URL:        "https://example.com/events",
				Tier:       1,
				TrustLevel: 5,
				License:    "CC0-1.0",
				Enabled:    true,
				MaxPages:   10,
				Schedule:   "weekly",
				Selectors:  raw,
			}

			// SourceConfigFromDomain must reconstruct the original SelectorConfig.
			cfg, err := SourceConfigFromDomain(src)
			require.NoError(t, err)
			assert.Equal(t, tt.selectors, cfg.Selectors,
				"SourceConfigFromDomain must round-trip SelectorConfig through JSON")
		})
	}
}

// TestSourceConfigFromDomain_EmptySelectors verifies that a nil/empty Selectors
// field (Tier 0 source) produces a zero SelectorConfig without error.
func TestSourceConfigFromDomain_EmptySelectors(t *testing.T) {
	t.Parallel()

	src := domainScraper.Source{
		Name:      "tier0-source",
		URL:       "https://example.com/events",
		Tier:      0,
		Selectors: nil,
	}

	cfg, err := SourceConfigFromDomain(src)
	require.NoError(t, err)
	assert.Equal(t, SelectorConfig{}, cfg.Selectors)
}

// TestSourceConfigFromDomain_InvalidJSON verifies that malformed JSONB in the
// Selectors field returns a wrapped error rather than silently zero-initialising.
func TestSourceConfigFromDomain_InvalidJSON(t *testing.T) {
	t.Parallel()

	src := domainScraper.Source{
		Name:      "bad-source",
		URL:       "https://example.com/events",
		Tier:      1,
		Selectors: []byte(`{not valid json`),
	}

	_, err := SourceConfigFromDomain(src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad-source")
}

// TestSourceConfigFromDomain_GraphQLRoundTrip verifies that a Tier 3 GraphQL
// config is correctly JSON-encoded on sync (sourceConfigToUpsertParams path) and
// decoded back by SourceConfigFromDomain (load path), preserving all fields.
func TestSourceConfigFromDomain_GraphQLRoundTrip(t *testing.T) {
	t.Parallel()

	original := &GraphQLConfig{
		Endpoint:    "https://graphql.datocms.com/",
		Token:       "abc123token",
		Query:       "{ allEvents { title slug } }",
		EventField:  "allEvents",
		TimeoutMs:   30000,
		URLTemplate: "https://example.com/events/{{.slug}}",
	}

	// Simulate what sourceConfigToUpsertParams does: JSON-encode for DB.
	rawJSON, err := json.Marshal(original)
	require.NoError(t, err)

	src := domainScraper.Source{
		Name:          "graphql-test",
		URL:           "https://example.com/calendar",
		Tier:          3,
		TrustLevel:    7,
		License:       "CC0-1.0",
		Enabled:       true,
		MaxPages:      10,
		Schedule:      "daily",
		GraphQLConfig: rawJSON,
	}

	cfg, err := SourceConfigFromDomain(src)
	require.NoError(t, err)
	require.NotNil(t, cfg.GraphQL, "GraphQL config must be non-nil for Tier 3 source")
	assert.Equal(t, original.Endpoint, cfg.GraphQL.Endpoint)
	assert.Equal(t, original.Token, cfg.GraphQL.Token)
	assert.Equal(t, original.Query, cfg.GraphQL.Query)
	assert.Equal(t, original.EventField, cfg.GraphQL.EventField)
	assert.Equal(t, original.TimeoutMs, cfg.GraphQL.TimeoutMs)
	assert.Equal(t, original.URLTemplate, cfg.GraphQL.URLTemplate)
}

// TestSourceConfigFromDomain_GraphQLNilForTier0 verifies that a Tier 0 source
// with no graphql_config produces a nil GraphQL field (no panic or error).
func TestSourceConfigFromDomain_GraphQLNilForTier0(t *testing.T) {
	t.Parallel()

	src := domainScraper.Source{
		Name:          "tier0-source",
		URL:           "https://example.com/events",
		Tier:          0,
		GraphQLConfig: nil,
	}

	cfg, err := SourceConfigFromDomain(src)
	require.NoError(t, err)
	assert.Nil(t, cfg.GraphQL, "GraphQL config must be nil for Tier 0 source")
}

// TestSourceConfigFromDomain_GraphQLInvalidJSON verifies that malformed
// graphql_config JSONB returns a wrapped error.
func TestSourceConfigFromDomain_GraphQLInvalidJSON(t *testing.T) {
	t.Parallel()

	src := domainScraper.Source{
		Name:          "bad-graphql-source",
		URL:           "https://example.com/calendar",
		Tier:          3,
		GraphQLConfig: []byte(`{not valid json`),
	}

	_, err := SourceConfigFromDomain(src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad-graphql-source")
}

// --------------------------------------------------------------------------
// GetURLs
// --------------------------------------------------------------------------

func TestSourceConfig_GetURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  SourceConfig
		want []string
	}{
		{
			name: "url only returns single-element slice",
			cfg:  SourceConfig{URL: "https://example.com/events"},
			want: []string{"https://example.com/events"},
		},
		{
			name: "urls only returns urls slice",
			cfg:  SourceConfig{URLs: []string{"https://example.com/a", "https://example.com/b"}},
			want: []string{"https://example.com/a", "https://example.com/b"},
		},
		{
			name: "urls takes precedence when both set",
			cfg:  SourceConfig{URL: "https://example.com/events", URLs: []string{"https://example.com/a"}},
			want: []string{"https://example.com/a"},
		},
		{
			name: "neither returns nil",
			cfg:  SourceConfig{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.cfg.GetURLs()
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// LoadSourceConfigs — urls field
// --------------------------------------------------------------------------

func TestLoadSourceConfigs_URLsField(t *testing.T) {
	t.Parallel()

	yamlContent := `
name: "Multi URL Source"
urls:
  - "https://example.com/events"
  - "https://example.com/workshops"
tier: 0
`
	dir := t.TempDir()
	writeYAML(t, dir, "multi.yaml", yamlContent)

	configs, err := LoadSourceConfigs(dir)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, []string{"https://example.com/events", "https://example.com/workshops"}, configs[0].URLs)
}
