package events

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
)

func TestAppendQualityWarnings_ImageValidationFlag(t *testing.T) {
	input := EventInput{
		Name:        "Test Event",
		Description: "Test description",
		Image:       "", // Missing image
		StartDate:   "2026-06-15T10:00:00-04:00",
	}

	t.Run("RequireImage=true adds missing_image warning", func(t *testing.T) {
		cfg := config.ValidationConfig{RequireImage: true}
		warnings := appendQualityWarnings([]ValidationWarning{}, input, nil, cfg)

		found := false
		for _, w := range warnings {
			if w.Code == "missing_image" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Expected missing_image warning when RequireImage=true, but not found")
		}
	})

	t.Run("RequireImage=false does NOT add missing_image warning", func(t *testing.T) {
		cfg := config.ValidationConfig{RequireImage: false}
		warnings := appendQualityWarnings([]ValidationWarning{}, input, nil, cfg)

		for _, w := range warnings {
			if w.Code == "missing_image" {
				t.Errorf("Unexpected missing_image warning when RequireImage=false: %+v", w)
			}
		}
	})

	t.Run("With image present, no warning regardless of flag", func(t *testing.T) {
		inputWithImage := input
		inputWithImage.Image = "https://example.com/image.jpg"

		// Test with RequireImage=true
		cfg := config.ValidationConfig{RequireImage: true}
		warnings := appendQualityWarnings([]ValidationWarning{}, inputWithImage, nil, cfg)
		for _, w := range warnings {
			if w.Code == "missing_image" {
				t.Errorf("Unexpected missing_image warning when image is present (RequireImage=true): %+v", w)
			}
		}

		// Test with RequireImage=false
		cfg = config.ValidationConfig{RequireImage: false}
		warnings = appendQualityWarnings([]ValidationWarning{}, inputWithImage, nil, cfg)
		for _, w := range warnings {
			if w.Code == "missing_image" {
				t.Errorf("Unexpected missing_image warning when image is present (RequireImage=false): %+v", w)
			}
		}
	})
}
