package events

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

func TestIngestService_GeographicBoundaryMode(t *testing.T) {
	t.Parallel()

	insideBoundary := config.GeographicBoundaryConfig{
		Localities: []string{"Toronto"},
		Mode:       "review",
	}

	rejectBoundary := config.GeographicBoundaryConfig{
		Localities: []string{"Toronto"},
		Mode:       "reject",
	}

	wellFormedEvent := func(locality string) EventInput {
		return EventInput{
			Name:        "Test Event",
			Description: "Test description",
			Image:       "https://example.com/image.jpg",
			StartDate:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			License:     "CC0-1.0",
			Location:    &PlaceInput{Name: "Test Venue", AddressLocality: locality},
		}
	}

	tests := []struct {
		name            string
		cfg             config.GeographicBoundaryConfig
		input           EventInput
		wantErr         bool
		wantWarningCode string
		wantNeedsReview bool
	}{
		{
			name:            "review mode: outside boundary -> warning, needs review",
			cfg:             insideBoundary,
			input:           wellFormedEvent("Vancouver"),
			wantErr:         false,
			wantWarningCode: "outside_geo_boundary",
			wantNeedsReview: true,
		},
		{
			name:            "review mode: inside boundary -> no boundary warning, not flagged for review",
			cfg:             insideBoundary,
			input:           wellFormedEvent("Toronto"),
			wantErr:         false,
			wantNeedsReview: false,
		},
		{
			name:    "reject mode (default): outside boundary -> hard error",
			cfg:     rejectBoundary,
			input:   wellFormedEvent("Vancouver"),
			wantErr: true,
		},
		{
			name: "reject mode (default with empty Mode): outside boundary -> hard error",
			cfg: config.GeographicBoundaryConfig{
				Localities: []string{"Toronto"},
			},
			input:   wellFormedEvent("Vancouver"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			service := NewIngestService(repo, "https://test.com", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true}, zerolog.Nop())
			service.WithGeographicBoundaryConfig(tt.cfg)

			result, err := service.Ingest(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Ingest() returned nil result")
			}

			if tt.wantWarningCode != "" {
				found := false
				for _, w := range result.Warnings {
					if w.Code == tt.wantWarningCode {
						found = true
						if w.Field != "location" {
							t.Errorf("warning field = %q, want %q", w.Field, "location")
						}
						break
					}
				}
				if !found {
					t.Errorf("expected warning code %q, got warnings: %v", tt.wantWarningCode, result.Warnings)
				}
			} else {
				for _, w := range result.Warnings {
					if w.Code == "outside_geo_boundary" {
						t.Errorf("unexpected outside_geo_boundary warning: %+v", w)
					}
				}
			}

			if result.NeedsReview != tt.wantNeedsReview {
				t.Errorf("NeedsReview = %v, want %v", result.NeedsReview, tt.wantNeedsReview)
			}
		})
	}
}
