package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/jsonld"
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
	CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error
	WithTransaction(ctx context.Context, fn func(txRepo SyncRepository) error) error
	GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyKey, error)
	InsertIdempotencyKey(ctx context.Context, params IdempotencyKeyParams) error
}

// IdempotencyKey represents a stored idempotency key entry.
type IdempotencyKey struct {
	Key         string
	RequestHash string
	EventULID   *string
	CreatedAt   time.Time
}

// IdempotencyKeyParams holds parameters for creating an idempotency key.
type IdempotencyKeyParams struct {
	Key         string
	RequestHash string
	EventULID   string
}

// OccurrenceCreateParams holds parameters for creating an event occurrence.
type OccurrenceCreateParams struct {
	EventID    pgtype.UUID
	StartTime  time.Time
	EndTime    *time.Time
	Timezone   string
	VirtualURL *string
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
	repo      SyncRepository
	validator *jsonld.Validator
}

// NewSyncService creates a new sync service.
// validator can be nil to disable SHACL validation.
func NewSyncService(repo SyncRepository, validator *jsonld.Validator) *SyncService {
	return &SyncService{
		repo:      repo,
		validator: validator,
	}
}

// SyncEventParams holds the input for syncing an event.
type SyncEventParams struct {
	Payload        map[string]any // JSON-LD payload
	IdempotencyKey string         // Optional idempotency key from header
}

// SyncEventResult holds the result of syncing an event.
type SyncEventResult struct {
	EventULID     string
	FederationURI string
	IsNew         bool
	IsDuplicate   bool // True if request was deduplicated via idempotency key
}

