package scraper

import (
	"strings"
	"testing"
)

func TestJoinSelectorTexts(t *testing.T) {
	tests := []struct {
		name      string
		selectors []string
		extractor func(string) string
		want      string
	}{
		{
			name:      "nil selectors returns empty",
			selectors: nil,
			extractor: func(s string) string { return "ignored" },
			want:      "",
		},
		{
			name:      "empty selectors returns empty",
			selectors: []string{},
			extractor: func(s string) string { return "ignored" },
			want:      "",
		},
		{
			name:      "single selector returns extracted text",
			selectors: []string{".title"},
			extractor: func(s string) string { return "Hello World" },
			want:      "Hello World",
		},
		{
			name:      "multiple selectors joined with space",
			selectors: []string{".summary", ".full"},
			extractor: func(s string) string {
				switch s {
				case ".summary":
					return "First part"
				case ".full":
					return "Second part"
				}
				return ""
			},
			want: "First part Second part",
		},
		{
			name:      "empty extractor results are skipped",
			selectors: []string{".summary", ".empty", ".full", ".also-empty", ".more"},
			extractor: func(s string) string {
				switch s {
				case ".summary":
					return "Start"
				case ".full":
					return "Middle"
				case ".more":
					return "End"
				}
				return ""
			},
			want: "Start Middle End",
		},
		{
			name:      "all selectors return empty yields empty",
			selectors: []string{".a", ".b", ".c"},
			extractor: func(s string) string { return "" },
			want:      "",
		},
		{
			name:      "whitespace-only result from extractor is included (extractor should trim)",
			selectors: []string{".a", ".b"},
			extractor: func(s string) string {
				switch s {
				case ".a":
					return "  hello  "
				case ".b":
					return "\tworld\n"
				}
				return ""
			},
			want: "  hello   \tworld\n",
		},
		{
			name:      "whitespace-only trimmed by extractor yields skipped",
			selectors: []string{".ws", ".real"},
			extractor: func(s string) string {
				switch s {
				case ".ws":
					return strings.TrimSpace("   ")
				case ".real":
					return "real"
				}
				return ""
			},
			want: "real",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinSelectorTexts(tt.selectors, tt.extractor)
			if got != tt.want {
				t.Errorf("joinSelectorTexts(%v, fn) = %q; want %q", tt.selectors, got, tt.want)
			}
		})
	}
}
