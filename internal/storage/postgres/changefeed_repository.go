package postgres

import (
	"context"

	"github.com/Togather-Foundation/server/internal/domain/federation"
)

// ChangeFeedRepository implements federation.ChangeFeedRepository using SQLc queries.
type ChangeFeedRepository struct {
	queries *Queries
}

// NewChangeFeedRepository creates a new change feed repository.
func NewChangeFeedRepository(queries *Queries) *ChangeFeedRepository {
	return &ChangeFeedRepository{
		queries: queries,
	}
}

// ListEventChanges fetches event changes from the database.
func (r *ChangeFeedRepository) ListEventChanges(ctx context.Context, arg federation.ListEventChangesParams) ([]federation.ListEventChangesRow, error) {
	// Convert domain params to SQLc params
	sqlcParams := ListEventChangesParams{
		Limit: arg.Limit,
	}

	// Set AfterSequence if provided (nullable parameter)
	if arg.AfterSequence > 0 {
		sqlcParams.AfterSequence.Int64 = arg.AfterSequence
		sqlcParams.AfterSequence.Valid = true
	}

	// Set timestamp filter if provided
	sqlcParams.AfterTimestamp = arg.Since

	// Set action filter (nullable text - empty string means no filter)
	if arg.Action != "" {
		sqlcParams.Action.String = arg.Action
		sqlcParams.Action.Valid = true
	}

	// Execute query
	rows, err := r.queries.ListEventChanges(ctx, sqlcParams)
	if err != nil {
		return nil, err
	}

	// Convert SQLc rows to domain rows
	result := make([]federation.ListEventChangesRow, len(rows))
	for i, row := range rows {
		result[i] = federation.ListEventChangesRow{
			ID:                row.ID,
			EventID:           row.EventID,
			Action:            row.Action,
			ChangedFields:     row.ChangedFields,
			Snapshot:          row.Snapshot,
			ChangedAt:         row.ChangedAt,
			SequenceNumber:    row.SequenceNumber,
			EventUlid:         row.EventUlid,
			FederationUri:     row.FederationUri,
			LicenseUrl:        row.LicenseUrl,
			LicenseStatus:     row.LicenseStatus,
			SourceTimestamp:   row.SourceTimestamp,
			ReceivedTimestamp: row.ReceivedTimestamp,
		}
	}

	return result, nil
}
