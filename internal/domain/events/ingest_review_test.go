package events

import (
	"testing"
	"time"
)

func TestNeedsReview(t *testing.T) {
	tests := []struct {
		name         string
		input        EventInput
		linkStatuses map[string]int
		expected     bool
	}{
		{
			name: "complete event",
			input: EventInput{
				Name:        "Complete Event",
				Description: "Full description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			linkStatuses: nil,
			expected:     false,
		},
		{
			name: "missing description",
			input: EventInput{
				Name:      "Event Name",
				Image:     "https://example.com/image.jpg",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			linkStatuses: nil,
			expected:     true,
		},
		{
			name: "missing image",
			input: EventInput{
				Name:        "Event Name",
				Description: "Description",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			linkStatuses: nil,
			expected:     true,
		},
		{
			name: "too far in future (>730 days)",
			input: EventInput{
				Name:        "Event Name",
				Description: "Description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(800 * 24 * time.Hour).Format(time.RFC3339),
			},
			linkStatuses: nil,
			expected:     true,
		},
		{
			name: "broken link",
			input: EventInput{
				Name:        "Event Name",
				Description: "Description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			linkStatuses: map[string]int{"https://example.com/image.jpg": 404},
			expected:     true,
		},
		{
			name: "all good links",
			input: EventInput{
				Name:        "Event Name",
				Description: "Description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			linkStatuses: map[string]int{"https://example.com/image.jpg": 200},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needsReview(tt.input, tt.linkStatuses)
			if result != tt.expected {
				t.Errorf("needsReview() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestReviewConfidence(t *testing.T) {
	tests := []struct {
		name     string
		input    EventInput
		flagged  bool
		expected float64
	}{
		{
			name: "complete event not flagged",
			input: EventInput{
				Name:        "Complete Event",
				Description: "Full description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  false,
			expected: 0.9,
		},
		{
			name: "complete event flagged",
			input: EventInput{
				Name:        "Complete Event",
				Description: "Full description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  true,
			expected: 0.8,
		},
		{
			name: "missing description",
			input: EventInput{
				Name:      "Event Name",
				Image:     "https://example.com/image.jpg",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  false,
			expected: 0.7,
		},
		{
			name: "missing image",
			input: EventInput{
				Name:        "Event Name",
				Description: "Description",
				StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  false,
			expected: 0.7,
		},
		{
			name: "too far in future",
			input: EventInput{
				Name:        "Event Name",
				Description: "Description",
				Image:       "https://example.com/image.jpg",
				StartDate:   time.Now().Add(800 * 24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  false,
			expected: 0.7,
		},
		{
			name: "missing all optional fields",
			input: EventInput{
				Name:      "Event Name",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  false,
			expected: 0.5,
		},
		{
			name: "missing all optional fields and flagged",
			input: EventInput{
				Name:      "Event Name",
				StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  true,
			expected: 0.4,
		},
		{
			name: "extremely low confidence",
			input: EventInput{
				Name:      "Event Name",
				StartDate: time.Now().Add(800 * 24 * time.Hour).Format(time.RFC3339),
			},
			flagged:  true,
			expected: 0.2, // 0.9 - 0.2 (no desc) - 0.2 (no image) - 0.2 (too far) - 0.1 (flagged)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reviewConfidence(tt.input, tt.flagged)
			// Use epsilon for floating point comparison
			epsilon := 0.0001
			if result < tt.expected-epsilon || result > tt.expected+epsilon {
				t.Errorf("reviewConfidence() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsTooFarFuture(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		startDate string
		days      int
		expected  bool
	}{
		{
			name:      "empty start date",
			startDate: "",
			days:      730,
			expected:  false,
		},
		{
			name:      "within threshold (30 days)",
			startDate: now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
			days:      730,
			expected:  false,
		},
		{
			name:      "exactly at threshold",
			startDate: now.Add(730 * 24 * time.Hour).Format(time.RFC3339),
			days:      730,
			expected:  false,
		},
		{
			name:      "just over threshold",
			startDate: now.Add(731 * 24 * time.Hour).Format(time.RFC3339),
			days:      730,
			expected:  true,
		},
		{
			name:      "way in future (1000 days)",
			startDate: now.Add(1000 * 24 * time.Hour).Format(time.RFC3339),
			days:      730,
			expected:  true,
		},
		{
			name:      "in the past",
			startDate: now.Add(-10 * 24 * time.Hour).Format(time.RFC3339),
			days:      730,
			expected:  false,
		},
		{
			name:      "invalid date format",
			startDate: "not-a-date",
			days:      730,
			expected:  false,
		},
		{
			name:      "custom threshold (100 days)",
			startDate: now.Add(150 * 24 * time.Hour).Format(time.RFC3339),
			days:      100,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTooFarFuture(tt.startDate, tt.days)
			if result != tt.expected {
				t.Errorf("isTooFarFuture() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFloatPtr(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{name: "zero", input: 0.0},
		{name: "positive", input: 42.5},
		{name: "negative", input: -10.3},
		{name: "large", input: 999999.99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := floatPtr(tt.input)
			if result == nil {
				t.Error("floatPtr() returned nil")
				return
			}
			if *result != tt.input {
				t.Errorf("floatPtr() = %v, want %v", *result, tt.input)
			}
		})
	}
}

func TestHashInput(t *testing.T) {
	tests := []struct {
		name    string
		input   EventInput
		wantErr bool
	}{
		{
			name: "simple event",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2024-01-01T10:00:00Z",
			},
			wantErr: false,
		},
		{
			name:    "empty event",
			input:   EventInput{},
			wantErr: false,
		},
		{
			name: "complex event",
			input: EventInput{
				Name:        "Tech Conference 2024",
				Description: "A comprehensive technology conference",
				StartDate:   "2024-06-15T09:00:00Z",
				Location:    &PlaceInput{Name: "Convention Center"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := hashInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("hashInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Check hash format (SHA256 hex = 64 characters)
				if len(hash) != 64 {
					t.Errorf("hashInput() hash length = %d, want 64", len(hash))
				}
				// Verify hex encoding
				for _, c := range hash {
					if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
						t.Errorf("hashInput() contains non-hex character: %c", c)
					}
				}
			}
		})
	}

	// Test determinism
	t.Run("determinism", func(t *testing.T) {
		input := EventInput{Name: "Test", StartDate: "2024-01-01T00:00:00Z"}
		hash1, _ := hashInput(input)
		hash2, _ := hashInput(input)
		hash3, _ := hashInput(input)

		if hash1 != hash2 || hash2 != hash3 {
			t.Errorf("hashInput() not deterministic: got %v, %v, %v", hash1, hash2, hash3)
		}
	})

	// Test uniqueness
	t.Run("uniqueness", func(t *testing.T) {
		input1 := EventInput{Name: "Event A", StartDate: "2024-01-01T00:00:00Z"}
		input2 := EventInput{Name: "Event B", StartDate: "2024-01-01T00:00:00Z"}
		input3 := EventInput{Name: "Event A", StartDate: "2024-01-02T00:00:00Z"}

		hash1, _ := hashInput(input1)
		hash2, _ := hashInput(input2)
		hash3, _ := hashInput(input3)

		if hash1 == hash2 || hash2 == hash3 || hash1 == hash3 {
			t.Errorf("hashInput() produced duplicate hashes")
		}
	})
}

func TestNewIngestService(t *testing.T) {
	service := NewIngestService(nil, "https://example.com")

	if service == nil {
		t.Error("NewIngestService() returned nil")
	}

	if service.nodeDomain != "https://example.com" {
		t.Errorf("NewIngestService() nodeDomain = %v, want https://example.com", service.nodeDomain)
	}

	if service.defaultTZ != "America/Toronto" {
		t.Errorf("NewIngestService() defaultTZ = %v, want America/Toronto", service.defaultTZ)
	}
}
