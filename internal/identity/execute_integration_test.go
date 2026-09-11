package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const (
	testPlaceA = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testPlaceB = "01ARZ3NDEKTSV4RRFFQ69G5FBV"

	artsdataURI  = "https://kg.artsdata.ca/resource/K11-24"
	artsdataURI2 = "https://kg.artsdata.ca/resource/K11-99"
	wikidataURI  = "https://www.wikidata.org/entity/Q1"
	invalidURI   = "https://example.com/not-a-valid-artsdata-uri"
)

func newTestExecutor(t *testing.T) (*Executor, *pgxpool.Pool) {
	t.Helper()
	pool := setupIdentity(t)
	store := NewStore(pool)
	decisions := NewDecisionStore(pool)
	return NewExecutor(store, store, decisions, NewNotDuplicateStore(pool)), pool
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func countDecisions(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countRows(t, pool, `SELECT count(*) FROM identity_decisions`)
}

func countIdentifiers(t *testing.T, pool *pgxpool.Pool, entityType, entityID string) int {
	t.Helper()
	return countRows(t, pool,
		`SELECT count(*) FROM entity_identifiers WHERE entity_type = $1 AND entity_id = $2`, entityType, entityID)
}

// failingDecisionStore aborts AppendTx so a test can prove that a decision-append
// failure rolls back the election performed earlier in the same transaction.
type failingDecisionStore struct{ err error }

func (f failingDecisionStore) Append(_ context.Context, _ DecisionRecord) (DecisionRecord, error) {
	return DecisionRecord{}, f.err
}
func (f failingDecisionStore) AppendTx(_ context.Context, _ *postgres.Queries, _ DecisionRecord) (DecisionRecord, error) {
	return DecisionRecord{}, f.err
}
func (f failingDecisionStore) List(_ context.Context, _ IdentityRef) ([]DecisionRecord, error) {
	return nil, nil
}
func (f failingDecisionStore) ListFeed(_ context.Context, _ postgres.ListIdentityDecisionsParams) ([]DecisionRecord, error) {
	return nil, nil
}

// failingNotDuplicateStore aborts UpsertNotDuplicateTx so a test can prove a
// not-duplicate upsert failure rolls back the decision appended earlier in the
// same transaction.
type failingNotDuplicateStore struct{ err error }

func (f failingNotDuplicateStore) GetNotDuplicate(_ context.Context, _ EntityType, _, _ string) (string, bool, error) {
	return "", false, nil
}
func (f failingNotDuplicateStore) UpsertNotDuplicateTx(_ context.Context, _ *postgres.Queries, _ EntityType, _, _, _, _, _ string) error {
	return f.err
}

// TestLinkIdentifier_UpsertAndElect verifies that a link upserts the identifier,
// elects it primary, and appends exactly one decision.
func TestLinkIdentifier_UpsertAndElect(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	rec, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority:  "artsdata",
		URI:        artsdataURI,
		Method:     "manual",
		Confidence: 1.0,
		Source:     "manual",
	}, "agent-test")
	require.NoError(t, err)
	require.Equal(t, ActionLink, rec.Action)
	require.NotEmpty(t, rec.ID)
	require.False(t, rec.CreatedAt.IsZero())
	require.Equal(t, EntityTypePlace, rec.EntityType)
	require.Equal(t, testPlaceA, rec.EntityID)

	require.Equal(t, 1, countIdentifiers(t, pool, string(ref.Type), ref.ULID))
	require.Equal(t, 1, countDecisions(t, pool))

	states := loadPrimaryStates(t, pool, string(ref.Type), ref.ULID)
	require.Len(t, states, 1)
	require.True(t, states[0].IsPrimary, "linked identifier must be elected primary")
	require.Equal(t, artsdataURI, states[0].URI)

	// Idempotent re-link: same URI must not create a second identifier row, but
	// does append a second decision.
	_, err = exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority:  "artsdata",
		URI:        artsdataURI,
		Method:     "manual",
		Confidence: 1.0,
		Source:     "manual",
	}, "agent-test")
	require.NoError(t, err)
	require.Equal(t, 1, countIdentifiers(t, pool, string(ref.Type), ref.ULID))
	require.Equal(t, 2, countDecisions(t, pool))
}

