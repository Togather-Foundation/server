package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// --- fakes -----------------------------------------------------------------

type fakeReadQueries struct {
	conflicts []postgres.ListConflictsRow
	decision  postgres.IdentityDecision
	exists    bool
}

func (f *fakeReadQueries) ListConflicts(_ context.Context, _ postgres.ListConflictsParams) ([]postgres.ListConflictsRow, error) {
	return f.conflicts, nil
}

func (f *fakeReadQueries) GetIdentityDecision(_ context.Context, _ string) (postgres.IdentityDecision, error) {
	return f.decision, nil
}

func (f *fakeReadQueries) EntityExists(_ context.Context, _ postgres.EntityExistsParams) (bool, error) {
	return f.exists, nil
}

type fakeIdentifierStore struct {
	obs map[string][]IdentifierObservation
}

func (f *fakeIdentifierStore) GetEntityIdentifiers(_ context.Context, ref IdentityRef) ([]IdentifierObservation, error) {
	return f.obs[string(ref.Type)+"|"+ref.ULID], nil
}

func (f *fakeIdentifierStore) RecordObservation(context.Context, IdentityRef, IdentifierObservation) (IdentifierObservation, error) {
	return IdentifierObservation{}, errors.New("unused")
}

func (f *fakeIdentifierStore) RecordObservationTx(context.Context, *postgres.Queries, IdentityRef, IdentifierObservation) (IdentifierObservation, error) {
	return IdentifierObservation{}, errors.New("unused")
}

type fakeDecisionStore struct {
	list     []DecisionRecord
	feed     []DecisionRecord
	feedArgs postgres.ListIdentityDecisionsParams
}

func (f *fakeDecisionStore) Append(context.Context, DecisionRecord) (DecisionRecord, error) {
	return DecisionRecord{}, errors.New("unused")
}

func (f *fakeDecisionStore) AppendTx(context.Context, *postgres.Queries, DecisionRecord) (DecisionRecord, error) {
	return DecisionRecord{}, errors.New("unused")
}

func (f *fakeDecisionStore) List(_ context.Context, _ IdentityRef) ([]DecisionRecord, error) {
	return f.list, nil
}

func (f *fakeDecisionStore) ListFeed(_ context.Context, arg postgres.ListIdentityDecisionsParams) ([]DecisionRecord, error) {
	f.feedArgs = arg
	return f.feed, nil
}

// --- cursor round-trips ----------------------------------------------------

func TestConflictsCursorRoundTrip(t *testing.T) {
	cursor := encodeConflictsCursor("artsdata", "https://kg.artsdata.ca/resource/K11-24", "01ARZ3NDEKTSV4RRFFQ69G5FAV", "01ARZ3NDEKTSV4RRFFQ69G5FBV")
	decoded, err := decodeConflictsCursor(cursor)
	require.NoError(t, err)
	require.Equal(t, "artsdata", decoded.Authority)
	require.Equal(t, "https://kg.artsdata.ca/resource/K11-24", decoded.URI)
	require.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", decoded.IDA)
	require.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FBV", decoded.IDB)
	require.Equal(t, cursor, encodeConflictsCursor(decoded.Authority, decoded.URI, decoded.IDA, decoded.IDB))
}

func TestDecisionsCursorRoundTrip(t *testing.T) {
	created := time.Date(2026, 9, 1, 12, 0, 0, 123000, time.UTC)
	cursor := encodeDecisionsCursor(created, "idn-01ARZ3NDEKTSV4RRFFQ69G5FAV")
	decoded, err := decodeDecisionsCursor(cursor)
	require.NoError(t, err)
	require.Equal(t, created, decoded.CreatedAt)
	require.Equal(t, "idn-01ARZ3NDEKTSV4RRFFQ69G5FAV", decoded.ID)
	require.Equal(t, cursor, encodeDecisionsCursor(decoded.CreatedAt, decoded.ID))
}

func TestDecodeCursor_Invalid(t *testing.T) {
	_, err := decodeConflictsCursor("not-base64!!")
	require.ErrorIs(t, err, ErrInvalidCursor)

	_, err = decodeConflictsCursor("")
	require.ErrorIs(t, err, ErrInvalidCursor)
}

// --- conflicts suppression -------------------------------------------------

func newConflictService(fq *fakeReadQueries, ids *fakeIdentifierStore) *service {
	return &service{q: fq, ids: ids, decisions: &fakeDecisionStore{}}
}

func conflictObs(authority, uri string) []IdentifierObservation {
	return []IdentifierObservation{{Authority: authority, URI: uri}}
}

