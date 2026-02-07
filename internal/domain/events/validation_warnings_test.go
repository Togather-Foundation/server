package events

import (
	"strings"
	"testing"
	"time"
)

func TestValidateEventInputWithWarnings_ReversedDates(t *testing.T) {
	tests := []struct {
		name            string
		input           EventInput
		wantErr         bool
		wantWarningCode string
	}{
		{
			name: "reversed dates - small gap, early morning end (likely timezone)",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T23:00:00Z", // 11 PM
				EndDate:   "2025-04-01T02:00:00Z", // 2 AM (early morning 0-4)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "reversed_dates_timezone_likely",
		},
		{
			name: "reversed dates - afternoon end (needs review)",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T22:00:00Z",
				EndDate:   "2025-04-01T14:00:00Z", // 2 PM (not early morning)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "reversed_dates_corrected_needs_review",
		},
		{
			name: "reversed dates - large gap (needs review)",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-03T10:00:00Z",
				EndDate:   "2025-04-01T10:00:00Z", // 2 days before
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "reversed_dates_corrected_needs_review",
		},
		{
			name: "reversed dates - exactly at 24h boundary (needs review)",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-02T10:00:00Z",
				EndDate:   "2025-04-01T10:00:00Z", // exactly 24 hours
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "reversed_dates_corrected_needs_review",
		},
		{
			name: "normal dates - no warning",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T12:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "",
		},
		{
			name: "no end date - no warning",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "",
		},
		{
			name: "same start and end - no warning",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T10:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "",
		},
		{
			name: "reversed with early morning end and reasonable duration - timezone likely",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T22:00:00Z", // 10 PM
				EndDate:   "2025-04-01T03:00:00Z", // 3 AM (early morning, would be 5h duration)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "reversed_dates_timezone_likely",
		},
		{
			name: "reversed by 1 hour at noon - needs review",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T13:00:00Z", // 1 PM
				EndDate:   "2025-04-01T12:00:00Z", // noon (not early morning)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:         false,
			wantWarningCode: "reversed_dates_corrected_needs_review",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateEventInputWithWarnings(tt.input, "https://test.com", nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateEventInputWithWarnings() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateEventInputWithWarnings() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("ValidateEventInputWithWarnings() returned nil result")
				return
			}

			if tt.wantWarningCode == "" {
				// Expect no warnings
				if len(result.Warnings) > 0 {
					t.Errorf("Expected no warnings, got %v", result.Warnings)
				}
			} else {
				// Expect specific warning code
				found := false
				for _, w := range result.Warnings {
					if w.Code == tt.wantWarningCode {
						found = true
						// Verify warning has required fields
						if w.Field == "" {
							t.Error("Warning missing Field")
						}
						if w.Message == "" {
							t.Error("Warning missing Message")
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected warning code %q, got warnings: %v", tt.wantWarningCode, result.Warnings)
				}
			}
		})
	}
}

func TestValidateEventInputWithWarnings_DetectNormalizationCorrections(t *testing.T) {
	tests := []struct {
		name            string
		original        EventInput
		normalized      EventInput
		wantWarningCode string
	}{
		{
			name: "detects auto-corrected timezone error (early morning, short duration)",
			original: EventInput{
				Name:      "Test Event",
				StartDate: "2025-03-31T23:00:00Z", // 11 PM
				EndDate:   "2025-03-31T02:00:00Z", // 2 AM (reversed)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			normalized: EventInput{
				Name:      "Test Event",
				StartDate: "2025-03-31T23:00:00Z",
				EndDate:   "2025-04-01T02:00:00Z", // Corrected to next day
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantWarningCode: "reversed_dates_timezone_likely",
		},
		{
			name: "detects corrected dates that need review (not early morning)",
			original: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T22:00:00Z",
				EndDate:   "2025-04-01T14:00:00Z", // 2 PM (reversed, not early morning)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			normalized: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T22:00:00Z",
				EndDate:   "2025-04-02T14:00:00Z", // Corrected
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantWarningCode: "reversed_dates_corrected_needs_review",
		},
		{
			name: "detects corrected dates with long duration (needs review)",
			original: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T03:00:00Z", // 3 AM (early morning but 7h before start)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			normalized: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-02T03:00:00Z", // Corrected to next day (17h duration)
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantWarningCode: "reversed_dates_corrected_needs_review",
		},
		{
			name: "no warning when dates unchanged",
			original: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T12:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			normalized: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T12:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantWarningCode: "",
		},
		{
			name:     "no warning when original is nil (backward compat)",
			original: EventInput{},
			normalized: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T02:00:00Z", // Reversed but no original to compare
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantWarningCode: "reversed_dates_corrected_needs_review", // Still generates warning for reversed dates in input
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result *ValidationResult
			var err error

			if tt.name == "no warning when original is nil (backward compat)" {
				// Test backward compatibility - pass nil for original
				result, err = ValidateEventInputWithWarnings(tt.normalized, "https://test.com", nil)
			} else {
				// Test with original input
				result, err = ValidateEventInputWithWarnings(tt.normalized, "https://test.com", &tt.original)
			}

			if err != nil {
				t.Errorf("ValidateEventInputWithWarnings() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("ValidateEventInputWithWarnings() returned nil result")
				return
			}

			if tt.wantWarningCode == "" {
				// Expect no warnings
				if len(result.Warnings) > 0 {
					t.Errorf("Expected no warnings, got %v", result.Warnings)
				}
			} else {
				// Expect specific warning code
				found := false
				for _, w := range result.Warnings {
					if w.Code == tt.wantWarningCode {
						found = true
						// Verify warning has required fields
						if w.Field == "" {
							t.Error("Warning missing Field")
						}
						if w.Message == "" {
							t.Error("Warning missing Message")
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected warning code %q, got warnings: %v", tt.wantWarningCode, result.Warnings)
				}
			}
		})
	}
}

func TestValidateEventInputWithWarnings_BackwardCompatibility(t *testing.T) {
	// Verify ValidateEventInput() still works (backward compatibility)
	input := EventInput{
		Name:      "Test Event",
		StartDate: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		Location:  &PlaceInput{Name: "Test Venue"},
	}

	validated, err := ValidateEventInput(input, "https://test.com")
	if err != nil {
		t.Errorf("ValidateEventInput() unexpected error = %v", err)
	}

	if validated.Name != input.Name {
		t.Errorf("ValidateEventInput() Name = %v, want %v", validated.Name, input.Name)
	}
}

func TestValidateEventInputWithWarnings_ErrorsStillRejected(t *testing.T) {
	// Verify that actual validation ERRORS still cause rejection
	tests := []struct {
		name        string
		input       EventInput
		wantErr     bool
		errContains string
	}{
		{
			name: "missing name - should error",
			input: EventInput{
				StartDate: "2025-04-01T10:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:     true,
			errContains: "name",
		},
		{
			name: "missing location - should error",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-01T10:00:00Z",
			},
			wantErr:     true,
			errContains: "location",
		},
		{
			name: "invalid startDate - should error",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "not-a-date",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr:     true,
			errContains: "startDate",
		},
		{
			name: "reversed dates - should warn, not error",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2025-04-02T10:00:00Z",
				EndDate:   "2025-04-01T10:00:00Z",
				Location:  &PlaceInput{Name: "Test Venue"},
			},
			wantErr: false, // Should NOT error, should warn
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateEventInputWithWarnings(tt.input, "https://test.com", nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateEventInputWithWarnings() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateEventInputWithWarnings() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateEventInputWithWarnings() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("ValidateEventInputWithWarnings() returned nil result")
			}
		})
	}
}

func TestValidationWarning_Structure(t *testing.T) {
	// Test that ValidationWarning has correct structure
	warning := ValidationWarning{
		Field:   "endDate",
		Message: "endDate is before startDate",
		Code:    "reversed_dates",
	}

	if warning.Field != "endDate" {
		t.Errorf("ValidationWarning.Field = %v, want %v", warning.Field, "endDate")
	}
	if warning.Message == "" {
		t.Error("ValidationWarning.Message should not be empty")
	}
	if warning.Code == "" {
		t.Error("ValidationWarning.Code should not be empty")
	}
}

func TestValidationResult_Structure(t *testing.T) {
	// Test that ValidationResult has correct structure
	input := EventInput{
		Name:      "Test Event",
		StartDate: "2025-04-01T10:00:00Z",
		Location:  &PlaceInput{Name: "Test Venue"},
	}

	result := &ValidationResult{
		Input: input,
		Warnings: []ValidationWarning{
			{
				Field:   "test",
				Message: "test message",
				Code:    "test_code",
			},
		},
	}

	if result.Input.Name != input.Name {
		t.Error("ValidationResult.Input not properly stored")
	}
	if len(result.Warnings) != 1 {
		t.Errorf("ValidationResult.Warnings length = %v, want 1", len(result.Warnings))
	}
}
