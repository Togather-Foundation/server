package events

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
)

// TestIngestService_ImageRequirementIntegration tests the full ingestion flow
// with different VALIDATION_REQUIRE_IMAGE settings to ensure events are either
// published directly or sent to review queue based on the flag.
func TestIngestService_ImageRequirementIntegration(t *testing.T) {
	t.Run("RequireImage=false - event without image publishes directly", func(t *testing.T) {
		repo := NewMockRepository()

		service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: false})

		input := EventInput{
			Name:        "Event Without Image",
			Description: "Has description",
			Image:       "", // Missing image
			StartDate:   "2026-06-15T10:00:00-04:00",
			EndDate:     "2026-06-15T18:00:00-04:00",
			Location: &PlaceInput{
				Name:            "Test Venue",
				StreetAddress:   "123 Test St",
				AddressLocality: "Toronto",
				AddressRegion:   "ON",
				PostalCode:      "M5H 2N2",
				AddressCountry:  "CA",
			},
		}

		result, err := service.Ingest(context.Background(), input)
		if err != nil {
			t.Fatalf("Ingest failed: %v", err)
		}

		if result.NeedsReview {
			t.Error("Expected NeedsReview=false when RequireImage=false and image is missing")
		}

		if result.Event.LifecycleState != "published" {
			t.Errorf("Expected lifecycle_state=published, got %s", result.Event.LifecycleState)
		}

		// Verify no warnings for missing image
		for _, w := range result.Warnings {
			if w.Code == "missing_image" {
				t.Error("Unexpected missing_image warning when RequireImage=false")
			}
		}
	})

	t.Run("RequireImage=true - event without image goes to review", func(t *testing.T) {
		repo := NewMockRepository()

		service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: true})

		input := EventInput{
			Name:        "Event Without Image",
			Description: "Has description",
			Image:       "", // Missing image
			StartDate:   "2026-06-15T10:00:00-04:00",
			EndDate:     "2026-06-15T18:00:00-04:00",
			Location: &PlaceInput{
				Name:            "Test Venue",
				StreetAddress:   "123 Test St",
				AddressLocality: "Toronto",
				AddressRegion:   "ON",
				PostalCode:      "M5H 2N2",
				AddressCountry:  "CA",
			},
		}

		result, err := service.Ingest(context.Background(), input)
		if err != nil {
			t.Fatalf("Ingest failed: %v", err)
		}

		if !result.NeedsReview {
			t.Error("Expected NeedsReview=true when RequireImage=true and image is missing")
		}

		if result.Event.LifecycleState != "pending_review" {
			t.Errorf("Expected lifecycle_state=pending_review, got %s", result.Event.LifecycleState)
		}

		// Verify missing_image warning is present
		foundWarning := false
		for _, w := range result.Warnings {
			if w.Code == "missing_image" {
				foundWarning = true
				break
			}
		}
		if !foundWarning {
			t.Error("Expected missing_image warning when RequireImage=true and image is missing")
		}
	})

	t.Run("Event with image publishes directly regardless of flag", func(t *testing.T) {
		testCases := []struct {
			name         string
			requireImage bool
		}{
			{"RequireImage=false", false},
			{"RequireImage=true", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				repo := NewMockRepository()

				service := NewIngestService(repo, "https://test.com", config.ValidationConfig{RequireImage: tc.requireImage})

				input := EventInput{
					Name:        "Event With Image",
					Description: "Has description and image",
					Image:       "https://example.com/image.jpg", // Has image
					StartDate:   "2026-06-15T10:00:00-04:00",
					EndDate:     "2026-06-15T18:00:00-04:00",
					Location: &PlaceInput{
						Name:            "Test Venue",
						StreetAddress:   "123 Test St",
						AddressLocality: "Toronto",
						AddressRegion:   "ON",
						PostalCode:      "M5H 2N2",
						AddressCountry:  "CA",
					},
				}

				result, err := service.Ingest(context.Background(), input)
				if err != nil {
					t.Fatalf("Ingest failed: %v", err)
				}

				if result.NeedsReview {
					t.Errorf("Expected NeedsReview=false when image is present (RequireImage=%v)", tc.requireImage)
				}

				if result.Event.LifecycleState != "published" {
					t.Errorf("Expected lifecycle_state=published, got %s", result.Event.LifecycleState)
				}

				// Verify no missing_image warning
				for _, w := range result.Warnings {
					if w.Code == "missing_image" {
						t.Errorf("Unexpected missing_image warning when image is present (RequireImage=%v)", tc.requireImage)
					}
				}
			})
		}
	})
}
