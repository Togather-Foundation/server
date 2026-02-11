package provenance

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidSourceType    = errors.New("invalid source type")
	ErrInvalidLicenseType   = errors.New("invalid license type")
	ErrInvalidTrustLevel    = errors.New("invalid trust level")
	ErrInvalidConfidence    = errors.New("invalid confidence value")
	ErrInvalidTimestamp     = errors.New("invalid timestamp")
	ErrMissingBaseURL       = errors.New("missing base URL")
	ErrInvalidBaseURL       = errors.New("invalid base URL format")
	ErrNonCC0License        = errors.New("non-CC0 license not allowed")
	ErrMissingSourceAttribution = errors.New("missing source attribution")
)

// ValidSourceTypes defines allowed source types per SEL specification
var ValidSourceTypes = map[string]bool{
	"scraper":    true,
	"partner":    true,
	"user":       true,
	"federation": true,
}

// ValidLicenseTypes defines allowed license types per SEL specification
var ValidLicenseTypes = map[string]bool{
	"cc0":         true,
	"cc-by":       true,
	"proprietary": true,
	"unknown":     true,
}

// CC0LicenseURL is the canonical CC0 1.0 Universal license URL
const CC0LicenseURL = "https://creativecommons.org/publicdomain/zero/1.0/"

// ValidateSource validates source creation parameters per SEL Core Profile §6.1
func ValidateSource(params CreateSourceParams) error {
	// Validate base URL
	if params.BaseURL == "" {
		return ErrMissingBaseURL
	}
	
	baseURL := strings.TrimSpace(params.BaseURL)
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("%w: must be valid http(s) URL", ErrInvalidBaseURL)
	}
	
	// Validate source type
	if !ValidSourceTypes[params.SourceType] {
		return fmt.Errorf("%w: got %q, expected one of: scraper, partner, user, federation", ErrInvalidSourceType, params.SourceType)
	}
	
	// Validate license type
	if !ValidLicenseTypes[params.LicenseType] {
		return fmt.Errorf("%w: got %q, expected one of: cc0, cc-by, proprietary, unknown", ErrInvalidLicenseType, params.LicenseType)
	}
	
	// Validate trust level (1-10 per SEL spec §6.1)
	if params.TrustLevel < 1 || params.TrustLevel > 10 {
		return fmt.Errorf("%w: must be between 1 and 10, got %d", ErrInvalidTrustLevel, params.TrustLevel)
	}
	
	// Validate license URL if provided
	if params.LicenseURL != "" {
		if _, err := url.Parse(params.LicenseURL); err != nil {
			return fmt.Errorf("invalid license URL: %w", err)
		}
	}
	
	return nil
}

// ValidateSourceAttribution validates event source attribution per SEL Core Profile §6.5
func ValidateSourceAttribution(attr EventSourceAttribution) error {
	if attr.SourceID == "" {
		return fmt.Errorf("%w: source_id is required", ErrMissingSourceAttribution)
	}
	
	if attr.SourceName == "" {
		return fmt.Errorf("%w: source_name is required", ErrMissingSourceAttribution)
	}
	
	if !ValidSourceTypes[attr.SourceType] {
		return fmt.Errorf("%w: got %q", ErrInvalidSourceType, attr.SourceType)
	}
	
	if attr.TrustLevel < 1 || attr.TrustLevel > 10 {
		return fmt.Errorf("%w: must be between 1 and 10, got %d", ErrInvalidTrustLevel, attr.TrustLevel)
	}
	
	if attr.Confidence != nil && (*attr.Confidence < 0.0 || *attr.Confidence > 1.0) {
		return fmt.Errorf("%w: must be between 0.0 and 1.0, got %f", ErrInvalidConfidence, *attr.Confidence)
	}
	
	if attr.RetrievedAt.IsZero() {
		return fmt.Errorf("%w: retrieved_at timestamp is required", ErrInvalidTimestamp)
	}
	
	// Validate license URL
	if attr.LicenseURL != "" {
		if _, err := url.Parse(attr.LicenseURL); err != nil {
			return fmt.Errorf("invalid license URL: %w", err)
		}
	}
	
	return nil
}

