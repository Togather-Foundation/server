package events

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
)

func TestNormalizeLocationName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Toronto", "toronto"},
		{"Old Toronto", "oldtoronto"},
		{"North York", "northyork"},
		{"St. Catharines", "stcatharines"},
		{"123 Main St.", "123mainst"},
		{"Montréal", "montréal"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLocationName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLocationName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCheckGeographicBoundary(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.GeographicBoundaryConfig
		input    EventInput
		wantErr  bool
		errField string
	}{
		{
			name:    "disabled when config empty",
			cfg:     config.GeographicBoundaryConfig{},
			input:   EventInput{Location: &PlaceInput{AddressLocality: "Toronto"}},
			wantErr: false,
		},
		{
			name:    "virtual event with no location",
			cfg:     config.GeographicBoundaryConfig{Regions: []string{"Ontario"}},
			input:   EventInput{Location: nil},
			wantErr: false,
		},
		{
			name:    "region matches",
			cfg:     config.GeographicBoundaryConfig{Regions: []string{"Ontario"}},
			input:   EventInput{Location: &PlaceInput{AddressRegion: "Ontario"}},
			wantErr: false,
		},
		{
			name:     "region mismatch rejects",
			cfg:      config.GeographicBoundaryConfig{Regions: []string{"Ontario"}},
			input:    EventInput{Location: &PlaceInput{AddressRegion: "Quebec"}},
			wantErr:  true,
			errField: "region",
		},
		{
			name:    "locality exact match",
			cfg:     config.GeographicBoundaryConfig{Localities: []string{"Toronto", "Mississauga"}},
			input:   EventInput{Location: &PlaceInput{AddressLocality: "Toronto"}},
			wantErr: false,
		},
		{
			name:     "locality mismatch rejects",
			cfg:      config.GeographicBoundaryConfig{Localities: []string{"Toronto"}},
			input:    EventInput{Location: &PlaceInput{AddressLocality: "Vancouver"}},
			wantErr:  true,
			errField: "locality",
		},
		{
			name:    "normalized match - spaces and uppercase",
			cfg:     config.GeographicBoundaryConfig{Localities: []string{"North York"}},
			input:   EventInput{Location: &PlaceInput{AddressLocality: "North York"}},
			wantErr: false,
		},
		{
			name:    "normalized match - punctuation stripped",
			cfg:     config.GeographicBoundaryConfig{Localities: []string{"St Catharines"}},
			input:   EventInput{Location: &PlaceInput{AddressLocality: "St. Catharines"}},
			wantErr: false,
		},
		{
			name: "region passes but locality fails - still rejected",
			cfg: config.GeographicBoundaryConfig{
				Regions:    []string{"Ontario"},
				Localities: []string{"Toronto"},
			},
			input: EventInput{Location: &PlaceInput{
				AddressRegion:   "Ontario",
				AddressLocality: "Vancouver",
			}},
			wantErr:  true,
			errField: "locality",
		},
		{
			name: "both region and locality pass",
			cfg: config.GeographicBoundaryConfig{
				Regions:    []string{"Ontario"},
				Localities: []string{"Toronto"},
			},
			input: EventInput{Location: &PlaceInput{
				AddressRegion:   "Ontario",
				AddressLocality: "Toronto",
			}},
			wantErr: false,
		},
		{
			name:    "region empty in event, localities configured - falls through to locality check",
			cfg:     config.GeographicBoundaryConfig{Localities: []string{"Toronto"}},
			input:   EventInput{Location: &PlaceInput{AddressLocality: "Toronto"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckGeographicBoundary(tt.input, tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				gbe, ok := err.(*ErrOutsideGeographicBoundary)
				if !ok {
					t.Fatalf("expected *ErrOutsideGeographicBoundary, got %T: %v", err, err)
				}
				if gbe.Field != tt.errField {
					t.Errorf("field = %q, want %q", gbe.Field, tt.errField)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