// SyncEvent processes an incoming federated event and upserts it.
func (s *SyncService) SyncEvent(ctx context.Context, params SyncEventParams) (*SyncEventResult, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate JSON-LD structure
	if err := s.validateJSONLD(params.Payload); err != nil {
		return nil, err
	}

	// Validate against SHACL shapes (if validator is configured)
	if s.validator != nil {
		if err := s.validator.ValidateEvent(ctx, params.Payload); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidJSONLD, err)
		}
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

	// Compute payload hash for idempotency checks
	payloadHash, err := computePayloadHash(params.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to compute payload hash: %w", err)
	}

	// Check idempotency key if provided
	if params.IdempotencyKey != "" {
		keyEntry, err := s.repo.GetIdempotencyKey(ctx, params.IdempotencyKey)
		if err == nil && keyEntry != nil {
			// Idempotency key exists
			if keyEntry.RequestHash == payloadHash && keyEntry.EventULID != nil {
				// Same payload, return existing event
				existing, err := s.repo.GetEventByFederationURI(ctx, federationURI)
				if err == nil {
					return &SyncEventResult{
						EventULID:     existing.ULID,
						FederationURI: federationURI,
						IsNew:         false,
						IsDuplicate:   true,
					}, nil
				}
			}
			// Different payload with same key - conflict
			return nil, fmt.Errorf("idempotency key conflict: different payload for same key")
		}
	}

	// Execute all database operations in a transaction
	var result *SyncEventResult
	err = s.repo.WithTransaction(ctx, func(txRepo SyncRepository) error {
		// Check for context cancellation before database operations
		if err := ctx.Err(); err != nil {
			return err
		}

		// Check if event already exists
		existing, err := txRepo.GetEventByFederationURI(ctx, federationURI)
		isNew := err != nil // If error, event doesn't exist

		// Extract origin node from federation URI
		originNodeID, err := s.extractOriginNode(ctx, txRepo, federationURI)
		if err != nil {
			return fmt.Errorf("failed to determine origin node: %w", err)
		}

		// Check for context cancellation before upsert
		if err := ctx.Err(); err != nil {
			return err
		}

		// Generate local ULID (or reuse existing one)
		var localULID string
		if isNew {
			ulid, err := ids.NewULID()
			if err != nil {
				return fmt.Errorf("failed to generate ULID: %w", err)
			}
			localULID = ulid
		} else {
			localULID = existing.ULID
		}

		// Map JSON-LD to database params
		dbParams, occurrenceData, err := s.mapJSONLDToEvent(params.Payload, localULID, federationURI, originNodeID)
		if err != nil {
			return err
		}

		// Upsert event
		event, err := txRepo.UpsertFederatedEvent(ctx, dbParams)
		if err != nil {
			return fmt.Errorf("failed to upsert event: %w", err)
		}

		// Create occurrence if we have occurrence data
		if occurrenceData != nil {
			occurrenceData.EventID = event.ID
			if err := txRepo.CreateOccurrence(ctx, *occurrenceData); err != nil {
				return fmt.Errorf("failed to create occurrence: %w", err)
			}
		}

		// Store idempotency key if provided
		if params.IdempotencyKey != "" {
			if err := txRepo.InsertIdempotencyKey(ctx, IdempotencyKeyParams{
				Key:         params.IdempotencyKey,
				RequestHash: payloadHash,
				EventULID:   localULID,
			}); err != nil {
				// Log but don't fail - idempotency is best-effort
				// This could fail if key already exists, which is fine
			}
		}

		result = &SyncEventResult{
			EventULID:     localULID,
			FederationURI: federationURI,
			IsNew:         isNew,
			IsDuplicate:   false,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
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
func (s *SyncService) extractOriginNode(ctx context.Context, repo SyncRepository, federationURI string) (pgtype.UUID, error) {
	parsedURL, err := url.Parse(federationURI)
	if err != nil {
		return pgtype.UUID{}, err
	}

	// Get federation node by domain
	node, err := repo.GetFederationNodeByDomain(ctx, parsedURL.Host)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("origin node not found for domain %s", parsedURL.Host)
	}

	return node.ID, nil
}

// mapJSONLDToEvent maps JSON-LD payload to database event params and occurrence data.
func (s *SyncService) mapJSONLDToEvent(payload map[string]any, localULID, federationURI string, originNodeID pgtype.UUID) (UpsertFederatedEventParams, *OccurrenceCreateParams, error) {
	// Extract required fields first
	name, err := extractRequiredString(payload, "name", ErrMissingRequiredField)
	if err != nil {
		return UpsertFederatedEventParams{}, nil, err
	}

	// Initialize params with defaults
	params := createDefaultEventParams(localULID, federationURI, originNodeID, name)

	// Extract optional fields
	extractOptionalEventFields(payload, &params)

	// Extract occurrence data if startDate present
	occurrence, err := extractOccurrenceData(payload, federationURI)
	if err != nil {
		return params, nil, err
	}

	// Set published_at from startDate if present
	if occurrence != nil {
		params.PublishedAt = pgtype.Timestamptz{Time: occurrence.StartTime, Valid: true}
	}

	return params, occurrence, nil
}

// extractRequiredString extracts and validates a required string field from JSON-LD.
func extractRequiredString(payload map[string]any, field string, errType error) (string, error) {
	value, ok := payload[field].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%w: %s", errType, field)
	}
	return value, nil
}

// createDefaultEventParams initializes event params with default values.
func createDefaultEventParams(localULID, federationURI string, originNodeID pgtype.UUID, name string) UpsertFederatedEventParams {
	return UpsertFederatedEventParams{
		Ulid:            localULID,
		Name:            name,
		OriginNodeID:    originNodeID,
		FederationUri:   pgtype.Text{String: federationURI, Valid: true},
		LifecycleState:  DefaultLifecycleState,
		LicenseUrl:      "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseStatus:   "cc0",
		Version:         DefaultEventVersion,
		InLanguage:      []string{"en"},
		DefaultLanguage: pgtype.Text{String: "en", Valid: true},
		EventDomain:     pgtype.Text{String: "arts", Valid: true},
		CreatedAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
}

// extractOptionalEventFields extracts optional fields from JSON-LD payload.
func extractOptionalEventFields(payload map[string]any, params *UpsertFederatedEventParams) {
	// Text fields
	if desc, ok := payload["description"].(string); ok && desc != "" {
		params.Description = pgtype.Text{String: desc, Valid: true}
	}
	if status, ok := payload["eventStatus"].(string); ok && status != "" {
		params.EventStatus = pgtype.Text{String: status, Valid: true}
	}
	if mode, ok := payload["eventAttendanceMode"].(string); ok && mode != "" {
		params.AttendanceMode = pgtype.Text{String: mode, Valid: true}
	}
	if urlStr, ok := payload["url"].(string); ok && urlStr != "" {
		params.PublicUrl = pgtype.Text{String: urlStr, Valid: true}
	}
	if image, ok := payload["image"].(string); ok && image != "" {
		params.ImageUrl = pgtype.Text{String: image, Valid: true}
	}

	// Boolean fields
	if accessible, ok := payload["isAccessibleForFree"].(bool); ok {
		params.IsAccessibleForFree = pgtype.Bool{Bool: accessible, Valid: true}
	}

	// Array fields
	if keywords, ok := payload["keywords"].([]any); ok {
		keywordStrs := make([]string, 0, len(keywords))
		for _, kw := range keywords {
			if kwStr, ok := kw.(string); ok {
				keywordStrs = append(keywordStrs, kwStr)
			}
		}
		params.Keywords = keywordStrs
	}

	// Language with override of default
	if lang, ok := payload["inLanguage"].(string); ok && lang != "" {
		params.InLanguage = []string{lang}
		params.DefaultLanguage = pgtype.Text{String: lang, Valid: true}
	}
}

// extractOccurrenceData extracts occurrence information from JSON-LD if startDate is present.
func extractOccurrenceData(payload map[string]any, federationURI string) (*OccurrenceCreateParams, error) {
	startDateStr, ok := payload["startDate"].(string)
	if !ok || startDateStr == "" {
		return nil, nil // No occurrence data
	}

	startTime, err := time.Parse(time.RFC3339, startDateStr)
	if err != nil {
		return nil, fmt.Errorf("%w: startDate", ErrInvalidDateFormat)
	}

	occurrence := &OccurrenceCreateParams{
		StartTime: startTime,
		Timezone:  "UTC",
	}

	// Optional: endDate
	if endDateStr, ok := payload["endDate"].(string); ok && endDateStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			occurrence.EndTime = &endTime
		}
	}

	// Virtual URL (required by constraint, use federation URI as fallback)
	if urlStr, ok := payload["url"].(string); ok && urlStr != "" {
		occurrence.VirtualURL = &urlStr
	} else {
		occurrence.VirtualURL = &federationURI
	}

	return occurrence, nil
}

// computePayloadHash generates a SHA-256 hash of the JSON-LD payload for idempotency checks.
func computePayloadHash(payload map[string]any) (string, error) {
	// Marshal to JSON (Go's json.Marshal produces deterministic output for maps)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Compute SHA-256 hash
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
