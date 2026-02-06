package tools

import (
	"testing"
)

// Test normalizeSearchTypes
func TestNormalizeSearchTypes(t *testing.T) {
	tests := []struct {
		name          string
		types         []string
		expectError   bool
		errorContains string
		expected      map[string]bool
	}{
		{
			name:  "empty types defaults to all",
			types: []string{},
			expected: map[string]bool{
				"event":        true,
				"place":        true,
				"organization": true,
			},
			expectError: false,
		},
		{
			name:  "single type - event",
			types: []string{"event"},
			expected: map[string]bool{
				"event": true,
			},
			expectError: false,
		},
		{
			name:  "single type - events (plural)",
			types: []string{"events"},
			expected: map[string]bool{
				"event": true,
			},
			expectError: false,
		},
		{
			name:  "single type - place",
			types: []string{"place"},
			expected: map[string]bool{
				"place": true,
			},
			expectError: false,
		},
		{
			name:  "single type - places (plural)",
			types: []string{"places"},
			expected: map[string]bool{
				"place": true,
			},
			expectError: false,
		},
		{
			name:  "single type - organization",
			types: []string{"organization"},
			expected: map[string]bool{
				"organization": true,
			},
			expectError: false,
		},
		{
			name:  "single type - org (short)",
			types: []string{"org"},
			expected: map[string]bool{
				"organization": true,
			},
			expectError: false,
		},
		{
			name:  "single type - orgs (short plural)",
			types: []string{"orgs"},
			expected: map[string]bool{
				"organization": true,
			},
			expectError: false,
		},
		{
			name:  "multiple types",
			types: []string{"event", "place"},
			expected: map[string]bool{
				"event": true,
				"place": true,
			},
			expectError: false,
		},
		{
			name:  "case insensitive",
			types: []string{"EVENT", "Place", "OrGaNiZaTiOn"},
			expected: map[string]bool{
				"event":        true,
				"place":        true,
				"organization": true,
			},
			expectError: false,
		},
		{
			name:  "whitespace trimming",
			types: []string{"  event  ", " place "},
			expected: map[string]bool{
				"event": true,
				"place": true,
			},
			expectError: false,
		},
		{
			name:          "invalid type",
			types:         []string{"invalid"},
			expectError:   true,
			errorContains: "unsupported type",
		},
		{
			name:  "empty strings ignored",
			types: []string{"event", "", "place"},
			expected: map[string]bool{
				"event": true,
				"place": true,
			},
			expectError: false,
		},
		{
			name:  "mixed plural and singular",
			types: []string{"events", "place", "organizations"},
			expected: map[string]bool{
				"event":        true,
				"place":        true,
				"organization": true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeSearchTypes(tt.types)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				if len(result) != len(tt.expected) {
					t.Errorf("expected %d types, got %d", len(tt.expected), len(result))
				}
				for k, v := range tt.expected {
					if result[k] != v {
						t.Errorf("expected %s=%v, got %v", k, v, result[k])
					}
				}
			}
		})
	}
}

// Test normalizeSearchLimit
func TestNormalizeSearchLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		expected int
	}{
		{
			name:     "zero to default",
			limit:    0,
			expected: defaultSearchLimit,
		},
		{
			name:     "negative to default",
			limit:    -5,
			expected: defaultSearchLimit,
		},
		{
			name:     "valid limit below max",
			limit:    50,
			expected: 50,
		},
		{
			name:     "valid limit at max",
			limit:    maxSearchLimit,
			expected: maxSearchLimit,
		},
		{
			name:     "over max capped",
			limit:    300,
			expected: maxSearchLimit,
		},
		{
			name:     "exactly at default",
			limit:    defaultSearchLimit,
			expected: defaultSearchLimit,
		},
		{
			name:     "one",
			limit:    1,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSearchLimit(tt.limit)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// Test distributeLimits
func TestDistributeLimits(t *testing.T) {
	tests := []struct {
		name     string
		total    int
		buckets  int
		expected []int
	}{
		{
			name:     "zero buckets",
			total:    100,
			buckets:  0,
			expected: nil,
		},
		{
			name:     "negative buckets",
			total:    100,
			buckets:  -1,
			expected: nil,
		},
		{
			name:     "even distribution",
			total:    30,
			buckets:  3,
			expected: []int{10, 10, 10},
		},
		{
			name:     "uneven distribution - remainder goes to first",
			total:    20,
			buckets:  3,
			expected: []int{7, 7, 6},
		},
		{
			name:     "single bucket",
			total:    100,
			buckets:  1,
			expected: []int{100},
		},
		{
			name:     "more buckets than total",
			total:    5,
			buckets:  10,
			expected: []int{1, 1, 1, 1, 1, 0, 0, 0, 0, 0},
		},
		{
			name:     "two buckets even",
			total:    10,
			buckets:  2,
			expected: []int{5, 5},
		},
		{
			name:     "two buckets uneven",
			total:    11,
			buckets:  2,
			expected: []int{6, 5},
		},
		{
			name:     "four buckets even",
			total:    40,
			buckets:  4,
			expected: []int{10, 10, 10, 10},
		},
		{
			name:     "four buckets uneven",
			total:    43,
			buckets:  4,
			expected: []int{11, 11, 11, 10},
		},
		{
			name:     "zero total",
			total:    0,
			buckets:  3,
			expected: []int{0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := distributeLimits(tt.total, tt.buckets)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
				return
			}

			sum := 0
			for i, v := range result {
				sum += v
				if v != tt.expected[i] {
					t.Errorf("bucket %d: expected %d, got %d", i, tt.expected[i], v)
				}
			}

			// Verify that sum equals total
			if sum != tt.total {
				t.Errorf("sum of buckets %d does not equal total %d", sum, tt.total)
			}

			// Verify distribution is fair (max difference of 1)
			if len(result) > 1 {
				min, max := result[0], result[0]
				for _, v := range result[1:] {
					if v < min {
						min = v
					}
					if v > max {
						max = v
					}
				}
				if max-min > 1 {
					t.Errorf("distribution is unfair: min=%d, max=%d, diff=%d", min, max, max-min)
				}
			}
		})
	}
}

// Helper function
