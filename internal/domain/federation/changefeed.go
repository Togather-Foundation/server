package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

var (
	ErrInvalidCursor     = errors.New("invalid cursor")
	ErrInvalidLimit      = fmt.Errorf("limit must be between %d and %d", MinChangeFeedLimit, MaxChangeFeedLimit)
	ErrInvalidAction     = errors.New("action must be 'create', 'update', or 'delete'")
	ErrInvalidSinceParam = errors.New("since parameter must be a valid timestamp")
)

// ChangeFeedRepository defines the data access interface for change feeds.
type ChangeFeedRepository interface {
	ListEventChanges(ctx context.Context, arg ListEventChangesParams) ([]ListEventChangesRow, error)
}

// ListEventChangesParams holds parameters for querying event changes.
type ListEventChangesParams struct {
	AfterSequence int64
	Since         pgtype.Timestamptz
	Action        string
	Limit         int32
}

// ListEventChangesRow represents a row returned from ListEventChanges query.
type ListEventChangesRow struct {
	ID                pgtype.UUID
	EventID           pgtype.UUID
	Action            string
	ChangedFields     []byte
	Snapshot          []byte
	ChangedAt         pgtype.Timestamptz
	SequenceNumber    pgtype.Int8
	EventUlid         string
	FederationUri     pgtype.Text
	LicenseUrl        string
	LicenseStatus     string
	SourceTimestamp   pgtype.Timestamptz
	ReceivedTimestamp pgtype.Timestamptz
}

// ChangeFeedService provides business logic for change feed operations.
type ChangeFeedService struct {
	repo    ChangeFeedRepository
	logger  zerolog.Logger
	baseURL string
}

// NewChangeFeedService creates a new change feed service.
func NewChangeFeedService(repo ChangeFeedRepository, logger zerolog.Logger, baseURL string) *ChangeFeedService {
	return &ChangeFeedService{
		repo:    repo,
		logger:  logger,
		baseURL: baseURL,
	}
}

// ChangeFeedParams holds parameters for fetching change feed entries.
type ChangeFeedParams struct {
	After           string    // Cursor for pagination (sequence-based)
	Since           time.Time // Filter by timestamp
	Action          string    // Filter by action type ('create', 'update', 'delete')
	Limit           int       // Number of entries to return (1-1000)
	IncludeSnapshot bool      // Include event snapshots in response
}

// ChangeEntry represents a single change in the feed.
type ChangeEntry struct {
	ID                string          `json:"id"`
	EventID           string          `json:"event_id"`
	EventULID         string          `json:"event_ulid"`
	Action            string          `json:"action"`
	ChangedFields     json.RawMessage `json:"changed_fields,omitempty"`
	Snapshot          json.RawMessage `json:"snapshot,omitempty"`
	ChangedAt         time.Time       `json:"changed_at"`
	SequenceNumber    int64           `json:"sequence_number"`
	FederationURI     string          `json:"federation_uri,omitempty"`
	LicenseURL        string          `json:"license_url"`
	LicenseStatus     string          `json:"license_status"`
	SourceTimestamp   time.Time       `json:"source_timestamp,omitempty"`
	ReceivedTimestamp time.Time       `json:"received_timestamp"`
}

// ChangeFeedResult represents the result of fetching changes.
type ChangeFeedResult struct {
	Cursor     string        `json:"cursor"`                // Current cursor position
	Changes    []ChangeEntry `json:"changes"`               // List of changes (renamed from Items)
	NextCursor string        `json:"next_cursor,omitempty"` // Next cursor for pagination
	HasMore    bool          `json:"has_more"`              // Whether there are more results
}

