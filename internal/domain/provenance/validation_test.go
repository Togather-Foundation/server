package provenance_test

import (
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/provenance"
)

func TestValidateSource(t *testing.T) {
	tests := []struct {
		name    string
		params  provenance.CreateSourceParams
		wantErr bool
		errType error
	}{
		{
			name: "valid_source",
			params: provenance.CreateSourceParams{
				Name:        "Ticketmaster Scraper",
				SourceType:  "scraper",
				BaseURL:     "https://ticketmaster.com",
				LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
				LicenseType: "cc0",
				TrustLevel:  8,
			},
			wantErr: false,
		},
		{
			name: "missing_base_url",
			params: provenance.CreateSourceParams{
				Name:        "Test Source",
				SourceType:  "scraper",
				BaseURL:     "",
				LicenseType: "cc0",
				TrustLevel:  5,
			},
			wantErr: true,
			errType: provenance.ErrMissingBaseURL,
		},
		{
			name: "invalid_base_url",
			params: provenance.CreateSourceParams{
				Name:        "Test Source",
				SourceType:  "scraper",
				BaseURL:     "not-a-url",
				LicenseType: "cc0",
				TrustLevel:  5,
			},
			wantErr: true,
			errType: provenance.ErrInvalidBaseURL,
		},
		{
			name: "invalid_source_type",
			params: provenance.CreateSourceParams{
				Name:        "Test Source",
				SourceType:  "invalid",
				BaseURL:     "https://example.com",
				LicenseType: "cc0",
				TrustLevel:  5,
			},
			wantErr: true,
			errType: provenance.ErrInvalidSourceType,
		},
		{
			name: "invalid_license_type",
			params: provenance.CreateSourceParams{
				Name:        "Test Source",
				SourceType:  "scraper",
				BaseURL:     "https://example.com",
				LicenseType: "invalid",
				TrustLevel:  5,
			},
			wantErr: true,
			errType: provenance.ErrInvalidLicenseType,
		},
		{
			name: "trust_level_too_low",
			params: provenance.CreateSourceParams{
				Name:        "Test Source",
				SourceType:  "scraper",
				BaseURL:     "https://example.com",
				LicenseType: "cc0",
				TrustLevel:  0,
			},
			wantErr: true,
			errType: provenance.ErrInvalidTrustLevel,
		},
		{
			name: "trust_level_too_high",
			params: provenance.CreateSourceParams{
				Name:        "Test Source",
				SourceType:  "scraper",
				BaseURL:     "https://example.com",
				LicenseType: "cc0",
				TrustLevel:  11,
			},
			wantErr: true,
			errType: provenance.ErrInvalidTrustLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provenance.ValidateSource(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errType != nil && !IsErrorType(err, tt.errType) {
				t.Errorf("ValidateSource() error = %v, wantErrType %v", err, tt.errType)
			}
		})
	}
}

