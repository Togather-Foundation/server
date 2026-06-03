package domain

import (
	"net/url"
	"strings"
	"testing"
)

func TestResolveAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		values     url.Values
		canonical  string
		alias      string
		initWarns  []string
		nilWarnPtr bool
		wantResult string
		wantWarnCt int
		wantSubstr string
	}{
		{
			name:       "canonical present",
			values:     url.Values{"q": {"hello"}},
			canonical:  "q",
			alias:      "search",
			wantResult: "hello",
			wantWarnCt: 0,
		},
		{
			name:       "alias only",
			values:     url.Values{"search": {"hello"}},
			canonical:  "q",
			alias:      "search",
			wantResult: "hello",
			wantWarnCt: 1,
			wantSubstr: `Unrecognised parameter alias "search"`,
		},
		{
			name:       "canonical wins over alias",
			values:     url.Values{"q": {"canon"}, "search": {"alias"}},
			canonical:  "q",
			alias:      "search",
			wantResult: "canon",
			wantWarnCt: 0,
		},
		{
			name:       "both absent",
			values:     url.Values{},
			canonical:  "q",
			alias:      "search",
			wantResult: "",
			wantWarnCt: 0,
		},
		{
			name:       "canonical present but empty",
			values:     url.Values{"q": {""}, "search": {"aliasval"}},
			canonical:  "q",
			alias:      "search",
			wantResult: "aliasval",
			wantWarnCt: 1,
			wantSubstr: `Unrecognised parameter alias "search"`,
		},
		{
			name:       "alias absent canonical present",
			values:     url.Values{"q": {"hello"}},
			canonical:  "q",
			alias:      "search",
			wantResult: "hello",
			wantWarnCt: 0,
		},
		{
			name:       "nil warnings",
			values:     url.Values{"search": {"hello"}},
			canonical:  "q",
			alias:      "search",
			nilWarnPtr: true,
			wantResult: "hello",
			wantWarnCt: 0,
		},
		{
			name:       "pre-populated warnings",
			values:     url.Values{"search": {"hello"}},
			canonical:  "q",
			alias:      "search",
			initWarns:  []string{"existing warning"},
			wantResult: "hello",
			wantWarnCt: 2,
			wantSubstr: `Unrecognised parameter alias "search"`,
		},
		{
			name:       "multiple calls accumulate",
			values:     url.Values{},
			canonical:  "",
			alias:      "",
			initWarns:  []string{"first"},
			wantResult: "",
			wantWarnCt: 3,
			wantSubstr: `Unrecognised parameter alias "search"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var warns []string
			if tt.initWarns != nil {
				warns = make([]string, len(tt.initWarns))
				copy(warns, tt.initWarns)
			}

			var warnPtr *[]string
			if !tt.nilWarnPtr {
				warnPtr = &warns
			}

			if tt.name == "multiple calls accumulate" {
				_ = ResolveAlias(url.Values{"search": {"hello"}}, "q", "search", warnPtr)
				_ = ResolveAlias(url.Values{"query": {"world"}}, "q", "query", warnPtr)
			} else {
				result := ResolveAlias(tt.values, tt.canonical, tt.alias, warnPtr)
				if result != tt.wantResult {
					t.Errorf("result = %q, want %q", result, tt.wantResult)
				}
			}

			if len(warns) != tt.wantWarnCt {
				t.Errorf("warnings length = %d, want %d; warnings = %q", len(warns), tt.wantWarnCt, warns)
			}
			if tt.wantSubstr != "" {
				found := false
				for _, w := range warns {
					if strings.Contains(w, tt.wantSubstr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("warnings = %q, want substring %q", warns, tt.wantSubstr)
				}
			}
		})
	}
}
