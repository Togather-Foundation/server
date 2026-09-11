package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// Decision actions recorded in identity_decisions. Phase 2 widens this set
// (merge/undo); the DB CHECK constraint currently allows only link|reject.
const (
	ActionLink   = "link"
	ActionReject = "reject"
)

// DecisionRecord is a flat representation of one identity_decisions row. It is
// flat (no nested domain objects) so it maps 1:1 to the table columns and to
// the admin REST/CLI JSON payload.
type DecisionRecord struct {
	ID              string         `json:"id"`          // "idn-{ulid}"
	CreatedAt       time.Time      `json:"created_at"`  // identity_decisions.created_at
	EntityType      EntityType     `json:"entity_type"` // place|organization
	EntityID        string         `json:"entity_id"`   // canonical ULID of the primary entity
	Action          string         `json:"action"`      // link|reject
	CounterpartType *EntityType    `json:"counterpart_type,omitempty"`
	CounterpartID   *string        `json:"counterpart_id,omitempty"`
	Rationale       string         `json:"rationale"`
	Citations       []string       `json:"citations"`
	Confidence      float64        `json:"confidence"`
	Actor           string         `json:"actor"`
	Reversible      bool           `json:"reversible"`
	UndoRef         string         `json:"undo_ref,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// DecisionStore persists the append-only identity_decisions ledger.
type DecisionStore interface {
	// Append appends one decision in its own transaction (convenience form).
	Append(ctx context.Context, rec DecisionRecord) (DecisionRecord, error)
	// AppendTx appends one decision inside the caller's transaction.
	AppendTx(ctx context.Context, q *postgres.Queries, rec DecisionRecord) (DecisionRecord, error)
	// List returns one entity's decisions, newest first.
	List(ctx context.Context, ref IdentityRef) ([]DecisionRecord, error)
	// ListFeed returns a keyset-paginated, filterable feed over all decisions.
	ListFeed(ctx context.Context, arg postgres.ListIdentityDecisionsParams) ([]DecisionRecord, error)
}

// decisionStore is the concrete DecisionStore backed by a pgx pool.
type decisionStore struct {
	pool *pgxpool.Pool
}

// NewDecisionStore creates a DecisionStore from a pgx connection pool.
func NewDecisionStore(pool *pgxpool.Pool) DecisionStore {
	return &decisionStore{pool: pool}
}

var _ DecisionStore = (*decisionStore)(nil)

// Append appends one decision in its own transaction.
func (s *decisionStore) Append(ctx context.Context, rec DecisionRecord) (DecisionRecord, error) {
	var out DecisionRecord
	err := s.withTx(ctx, func(q *postgres.Queries) error {
		var err error
		out, err = s.AppendTx(ctx, q, rec)
		return err
	})
	if err != nil {
		return DecisionRecord{}, err
	}
	return out, nil
}

// withTx runs fn in a READ COMMITTED transaction, committing on success and
// rolling back on error.
func (s *decisionStore) withTx(ctx context.Context, fn func(q *postgres.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("identity: begin transaction: %w", err)
	}

	if err := fn(postgres.New(tx)); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("identity: rollback after error %v: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("identity: commit transaction: %w", err)
	}
	return nil
}

// AppendTx appends one decision inside the caller's transaction. It generates
// the decision id when absent and returns the record with CreatedAt populated
// from the database.
func (s *decisionStore) AppendTx(ctx context.Context, q *postgres.Queries, rec DecisionRecord) (DecisionRecord, error) {
	if rec.ID == "" {
		rec.ID = "idn-" + ulid.Make().String()
	}

	confidence, err := numericFromFloat(rec.Confidence)
	if err != nil {
		return DecisionRecord{}, fmt.Errorf("identity: %w", err)
	}

	citations := rec.Citations
	if citations == nil {
		citations = []string{}
	}
	citationBytes, err := json.Marshal(citations)
	if err != nil {
		return DecisionRecord{}, fmt.Errorf("identity: marshal citations: %w", err)
	}

	var metadata []byte
	if rec.Metadata != nil {
		metadata, err = json.Marshal(rec.Metadata)
		if err != nil {
			return DecisionRecord{}, fmt.Errorf("identity: marshal metadata: %w", err)
		}
	}

	createdAt, err := q.InsertIdentityDecision(ctx, postgres.InsertIdentityDecisionParams{
		ID:              rec.ID,
		EntityType:      string(rec.EntityType),
		EntityID:        rec.EntityID,
		Action:          rec.Action,
		CounterpartType: entityTypeToText(rec.CounterpartType),
		CounterpartID:   stringPtrToText(rec.CounterpartID),
		Rationale:       rec.Rationale,
		Citations:       citationBytes,
		Confidence:      confidence,
		Actor:           rec.Actor,
		Reversible:      rec.Reversible,
		UndoRef:         stringToText(rec.UndoRef),
		Metadata:        metadata,
	})
	if err != nil {
		return DecisionRecord{}, fmt.Errorf("identity: append decision: %w", err)
	}

	rec.CreatedAt = createdAt.Time
	return rec, nil
}

// List returns one entity's decisions, newest first.
func (s *decisionStore) List(ctx context.Context, ref IdentityRef) ([]DecisionRecord, error) {
	q := postgres.New(s.pool)
	rows, err := q.ListIdentityDecisionsByEntity(ctx, postgres.ListIdentityDecisionsByEntityParams{
		EntityType: string(ref.Type),
		EntityID:   ref.ULID,
	})
	if err != nil {
		return nil, fmt.Errorf("identity: list decisions: %w", err)
	}

	recs := make([]DecisionRecord, 0, len(rows))
	for _, row := range rows {
		rec, err := decisionRecordFromModel(row)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// ListFeed returns a keyset-paginated, filterable feed over all decisions.
func (s *decisionStore) ListFeed(ctx context.Context, arg postgres.ListIdentityDecisionsParams) ([]DecisionRecord, error) {
	q := postgres.New(s.pool)
	rows, err := q.ListIdentityDecisions(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("identity: list decision feed: %w", err)
	}

	recs := make([]DecisionRecord, 0, len(rows))
	for _, row := range rows {
		rec, err := decisionRecordFromModel(row)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// decisionRecordFromModel converts a SQLc identity_decisions row to a domain
// DecisionRecord.
func decisionRecordFromModel(d postgres.IdentityDecision) (DecisionRecord, error) {
	rec := DecisionRecord{
		ID:         d.ID,
		CreatedAt:  d.CreatedAt.Time,
		EntityType: EntityType(d.EntityType),
		EntityID:   d.EntityID,
		Action:     d.Action,
		Rationale:  d.Rationale,
		Confidence: numericToFloat64(d.Confidence),
		Actor:      d.Actor,
		Reversible: d.Reversible,
	}

	if d.CounterpartType.Valid {
		t := EntityType(d.CounterpartType.String)
		rec.CounterpartType = &t
	}
	if d.CounterpartID.Valid {
		id := d.CounterpartID.String
		rec.CounterpartID = &id
	}
	if d.UndoRef.Valid {
		rec.UndoRef = d.UndoRef.String
	}

	rec.Citations = []string{}
	if len(d.Citations) > 0 {
		if err := json.Unmarshal(d.Citations, &rec.Citations); err != nil {
			return DecisionRecord{}, fmt.Errorf("identity: decode citations: %w", err)
		}
	}

	if len(d.Metadata) > 0 {
		if err := json.Unmarshal(d.Metadata, &rec.Metadata); err != nil {
			return DecisionRecord{}, fmt.Errorf("identity: decode metadata: %w", err)
		}
	}

	return rec, nil
}

func stringToText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func stringPtrToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func entityTypeToText(t *EntityType) pgtype.Text {
	if t == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*t), Valid: true}
}