// TestLinkIdentifier_ElectsPrimary verifies a higher-ranked second link demotes the
// first and leaves exactly one primary.
func TestLinkIdentifier_ElectsPrimary(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}

	_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority: "artsdata", URI: artsdataURI, Method: "auto_low", Confidence: 0.90, Source: "reconciliation",
	}, "agent-test")
	require.NoError(t, err)

	_, err = exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority: "artsdata", URI: artsdataURI2, Method: "auto_high", Confidence: 0.99, Source: "reconciliation",
	}, "agent-test")
	require.NoError(t, err)

	states := loadPrimaryStates(t, pool, string(ref.Type), ref.ULID)
	require.Len(t, states, 2)

	var primaries int
	for _, s := range states {
		if s.IsPrimary {
			primaries++
			require.Equal(t, artsdataURI2, s.URI, "auto_high must win")
		}
	}
	require.Equal(t, 1, primaries)
	require.Equal(t, 2, countDecisions(t, pool))
}

// TestLinkIdentifier_EmptyActor verifies an empty actor is a structural error with
// no decision and no identifier written.
func TestLinkIdentifier_EmptyActor(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority: "artsdata", URI: artsdataURI, Method: "manual",
	}, "")
	require.ErrorIs(t, err, ErrEmptyActor)
	require.Equal(t, 0, countDecisions(t, pool))
	require.Equal(t, 0, countIdentifiers(t, pool, string(ref.Type), ref.ULID))
}

// TestLinkIdentifier_URIRejected verifies a URI failing the authority pattern is a
// structural error with no decision and no identifier written.
func TestLinkIdentifier_URIRejected(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority: "artsdata", URI: invalidURI, Method: "manual",
	}, "agent-test")
	require.ErrorIs(t, err, ErrInvalidURI)
	require.Equal(t, 0, countDecisions(t, pool))
	require.Equal(t, 0, countIdentifiers(t, pool, string(ref.Type), ref.ULID))
}

// TestLinkIdentifier_UnknownAuthority verifies an unregistered authority is a
// structural error with no decision written.
func TestLinkIdentifier_UnknownAuthority(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
		Authority: "no-such-authority", URI: artsdataURI, Method: "manual",
	}, "agent-test")
	require.ErrorIs(t, err, ErrUnknownAuthority)
	require.Equal(t, 0, countDecisions(t, pool))
	require.Equal(t, 0, countIdentifiers(t, pool, string(ref.Type), ref.ULID))
}

// TestReject_RecordsNotDuplicate verifies a Reject appends a decision and a
// not-duplicate row with the canonical (id_a < id_b) ordering and a fingerprint.
func TestReject_RecordsNotDuplicate(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	// Both entities assert the same artsdata identifier (a shared signal).
	a := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	b := IdentityRef{Type: EntityTypePlace, ULID: testPlaceB}
	for _, ref := range []IdentityRef{a, b} {
		_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
			Authority: "artsdata", URI: artsdataURI, Method: "auto_high", Confidence: 0.99, Source: "reconciliation",
		}, "agent-test")
		require.NoError(t, err)
	}

	rec, err := exec.Reject(ctx, a, b, "agent-test", "different address/operator")
	require.NoError(t, err)
	require.Equal(t, ActionReject, rec.Action)
	require.Equal(t, EntityTypePlace, rec.EntityType)
	require.Equal(t, testPlaceA, rec.EntityID, "entity_id must be the canonical (smaller) id")
	require.NotNil(t, rec.CounterpartID)
	require.Equal(t, testPlaceB, *rec.CounterpartID)

	var fp string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT evidence_fingerprint FROM identity_not_duplicates WHERE entity_type=$1 AND id_a=$2 AND id_b=$3`,
		string(EntityTypePlace), testPlaceA, testPlaceB).Scan(&fp))
	require.NotEmpty(t, fp)
	require.Equal(t, fp, rec.Metadata["evidence_fingerprint"])

	// Re-rejecting with identical evidence keeps the fingerprint stable.
	_, err = exec.Reject(ctx, a, b, "agent-test", "different address/operator")
	require.NoError(t, err)

	var fp2 string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT evidence_fingerprint FROM identity_not_duplicates WHERE entity_type=$1 AND id_a=$2 AND id_b=$3`,
		string(EntityTypePlace), testPlaceA, testPlaceB).Scan(&fp2))
	require.Equal(t, fp, fp2, "identical evidence must keep the fingerprint stable")
	require.Equal(t, 4, countDecisions(t, pool), "2 links + 2 rejects")
}

