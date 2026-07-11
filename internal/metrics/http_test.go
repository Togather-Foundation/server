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
		{
			name:     "ulid in path",
			input:    "/api/v1/admin/events/01JMKZGVPF3KD1NKPGGNN37HBM",
			expected: "/api/v1/admin/events/{id}",
		},
		{
			name:     "numeric id in path",
			input:    "/api/v1/events/42",
			expected: "/api/v1/events/{id}",
		},
		{
			name:     "ulid and numeric segments",
			input:    "/api/v1/organizations/01ARZ3NDEKTSV4RRFFQ69G5FAV/nested/123",
			expected: "/api/v1/organizations/{id}/nested/{id}",
		},
		{
			name:     "ulid in query",
			input:    "/api/v1/events/01JMKZGVPF3KD1NKPGGNN37HBM/occurrences",
			expected: "/api/v1/events/{id}/occurrences",
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