// GetChanges retrieves event changes with pagination and filtering.
func (s *ChangeFeedService) GetChanges(ctx context.Context, params ChangeFeedParams) (*ChangeFeedResult, error) {
	start := time.Now()

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate and set defaults
	if params.Limit <= 0 {
		params.Limit = DefaultChangeFeedLimit
	}
	if params.Limit > MaxChangeFeedLimit {
		s.logger.Warn().
			Int("requested_limit", params.Limit).
			Msgf("change feed: limit exceeds maximum, using %d", MaxChangeFeedLimit)
		return nil, ErrInvalidLimit
	}

	// Validate action filter
	if params.Action != "" && params.Action != "create" && params.Action != "update" && params.Action != "delete" {
		s.logger.Warn().
			Str("action", params.Action).
			Msg("change feed: invalid action filter")
		return nil, ErrInvalidAction
	}

	// Decode cursor to sequence number
	var afterSeq int64
	if params.After != "" {
		seq, err := pagination.DecodeChangeCursor(params.After)
		if err != nil {
			s.logger.Warn().
				Err(err).
				Str("cursor", params.After).
				Msg("change feed: invalid cursor")
			return nil, ErrInvalidCursor
		}
		afterSeq = seq
	}

	s.logger.Debug().
		Int64("after_sequence", afterSeq).
		Str("action", params.Action).
		Time("since", params.Since).
		Int("limit", params.Limit).
		Bool("include_snapshot", params.IncludeSnapshot).
		Msg("change feed: fetching changes")

	// Build query parameters
	queryParams := ListEventChangesParams{
		AfterSequence: afterSeq,
		Limit:         int32(params.Limit + 1), // Fetch one extra to check if there are more results
		Action:        params.Action,
	}

	// Set timestamp filter
	if !params.Since.IsZero() {
		queryParams.Since = pgtype.Timestamptz{
			Time:  params.Since,
			Valid: true,
		}
	}

	// Check for context cancellation before database query
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Execute query
	rows, err := s.repo.ListEventChanges(ctx, queryParams)
	if err != nil {
		s.logger.Error().
			Err(err).
			Int64("after_sequence", afterSeq).
			Str("action", params.Action).
			Dur("duration_ms", time.Since(start)).
			Msg("change feed: database query failed")
		return nil, err
	}

	s.logger.Debug().
		Int("row_count", len(rows)).
		Dur("query_duration_ms", time.Since(start)).
		Msg("change feed: query completed")

	// Check for context cancellation before processing results
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Check if there are more results
	hasMore := len(rows) > params.Limit
	if hasMore {
		rows = rows[:params.Limit]
	}

	// Convert to domain model
	items := make([]ChangeEntry, 0, len(rows))
	for _, row := range rows {
		entry := ChangeEntry{
			ID:             ids.UUIDToString(row.ID),
			EventID:        ids.UUIDToString(row.EventID),
			EventULID:      row.EventUlid,
			Action:         row.Action,
			ChangedAt:      row.ChangedAt.Time,
			SequenceNumber: row.SequenceNumber.Int64,
			LicenseURL:     row.LicenseUrl,
			LicenseStatus:  row.LicenseStatus,
		}

		// Include federation URI if present
		if row.FederationUri.Valid {
			entry.FederationURI = row.FederationUri.String
		}

		// Include timestamps if valid
		if row.SourceTimestamp.Valid {
			entry.SourceTimestamp = row.SourceTimestamp.Time
		}
		if row.ReceivedTimestamp.Valid {
			entry.ReceivedTimestamp = row.ReceivedTimestamp.Time
		}

		// Include changed fields if present
		if len(row.ChangedFields) > 0 {
			entry.ChangedFields = row.ChangedFields
		}

		// Include snapshot if requested and present
		if params.IncludeSnapshot && len(row.Snapshot) > 0 {
			// For DELETE actions, snapshot is already in proper format (tombstone payload)
			if row.Action == "delete" {
				entry.Snapshot = row.Snapshot
			} else {
				// Transform database snapshot to JSON-LD format for create/update actions
				transformedSnapshot, err := s.transformSnapshotToJSONLD(row.Snapshot, s.baseURL)
				if err != nil {
					s.logger.Warn().
						Err(err).
						Str("event_id", entry.EventID).
						Msg("change feed: failed to transform snapshot, including raw snapshot")
					entry.Snapshot = row.Snapshot
				} else {
					entry.Snapshot = transformedSnapshot
				}
			}
		}

		items = append(items, entry)
	}

	// Generate current and next cursors
	currentCursor := pagination.EncodeChangeCursor(afterSeq) // Current cursor position
	var nextCursor string
	if hasMore && len(items) > 0 {
		lastSeq := items[len(items)-1].SequenceNumber
		nextCursor = pagination.EncodeChangeCursor(lastSeq)
	}

	s.logger.Info().
		Int("item_count", len(items)).
		Bool("has_more", hasMore).
		Str("cursor", currentCursor).
		Str("next_cursor", nextCursor).
		Dur("duration_ms", time.Since(start)).
		Msg("change feed: completed successfully")

	return &ChangeFeedResult{
		Cursor:     currentCursor,
		Changes:    items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// transformSnapshotToJSONLD converts a database snapshot (raw column names) to JSON-LD format.
func (s *ChangeFeedService) transformSnapshotToJSONLD(dbSnapshot json.RawMessage, baseURL string) (json.RawMessage, error) {
	// Parse database snapshot
	var dbData map[string]any
	if err := json.Unmarshal(dbSnapshot, &dbData); err != nil {
		return nil, fmt.Errorf("unmarshal db snapshot: %w", err)
	}

	// Extract required fields
	ulid, _ := dbData["ulid"].(string)
	name, _ := dbData["name"].(string)

	// Build JSON-LD object
	jsonLD := map[string]any{
		"@context": "https://togather.foundation/contexts/sel/v0.1.jsonld",
		"@type":    "Event",
		"name":     name,
	}

	// Add @id if we have ULID and baseURL - use canonical URI format per Interop Profile §1.1
	if ulid != "" && baseURL != "" {
		if uri, err := ids.BuildCanonicalURI(baseURL, "events", ulid); err == nil {
			jsonLD["@id"] = uri
		} else {
			// Fallback to API path if canonical URI building fails
			s.logger.Warn().
				Err(err).
				Str("ulid", ulid).
				Msg("change feed: failed to build canonical URI, using API path")
			jsonLD["@id"] = fmt.Sprintf("%s/api/v1/events/%s", baseURL, ulid)
		}
	}

	// Add optional fields if present
	if desc, ok := dbData["description"].(string); ok && desc != "" {
		jsonLD["description"] = desc
	}
	if lifecycleState, ok := dbData["lifecycle_state"].(string); ok && lifecycleState != "" {
		jsonLD["lifecycleState"] = lifecycleState
	}
	if eventDomain, ok := dbData["event_domain"].(string); ok && eventDomain != "" {
		jsonLD["eventDomain"] = eventDomain
	}
	if licenseURL, ok := dbData["license_url"].(string); ok && licenseURL != "" {
		jsonLD["license"] = licenseURL
	}

	// Marshal back to JSON
	result, err := json.Marshal(jsonLD)
	if err != nil {
		return nil, fmt.Errorf("marshal json-ld: %w", err)
	}

	return result, nil
}
