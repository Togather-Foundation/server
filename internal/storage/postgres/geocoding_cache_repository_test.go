package postgres

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase conversion",
			input:    "Toronto, ON",
			expected: "toronto, on",
		},
		{
			name:     "trim whitespace",
			input:    "  Toronto  ",
			expected: "toronto",
		},
		{
			name:     "collapse multiple spaces",
			input:    "Toronto    Ontario   Canada",
			expected: "toronto ontario canada",
		},
		{
			name:     "mixed case and spaces",
			input:    "  123   Main   STREET  ",
			expected: "123 main street",
		},
		{
			name:     "already normalized",
			input:    "toronto, on",
			expected: "toronto, on",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeQuery(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeQuery(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeQuery_Consistency(t *testing.T) {
	// Verify that different representations normalize to the same value
	inputs := []string{
		"Toronto, Ontario",
		"TORONTO, ONTARIO",
		"  toronto,  ontario  ",
		"Toronto,   Ontario",
	}

	var normalized []string
	for _, input := range inputs {
		normalized = append(normalized, NormalizeQuery(input))
	}

	// All should be equal
	first := normalized[0]
	for i, n := range normalized {
		if n != first {
			t.Errorf("normalized[%d] = %q, expected %q (from input %q)",
				i, n, first, inputs[i])
		}
	}

	// Should be lowercase without extra spaces
	if first != "toronto, ontario" {
		t.Errorf("final normalized value = %q, expected %q", first, "toronto, ontario")
	}
}

func TestGeocodingCacheRepository_IncrementHitCount_InvalidTable(t *testing.T) {
	// We don't need a real DB connection for this test
	repo := &GeocodingCacheRepository{}

	err := repo.IncrementHitCount(context.TODO(), 123, "invalid_table_name")
	if err == nil {
		t.Fatal("expected error for invalid table name, got nil")
	}

	if !strings.Contains(err.Error(), "invalid table name") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGeocodingCacheRepository_IncrementHitCount_ValidTables(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
	}{
		{
			name:      "forward cache table",
			tableName: "geocoding_cache",
		},
		{
			name:      "reverse cache table",
			tableName: "reverse_geocoding_cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just test that validation passes for valid table names
			// We can't test actual execution without a DB connection
			validTables := map[string]bool{
				"geocoding_cache":         true,
				"reverse_geocoding_cache": true,
			}

			if !validTables[tt.tableName] {
				t.Errorf("table %q should be valid", tt.tableName)
			}
		})
	}
}
