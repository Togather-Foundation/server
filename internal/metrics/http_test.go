package metrics

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "static path",
			input:    "/api/v1/events",
			expected: "/api/v1/events",
		},
		{
			name:     "single param",
			input:    "/api/v1/events/{id}",
			expected: "/api/v1/events/{param}",
		},
		{
			name:     "multiple params",
			input:    "/api/v1/admin/events/{id}/publish",
			expected: "/api/v1/admin/events/{param}/publish",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "non-path input",
			input:    "api/v1/events/{id}",
			expected: "api/v1/events/{id}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.input)
			if got != tt.expected {
				t.Fatalf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
