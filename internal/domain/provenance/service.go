package provenance

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetOrCreateSource(ctx context.Context, params CreateSourceParams) (*Source, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("provenance service not configured")
	}

	baseURL := strings.TrimSpace(params.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("source base url required")
	}

	if existing, err := s.repo.GetByBaseURL(ctx, baseURL); err == nil && existing != nil {
		return existing, nil
	}

	params.BaseURL = baseURL
	return s.repo.Create(ctx, params)
}

// EventSourceAttribution represents source attribution for an event
type EventSourceAttribution struct {
	SourceID      string
	SourceName    string
	SourceType    string
	SourceURL     string
	TrustLevel    int
	LicenseURL    string
	LicenseType   string
	Confidence    *float64
	RetrievedAt   time.Time
	SourceEventID *string
}

// FieldProvenanceInfo represents field-level provenance information
type FieldProvenanceInfo struct {
	FieldPath    string
	SourceID     string
	SourceName   string
	SourceType   string
	TrustLevel   int
	LicenseURL   string
	LicenseType  string
	Confidence   float64
	ObservedAt   time.Time
	ValuePreview *string
}

// GetEventSourceAttribution retrieves all sources that contributed to an event
// Returns sources ordered by trust level DESC, confidence DESC, retrieved_at DESC
// per FR-024 and FR-029
func (s *Service) GetEventSourceAttribution(ctx context.Context, eventID string) ([]EventSourceAttribution, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("provenance service not configured")
	}

	return s.repo.GetEventSources(ctx, eventID)
}

// GetFieldProvenance retrieves field-level provenance for an event
// Optionally filtered by specific field paths
// Only returns active (applied_to_canonical = true, superseded_at IS NULL) records
// Ordered by conflict resolution priority: trust_level DESC, confidence DESC, observed_at DESC
func (s *Service) GetFieldProvenance(ctx context.Context, eventID string, fieldPaths []string) ([]FieldProvenanceInfo, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("provenance service not configured")
	}

	if len(fieldPaths) > 0 {
		return s.repo.GetFieldProvenanceForPaths(ctx, eventID, fieldPaths)
	}

	return s.repo.GetFieldProvenance(ctx, eventID)
}

// GetCanonicalFieldProvenance returns the winning field value based on conflict resolution
// Priority: trust_level DESC, confidence DESC, observed_at DESC
func (s *Service) GetCanonicalFieldProvenance(ctx context.Context, eventID string, fieldPath string) (*FieldProvenanceInfo, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("provenance service not configured")
	}

	return s.repo.GetCanonicalFieldValue(ctx, eventID, fieldPath)
}

// GroupProvenanceByField organizes field provenance by field path
func GroupProvenanceByField(provenance []FieldProvenanceInfo) map[string][]FieldProvenanceInfo {
	result := make(map[string][]FieldProvenanceInfo)
	for _, p := range provenance {
		result[p.FieldPath] = append(result[p.FieldPath], p)
	}
	return result
}
