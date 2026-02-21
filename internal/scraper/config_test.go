package scraper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			name:    "invalid tier 2",
			cfg:     func() SourceConfig { c := validTier0; c.Tier = 2; return c }(),
			wantErr: "tier: must be 0 or 1",
		},
		{
			name:    "invalid tier negative",
			cfg:     func() SourceConfig { c := validTier0; c.Tier = -1; return c }(),
			wantErr: "tier: must be 0 or 1",
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
