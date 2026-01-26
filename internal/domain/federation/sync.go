package federation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidJSONLD        = errors.New("invalid JSON-LD payload")
	ErrMissingID            = errors.New("missing @id field in JSON-LD")
	ErrMissingType          = errors.New("missing @type field in JSON-LD")
	ErrUnsupportedType      = errors.New("unsupported @type")
	ErrMissingRequiredField = errors.New("missing required field")
	ErrInvalidDateFormat    = errors.New("invalid date format (must be RFC3339)")
)

// SyncRepository defines the data access interface for federation sync operations.
type SyncRepository interface {
	GetEventByFederationURI(ctx context.Context, federationUri string) (Event, error)
	UpsertFederatedEvent(ctx context.Context, arg UpsertFederatedEventParams) (Event, error)
	GetFederationNodeByDomain(ctx context.Context, nodeDomain string) (FederationNode, error)
}

// Event represents a minimal event structure (matching database model).
type Event struct {
	ID            pgtype.UUID
	ULID          string
	Name          string
	FederationURI pgtype.Text
	OriginNodeID  pgtype.UUID
}

// FederationNode represents a minimal federation node structure.
type FederationNode struct {
	ID         pgtype.UUID
	NodeDomain string
}

// UpsertFederatedEventParams matches the generated SQLc params.
type UpsertFederatedEventParams struct {
	Ulid                  string
	Name                  string
	Description           pgtype.Text
	LifecycleState        string
	EventStatus           pgtype.Text
	AttendanceMode        pgtype.Text
	OrganizerID           pgtype.UUID
	PrimaryVenueID        pgtype.UUID
	SeriesID              pgtype.UUID
	ImageUrl              pgtype.Text
	PublicUrl             pgtype.Text
	VirtualUrl            pgtype.Text
	Keywords              []string
	InLanguage            []string
	DefaultLanguage       pgtype.Text
	IsAccessibleForFree   pgtype.Bool
	AccessibilityFeatures []string
	EventDomain           pgtype.Text
	OriginNodeID          pgtype.UUID
	FederationUri         pgtype.Text
	LicenseUrl            string
	LicenseStatus         string
	Confidence            pgtype.Numeric
	QualityScore          pgtype.Int4
	Version               int32
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
	PublishedAt           pgtype.Timestamptz
}

// SyncService handles federation sync operations.
type SyncService struct {
	repo SyncRepository
}

// NewSyncService creates a new sync service.
func NewSyncService(repo SyncRepository) *SyncService {
	return &SyncService{
		repo: repo,
	}
}

// SyncEventParams holds the input for syncing an event.
type SyncEventParams struct {
	Payload map[string]any // JSON-LD payload
}

// SyncEventResult holds the result of syncing an event.
type SyncEventResult struct {
	EventULID     string
	FederationURI string
	IsNew         bool
}

// SyncEvent processes an incoming federated event and upserts it.
func (s *SyncService) SyncEvent(ctx context.Context, params SyncEventParams) (*SyncEventResult, error) {
	// Validate JSON-LD structure
	if err := s.validateJSONLD(params.Payload); err != nil {
		return nil, err
	}

	// Extract @id (federation URI)
	federationURI, err := s.extractID(params.Payload)
	if err != nil {
		return nil, err
	}

	// Extract and validate @type
	eventType, err := s.extractType(params.Payload)
	if err != nil {
		return nil, err
	}
	if eventType != "Event" {
		return nil, fmt.Errorf("%w: expected Event, got %s", ErrUnsupportedType, eventType)
	}

	// Check if event already exists
	existing, err := s.repo.GetEventByFederationURI(ctx, federationURI)
	isNew := err != nil // If error, event doesn't exist

	// Extract origin node from federation URI
	originNodeID, err := s.extractOriginNode(ctx, federationURI)
	if err != nil {
		return nil, fmt.Errorf("failed to determine origin node: %w", err)
	}

	// Generate local ULID (or reuse existing one)
	var localULID string
	if isNew {
		ulid, err := ids.NewULID()
		if err != nil {
			return nil, fmt.Errorf("failed to generate ULID: %w", err)
		}
		localULID = ulid
	} else {
		localULID = existing.ULID
	}

	// Map JSON-LD to database params
	dbParams, err := s.mapJSONLDToEvent(params.Payload, localULID, federationURI, originNodeID)
	if err != nil {
		return nil, err
	}

	// Upsert event
	_, err = s.repo.UpsertFederatedEvent(ctx, dbParams)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert event: %w", err)
	}

	return &SyncEventResult{
		EventULID:     localULID,
		FederationURI: federationURI,
		IsNew:         isNew,
	}, nil
}

