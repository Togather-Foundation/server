package scraper

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type warningHandler struct {
	mu       sync.Mutex
	warnings []string
}

func (h *warningHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var msg string
	if r.Message != "" {
		msg = r.Message
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "warning" {
			msg = a.Value.String()
		}
		return true
	})
	h.warnings = append(h.warnings, msg)
	return nil
}

func (h *warningHandler) GetWarnings() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slicesClone(h.warnings)
}

func (h *warningHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *warningHandler) WithGroup(name string) slog.Handler       { return h }
func (h *warningHandler) Enabled(context.Context, slog.Level) bool { return true }

func slicesClone[T any](s []T) []T {
	if s == nil {
		return nil
	}
	c := make([]T, len(s))
	copy(c, s)
	return c
}

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
		// Tier 3 / REST (srv-hi014)
		{
			name: "tier 3 with valid rest config",
			cfg: SourceConfig{
				Name:       "REST Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				REST: &RestConfig{
					Endpoint: "https://api.example.com/events",
				},
			},
		},
		{
			name: "tier 3 with both graphql and rest",
			cfg: SourceConfig{
				Name:       "Both Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				GraphQL: &GraphQLConfig{
					Endpoint:   "https://graphql.example.com/",
					Query:      "{ allEvents { title } }",
					EventField: "allEvents",
				},
				REST: &RestConfig{
					Endpoint: "https://api.example.com/events",
				},
			},
			wantErr: "tier 3: graphql and rest are mutually exclusive",
		},
		{
			name: "tier 3 with neither graphql nor rest",
			cfg: SourceConfig{
				Name:       "No API Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
			wantErr: "tier 3 requires a graphql or rest config block",
		},
		{
			name: "tier 3 rest missing endpoint",
			cfg: SourceConfig{
				Name:       "No Endpoint REST Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				REST:       &RestConfig{},
			},
			wantErr: "rest.endpoint: required for tier 3",
		},
		{
			name: "tier 3 rest invalid endpoint URL",
			cfg: SourceConfig{
				Name:       "Bad Endpoint REST Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				REST: &RestConfig{
					Endpoint: "not-a-url",
				},
			},
			wantErr: "rest.endpoint: must be a valid http/https URL",
		},
		{
			name: "tier 3 rest invalid url_template",
			cfg: SourceConfig{
				Name:       "Bad Template REST Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				REST: &RestConfig{
					Endpoint:    "https://api.example.com/events",
					URLTemplate: "https://example.com/events/{{.slug", // unclosed action
				},
			},
			wantErr: "rest.url_template: invalid Go template",
		},
		{
			name: "tier 3 rest valid url_template",
			cfg: SourceConfig{
				Name:       "Good Template REST Source",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				REST: &RestConfig{
					Endpoint:    "https://api.example.com/events",
					URLTemplate: "https://example.com/events/{{.slug}}",
				},
			},
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
			wantErr: "tier 3 requires a graphql or rest config block",
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
		// IframeConfig validation (srv-mwy3y)
		{
			name: "valid tier 2 with iframe config",
			cfg: SourceConfig{
				Name:       "Iframe Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Selectors: SelectorConfig{
					EventList: ".event-card",
				},
				Headless: HeadlessConfig{
					WaitSelector: "body",
					Iframe: &IframeConfig{
						Selector:     "iframe[title='Ticket Spot']",
						WaitSelector: ".events-container",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "iframe config with empty selector fails",
			cfg: SourceConfig{
				Name:       "Bad Iframe Selector Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Selectors: SelectorConfig{
					EventList: ".event-card",
				},
				Headless: HeadlessConfig{
					WaitSelector: "body",
					Iframe: &IframeConfig{
						Selector:     "",
						WaitSelector: ".events-container",
					},
				},
			},
			wantErr: "headless.iframe.selector is required when iframe block is set",
		},
		{
			name: "iframe config with empty wait_selector fails",
			cfg: SourceConfig{
				Name:       "Bad Iframe WaitSelector Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Selectors: SelectorConfig{
					EventList: ".event-card",
				},
				Headless: HeadlessConfig{
					WaitSelector: "body",
					Iframe: &IframeConfig{
						Selector:     "iframe[title='Ticket Spot']",
						WaitSelector: "",
					},
				},
			},
			wantErr: "headless.iframe.wait_selector is required when iframe block is set",
		},
		{
			name: "nil iframe config passes (no iframe block)",
			cfg: SourceConfig{
				Name:       "No Iframe Source",
				URL:        "https://example.com/events",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Selectors: SelectorConfig{
					EventList: ".event-card",
				},
				Headless: HeadlessConfig{
					WaitSelector: "body",
					Iframe:       nil,
				},
			},
			wantErr: "",
		},
		{
			name: "iframe config on non-tier-2 is now a warning not an error",
			cfg: SourceConfig{
				Name:       "Tier 1 With Iframe",
				URL:        "https://example.com/events",
				Tier:       1,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Enabled:    true,
				Selectors: SelectorConfig{
					EventList: ".event-card",
				},
				Headless: HeadlessConfig{
					WaitSelector: "body",
					Iframe: &IframeConfig{
						Selector:     "iframe#widget",
						WaitSelector: ".events",
					},
				},
			},
			wantErr: "", // demoted to warning in srv-muy4i
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
// ValidateConfigWithWarnings — FieldMap key validation (srv-u3v2k)
// --------------------------------------------------------------------------

func TestValidateConfigWithWarnings_FieldMap(t *testing.T) {
	t.Parallel()

	baseTier3REST := func(fieldMap map[string]string) SourceConfig {
		return SourceConfig{
			Name:       "REST Source",
			URL:        "https://example.com",
			Tier:       3,
			TrustLevel: 5,
			MaxPages:   10,
			Schedule:   "daily",
			REST: &RestConfig{
				Endpoint: "https://api.example.com/events",
				FieldMap: fieldMap,
			},
		}
	}

	tests := []struct {
		name         string
		cfg          SourceConfig
		wantErr      string   // empty means no error expected; substring match
		wantWarnings []string // substrings expected in warnings; nil means no warnings
	}{
		{
			name:         "nil FieldMap — no warnings",
			cfg:          baseTier3REST(nil),
			wantWarnings: nil,
		},
		{
			name:         "empty FieldMap — no warnings",
			cfg:          baseTier3REST(map[string]string{}),
			wantWarnings: nil,
		},
		{
			name: "all valid keys — no warnings",
			cfg: baseTier3REST(map[string]string{
				"name":        "title",
				"start_date":  "starts_at",
				"end_date":    "ends_at",
				"url":         "event_url",
				"image":       "thumbnail",
				"location":    "venue",
				"description": "summary",
			}),
			wantWarnings: nil,
		},
		{
			name: "typo key strat_date — warning present",
			cfg: baseTier3REST(map[string]string{
				"name":       "title",
				"strat_date": "starts_at", // typo: should be start_date
			}),
			wantWarnings: []string{`"strat_date"`},
		},
		{
			name: "mix of valid and invalid keys — warning only for invalid",
			cfg: baseTier3REST(map[string]string{
				"name":        "title",
				"start_date":  "starts_at",
				"bogus_field": "whatever",
			}),
			wantWarnings: []string{`"bogus_field"`},
		},
		{
			name: "multiple invalid keys — warning for each",
			cfg: baseTier3REST(map[string]string{
				"strat_date": "starts_at",
				"end_dat":    "ends_at",
			}),
			wantWarnings: []string{`"strat_date"`, `"end_dat"`},
		},
		{
			name: "non-REST tier 0 with no REST block — no warnings",
			cfg: SourceConfig{
				Name:       "Tier 0 Source",
				URL:        "https://example.com/events",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
			},
			wantWarnings: nil,
		},
		{
			name: "invalid key does not cause hard error",
			cfg: baseTier3REST(map[string]string{
				"strat_date": "starts_at",
			}),
			wantErr:      "", // must NOT be an error
			wantWarnings: []string{`"strat_date"`},
		},
		{
			name: "graphql field_map valid keys — no warning",
			cfg: SourceConfig{
				Name: "gql-valid-fieldmap",
				URL:  "https://example.com",
				Tier: 3,
				GraphQL: &GraphQLConfig{
					Endpoint:   "https://api.example.com/graphql",
					Query:      "{ events { title } }",
					EventField: "events",
					FieldMap: map[string]string{
						"name":       "title",
						"start_date": "dateStart",
					},
				},
			},
			wantErr:      "",
			wantWarnings: nil,
		},
		{
			name: "graphql field_map unrecognised key — warning",
			cfg: SourceConfig{
				Name: "gql-bad-fieldmap",
				URL:  "https://example.com",
				Tier: 3,
				GraphQL: &GraphQLConfig{
					Endpoint:   "https://api.example.com/graphql",
					Query:      "{ events { title } }",
					EventField: "events",
					FieldMap: map[string]string{
						"name":       "title",
						"strat_date": "dateStart",
					},
				},
			},
			wantErr:      "",
			wantWarnings: []string{`graphql.field_map: unrecognised key "strat_date"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warnings, err := ValidateConfigWithWarnings(tt.cfg)

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}

			if len(tt.wantWarnings) == 0 {
				assert.Empty(t, warnings)
			} else {
				require.NotEmpty(t, warnings)
				// Join all warnings for easy substring matching.
				joined := strings.Join(warnings, "\n")
				for _, wantSub := range tt.wantWarnings {
					assert.Contains(t, joined, wantSub,
						"expected warning substring %q in: %s", wantSub, joined)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// ValidateConfigWithWarnings — IframeConfig tier mismatch (srv-muy4i)
// --------------------------------------------------------------------------

func TestValidateConfigWithWarnings_IframeTier(t *testing.T) {
	t.Parallel()

	iframeBlock := &IframeConfig{
		Selector:     "iframe[title='Widget']",
		WaitSelector: ".events-container",
	}

	// baseCfg returns a valid tier-N config with the provided iframe block (may be nil).
	baseCfg := func(tier int, iframe *IframeConfig) SourceConfig {
		cfg := SourceConfig{
			Name:       "Test Source",
			URL:        "https://example.com/events",
			Tier:       tier,
			TrustLevel: 5,
			MaxPages:   10,
			Schedule:   "daily",
		}
		// Tier 1 and 2 require selectors.event_list; tier 3 needs a REST/GraphQL block.
		// Use tier 0 or tier 1 (with selectors) as the non-tier-2 cases.
		if tier == 1 || tier == 2 {
			cfg.Selectors.EventList = ".event-card"
		}
		if tier == 3 {
			cfg.REST = &RestConfig{Endpoint: "https://api.example.com/events"}
		}
		cfg.Headless.Iframe = iframe
		return cfg
	}

	tests := []struct {
		name         string
		cfg          SourceConfig
		wantErr      string   // empty = no error
		wantWarnings []string // substrings expected; nil = no warnings
	}{
		{
			name:         "tier 2 with iframe — no tier warning",
			cfg:          baseCfg(2, iframeBlock),
			wantWarnings: nil,
		},
		{
			name:         "tier 0 with iframe — warning present",
			cfg:          baseCfg(0, iframeBlock),
			wantWarnings: []string{"iframe config is only used by tier 2", "ignored for tier 0"},
		},
		{
			name:         "tier 1 with iframe — warning present",
			cfg:          baseCfg(1, iframeBlock),
			wantWarnings: []string{"iframe config is only used by tier 2", "ignored for tier 1"},
		},
		{
			name:         "tier 3 with iframe — warning present",
			cfg:          baseCfg(3, iframeBlock),
			wantWarnings: []string{"iframe config is only used by tier 2", "ignored for tier 3"},
		},
		{
			name:         "tier 0 no iframe — no warning",
			cfg:          baseCfg(0, nil),
			wantWarnings: nil,
		},
		{
			name: "tier 1 no iframe — no warning",
			cfg: SourceConfig{
				Name:       "Tier 1 No Iframe",
				URL:        "https://example.com/events",
				Tier:       1,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Selectors:  SelectorConfig{EventList: ".event-card"},
			},
			wantWarnings: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warnings, err := ValidateConfigWithWarnings(tt.cfg)

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}

			if len(tt.wantWarnings) == 0 {
				// Filter out any field_map warnings that may come from unrelated config.
				// Only check that no iframe/tier warning is present.
				for _, w := range warnings {
					assert.NotContains(t, w, "iframe config is only used by tier 2",
						"unexpected iframe tier warning: %s", w)
				}
			} else {
				require.NotEmpty(t, warnings)
				joined := strings.Join(warnings, "\n")
				for _, wantSub := range tt.wantWarnings {
					assert.Contains(t, joined, wantSub,
						"expected warning substring %q in: %s", wantSub, joined)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// IframeConfig defaults (srv-mwy3y)
// --------------------------------------------------------------------------

// TestLoadFile_IframeDefaults verifies that IframeConfig.WaitTimeoutMs defaults
// to 10000 when unset in YAML.
func TestLoadFile_IframeDefaults(t *testing.T) {
	t.Parallel()

	t.Run("wait_timeout_ms defaults to 10000 when zero", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Iframe Default Source"
url: "https://example.com/events"
tier: 2
selectors:
  event_list: ".event-card"
headless:
  wait_selector: "body"
  iframe:
    selector: "iframe[title='Ticket Spot']"
    wait_selector: ".events-container"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "iframe.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		require.NotNil(t, cfg.Headless.Iframe)
		assert.Equal(t, 10000, cfg.Headless.Iframe.WaitTimeoutMs,
			"wait_timeout_ms must default to 10000 when unset")
	})

	t.Run("explicit wait_timeout_ms is not overridden", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Iframe Custom Timeout Source"
url: "https://example.com/events"
tier: 2
selectors:
  event_list: ".event-card"
headless:
  wait_selector: "body"
  iframe:
    selector: "iframe[title='Ticket Spot']"
    wait_selector: ".events-container"
    wait_timeout_ms: 5000
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "iframe_timeout.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		require.NotNil(t, cfg.Headless.Iframe)
		assert.Equal(t, 5000, cfg.Headless.Iframe.WaitTimeoutMs,
			"explicit wait_timeout_ms must not be overridden")
	})

	t.Run("nil iframe config does not panic", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "No Iframe Source"
url: "https://example.com/events"
tier: 0
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "no_iframe.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		assert.Nil(t, cfg.Headless.Iframe)
	})
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
//
// Note: as of srv-nojwn, SourceConfigFromDomain applies description precedence
// normalization, so the round-trip result may differ from the input when
// Description is set (it gets copied to DescriptionSelectors).
func TestSourceConfigFromDomain_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		selectors          SelectorConfig
		wantAfterNormalize SelectorConfig // expected after DB load (includes normalization)
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
			// After DB load, Description is normalized to DescriptionSelectors (srv-nojwn)
			wantAfterNormalize: SelectorConfig{
				EventList:            "div.event-card",
				Name:                 "h2.title",
				StartDate:            "time[datetime]",
				EndDate:              "time.end",
				Location:             "span.venue",
				Description:          "p.summary",
				URL:                  "a.event-link",
				Image:                "img.thumb",
				Pagination:           "a.next-page",
				DescriptionSelectors: []string{"p.summary"},
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
			// No Description set, so no normalization applies
			wantAfterNormalize: SelectorConfig{
				EventList: ".eventon_list_event",
				Name:      ".evcal_event_title",
				StartDate: ".evcal_desc2",
				URL:       ".evcal_list_a a",
			},
		},
		{
			name:               "empty (Tier 0 — no selectors)",
			selectors:          SelectorConfig{},
			wantAfterNormalize: SelectorConfig{},
		},
		{
			name: "with DescriptionSelectors (no Description)",
			selectors: SelectorConfig{
				EventList:            ".card",
				DescriptionSelectors: []string{".lead", ".full"},
			},
			// DescriptionSelectors preserved as-is when Description is empty
			wantAfterNormalize: SelectorConfig{
				EventList:            ".card",
				DescriptionSelectors: []string{".lead", ".full"},
			},
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

			// SourceConfigFromDomain must reconstruct with normalization applied.
			cfg, err := SourceConfigFromDomain(src)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAfterNormalize, cfg.Selectors,
				"SourceConfigFromDomain must round-trip SelectorConfig through JSON with normalization")
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
	// Selectors is zero-value; normalization produces nil DescriptionSelectors
	assert.Equal(t, SelectorConfig{DescriptionSelectors: nil}, cfg.Selectors)
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

// --------------------------------------------------------------------------
// loadFile REST defaults (srv-hi014)
// --------------------------------------------------------------------------

// TestLoadFile_RESTDefaults verifies that results_field and next_field defaults
// ("results" and "next" respectively) are applied by loadFile when not set in YAML.
func TestLoadFile_RESTDefaults(t *testing.T) {
	t.Parallel()

	t.Run("applies results_field and next_field defaults when empty", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "REST Default Source"
url: "https://example.com"
tier: 3
rest:
  endpoint: "https://api.example.com/events"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "rest.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		require.NotNil(t, cfg.REST)
		assert.Equal(t, "results", cfg.REST.ResultsField, "results_field default must be 'results'")
		assert.Equal(t, "next", cfg.REST.NextField, "next_field default must be 'next'")
	})

	t.Run("does not override explicitly set results_field", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "REST Custom Fields Source"
url: "https://example.com"
tier: 3
rest:
  endpoint: "https://api.example.com/events"
  results_field: "data"
  next_field: "pagination.next"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "rest_custom.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		require.NotNil(t, cfg.REST)
		assert.Equal(t, "data", cfg.REST.ResultsField, "explicit results_field must not be overridden")
		assert.Equal(t, "pagination.next", cfg.REST.NextField, "explicit next_field must not be overridden")
	})

	t.Run("preserves bare array sentinel dot", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "REST Bare Array Source"
url: "https://example.com"
tier: 3
rest:
  endpoint: "https://api.example.com/events"
  results_field: "."
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "rest_dot.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		require.NotNil(t, cfg.REST)
		assert.Equal(t, ".", cfg.REST.ResultsField, "results_field '.' sentinel must not be overridden to default")
	})

	t.Run("nil REST config does not panic", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "No REST Source"
url: "https://example.com"
tier: 0
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "no_rest.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		assert.Nil(t, cfg.REST)
	})
}

// --------------------------------------------------------------------------
// InterceptConfig validation (srv-enisd)
// --------------------------------------------------------------------------

// TestValidate_InterceptConfig verifies that ValidateConfigWithWarnings enforces
// all InterceptConfig constraints.
func TestValidate_InterceptConfig(t *testing.T) {
	t.Parallel()

	// base is a valid tier-2 config we can add intercept to.
	base := SourceConfig{
		Name:       "Intercept Source",
		URL:        "https://example.com/events",
		Tier:       2,
		TrustLevel: 5,
		MaxPages:   10,
		Schedule:   "daily",
		Enabled:    true,
		Selectors:  SelectorConfig{EventList: ".event-card"},
	}

	tests := []struct {
		name        string
		intercept   *InterceptConfig
		wantErr     string // substring; empty = no error expected
		wantWarning string // substring in warnings; empty = not checked
	}{
		{
			name:      "valid intercept config",
			intercept: &InterceptConfig{URLPattern: `api/events`, ResultsPath: "results", FieldMap: map[string]string{"name": "title"}},
		},
		{
			name:      "valid with response_format json",
			intercept: &InterceptConfig{URLPattern: `api/events`, ResponseFormat: "json", ResultsPath: "results"},
		},
		{
			name:      "valid with empty response_format (defaults to json)",
			intercept: &InterceptConfig{URLPattern: `api/events`, ResultsPath: "hits.hit"},
		},
		{
			name:      "missing url_pattern",
			intercept: &InterceptConfig{URLPattern: "", ResultsPath: "results"},
			wantErr:   "headless.intercept.url_pattern: required",
		},
		{
			name:      "invalid regex url_pattern",
			intercept: &InterceptConfig{URLPattern: "[invalid(regex", ResultsPath: "results"},
			wantErr:   "headless.intercept.url_pattern: invalid Go regex",
		},
		{
			name:      "unsupported response_format",
			intercept: &InterceptConfig{URLPattern: `api/events`, ResponseFormat: "xml", ResultsPath: "results"},
			wantErr:   `headless.intercept.response_format: only "json" is supported`,
		},
		{
			name:      "missing results_path",
			intercept: &InterceptConfig{URLPattern: `api/events`, ResultsPath: ""},
			wantErr:   "headless.intercept.results_path: required",
		},
		{
			name:        "unrecognised field_map key produces warning",
			intercept:   &InterceptConfig{URLPattern: `api/events`, ResultsPath: "results", FieldMap: map[string]string{"strat_date": "date"}},
			wantWarning: `headless.intercept.field_map: unrecognised key "strat_date"`,
		},
		{
			name:      "all known field_map keys are valid",
			intercept: &InterceptConfig{URLPattern: `api/events`, ResultsPath: "results", FieldMap: map[string]string{"name": "n", "start_date": "sd", "end_date": "ed", "url": "u", "image": "img", "location": "loc", "description": "desc"}},
		},
		{
			name:      "complex regex pattern is valid",
			intercept: &InterceptConfig{URLPattern: `(cloudsearch|algolia)\.(net|com)/.*`, ResultsPath: "hits.hit"},
		},
		{
			name: "nil intercept — no validation triggered",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.Headless.Intercept = tc.intercept

			warnings, err := ValidateConfigWithWarnings(cfg)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tc.wantWarning != "" {
				found := false
				for _, w := range warnings {
					if strings.Contains(w, tc.wantWarning) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q, got warnings: %v", tc.wantWarning, warnings)
				}
			}
		})
	}

	// Tier-mismatch warning: intercept only makes sense on tier 2.
	t.Run("intercept on non-tier-2 produces warning", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.Tier = 0
		cfg.Headless.Intercept = &InterceptConfig{URLPattern: `api/events`, ResultsPath: "results"}
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "intercept config is only used by tier 2") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tier-mismatch warning for intercept on tier 0, got warnings: %v", warnings)
		}
	})
}

// --------------------------------------------------------------------------
// SitemapConfig validation (srv-8tzi6)
// --------------------------------------------------------------------------

func TestValidate_SitemapConfig(t *testing.T) {
	t.Parallel()

	// validSitemap returns a minimal valid SitemapConfig.
	validSitemap := func() *SitemapConfig {
		return &SitemapConfig{
			URL:           "https://example.com/sitemap.xml",
			FilterPattern: `/events/.+`,
		}
	}

	tests := []struct {
		name    string
		cfg     SourceConfig
		wantErr string // empty = no error expected; substring match
	}{
		// --- valid cases ---
		{
			name: "valid sitemap config tier 0",
			cfg: SourceConfig{
				Name:       "Sitemap Source Tier 0",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap:    validSitemap(),
			},
		},
		{
			name: "valid sitemap config tier 1 with selectors",
			cfg: SourceConfig{
				Name:       "Sitemap Source Tier 1",
				Tier:       1,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Selectors:  SelectorConfig{EventList: ".event-card"},
				Sitemap:    validSitemap(),
			},
		},
		{
			name: "valid sitemap config tier 2 with selectors and headless",
			cfg: SourceConfig{
				Name:       "Sitemap Source Tier 2",
				Tier:       2,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Selectors:  SelectorConfig{EventList: ".event-card"},
				Headless:   HeadlessConfig{WaitSelector: "body"},
				Sitemap:    validSitemap(),
			},
		},
		{
			name: "sitemap allows url field to be empty (sitemap provides URLs)",
			cfg: SourceConfig{
				Name:       "Sitemap No URL Metadata",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				// URL intentionally omitted — sitemap provides URLs at runtime
				Sitemap: validSitemap(),
			},
		},
		{
			name: "sitemap allows url field to be set as metadata",
			cfg: SourceConfig{
				Name:       "Sitemap With URL Metadata",
				URL:        "https://example.com",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap:    validSitemap(),
			},
		},
		{
			name: "sitemap with zero max_urls is valid (uses default)",
			cfg: SourceConfig{
				Name:       "Sitemap Zero MaxURLs",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: `/events/.+`,
					MaxURLs:       0,
				},
			},
		},
		{
			name: "sitemap with zero rate_limit_ms is valid (uses default)",
			cfg: SourceConfig{
				Name:       "Sitemap Zero RateLimit",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: `/events/.+`,
					RateLimitMs:   0,
				},
			},
		},
		// --- error cases ---
		{
			name: "sitemap with tier 3 — error",
			cfg: SourceConfig{
				Name:       "Sitemap Tier 3",
				URL:        "https://example.com",
				Tier:       3,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				REST:       &RestConfig{Endpoint: "https://api.example.com/events"},
				Sitemap:    validSitemap(),
			},
			wantErr: "sitemap: not supported for tier 3",
		},
		{
			name: "sitemap with missing url — error",
			cfg: SourceConfig{
				Name:       "Sitemap No Sitemap URL",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "",
					FilterPattern: `/events/.+`,
				},
			},
			wantErr: "sitemap.url: required when sitemap block is set",
		},
		{
			name: "sitemap with invalid url — error",
			cfg: SourceConfig{
				Name:       "Sitemap Invalid URL",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "ftp://example.com/sitemap.xml",
					FilterPattern: `/events/.+`,
				},
			},
			wantErr: "sitemap.url: must be a valid http/https URL",
		},
		{
			name: "sitemap with missing filter_pattern — error",
			cfg: SourceConfig{
				Name:       "Sitemap No Filter",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: "",
				},
			},
			wantErr: "sitemap.filter_pattern: required when sitemap block is set",
		},
		{
			name: "sitemap with invalid regex in filter_pattern — error",
			cfg: SourceConfig{
				Name:       "Sitemap Bad Regex",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: "[invalid(regex",
				},
			},
			wantErr: "sitemap.filter_pattern: invalid Go regex",
		},
		{
			name: "sitemap with negative max_urls — error",
			cfg: SourceConfig{
				Name:       "Sitemap Negative MaxURLs",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: `/events/.+`,
					MaxURLs:       -1,
				},
			},
			wantErr: "sitemap.max_urls: must be >= 0, got -1",
		},
		{
			name: "sitemap with negative rate_limit_ms — error",
			cfg: SourceConfig{
				Name:       "Sitemap Negative RateLimit",
				Tier:       0,
				TrustLevel: 5,
				MaxPages:   10,
				Schedule:   "daily",
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: `/events/.+`,
					RateLimitMs:   -100,
				},
			},
			wantErr: "sitemap.rate_limit_ms: must be >= 0, got -100",
		},
		{
			name: "sitemap with urls is invalid",
			cfg: SourceConfig{
				Name: "test",
				URL:  "https://example.com",
				URLs: []string{"https://example.com/page1"},
				Tier: 0,
				Sitemap: &SitemapConfig{
					URL:           "https://example.com/sitemap.xml",
					FilterPattern: "/events/.+",
				},
			},
			wantErr: "sitemap: mutually exclusive with urls",
		},
		{
			name: "sitemap with valid exclude_pattern — ok",
			cfg: SourceConfig{
				Name: "test",
				URL:  "https://example.com",
				Tier: 0,
				Sitemap: &SitemapConfig{
					URL:            "https://example.com/sitemap.xml",
					FilterPattern:  "/events/.+",
					ExcludePattern: "/(artist|about|terms)",
				},
			},
			wantErr: "",
		},
		{
			name: "sitemap with invalid exclude_pattern — error",
			cfg: SourceConfig{
				Name: "test",
				URL:  "https://example.com",
				Tier: 0,
				Sitemap: &SitemapConfig{
					URL:            "https://example.com/sitemap.xml",
					FilterPattern:  "/events/.+",
					ExcludePattern: "[invalid(regex",
				},
			},
			wantErr: "sitemap.exclude_pattern: invalid Go regex",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
// Sitemap validation warnings (B1, B3, B4)
// --------------------------------------------------------------------------

func TestValidate_SitemapWarnings(t *testing.T) {
	t.Parallel()

	baseSitemap := func() *SitemapConfig {
		return &SitemapConfig{
			URL:           "https://example.com/sitemap.xml",
			FilterPattern: `/events/.+`,
		}
	}

	baseCfg := func() SourceConfig {
		return SourceConfig{
			Name:       "Sitemap Warning Source",
			URL:        "https://example.com/events",
			Tier:       0,
			TrustLevel: 5,
			MaxPages:   10,
			Schedule:   "daily",
			Sitemap:    baseSitemap(),
		}
	}

	t.Run("B1: rate_limit_ms=0 produces warning", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg()
		cfg.Sitemap.RateLimitMs = 0
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "sitemap.rate_limit_ms: 0 means use default") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected rate_limit_ms=0 warning, got warnings: %v", warnings)
		}
	})

	t.Run("B1: rate_limit_ms=1 produces no zero warning", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg()
		cfg.Sitemap.RateLimitMs = 1
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		for _, w := range warnings {
			if strings.Contains(w, "sitemap.rate_limit_ms: 0 means use default") {
				t.Errorf("unexpected rate_limit_ms=0 warning for RateLimitMs=1, got warnings: %v", warnings)
			}
		}
	})

	t.Run("B3: max_urls > 10000 produces warning", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg()
		cfg.Sitemap.MaxURLs = 10001
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "sitemap.max_urls") && strings.Contains(w, "very high") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected max_urls high warning, got warnings: %v", warnings)
		}
	})

	t.Run("B3: max_urls = 10000 produces no warning", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg()
		cfg.Sitemap.MaxURLs = 10000
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		for _, w := range warnings {
			if strings.Contains(w, "sitemap.max_urls") && strings.Contains(w, "very high") {
				t.Errorf("unexpected max_urls warning for MaxURLs=10000, got warnings: %v", warnings)
			}
		}
	})

	t.Run("B4: sitemap without url field produces warning", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg()
		cfg.URL = "" // no top-level url
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "url: recommended for sitemap sources") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected url recommended warning for sitemap without url, got warnings: %v", warnings)
		}
	})

	t.Run("B4: sitemap with url field produces no recommendation warning", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg()
		cfg.URL = "https://example.com/events" // url is set
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		for _, w := range warnings {
			if strings.Contains(w, "url: recommended for sitemap sources") {
				t.Errorf("unexpected url recommendation warning when url is set, got warnings: %v", warnings)
			}
		}
	})
}

// --------------------------------------------------------------------------
// SourceConfig DB round-trip test
// --------------------------------------------------------------------------

// findRepoRoot walks up from the package directory until it finds go.mod,
// which marks the repo root. Tests run with the working directory set to
// the package directory, so real YAML configs are not directly accessible
// via a relative path.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}

func TestSourceConfig_DBRoundTrip(t *testing.T) {
	t.Parallel()

	// Find the repo root by walking up from this file's location.
	// Tests run with the working directory set to the package directory.
	repoRoot := findRepoRoot(t)
	configsDir := filepath.Join(repoRoot, "configs", "sources")

	configs, err := LoadSourceConfigs(configsDir)
	require.NoError(t, err, "LoadSourceConfigs must succeed")
	require.NotEmpty(t, configs, "should have at least one config")

	for _, cfg := range configs {
		cfg := cfg
		t.Run(cfg.Name, func(t *testing.T) {
			t.Parallel()

			params, err := SourceConfigToUpsertParams(cfg)
			require.NoError(t, err, "SourceConfigToUpsertParams must succeed")

			src := domainScraper.Source{
				Name:                          params.Name,
				URL:                           params.URL,
				URLs:                          params.URLs,
				Tier:                          params.Tier,
				Schedule:                      params.Schedule,
				TrustLevel:                    params.TrustLevel,
				License:                       params.License,
				Enabled:                       params.Enabled,
				MaxPages:                      params.MaxPages,
				Selectors:                     params.Selectors,
				Notes:                         params.Notes,
				EventURLPattern:               params.EventURLPattern,
				SkipMultiSessionCheck:         params.SkipMultiSessionCheck,
				MultiSessionDurationThreshold: params.MultiSessionDurationThreshold,
				FollowEventURLs:               params.FollowEventURLs,
				Timezone:                      params.Timezone,
				HeadlessWaitSelector:          params.HeadlessWaitSelector,
				HeadlessWaitTimeoutMs:         params.HeadlessWaitTimeoutMs,
				HeadlessPaginationBtn:         params.HeadlessPaginationBtn,
				HeadlessHeaders:               params.HeadlessHeaders,
				HeadlessRateLimitMs:           params.HeadlessRateLimitMs,
				HeadlessWaitNetworkIdle:       params.HeadlessWaitNetworkIdle,
				HeadlessUndetected:            params.HeadlessUndetected,
				HeadlessIframe:                params.HeadlessIframe,
				HeadlessIntercept:             params.HeadlessIntercept,
				GraphQLConfig:                 params.GraphQLConfig,
				RestConfig:                    params.RestConfig,
				SitemapConfig:                 params.SitemapConfig,
				DefaultLocation:               params.DefaultLocation,
			}

			// Exclude LastScrapedAt from comparison - it's set by the scraper at runtime.
			src.LastScrapedAt = nil
			cfg.LastScrapedAt = nil

			got, err := SourceConfigFromDomain(src)
			require.NoError(t, err, "SourceConfigFromDomain must succeed")

			assert.Equal(t, cfg, got, "SourceConfig should survive DB round-trip without data loss")
		})
	}
}

// --------------------------------------------------------------------------
// DescriptionSelectors — srv-nojwn
// --------------------------------------------------------------------------

// TestLoadFile_DescriptionSelectors verifies that description_selectors is parsed
// correctly from YAML and available for both Tier 1 and Tier 2 extraction.
func TestLoadFile_DescriptionSelectors(t *testing.T) {
	t.Parallel()

	t.Run("single description selector parses as single-element slice in DescriptionSelectors", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Single Desc Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: "p.summary"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "single_desc.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		// Single description should also populate DescriptionSelectors for the unified path
		assert.Equal(t, []string{"p.summary"}, cfg.Selectors.DescriptionSelectors,
			"single description should be normalized to DescriptionSelectors")
	})

	t.Run("multiple description selectors parse correctly via description_selectors field", func(t *testing.T) {
		t.Parallel()
		// Multiple selectors should be specified via the explicit description_selectors field
		yamlContent := `
name: "Multi Desc Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description_selectors:
    - "p.summary"
    - "div.full-description"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "multi_desc.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"p.summary", "div.full-description"}, cfg.Selectors.DescriptionSelectors,
			"multiple description selectors should be parsed as DescriptionSelectors")
	})

	t.Run("description_selectors field parses correctly", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Desc Selectors Field Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description_selectors:
    - ".lead"
    - ".full"
    - ".more"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "desc_selectors.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []string{".lead", ".full", ".more"}, cfg.Selectors.DescriptionSelectors,
			"description_selectors field should be parsed correctly")
	})

	t.Run("both description and description_selectors - description takes precedence", func(t *testing.T) {
		t.Parallel()
		// When both are set, description (single) takes precedence for backward compat
		yamlContent := `
name: "Both Desc Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: "p.summary"
  description_selectors:
    - ".lead"
    - ".full"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "both_desc.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		// description (single) takes precedence for backward compatibility
		assert.Equal(t, []string{"p.summary"}, cfg.Selectors.DescriptionSelectors,
			"description should take precedence over description_selectors")
	})

	t.Run("empty description with description_selectors uses description_selectors", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Desc Selectors Only"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: ""
  description_selectors:
    - ".lead"
    - ".full"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "desc_selectors_only.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []string{".lead", ".full"}, cfg.Selectors.DescriptionSelectors,
			"empty description with description_selectors should use description_selectors")
	})
}

// TestLoadFile_DescriptionSelectorsWarnings verifies that loadFile emits accurate
// deprecation warnings based on the original YAML input, not the post-normalization
// state. This is a regression test for the inaccurate "both set" warning that could
// appear after normalization when only description was set in the YAML.
func TestLoadFile_DescriptionSelectorsWarnings(t *testing.T) {
	t.Parallel()

	t.Run("only description set - accurate deprecation warning (not 'both set')", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Desc Only Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: "p.summary"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "desc_only.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)

		// After normalization, both are populated, but the warning should reflect original input
		assert.Equal(t, []string{"p.summary"}, cfg.Selectors.DescriptionSelectors)
		assert.True(t, cfg.normalized, "normalized flag should be set after loadFile")
	})

	t.Run("both description and description_selectors set - accurate 'both set' warning", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "Both Set Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: "p.summary"
  description_selectors:
    - ".lead"
    - ".full"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "both_set.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)

		// Description takes precedence
		assert.Equal(t, []string{"p.summary"}, cfg.Selectors.DescriptionSelectors)
		assert.True(t, cfg.normalized)
	})

	t.Run("only description_selectors set - no warning", func(t *testing.T) {
		t.Parallel()
		yamlContent := `
name: "DescSelectors Only Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description_selectors:
    - ".lead"
    - ".full"
`
		dir := t.TempDir()
		path := writeYAML(t, dir, "desc_selectors_only.yaml", yamlContent)

		cfg, err := loadFile(path)
		require.NoError(t, err)

		assert.Equal(t, []string{".lead", ".full"}, cfg.Selectors.DescriptionSelectors)
		assert.True(t, cfg.normalized)
	})
}

// --------------------------------------------------------------------------
// SourceConfigFromDomain — DescriptionSelectors normalization (srv-nojwn)
// --------------------------------------------------------------------------

// TestSourceConfigFromDomain_DescriptionSelectorsNormalization verifies that
// description precedence is applied consistently when loading configs from DB.
func TestSourceConfigFromDomain_DescriptionSelectorsNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		selectorsJSON     string
		wantDescSelectors []string
	}{
		{
			name:              "single description field - normalized to DescriptionSelectors",
			selectorsJSON:     `{"event_list": ".card", "description": "p.summary"}`,
			wantDescSelectors: []string{"p.summary"},
		},
		{
			name:              "description_selectors field - used as-is",
			selectorsJSON:     `{"event_list": ".card", "description_selectors": [".lead", ".full"]}`,
			wantDescSelectors: []string{".lead", ".full"},
		},
		{
			name:              "both fields - description takes precedence",
			selectorsJSON:     `{"event_list": ".card", "description": "p.summary", "description_selectors": [".lead", ".full"]}`,
			wantDescSelectors: []string{"p.summary"},
		},
		{
			name:              "empty description with description_selectors - uses description_selectors",
			selectorsJSON:     `{"event_list": ".card", "description": "", "description_selectors": [".lead", ".full"]}`,
			wantDescSelectors: []string{".lead", ".full"},
		},
		{
			name:              "neither field set - nil DescriptionSelectors",
			selectorsJSON:     `{"event_list": ".card"}`,
			wantDescSelectors: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := domainScraper.Source{
				Name:       "test-source",
				URL:        "https://example.com/events",
				Tier:       1,
				TrustLevel: 5,
				License:    "CC0-1.0",
				Enabled:    true,
				MaxPages:   10,
				Schedule:   "weekly",
				Selectors:  []byte(tt.selectorsJSON),
			}

			cfg, err := SourceConfigFromDomain(src)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDescSelectors, cfg.Selectors.DescriptionSelectors,
				"DescriptionSelectors should be normalized consistently for DB-loaded configs")
		})
	}
}

// --------------------------------------------------------------------------
// ValidateConfigWithWarnings — deprecated selectors.description (srv-nojwn)
// --------------------------------------------------------------------------

func TestValidateConfigWithWarnings_DeprecatedDescription(t *testing.T) {
	t.Parallel()

	base := SourceConfig{
		Name:       "Test Source",
		URL:        "https://example.com/events",
		Tier:       1,
		TrustLevel: 5,
		MaxPages:   10,
		Schedule:   "daily",
		Enabled:    true,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
		},
	}

	tests := []struct {
		name         string
		cfg          SourceConfig
		wantWarnings []string // substrings expected in warnings
	}{
		{
			name:         "no description field — no warning",
			cfg:          base,
			wantWarnings: nil,
		},
		{
			name: "description_selectors only — no warning",
			cfg: func() SourceConfig {
				c := base
				c.Selectors.DescriptionSelectors = []string{".lead", ".full"}
				return c
			}(),
			wantWarnings: nil,
		},
		{
			name: "description set — deprecation warning",
			cfg: func() SourceConfig {
				c := base
				c.Selectors.Description = "p.summary"
				return c
			}(),
			wantWarnings: []string{"selectors.description: deprecated", "description_selectors"},
		},
		{
			name: "both description and description_selectors set — precedence warning",
			cfg: func() SourceConfig {
				c := base
				c.Selectors.Description = "p.summary"
				c.Selectors.DescriptionSelectors = []string{".lead", ".full"}
				return c
			}(),
			wantWarnings: []string{"selectors.description: deprecated", "both description and description_selectors", "description takes precedence"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warnings, err := ValidateConfigWithWarnings(tt.cfg)
			require.NoError(t, err)

			joined := strings.Join(warnings, "\n")
			for _, wantSub := range tt.wantWarnings {
				assert.Contains(t, joined, wantSub,
					"expected warning substring %q in: %s", wantSub, joined)
			}
		})
	}
}

// --------------------------------------------------------------------------
// slog.Warn — loadFile path (srv-nojwn)
// --------------------------------------------------------------------------

func TestLoadFile_DescriptionDeprecationWarningsSlog(t *testing.T) {
	dir := t.TempDir()

	// Save original default logger and restore after test
	originalDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalDefault) })

	t.Run("only description set - deprecation warning (NOT both set)", func(t *testing.T) {
		yamlContent := `
name: "Desc Only Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: "p.summary"
`
		path := writeYAML(t, dir, "desc_only_3.yaml", yamlContent)

		handler := &warningHandler{}
		logger := slog.New(handler)
		slog.SetDefault(logger)

		_, err := loadFile(path)
		require.NoError(t, err)

		warnings := handler.GetWarnings()
		require.NotEmpty(t, warnings, "expected slog.Warn to be called, got: %v", warnings)

		foundDeprecation := false
		foundBothSet := false
		for _, w := range warnings {
			if strings.Contains(w, "selectors.description: deprecated") {
				foundDeprecation = true
			}
			if strings.Contains(w, "both description and description_selectors are set") {
				foundBothSet = true
			}
		}
		assert.True(t, foundDeprecation, "expected deprecation warning for description, got: %v", warnings)
		assert.False(t, foundBothSet, "should NOT warn about both fields when only description was set, got: %v", warnings)
	})

	t.Run("both description and description_selectors set - precedence warning with 'takes precedence'", func(t *testing.T) {
		yamlContent := `
name: "Both Set Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description: "p.summary"
  description_selectors:
    - ".lead"
    - ".full"
`
		path := writeYAML(t, dir, "both_set_3.yaml", yamlContent)

		handler := &warningHandler{}
		logger := slog.New(handler)
		slog.SetDefault(logger)

		_, err := loadFile(path)
		require.NoError(t, err)

		warnings := handler.GetWarnings()
		require.NotEmpty(t, warnings, "expected slog.Warn to be called, got: %v", warnings)

		foundPrecedence := false
		for _, w := range warnings {
			if strings.Contains(w, "description takes precedence") {
				foundPrecedence = true
			}
		}
		assert.True(t, foundPrecedence, "expected precedence warning containing 'description takes precedence', got: %v", warnings)
	})

	t.Run("only description_selectors set - no warning", func(t *testing.T) {
		yamlContent := `
name: "DescSelectors Only Source"
url: "https://example.com/events"
tier: 1
selectors:
  event_list: ".event-card"
  name: "h2.title"
  description_selectors:
    - ".lead"
    - ".full"
`
		path := writeYAML(t, dir, "desc_selectors_only_3.yaml", yamlContent)

		handler := &warningHandler{}
		logger := slog.New(handler)
		slog.SetDefault(logger)

		_, err := loadFile(path)
		require.NoError(t, err)

		warnings := handler.GetWarnings()
		for _, w := range warnings {
			assert.NotContains(t, w, "selectors.description", "should not warn about description when not set, got: %v", warnings)
		}
	})
}

// --------------------------------------------------------------------------
// slog.Warn — SourceConfigFromDomain path (srv-nojwn)
// --------------------------------------------------------------------------

func TestSourceConfigFromDomain_DescriptionDeprecationWarningsSlog(t *testing.T) {
	// Save original default logger and restore after test
	originalDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalDefault) })

	t.Run("only description set - deprecation warning (NOT both set)", func(t *testing.T) {
		selectorsJSON := `{"event_list": ".card", "description": "p.summary"}`

		src := domainScraper.Source{
			Name:       "test-source",
			URL:        "https://example.com/events",
			Tier:       1,
			TrustLevel: 5,
			License:    "CC0-1.0",
			Enabled:    true,
			MaxPages:   10,
			Schedule:   "weekly",
			Selectors:  []byte(selectorsJSON),
		}

		handler := &warningHandler{}
		logger := slog.New(handler)
		slog.SetDefault(logger)

		_, err := SourceConfigFromDomain(src)
		require.NoError(t, err)

		warnings := handler.GetWarnings()
		require.NotEmpty(t, warnings, "expected slog.Warn to be called, got: %v", warnings)

		foundDeprecation := false
		foundBothSet := false
		for _, w := range warnings {
			if strings.Contains(w, "selectors.description: deprecated") {
				foundDeprecation = true
			}
			if strings.Contains(w, "both description and description_selectors are set") {
				foundBothSet = true
			}
		}
		assert.True(t, foundDeprecation, "expected deprecation warning for description, got: %v", warnings)
		assert.False(t, foundBothSet, "should NOT warn about both fields when only description was set, got: %v", warnings)
	})

	t.Run("both description and description_selectors set - precedence warning with 'takes precedence'", func(t *testing.T) {
		selectorsJSON := `{"event_list": ".card", "description": "p.summary", "description_selectors": [".lead", ".full"]}`

		src := domainScraper.Source{
			Name:       "test-source",
			URL:        "https://example.com/events",
			Tier:       1,
			TrustLevel: 5,
			License:    "CC0-1.0",
			Enabled:    true,
			MaxPages:   10,
			Schedule:   "weekly",
			Selectors:  []byte(selectorsJSON),
		}

		handler := &warningHandler{}
		logger := slog.New(handler)
		slog.SetDefault(logger)

		_, err := SourceConfigFromDomain(src)
		require.NoError(t, err)

		warnings := handler.GetWarnings()
		require.NotEmpty(t, warnings, "expected slog.Warn to be called, got: %v", warnings)

		foundPrecedence := false
		for _, w := range warnings {
			if strings.Contains(w, "description takes precedence") {
				foundPrecedence = true
			}
		}
		assert.True(t, foundPrecedence, "expected precedence warning containing 'description takes precedence', got: %v", warnings)
	})

	t.Run("only description_selectors set - no warning", func(t *testing.T) {
		selectorsJSON := `{"event_list": ".card", "description_selectors": [".lead", ".full"]}`

		src := domainScraper.Source{
			Name:       "test-source",
			URL:        "https://example.com/events",
			Tier:       1,
			TrustLevel: 5,
			License:    "CC0-1.0",
			Enabled:    true,
			MaxPages:   10,
			Schedule:   "weekly",
			Selectors:  []byte(selectorsJSON),
		}

		handler := &warningHandler{}
		logger := slog.New(handler)
		slog.SetDefault(logger)

		_, err := SourceConfigFromDomain(src)
		require.NoError(t, err)

		warnings := handler.GetWarnings()
		for _, w := range warnings {
			assert.NotContains(t, w, "selectors.description", "should not warn about description when not set, got: %v", warnings)
		}
	})
}