func TestConflicts_ExcludesSuppressedByDefault(t *testing.T) {
	// Pair A (suppressed): stored fingerprint matches the current shared signals.
	obsA := conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-24")
	obsB := conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-24")
	fp := Fingerprint(obsA, obsB)

	ids := &fakeIdentifierStore{obs: map[string][]IdentifierObservation{
		"place|A": obsA,
		"place|B": obsB,
		"place|C": conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-99"),
		"place|D": conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-99"),
	}}

	fq := &fakeReadQueries{conflicts: []postgres.ListConflictsRow{
		{
			EntityType:          "place",
			EntityIDA:           "A",
			EntityIDB:           "B",
			AuthorityCode:       "artsdata",
			IdentifierUri:       "https://kg.artsdata.ca/resource/K11-24",
			Score:               0.99,
			EvidenceFingerprint: pgtype.Text{String: fp, Valid: true},
			DecisionID:          pgtype.Text{String: "idn-1", Valid: true},
		},
		{
			EntityType:    "place",
			EntityIDA:     "C",
			EntityIDB:     "D",
			AuthorityCode: "artsdata",
			IdentifierUri: "https://kg.artsdata.ca/resource/K11-99",
			Score:         0.90,
		},
	}}

	svc := newConflictService(fq, ids)
	resp, err := svc.Conflicts(context.Background(), ConflictsParams{Type: EntityTypePlace, Limit: 10})
	require.NoError(t, err)

	require.Len(t, resp.Items, 1)
	require.Equal(t, "C", resp.Items[0].Ref.ULID)
	require.False(t, resp.Items[0].Suppressed)
}