func TestValidateSourceAttribution(t *testing.T) {
	confidence := 0.95

	tests := []struct {
		name    string
		attr    provenance.EventSourceAttribution
		wantErr bool
		errType error
	}{
		{
			name: "valid_attribution",
			attr: provenance.EventSourceAttribution{
				SourceID:    "source123",
				SourceName:  "Ticketmaster",
				SourceType:  "scraper",
				TrustLevel:  8,
				Confidence:  &confidence,
				RetrievedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing_source_id",
			attr: provenance.EventSourceAttribution{
				SourceID:    "",
				SourceName:  "Ticketmaster",
				SourceType:  "scraper",
				TrustLevel:  8,
				RetrievedAt: time.Now(),
			},
			wantErr: true,
			errType: provenance.ErrMissingSourceAttribution,
		},
		{
			name: "invalid_confidence",
			attr: provenance.EventSourceAttribution{
				SourceID:    "source123",
				SourceName:  "Ticketmaster",
				SourceType:  "scraper",
				TrustLevel:  8,
				Confidence:  ptrFloat(1.5), // > 1.0
				RetrievedAt: time.Now(),
			},
			wantErr: true,
			errType: provenance.ErrInvalidConfidence,
		},
		{
			name: "missing_timestamp",
			attr: provenance.EventSourceAttribution{
				SourceID:   "source123",
				SourceName: "Ticketmaster",
				SourceType: "scraper",
				TrustLevel: 8,
			},
			wantErr: true,
			errType: provenance.ErrInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provenance.ValidateSourceAttribution(tt.attr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSourceAttribution() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errType != nil && !IsErrorType(err, tt.errType) {
				t.Errorf("ValidateSourceAttribution() error = %v, wantErrType %v", err, tt.errType)
			}
		})
	}
}

func TestValidateFieldProvenance(t *testing.T) {
	tests := []struct {
		name    string
		fp      provenance.FieldProvenanceInfo
		wantErr bool
		errType error
	}{
		{
			name: "valid_field_provenance",
			fp: provenance.FieldProvenanceInfo{
				FieldPath:  "/name",
				SourceID:   "source123",
				SourceType: "scraper",
				TrustLevel: 8,
				Confidence: 0.95,
				ObservedAt: time.Now().Add(-1 * time.Hour),
			},
			wantErr: false,
		},
		{
			name: "invalid_field_path",
			fp: provenance.FieldProvenanceInfo{
				FieldPath:  "name", // Missing leading /
				SourceID:   "source123",
				SourceType: "scraper",
				TrustLevel: 8,
				Confidence: 0.95,
				ObservedAt: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "future_timestamp",
			fp: provenance.FieldProvenanceInfo{
				FieldPath:  "/name",
				SourceID:   "source123",
				SourceType: "scraper",
				TrustLevel: 8,
				Confidence: 0.95,
				ObservedAt: time.Now().Add(10 * time.Minute), // In the future
			},
			wantErr: true,
			errType: provenance.ErrInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provenance.ValidateFieldProvenance(tt.fp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFieldProvenance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errType != nil && !IsErrorType(err, tt.errType) {
				t.Errorf("ValidateFieldProvenance() error = %v, wantErrType %v", err, tt.errType)
			}
		})
	}
}

func TestRequireCC0License(t *testing.T) {
	tests := []struct {
		name        string
		licenseType string
		licenseURL  string
		wantErr     bool
	}{
		{
			name:        "valid_cc0",
			licenseType: "cc0",
			licenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			wantErr:     false,
		},
		{
			name:        "valid_cc0_no_trailing_slash",
			licenseType: "cc0",
			licenseURL:  "https://creativecommons.org/publicdomain/zero/1.0",
			wantErr:     false,
		},
		{
			name:        "non_cc0_license",
			licenseType: "cc-by",
			licenseURL:  "https://creativecommons.org/licenses/by/4.0/",
			wantErr:     true,
		},
		{
			name:        "wrong_license_url",
			licenseType: "cc0",
			licenseURL:  "https://example.com/license",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provenance.RequireCC0License(tt.licenseType, tt.licenseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequireCC0License() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProvenanceTracking(t *testing.T) {
	now := time.Now()
	confidence := 0.95

	tests := []struct {
		name            string
		sources         []provenance.EventSourceAttribution
		fieldProvenance []provenance.FieldProvenanceInfo
		wantErr         bool
	}{
		{
			name: "valid_provenance",
			sources: []provenance.EventSourceAttribution{
				{
					SourceID:    "src1",
					SourceName:  "Source 1",
					SourceType:  "scraper",
					TrustLevel:  8,
					Confidence:  &confidence,
					RetrievedAt: now,
				},
			},
			fieldProvenance: []provenance.FieldProvenanceInfo{
				{
					FieldPath:  "/name",
					SourceID:   "src1",
					SourceType: "scraper",
					TrustLevel: 8,
					Confidence: 0.95,
					ObservedAt: now,
				},
			},
			wantErr: false,
		},
		{
			name:            "missing_sources",
			sources:         []provenance.EventSourceAttribution{},
			fieldProvenance: []provenance.FieldProvenanceInfo{},
			wantErr:         true,
		},
		{
			name: "unknown_source_in_field_provenance",
			sources: []provenance.EventSourceAttribution{
				{
					SourceID:    "src1",
					SourceName:  "Source 1",
					SourceType:  "scraper",
					TrustLevel:  8,
					Confidence:  &confidence,
					RetrievedAt: now,
				},
			},
			fieldProvenance: []provenance.FieldProvenanceInfo{
				{
					FieldPath:  "/name",
					SourceID:   "unknown_source", // References non-existent source
					SourceType: "scraper",
					TrustLevel: 8,
					Confidence: 0.95,
					ObservedAt: now,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provenance.ValidateProvenanceTracking(tt.sources, tt.fieldProvenance)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProvenanceTracking() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper functions

func ptrFloat(f float64) *float64 {
	return &f
}

// IsErrorType checks if an error wraps a specific error type
func IsErrorType(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	// Simple string contains check for now
	return err.Error() != "" && target.Error() != "" &&
		(err == target || err.Error()[:min(len(err.Error()), len(target.Error()))] == target.Error()[:min(len(err.Error()), len(target.Error()))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
