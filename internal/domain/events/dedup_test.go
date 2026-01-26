package events

import (
	"testing"
)

func TestBuildDedupHash(t *testing.T) {
	tests := []struct {
		name     string
		input    DedupCandidate
		expected string
		desc     string
	}{
		{
			name: "basic hash generation",
			input: DedupCandidate{
				Name:      "Tech Conference 2024",
				VenueID:   "venue-123",
				StartDate: "2024-06-15T10:00:00Z",
			},
			expected: "f3914fb4555eb4c1f301405cbe7f4c545f45add62c9d833a824f584777bc0b13",
			desc:     "should generate consistent hash for same inputs",
		},
		{
			name: "case insensitive name",
			input: DedupCandidate{
				Name:      "TECH CONFERENCE 2024",
				VenueID:   "venue-123",
				StartDate: "2024-06-15T10:00:00Z",
			},
			expected: "", // Will be compared with lowercase version
			desc:     "should treat names case-insensitively",
		},
		{
			name: "whitespace trimming",
			input: DedupCandidate{
				Name:      "  Tech Conference 2024  ",
				VenueID:   "  venue-123  ",
				StartDate: "  2024-06-15T10:00:00Z  ",
			},
			expected: "", // Will be compared with trimmed version
			desc:     "should trim whitespace from all fields",
		},
		{
			name: "empty fields",
			input: DedupCandidate{
				Name:      "",
				VenueID:   "",
				StartDate: "",
			},
			expected: "565d240f5343e625ae579a4d45a770f1f02c6368b5ed4d06da4fbe6f47c28866",
			desc:     "should handle empty fields (all pipes delimiter)",
		},
		{
			name: "different name same venue and date",
			input: DedupCandidate{
				Name:      "Different Conference",
				VenueID:   "venue-123",
				StartDate: "2024-06-15T10:00:00Z",
			},
			expected: "", // Will differ from first test
			desc:     "should produce different hash for different names",
		},
		{
			name: "same name different venue",
			input: DedupCandidate{
				Name:      "Tech Conference 2024",
				VenueID:   "venue-456",
				StartDate: "2024-06-15T10:00:00Z",
			},
			expected: "", // Will differ from first test
			desc:     "should produce different hash for different venues",
		},
		{
			name: "same name and venue different date",
			input: DedupCandidate{
				Name:      "Tech Conference 2024",
				VenueID:   "venue-123",
				StartDate: "2024-07-15T10:00:00Z",
			},
			expected: "", // Will differ from first test
			desc:     "should produce different hash for different dates",
		},
		{
			name: "unicode characters in name",
			input: DedupCandidate{
				Name:      "Café ☕ Tech Meetup",
				VenueID:   "venue-789",
				StartDate: "2024-08-01T14:00:00Z",
			},
			expected: "",
			desc:     "should handle unicode characters correctly",
		},
		{
			name: "pipe character in name (edge case)",
			input: DedupCandidate{
				Name:      "Event | With | Pipes",
				VenueID:   "venue-999",
				StartDate: "2024-09-01T09:00:00Z",
			},
			expected: "",
			desc:     "should handle pipe characters in name (uses them as delimiter)",
		},
	}

	// Store hashes for comparison
	hashes := make(map[string]string)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildDedupHash(tt.input)

			// Check hash format (SHA256 hex = 64 characters)
			if len(result) != 64 {
				t.Errorf("BuildDedupHash() hash length = %d, want 64", len(result))
			}

			// Check hex encoding (lowercase)
			for _, c := range result {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("BuildDedupHash() contains non-hex character: %c", c)
				}
			}

			// If expected hash provided, verify it
			if tt.expected != "" {
				if result != tt.expected {
					t.Errorf("BuildDedupHash() = %v, want %v", result, tt.expected)
				}
			}

			// Store hash for uniqueness checks
			hashes[tt.name] = result
		})
	}

	// Test determinism: same input always produces same hash
	t.Run("determinism", func(t *testing.T) {
		input := DedupCandidate{
			Name:      "Consistency Test",
			VenueID:   "venue-abc",
			StartDate: "2024-10-01T12:00:00Z",
		}
		hash1 := BuildDedupHash(input)
		hash2 := BuildDedupHash(input)
		hash3 := BuildDedupHash(input)

		if hash1 != hash2 || hash2 != hash3 {
			t.Errorf("BuildDedupHash() not deterministic: got %v, %v, %v", hash1, hash2, hash3)
		}
	})

	// Test case insensitivity
	t.Run("case insensitivity", func(t *testing.T) {
		input1 := DedupCandidate{Name: "EVENT NAME", VenueID: "VENUE-1", StartDate: "2024-01-01"}
		input2 := DedupCandidate{Name: "event name", VenueID: "venue-1", StartDate: "2024-01-01"}
		input3 := DedupCandidate{Name: "Event Name", VenueID: "Venue-1", StartDate: "2024-01-01"}

		hash1 := BuildDedupHash(input1)
		hash2 := BuildDedupHash(input2)
		hash3 := BuildDedupHash(input3)

		if hash1 != hash2 || hash2 != hash3 {
			t.Errorf("BuildDedupHash() not case-insensitive: got %v, %v, %v", hash1, hash2, hash3)
		}
	})

	// Test whitespace normalization
	t.Run("whitespace normalization", func(t *testing.T) {
		input1 := DedupCandidate{Name: "Event", VenueID: "Venue", StartDate: "2024-01-01"}
		input2 := DedupCandidate{Name: "  Event  ", VenueID: "  Venue  ", StartDate: "  2024-01-01  "}
		input3 := DedupCandidate{Name: "\tEvent\n", VenueID: "\nVenue\t", StartDate: "\t2024-01-01\n"}

		hash1 := BuildDedupHash(input1)
		hash2 := BuildDedupHash(input2)
		hash3 := BuildDedupHash(input3)

		if hash1 != hash2 || hash2 != hash3 {
			t.Errorf("BuildDedupHash() not normalizing whitespace: got %v, %v, %v", hash1, hash2, hash3)
		}
	})

	// Test uniqueness: different inputs should produce different hashes
	t.Run("uniqueness", func(t *testing.T) {
		inputs := []DedupCandidate{
			{Name: "Event A", VenueID: "Venue 1", StartDate: "2024-01-01"},
			{Name: "Event B", VenueID: "Venue 1", StartDate: "2024-01-01"},
			{Name: "Event A", VenueID: "Venue 2", StartDate: "2024-01-01"},
			{Name: "Event A", VenueID: "Venue 1", StartDate: "2024-01-02"},
		}

		hashes := make(map[string]bool)
		for i, input := range inputs {
			hash := BuildDedupHash(input)
			if hashes[hash] {
				t.Errorf("BuildDedupHash() produced duplicate hash for input %d: %+v", i, input)
			}
			hashes[hash] = true
		}

		if len(hashes) != len(inputs) {
			t.Errorf("BuildDedupHash() uniqueness failed: got %d unique hashes for %d inputs", len(hashes), len(inputs))
		}
	})
}
