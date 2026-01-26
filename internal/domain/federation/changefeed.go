package federation

import (
	"context"
	"errors"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidCursor     = errors.New("invalid cursor")
	ErrInvalidLimit      = errors.New("limit must be between 1 and 1000")
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
	SourceTimestamp   pgtype.Timestamptz
	ReceivedTimestamp pgtype.Timestamptz
}

// ChangeFeedService provides business logic for change feed operations.
type ChangeFeedService struct {
	repo ChangeFeedRepository
}

// NewChangeFeedService creates a new change feed service.
func NewChangeFeedService(repo ChangeFeedRepository) *ChangeFeedService {
	return &ChangeFeedService{
		repo: repo,
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
	ID                string    `json:"id"`
	EventID           string    `json:"event_id"`
	EventULID         string    `json:"event_ulid"`
	Action            string    `json:"action"`
	ChangedFields     []byte    `json:"changed_fields,omitempty"`
	Snapshot          []byte    `json:"snapshot,omitempty"`
	ChangedAt         time.Time `json:"changed_at"`
	SequenceNumber    int64     `json:"sequence_number"`
	FederationURI     string    `json:"federation_uri,omitempty"`
	SourceTimestamp   time.Time `json:"source_timestamp,omitempty"`
	ReceivedTimestamp time.Time `json:"received_timestamp"`
}

// ChangeFeedResult represents the result of fetching changes.
type ChangeFeedResult struct {
	Items      []ChangeEntry `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
	HasMore    bool          `json:"has_more"`
}

// GetChanges retrieves event changes with pagination and filtering.
func (s *ChangeFeedService) GetChanges(ctx context.Context, params ChangeFeedParams) (*ChangeFeedResult, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate and set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 1000 {
		return nil, ErrInvalidLimit
	}

	// Validate action filter
	if params.Action != "" && params.Action != "create" && params.Action != "update" && params.Action != "delete" {
		return nil, ErrInvalidAction
	}

	// Decode cursor to sequence number
	var afterSeq int64
	if params.After != "" {
		seq, err := pagination.DecodeChangeCursor(params.After)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		afterSeq = seq
	}

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
		return nil, err
	}

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
			entry.Snapshot = row.Snapshot
		}

		items = append(items, entry)
	}

	// Generate next cursor if there are more results
	var nextCursor string
	if hasMore && len(items) > 0 {
		lastSeq := items[len(items)-1].SequenceNumber
		nextCursor = pagination.EncodeChangeCursor(lastSeq)
	}

	return &ChangeFeedResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
