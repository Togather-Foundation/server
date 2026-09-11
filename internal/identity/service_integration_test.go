package identity

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) (Service, *Store) {
	t.Helper()
	pool := setupIdentity(t)
	store := NewStore(pool)
	return NewService(postgres.New(pool), store, NewDecisionStore(pool)), store
}

func seedObservation(t *testing.T, store *Store, typ EntityType, id, uri string) {
	t.Helper()
	_, err := store.RecordObservation(context.Background(), IdentityRef{Type: typ, ULID: id}, IdentifierObservation{
		Authority:  "artsdata",
		URI:        uri,
		Method:     "auto_high",
		Confidence: 0.9,
		Source:     "reconciliation",
	})
	require.NoError(t, err)
}

// TestService_Conflicts_KeysetPaginationAndFiltering seeds three places sharing
// one (authority, uri) plus a second pair, pages through with limit=2, and
// asserts no skips/dupes and correct entity_type filtering against a
// simultaneously-seeded organization pair.
func TestService_Conflicts_KeysetPaginationAndFiltering(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()

	uriX := "https://kg.artsdata.ca/resource/K11-X"
	uriY := "https://kg.artsdata.ca/resource/K11-Y"
	uriZ := "https://kg.artsdata.ca/resource/K11-Z"

	for _, id := range []string{"place-a", "place-b", "place-c"} {
		seedObservation(t, store, EntityTypePlace, id, uriX)
	}
	seedObservation(t, store, EntityTypePlace, "place-d", uriY)
	seedObservation(t, store, EntityTypePlace, "place-e", uriY)

	seedObservation(t, store, EntityTypeOrganization, "org-a", uriZ)
	seedObservation(t, store, EntityTypeOrganization, "org-b", uriZ)

	// Page through type=place with limit=2, collecting every pair.
	var seen []ConflictItem
	cursor := ""
	for {
		resp, err := svc.Conflicts(ctx, ConflictsParams{Type: EntityTypePlace, Limit: 2, Cursor: cursor})
		require.NoError(t, err)
		seen = append(seen, resp.Items...)
		if resp.NextCursor == nil {
			break
		}
		cursor = *resp.NextCursor
	}

	require.Len(t, seen, 4, "3 pairs sharing uriX + 1 pair sharing uriY")

	keys := map[string]bool{}
	var order []string
	for _, it := range seen {
		key := it.Ref.ULID + "<" + it.Candidate.ULID
		require.False(t, keys[key], "duplicate pair %s", key)
		keys[key] = true
		require.Equal(t, EntityTypePlace, it.Ref.Type)
		require.Equal(t, EntityTypePlace, it.Candidate.Type)
		order = append(order, it.Ref.ULID)
	}
	require.Equal(t, []string{"place-a", "place-a", "place-b", "place-d"}, order,
		"pairs must be ordered by (authority, uri, entity_id_a, entity_id_b)")

	// type=organization returns only the single org pair.
	resp, err := svc.Conflicts(ctx, ConflictsParams{Type: EntityTypeOrganization, Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "org-a", resp.Items[0].Ref.ULID)
	require.Equal(t, "org-b", resp.Items[0].Candidate.ULID)
}

// TestService_Conflicts_PriorDecisionSuppression seeds two pairs and suppresses
// one, then asserts suppressed pairs are excluded by default and included with
// their prior_decision when include_suppressed=true.
func TestService_Conflicts_PriorDecisionSuppression(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)
	svc := NewService(postgres.New(pool), store, NewDecisionStore(pool))
	ctx := context.Background()

	uriX := "https://kg.artsdata.ca/resource/K11-X"
	uriY := "https://kg.artsdata.ca/resource/K11-Y"

	for _, id := range []string{"place-a", "place-b"} {
		seedObservation(t, store, EntityTypePlace, id, uriX)
	}
	seedObservation(t, store, EntityTypePlace, "place-c", uriY)
	seedObservation(t, store, EntityTypePlace, "place-d", uriY)

	// Suppress (place-a, place-b) with a decision + not-duplicate row whose
	// evidence fingerprint matches the pair's current shared signals.
	fp := Fingerprint(
		[]IdentifierObservation{{Authority: "artsdata", URI: uriX}},
		[]IdentifierObservation{{Authority: "artsdata", URI: uriX}},
	)
	_, err := pool.Exec(ctx, `
		INSERT INTO identity_decisions (id, entity_type, entity_id, action, counterpart_type, counterpart_id, rationale, citations, confidence, actor, reversible)
		VALUES ('idn-sup', 'place', 'place-a', 'reject', 'place', 'place-b', 'distinct', '[]', 0, 'agent', true)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO identity_not_duplicates (entity_type, id_a, id_b, evidence_fingerprint, decision_id, created_by)
		VALUES ('place', 'place-a', 'place-b', $1, 'idn-sup', 'agent')`, fp)
	require.NoError(t, err)

	// Default: suppressed pair excluded.
	resp, err := svc.Conflicts(ctx, ConflictsParams{Type: EntityTypePlace, Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "place-c", resp.Items[0].Ref.ULID)
	require.False(t, resp.Items[0].Suppressed)

	// include_suppressed=true: both pairs, the suppressed one with prior_decision.
	resp, err = svc.Conflicts(ctx, ConflictsParams{Type: EntityTypePlace, Limit: 10, IncludeSuppressed: true})
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)

	byPair := map[string]ConflictItem{}
	for _, it := range resp.Items {
		byPair[it.Ref.ULID] = it
	}
	require.Contains(t, byPair, "place-a")
	require.Contains(t, byPair, "place-c")

	suppressed := byPair["place-a"]
	require.True(t, suppressed.Suppressed)
	require.NotNil(t, suppressed.PriorDecision)
	require.Equal(t, "idn-sup", suppressed.PriorDecision.ID)
	require.Equal(t, "distinct", suppressed.PriorDecision.Rationale)

	require.False(t, byPair["place-c"].Suppressed)
	require.Nil(t, byPair["place-c"].PriorDecision)
}