func TestConflicts_IncludesSuppressedWithPriorDecision(t *testing.T) {
	obsA := conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-24")
	obsB := conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-24")
	fp := Fingerprint(obsA, obsB)

	ids := &fakeIdentifierStore{obs: map[string][]IdentifierObservation{
		"place|A": obsA,
		"place|B": obsB,
	}}

	prior := postgres.IdentityDecision{
		ID:         "idn-1",
		CreatedAt:  pgtype.Timestamptz{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EntityType: "place",
		EntityID:   "A",
		Action:     "reject",
		Rationale:  "different address",
		Actor:      "agent",
		Reversible: true,
	}

	fq := &fakeReadQueries{
		decision: prior,
		conflicts: []postgres.ListConflictsRow{{
			EntityType:          "place",
			EntityIDA:           "A",
			EntityIDB:           "B",
			AuthorityCode:       "artsdata",
			IdentifierUri:       "https://kg.artsdata.ca/resource/K11-24",
			Score:               0.99,
			EvidenceFingerprint: pgtype.Text{String: fp, Valid: true},
			DecisionID:          pgtype.Text{String: "idn-1", Valid: true},
		}},
	}

	svc := newConflictService(fq, ids)
	resp, err := svc.Conflicts(context.Background(), ConflictsParams{Type: EntityTypePlace, Limit: 10, IncludeSuppressed: true})
	require.NoError(t, err)

	require.Len(t, resp.Items, 1)
	item := resp.Items[0]
	require.True(t, item.Suppressed)
	require.NotNil(t, item.PriorDecision)
	require.Equal(t, "idn-1", item.PriorDecision.ID)
	require.Equal(t, "different address", item.PriorDecision.Rationale)
}

func TestConflicts_SuppressedOnChangedEvidence(t *testing.T) {
	// The stored fingerprint no longer matches the current signals: an extra
	// shared identifier was added, so the pair resurfaces (not suppressed).
	obsA := conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-24")
	obsB := conflictObs("artsdata", "https://kg.artsdata.ca/resource/K11-24")
	staleFP := Fingerprint(obsA, obsB)

	// Current evidence differs (an additional shared signal).
	obsACurrent := append(append([]IdentifierObservation{}, obsA...), conflictObs("wikidata", "https://www.wikidata.org/wiki/Q1")...)
	obsBCurrent := append(append([]IdentifierObservation{}, obsB...), conflictObs("wikidata", "https://www.wikidata.org/wiki/Q1")...)

	ids := &fakeIdentifierStore{obs: map[string][]IdentifierObservation{
		"place|A": obsACurrent,
		"place|B": obsBCurrent,
	}}

	fq := &fakeReadQueries{conflicts: []postgres.ListConflictsRow{{
		EntityType:          "place",
		EntityIDA:           "A",
		EntityIDB:           "B",
		AuthorityCode:       "artsdata",
		IdentifierUri:       "https://kg.artsdata.ca/resource/K11-24",
		Score:               0.99,
		EvidenceFingerprint: pgtype.Text{String: staleFP, Valid: true},
		DecisionID:          pgtype.Text{String: "idn-1", Valid: true},
	}}}

	svc := newConflictService(fq, ids)
	resp, err := svc.Conflicts(context.Background(), ConflictsParams{Type: EntityTypePlace, Limit: 10})
	require.NoError(t, err)

	require.Len(t, resp.Items, 1)
	require.False(t, resp.Items[0].Suppressed)
	require.Nil(t, resp.Items[0].PriorDecision)
}

// --- decisions feed --------------------------------------------------------

func TestDecisions_FeedPassesSinceTypeAndCursor(t *testing.T) {
	older := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	feed := &fakeDecisionStore{feed: []DecisionRecord{
		{ID: "idn-newer", CreatedAt: newer, EntityType: EntityTypePlace, EntityID: "A", Action: ActionLink},
		{ID: "idn-older", CreatedAt: older, EntityType: EntityTypePlace, EntityID: "A", Action: ActionLink},
	}}

	svc := &service{q: &fakeReadQueries{}, ids: &fakeIdentifierStore{}, decisions: feed}

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cursor := encodeDecisionsCursor(newer, "idn-newer")
	typ := EntityTypePlace

	resp, err := svc.Decisions(context.Background(), DecisionsParams{
		Type:   &typ,
		Since:  &since,
		Limit:  1,
		Cursor: cursor,
	})
	require.NoError(t, err)

	// Cursor/since/type must be forwarded to the keyset query.
	require.True(t, feed.feedArgs.EntityType.Valid)
	require.Equal(t, "place", feed.feedArgs.EntityType.String)
	require.True(t, feed.feedArgs.Since.Valid)
	require.Equal(t, since, feed.feedArgs.Since.Time)
	require.True(t, feed.feedArgs.CursorCreatedAt.Valid)
	require.Equal(t, newer, feed.feedArgs.CursorCreatedAt.Time)
	require.Equal(t, "idn-newer", feed.feedArgs.CursorID.String)
	require.Equal(t, int32(2), feed.feedArgs.Limit, "must over-fetch limit+1")

	// Page returns one item; next_cursor encodes the last returned row.
	require.Len(t, resp.Items, 1)
	require.Equal(t, "idn-newer", resp.Items[0].ID)
	require.NotNil(t, resp.NextCursor)
	require.Equal(t, encodeDecisionsCursor(newer, "idn-newer"), *resp.NextCursor)
}

// --- view ------------------------------------------------------------------

func TestView_NotFound(t *testing.T) {
	svc := &service{q: &fakeReadQueries{exists: false}, ids: &fakeIdentifierStore{}, decisions: &fakeDecisionStore{}}
	_, err := svc.View(context.Background(), IdentityRef{Type: EntityTypePlace, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"})
	require.ErrorIs(t, err, ErrEntityNotFound)
}

func TestView_AssemblesPrimaryAndDecisions(t *testing.T) {
	ids := &fakeIdentifierStore{obs: map[string][]IdentifierObservation{
		"place|A": {
			{Authority: "artsdata", URI: "https://kg.artsdata.ca/resource/K11-24", Method: "manual", Confidence: 1.0, IsPrimary: true, Source: "manual"},
			{Authority: "artsdata", URI: "https://kg.artsdata.ca/resource/K11-25", Method: "auto_high", Confidence: 0.9, IsPrimary: false, Source: "reconciliation"},
		},
	}}
	decisions := &fakeDecisionStore{list: []DecisionRecord{{
		ID: "idn-1", EntityType: EntityTypePlace, EntityID: "A", Action: ActionLink,
	}}}

	svc := &service{q: &fakeReadQueries{exists: true}, ids: ids, decisions: decisions}
	view, err := svc.View(context.Background(), IdentityRef{Type: EntityTypePlace, ULID: "A"})
	require.NoError(t, err)

	require.Len(t, view.Identifiers, 2)
	require.Equal(t, "https://kg.artsdata.ca/resource/K11-24", view.Primary["artsdata"])
	require.Len(t, view.Decisions, 1)
}

func TestView_InvalidEntityType(t *testing.T) {
	svc := &service{q: &fakeReadQueries{}, ids: &fakeIdentifierStore{}, decisions: &fakeDecisionStore{}}
	_, err := svc.View(context.Background(), IdentityRef{Type: EntityType("event"), ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"})
	require.ErrorIs(t, err, ErrInvalidEntityType)
}
