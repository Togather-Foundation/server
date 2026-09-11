package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrEntityNotFound is returned when a valid ULID references an entity that
// does not exist (or has been soft-deleted). The HTTP layer maps it to 404.
var ErrEntityNotFound = errors.New("entity not found")

// ErrInvalidCursor is returned when an opaque pagination cursor cannot be
// decoded. It is a structural (BadRequest-class) error.
var ErrInvalidCursor = errors.New("invalid cursor")

// IdentifierView is the REST projection of a single identifier observation.
type IdentifierView struct {
	Authority  string    `json:"authority"`
	URI        string    `json:"uri"`
	Method     string    `json:"method"`
	Confidence float64   `json:"confidence"`
	IsPrimary  bool      `json:"is_primary"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
}

// IdentityView is the REST response for a single entity's identity state.
type IdentityView struct {
	Ref         IdentityRef       `json:"ref"`
	Identifiers []IdentifierView  `json:"identifiers"`
	Primary     map[string]string `json:"primary"` // authority -> URI
	Decisions   []DecisionRecord  `json:"decisions"`
}

// ConflictItem is one unordered pair of entities that share an identifier.
type ConflictItem struct {
	Ref           IdentityRef     `json:"ref"`
	Candidate     IdentityRef     `json:"candidate"`
	Authority     string          `json:"authority"`
	URI           string          `json:"uri"`
	Score         float64         `json:"score"` // max confidence of the two observations
	Suppressed    bool            `json:"suppressed"`
	PriorDecision *DecisionRecord `json:"prior_decision,omitempty"` // set when suppressed
}

// ConflictsResponse is the paginated conflicts feed.
type ConflictsResponse struct {
	Items      []ConflictItem `json:"items"`
	NextCursor *string        `json:"next_cursor"` // opaque (authority, uri, entity_id of last item)
}

// DecisionsResponse is the paginated decisions feed.
type DecisionsResponse struct {
	Items      []DecisionRecord `json:"items"`
	NextCursor *string          `json:"next_cursor"` // opaque (created_at, id of last row)
}

// ConflictsParams carries the conflicts feed filters and pagination.
type ConflictsParams struct {
	Type              EntityType
	Limit             int
	Cursor            string
	IncludeSuppressed bool
}

// DecisionsParams carries the decisions feed filters and pagination.
type DecisionsParams struct {
	Type   *EntityType // optional entity-type filter
	Since  *time.Time  // inclusive RFC3339 lower bound on created_at
	Limit  int
	Cursor string
}

// Service assembles identity read views for the admin REST surface.
type Service interface {
	View(ctx context.Context, ref IdentityRef) (IdentityView, error)
	Conflicts(ctx context.Context, arg ConflictsParams) (ConflictsResponse, error)
	Decisions(ctx context.Context, arg DecisionsParams) (DecisionsResponse, error)
}

// Writer records link/reject decisions. *Executor implements it.
type Writer interface {
	LinkIdentifier(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error)
	Reject(ctx context.Context, a, b IdentityRef, actor, reason string) (DecisionRecord, error)
}

// readQueries is the subset of SQLc queries the read service needs. It is
// narrow so tests can fake it; postgres.Querier satisfies it.
type readQueries interface {
	ListConflicts(ctx context.Context, arg postgres.ListConflictsParams) ([]postgres.ListConflictsRow, error)
	GetIdentityDecision(ctx context.Context, id string) (postgres.IdentityDecision, error)
	EntityExists(ctx context.Context, arg postgres.EntityExistsParams) (bool, error)
}

// service is the concrete Service backed by SQLc queries and the identifier /
// decision stores.
type service struct {
	q         readQueries
	ids       IdentifierStore
	decisions DecisionStore
}

// NewService assembles a Service from its collaborators.
func NewService(q readQueries, ids IdentifierStore, decisions DecisionStore) Service {
	return &service{q: q, ids: ids, decisions: decisions}
}

// View assembles an entity's identity view: identifiers, primary map, and
// decision history. A valid-but-absent ULID returns ErrEntityNotFound.
func (s *service) View(ctx context.Context, ref IdentityRef) (IdentityView, error) {
	if err := validateEntityType(ref.Type); err != nil {
		return IdentityView{}, err
	}

	exists, err := s.q.EntityExists(ctx, postgres.EntityExistsParams{
		EntityType: string(ref.Type),
		EntityID:   ref.ULID,
	})
	if err != nil {
		return IdentityView{}, fmt.Errorf("identity: check entity existence: %w", err)
	}
	if !exists {
		return IdentityView{}, fmt.Errorf("%w: %s %s", ErrEntityNotFound, ref.Type, ref.ULID)
	}

	obs, err := s.ids.GetEntityIdentifiers(ctx, ref)
	if err != nil {
		return IdentityView{}, err
	}

	identifiers := make([]IdentifierView, 0, len(obs))
	primary := make(map[string]string, len(obs))
	for _, o := range obs {
		identifiers = append(identifiers, IdentifierView{
			Authority:  o.Authority,
			URI:        o.URI,
			Method:     o.Method,
			Confidence: o.Confidence,
			IsPrimary:  o.IsPrimary,
			Source:     o.Source,
			ObservedAt: o.ObservedAt,
		})
		if o.IsPrimary {
			primary[o.Authority] = o.URI
		}
	}

	decisions, err := s.decisions.List(ctx, ref)
	if err != nil {
		return IdentityView{}, err
	}

	return IdentityView{
		Ref:         ref,
		Identifiers: identifiers,
		Primary:     primary,
		Decisions:   decisions,
	}, nil
}

// Conflicts returns the Phase 1 conflict feed: a self-join of
// entity_identifiers on shared (authority, uri), each unordered pair once.
// Suppressed pairs (stored evidence fingerprint matches the current signal
// fingerprint) are excluded by default and included with prior_decision when
// IncludeSuppressed is true.
func (s *service) Conflicts(ctx context.Context, arg ConflictsParams) (ConflictsResponse, error) {
	if err := validateEntityType(arg.Type); err != nil {
		return ConflictsResponse{}, err
	}
	if arg.Limit < 1 {
		arg.Limit = 1
	}

	var cur conflictsCursor
	if arg.Cursor != "" {
		var err error
		cur, err = decodeConflictsCursor(arg.Cursor)
		if err != nil {
			return ConflictsResponse{}, err
		}
	}

	rows, err := s.q.ListConflicts(ctx, postgres.ListConflictsParams{
		EntityType:      string(arg.Type),
		CursorAuthority: textOrNull(cur.Authority),
		CursorUri:       textOrNull(cur.URI),
		CursorIDA:       textOrNull(cur.IDA),
		CursorIDB:       textOrNull(cur.IDB),
		Limit:           int32(arg.Limit) + 1,
	})
	if err != nil {
		return ConflictsResponse{}, fmt.Errorf("identity: list conflicts: %w", err)
	}

	// Over-fetch one row to detect a next page; the +1th row is never returned.
	hasMore := len(rows) > arg.Limit
	if hasMore {
		rows = rows[:arg.Limit]
	}

	// Preload identifier observations for every entity referenced by a row that
	// carries a stored evidence fingerprint (the only rows that need a current
	// fingerprint), loading each entity exactly once to avoid N+1 loads.
	memo := map[string][]IdentifierObservation{}
	load := func(ref IdentityRef) ([]IdentifierObservation, error) {
		key := string(ref.Type) + "|" + ref.ULID
		if obs, ok := memo[key]; ok {
			return obs, nil
		}
		obs, err := s.ids.GetEntityIdentifiers(ctx, ref)
		if err != nil {
			return nil, err
		}
		memo[key] = obs
		return obs, nil
	}

	items := make([]ConflictItem, 0, len(rows))
	for _, row := range rows {
		item := ConflictItem{
			Ref:       IdentityRef{Type: arg.Type, ULID: row.EntityIDA},
			Candidate: IdentityRef{Type: arg.Type, ULID: row.EntityIDB},
			Authority: row.AuthorityCode,
			URI:       row.IdentifierUri,
			Score:     row.Score,
		}

		// No suppression row → never suppressed.
		if !row.EvidenceFingerprint.Valid || row.EvidenceFingerprint.String == "" {
			items = append(items, item)
			continue
		}

		obsA, err := load(IdentityRef{Type: arg.Type, ULID: row.EntityIDA})
		if err != nil {
			return ConflictsResponse{}, err
		}
		obsB, err := load(IdentityRef{Type: arg.Type, ULID: row.EntityIDB})
		if err != nil {
			return ConflictsResponse{}, err
		}

		// Evidence changed → not suppressed.
		if row.EvidenceFingerprint.String != Fingerprint(obsA, obsB) {
			items = append(items, item)
			continue
		}

		item.Suppressed = true
		if arg.IncludeSuppressed && row.DecisionID.Valid && row.DecisionID.String != "" {
			rec, err := s.decisionByID(ctx, row.DecisionID.String)
			if err != nil {
				return ConflictsResponse{}, err
			}
			item.PriorDecision = &rec
		}

		if arg.IncludeSuppressed {
			items = append(items, item)
		}
	}

	resp := ConflictsResponse{Items: items}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		next := encodeConflictsCursor(last.AuthorityCode, last.IdentifierUri, last.EntityIDA, last.EntityIDB)
		resp.NextCursor = &next
	}
	return resp, nil
}

// decisionByID loads a single decision record by its id.
func (s *service) decisionByID(ctx context.Context, id string) (DecisionRecord, error) {
	d, err := s.q.GetIdentityDecision(ctx, id)
	if err != nil {
		return DecisionRecord{}, fmt.Errorf("identity: get decision %q: %w", id, err)
	}
	return decisionRecordFromModel(d)
}

// Decisions returns the newest-first decisions feed.
func (s *service) Decisions(ctx context.Context, arg DecisionsParams) (DecisionsResponse, error) {
	if arg.Type != nil {
		if err := validateEntityType(*arg.Type); err != nil {
			return DecisionsResponse{}, err
		}
	}
	if arg.Limit < 1 {
		arg.Limit = 1
	}

	var cur decisionsCursor
	if arg.Cursor != "" {
		var err error
		cur, err = decodeDecisionsCursor(arg.Cursor)
		if err != nil {
			return DecisionsResponse{}, err
		}
	}

	params := postgres.ListIdentityDecisionsParams{Limit: int32(arg.Limit) + 1}
	if arg.Type != nil {
		params.EntityType = pgtype.Text{String: string(*arg.Type), Valid: true}
	}
	if arg.Since != nil {
		params.Since = pgtype.Timestamptz{Time: *arg.Since, Valid: true}
	}
	if !cur.CreatedAt.IsZero() {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: cur.CreatedAt, Valid: true}
		params.CursorID = pgtype.Text{String: cur.ID, Valid: true}
	}

	recs, err := s.decisions.ListFeed(ctx, params)
	if err != nil {
		return DecisionsResponse{}, err
	}

	// Over-fetch one row to detect a next page; the +1th row is never returned.
	resp := DecisionsResponse{Items: recs}
	if len(recs) > arg.Limit {
		recs = recs[:arg.Limit]
		resp.Items = recs
		last := recs[len(recs)-1]
		next := encodeDecisionsCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &next
	}
	return resp, nil
}

// validateEntityType rejects unsupported entity types with ErrInvalidEntityType.
func validateEntityType(t EntityType) error {
	if t != EntityTypePlace && t != EntityTypeOrganization {
		return fmt.Errorf("%w: %q", ErrInvalidEntityType, t)
	}
	return nil
}

func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// --- cursor encoding -------------------------------------------------------

type conflictsCursor struct {
	Authority string `json:"authority"`
	URI       string `json:"uri"`
	IDA       string `json:"id_a"`
	IDB       string `json:"id_b"`
}

type decisionsCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeConflictsCursor(authority, uri, idA, idB string) string {
	return encodeCursor(conflictsCursor{Authority: authority, URI: uri, IDA: idA, IDB: idB})
}

func decodeConflictsCursor(cursor string) (conflictsCursor, error) {
	var c conflictsCursor
	if err := decodeCursor(cursor, &c); err != nil {
		return conflictsCursor{}, err
	}
	return c, nil
}

func encodeDecisionsCursor(createdAt time.Time, id string) string {
	return encodeCursor(decisionsCursor{CreatedAt: createdAt, ID: id})
}

func decodeDecisionsCursor(cursor string) (decisionsCursor, error) {
	var c decisionsCursor
	if err := decodeCursor(cursor, &c); err != nil {
		return decisionsCursor{}, err
	}
	return c, nil
}

func encodeCursor(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(cursor string, v any) error {
	if cursor == "" {
		return fmt.Errorf("%w: empty", ErrInvalidCursor)
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	return nil
}