// validateJSONLD performs basic JSON-LD validation.
func (s *SyncService) validateJSONLD(payload map[string]any) error {
	if payload == nil || len(payload) == 0 {
		return ErrInvalidJSONLD
	}

	// Check for @context
	if _, ok := payload["@context"]; !ok {
		return fmt.Errorf("%w: missing @context", ErrInvalidJSONLD)
	}

	return nil
}

// extractID extracts the @id field from JSON-LD.
func (s *SyncService) extractID(payload map[string]any) (string, error) {
	id, ok := payload["@id"]
	if !ok {
		return "", ErrMissingID
	}

	idStr, ok := id.(string)
	if !ok || idStr == "" {
		return "", ErrMissingID
	}

	// Validate it's a valid URL
	_, err := url.Parse(idStr)
	if err != nil {
		return "", fmt.Errorf("%w: invalid URL format", ErrMissingID)
	}

	return idStr, nil
}

// extractType extracts the @type field from JSON-LD.
func (s *SyncService) extractType(payload map[string]any) (string, error) {
	typeVal, ok := payload["@type"]
	if !ok {
		return "", ErrMissingType
	}

	typeStr, ok := typeVal.(string)
	if !ok || typeStr == "" {
		return "", ErrMissingType
	}

	return typeStr, nil
}

// extractOriginNode determines the origin node ID from the federation URI.
func (s *SyncService) extractOriginNode(ctx context.Context, federationURI string) (pgtype.UUID, error) {
	parsedURL, err := url.Parse(federationURI)
	if err != nil {
		return pgtype.UUID{}, err
	}

	// Get federation node by domain
	node, err := s.repo.GetFederationNodeByDomain(ctx, parsedURL.Host)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("origin node not found for domain %s", parsedURL.Host)
	}

	return node.ID, nil
}

// mapJSONLDToEvent maps JSON-LD payload to database event params.
func (s *SyncService) mapJSONLDToEvent(payload map[string]any, localULID, federationURI string, originNodeID pgtype.UUID) (UpsertFederatedEventParams, error) {
	params := UpsertFederatedEventParams{
		Ulid:           localULID,
		OriginNodeID:   originNodeID,
		FederationUri:  pgtype.Text{String: federationURI, Valid: true},
		LifecycleState: "published", // Default
		LicenseUrl:     "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseStatus:  "cc0",
		Version:        1,
		CreatedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	// Required: name
	name, ok := payload["name"].(string)
	if !ok || name == "" {
		return params, fmt.Errorf("%w: name", ErrMissingRequiredField)
	}
	params.Name = name

	// Optional: description
	if desc, ok := payload["description"].(string); ok && desc != "" {
		params.Description = pgtype.Text{String: desc, Valid: true}
	}

	// Optional: startDate (required for validation but we're just storing it)
	// Note: startDate goes into event_occurrences table, not events table
	// For now, we'll just validate it exists
	if startDate, ok := payload["startDate"].(string); ok {
		if _, err := time.Parse(time.RFC3339, startDate); err != nil {
			return params, fmt.Errorf("%w: startDate", ErrInvalidDateFormat)
		}
		params.PublishedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	// Optional: eventStatus
	if status, ok := payload["eventStatus"].(string); ok && status != "" {
		params.EventStatus = pgtype.Text{String: status, Valid: true}
	}

	// Optional: eventAttendanceMode
	if mode, ok := payload["eventAttendanceMode"].(string); ok && mode != "" {
		params.AttendanceMode = pgtype.Text{String: mode, Valid: true}
	}

	// Optional: url
	if urlStr, ok := payload["url"].(string); ok && urlStr != "" {
		params.PublicUrl = pgtype.Text{String: urlStr, Valid: true}
	}

	// Optional: image
	if image, ok := payload["image"].(string); ok && image != "" {
		params.ImageUrl = pgtype.Text{String: image, Valid: true}
	}

	// Optional: keywords
	if keywords, ok := payload["keywords"].([]any); ok {
		keywordStrs := make([]string, 0, len(keywords))
		for _, kw := range keywords {
			if kwStr, ok := kw.(string); ok {
				keywordStrs = append(keywordStrs, kwStr)
			}
		}
		params.Keywords = keywordStrs
	}

	// Optional: inLanguage
	if lang, ok := payload["inLanguage"].(string); ok && lang != "" {
		params.InLanguage = []string{lang}
		params.DefaultLanguage = pgtype.Text{String: lang, Valid: true}
	} else {
		params.InLanguage = []string{"en"}
		params.DefaultLanguage = pgtype.Text{String: "en", Valid: true}
	}

	// Optional: isAccessibleForFree
	if accessible, ok := payload["isAccessibleForFree"].(bool); ok {
		params.IsAccessibleForFree = pgtype.Bool{Bool: accessible, Valid: true}
	}

	// Optional: event domain (default to "arts")
	params.EventDomain = pgtype.Text{String: "arts", Valid: true}

	return params, nil
}