// ValidateFieldProvenance validates field-level provenance per SEL Core Profile §6.3
func ValidateFieldProvenance(fp FieldProvenanceInfo) error {
	if fp.FieldPath == "" {
		return errors.New("field_path is required")
	}
	
	// Validate field path is a valid JSON Pointer (starts with /)
	if !strings.HasPrefix(fp.FieldPath, "/") {
		return fmt.Errorf("field_path must be valid JSON Pointer, got: %s", fp.FieldPath)
	}
	
	if fp.SourceID == "" {
		return fmt.Errorf("%w: source_id is required for field %s", ErrMissingSourceAttribution, fp.FieldPath)
	}
	
	if !ValidSourceTypes[fp.SourceType] {
		return fmt.Errorf("%w: got %q for field %s", ErrInvalidSourceType, fp.SourceType, fp.FieldPath)
	}
	
	if fp.TrustLevel < 1 || fp.TrustLevel > 10 {
		return fmt.Errorf("%w: must be between 1 and 10, got %d for field %s", ErrInvalidTrustLevel, fp.TrustLevel, fp.FieldPath)
	}
	
	if fp.Confidence < 0.0 || fp.Confidence > 1.0 {
		return fmt.Errorf("%w: must be between 0.0 and 1.0, got %f for field %s", ErrInvalidConfidence, fp.Confidence, fp.FieldPath)
	}
	
	if fp.ObservedAt.IsZero() {
		return fmt.Errorf("%w: observed_at is required for field %s", ErrInvalidTimestamp, fp.FieldPath)
	}
	
	// Validate observed_at is not in the future (with 5 minute tolerance for clock skew)
	if fp.ObservedAt.After(time.Now().Add(5 * time.Minute)) {
		return fmt.Errorf("%w: observed_at cannot be in the future for field %s", ErrInvalidTimestamp, fp.FieldPath)
	}
	
	return nil
}

// RequireCC0License validates that a source has CC0 license per SEL Core Profile §7.1
func RequireCC0License(licenseType string, licenseURL string) error {
	if licenseType != "cc0" {
		return fmt.Errorf("%w: only CC0 sources accepted at ingestion boundary, got: %s", ErrNonCC0License, licenseType)
	}
	
	// Normalize license URL for comparison
	normalizedURL := strings.TrimSpace(strings.ToLower(licenseURL))
	normalizedExpected := strings.ToLower(CC0LicenseURL)
	
	if normalizedURL != normalizedExpected && normalizedURL != strings.TrimRight(normalizedExpected, "/") {
		return fmt.Errorf("%w: expected %s, got: %s", ErrNonCC0License, CC0LicenseURL, licenseURL)
	}
	
	return nil
}

// ValidateProvenanceTracking validates that an event has complete provenance per SEL Core Profile §6
func ValidateProvenanceTracking(sources []EventSourceAttribution, fieldProvenance []FieldProvenanceInfo) error {
	if len(sources) == 0 {
		return fmt.Errorf("%w: event must have at least one source", ErrMissingSourceAttribution)
	}
	
	// Validate each source attribution
	for i, source := range sources {
		if err := ValidateSourceAttribution(source); err != nil {
			return fmt.Errorf("source[%d]: %w", i, err)
		}
	}
	
	// Validate field provenance entries
	for i, fp := range fieldProvenance {
		if err := ValidateFieldProvenance(fp); err != nil {
			return fmt.Errorf("field_provenance[%d]: %w", i, err)
		}
	}
	
	// Ensure all field provenance references valid sources
	sourceIDs := make(map[string]bool)
	for _, source := range sources {
		sourceIDs[source.SourceID] = true
	}
	
	for i, fp := range fieldProvenance {
		if !sourceIDs[fp.SourceID] {
			return fmt.Errorf("field_provenance[%d]: references unknown source_id %s", i, fp.SourceID)
		}
	}
	
	return nil
}
