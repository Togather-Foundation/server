package postgres

import "testing"

func TestEscapeILIKEPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "normal text",
			input:    "Toronto",
			expected: "Toronto",
		},
		{
			name:     "percent sign",
			input:    "100% effort",
			expected: `100\% effort`,
		},
		{
			name:     "underscore",
			input:    "test_pattern",
			expected: `test\_pattern`,
		},
		{
			name:     "backslash",
			input:    `test\path`,
			expected: `test\\path`,
		},
		{
			name:     "SQL injection attempt",
			input:    `%'; DROP TABLE events; --`,
			expected: `\%'; DROP TABLE events; --`,
		},
		{
			name:     "multiple wildcards",
			input:    `%_test_%_`,
			expected: `\%\_test\_\%\_`,
		},
		{
			name:     "mixed escape characters",
			input:    `\%_test`,
			expected: `\\\%\_test`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeILIKEPattern(tt.input)
			if got != tt.expected {
				t.Errorf("escapeILIKEPattern(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