// TestReject_ReopensOnEvidenceChange verifies that adding a new shared signal
// changes the stored fingerprint (the re-open condition).
func TestReject_ReopensOnEvidenceChange(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	a := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	b := IdentityRef{Type: EntityTypePlace, ULID: testPlaceB}

	for _, ref := range []IdentityRef{a, b} {
		_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
			Authority: "artsdata", URI: artsdataURI, Method: "auto_high", Confidence: 0.99, Source: "reconciliation",
		}, "agent-test")
		require.NoError(t, err)
	}

	_, err := exec.Reject(ctx, a, b, "agent-test", "distinct")
	require.NoError(t, err)

	fpBefore := getNotDuplicateFingerprint(t, pool, string(EntityTypePlace), testPlaceA, testPlaceB)

	// A new, shared signal (wikidata on both sides) materially changes the evidence.
	for _, ref := range []IdentityRef{a, b} {
		_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
			Authority: "wikidata", URI: wikidataURI, Method: "auto_high", Confidence: 0.95, Source: "reconciliation",
		}, "agent-test")
		require.NoError(t, err)
	}

	_, err = exec.Reject(ctx, a, b, "agent-test", "still distinct")
	require.NoError(t, err)

	fpAfter := getNotDuplicateFingerprint(t, pool, string(EntityTypePlace), testPlaceA, testPlaceB)
	require.NotEqual(t, fpBefore, fpAfter, "changed evidence must change the fingerprint (re-open)")
}

// TestReject_EmptyActor verifies an empty actor is a structural error with no
// decision and no not-duplicate row.
func TestReject_EmptyActor(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	a := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	b := IdentityRef{Type: EntityTypePlace, ULID: testPlaceB}

	_, err := exec.Reject(ctx, a, b, "", "distinct")
	require.ErrorIs(t, err, ErrEmptyActor)
	require.Equal(t, 0, countDecisions(t, pool))
	require.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM identity_not_duplicates`))
}

// TestReject_StructuralErrors verifies entity-type and same-entity structural
// failures write nothing.
func TestReject_StructuralErrors(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	a := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	org := IdentityRef{Type: EntityTypeOrganization, ULID: testPlaceB}

	_, err := exec.Reject(ctx, a, org, "agent-test", "distinct")
	require.ErrorIs(t, err, ErrMismatchedEntityTypes)

	_, err = exec.Reject(ctx, a, a, "agent-test", "distinct")
	require.ErrorIs(t, err, ErrSameEntityPair)

	require.Equal(t, 0, countDecisions(t, pool))
	require.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM identity_not_duplicates`))
}

// TestLinkIdentifier_AtomicRollback verifies a decision-append failure rolls back
// the election performed earlier in the same transaction.
func TestLinkIdentifier_AtomicRollback(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)
	exec := NewExecutor(store, store, failingDecisionStore{err: errors.New("decision append failed")}, NewNotDuplicateStore(pool))

	ref := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	_, err := exec.LinkIdentifier(context.Background(), ref, IdentifierObservation{
		Authority: "artsdata", URI: artsdataURI, Method: "manual", Confidence: 1.0, Source: "manual",
	}, "agent-test")
	require.Error(t, err)

	require.Equal(t, 0, countIdentifiers(t, pool, string(ref.Type), ref.ULID),
		"election must roll back when the decision append fails")
	require.Equal(t, 0, countDecisions(t, pool))
}

// TestLinkIdentifier_InvalidEntityType verifies an unsupported entity type is a
// structural error with no decision and no identifier written.
func TestLinkIdentifier_InvalidEntityType(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()

	_, err := exec.LinkIdentifier(ctx, IdentityRef{Type: EntityType("event"), ULID: testPlaceA},
		IdentifierObservation{Authority: "artsdata", URI: artsdataURI, Method: "manual"}, "agent-test")
	require.ErrorIs(t, err, ErrInvalidEntityType)
	require.Equal(t, 0, countDecisions(t, pool))
	require.Equal(t, 0, countIdentifiers(t, pool, "event", testPlaceA))
}

// TestReject_AtomicRollback verifies a not-duplicate upsert failure rolls back the
// decision appended earlier in the same transaction.
func TestReject_AtomicRollback(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)
	exec := NewExecutor(store, store, NewDecisionStore(pool), failingNotDuplicateStore{err: errors.New("not-duplicate upsert failed")})

	a := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	b := IdentityRef{Type: EntityTypePlace, ULID: testPlaceB}

	_, err := exec.Reject(context.Background(), a, b, "agent-test", "distinct")
	require.Error(t, err)

	require.Equal(t, 0, countDecisions(t, pool), "decision must roll back when the not-duplicate upsert fails")
	require.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM identity_not_duplicates`))
}

// TestNotDuplicateStore_GetNotDuplicate verifies the read half: a Reject writes a
// not-duplicate row, and GetNotDuplicate returns the stored fingerprint (with
// canonicalization) or found=false before any suppression.
func TestNotDuplicateStore_GetNotDuplicate(t *testing.T) {
	exec, pool := newTestExecutor(t)
	ctx := context.Background()
	nds := NewNotDuplicateStore(pool)

	a := IdentityRef{Type: EntityTypePlace, ULID: testPlaceA}
	b := IdentityRef{Type: EntityTypePlace, ULID: testPlaceB}
	for _, ref := range []IdentityRef{a, b} {
		_, err := exec.LinkIdentifier(ctx, ref, IdentifierObservation{
			Authority: "artsdata", URI: artsdataURI, Method: "auto_high", Confidence: 0.99, Source: "reconciliation",
		}, "agent-test")
		require.NoError(t, err)
	}

	_, found, err := nds.GetNotDuplicate(ctx, EntityTypePlace, testPlaceA, testPlaceB)
	require.NoError(t, err)
	require.False(t, found)

	_, err = exec.Reject(ctx, a, b, "agent-test", "distinct")
	require.NoError(t, err)

	fp, found, err := nds.GetNotDuplicate(ctx, EntityTypePlace, testPlaceA, testPlaceB)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, fp)

	fpSwap, foundSwap, err := nds.GetNotDuplicate(ctx, EntityTypePlace, testPlaceB, testPlaceA)
	require.NoError(t, err)
	require.True(t, foundSwap)
	require.Equal(t, fp, fpSwap, "GetNotDuplicate must canonicalize the pair")
}

func getNotDuplicateFingerprint(t *testing.T, pool *pgxpool.Pool, entityType, idA, idB string) string {
	t.Helper()
	var fp string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT evidence_fingerprint FROM identity_not_duplicates WHERE entity_type=$1 AND id_a=$2 AND id_b=$3`,
		entityType, idA, idB).Scan(&fp))
	return fp
}
