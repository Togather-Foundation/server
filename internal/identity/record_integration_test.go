package identity

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// TestDecisionStore_RoundTrip verifies the decision read/store paths: Append,
// List, ListFeed, and decisionRecordFromModel round-trip appended decisions with
// full field fidelity, ordering, and keyset/filter behaviour.
func TestDecisionStore_RoundTrip(t *testing.T) {
	pool := setupIdentity(t)
	decisions := NewDecisionStore(pool)
	ctx := context.Background()

	counterpartType := EntityTypePlace
	counterpartID := testPlaceB

	link := DecisionRecord{
		EntityType: EntityTypePlace,
		EntityID:   testPlaceA,
		Action:     ActionLink,
		Citations:  []string{},
		Confidence: 0.95,
		Actor:      "agent-a",
		Reversible: true,
		Metadata:   map[string]any{"authority": "artsdata", "uri": artsdataURI},
	}

	reject := DecisionRecord{
		EntityType:      EntityTypePlace,
		EntityID:        testPlaceA,
		Action:          ActionReject,
		CounterpartType: &counterpartType,
		CounterpartID:   &counterpartID,
		Rationale:       "different operator",
		Citations:       []string{"https://example.com/evidence/1", "https://example.com/evidence/2"},
		Confidence:      0.0,
		Actor:           "agent-b",
		Reversible:      true,
		Metadata:        map[string]any{"evidence_fingerprint": "abc123"},
	}

	// Append via the own-tx convenience.
	linkOut, err := decisions.Append(ctx, link)
	require.NoError(t, err)
	require.NotEmpty(t, linkOut.ID)
	require.False(t, linkOut.CreatedAt.IsZero())

	rejectOut, err := decisions.Append(ctx, reject)
	require.NoError(t, err)
	require.NotEmpty(t, rejectOut.ID)
	require.NotEqual(t, linkOut.ID, rejectOut.ID)

	// List by entity: newest first.
	recs, err := decisions.List(ctx, IdentityRef{Type: EntityTypePlace, ULID: testPlaceA})
	require.NoError(t, err)
	require.Len(t, recs, 2)
	require.Equal(t, ActionReject, recs[0].Action, "List must return newest first")
	require.Equal(t, ActionLink, recs[1].Action)

	// Field fidelity on the decoded reject decision.
	got := recs[0]
	require.Equal(t, rejectOut.ID, got.ID)
	require.NotNil(t, got.CounterpartType)
	require.Equal(t, EntityTypePlace, *got.CounterpartType)
	require.NotNil(t, got.CounterpartID)
	require.Equal(t, testPlaceB, *got.CounterpartID)
	require.Equal(t, "different operator", got.Rationale)
	require.Equal(t, []string{"https://example.com/evidence/1", "https://example.com/evidence/2"}, got.Citations)
	require.Equal(t, "agent-b", got.Actor)
	require.True(t, got.Reversible)
	require.Equal(t, "abc123", got.Metadata["evidence_fingerprint"])

	// Field fidelity on the decoded link decision.
	gotLink := recs[1]
	require.Nil(t, gotLink.CounterpartType)
	require.Nil(t, gotLink.CounterpartID)
	require.Equal(t, "artsdata", gotLink.Metadata["authority"])
	require.InDelta(t, 0.95, gotLink.Confidence, 1e-6)

	// ListFeed: unfiltered, newest first.
	feed, err := decisions.ListFeed(ctx, postgres.ListIdentityDecisionsParams{Limit: 100})
	require.NoError(t, err)
	require.Len(t, feed, 2)
	require.Equal(t, ActionReject, feed[0].Action)

	// ListFeed: filter by action.
	links, err := decisions.ListFeed(ctx, postgres.ListIdentityDecisionsParams{
		Action: pgtype.Text{String: ActionLink, Valid: true},
		Limit:  100,
	})
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, ActionLink, links[0].Action)

	// ListFeed: filter by entity_type (matches both).
	places, err := decisions.ListFeed(ctx, postgres.ListIdentityDecisionsParams{
		EntityType: pgtype.Text{String: string(EntityTypePlace), Valid: true},
		Limit:      100,
	})
	require.NoError(t, err)
	require.Len(t, places, 2)

	// ListFeed: keyset pagination — page 1 (limit 1), then continue from its cursor.
	page1, err := decisions.ListFeed(ctx, postgres.ListIdentityDecisionsParams{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page1, 1)
	require.Equal(t, ActionReject, page1[0].Action)

	page2, err := decisions.ListFeed(ctx, postgres.ListIdentityDecisionsParams{
		CursorCreatedAt: pgtype.Timestamptz{Time: page1[0].CreatedAt, Valid: true},
		CursorID:        pgtype.Text{String: page1[0].ID, Valid: true},
		Limit:           1,
	})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Equal(t, ActionLink, page2[0].Action)
	require.NotEqual(t, page1[0].ID, page2[0].ID)

	// ListFeed: since lower bound (inclusive) returns both.
	all, err := decisions.ListFeed(ctx, postgres.ListIdentityDecisionsParams{
		Since: pgtype.Timestamptz{Time: linkOut.CreatedAt.Add(-time.Second), Valid: true},
		Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, all, 2)
}
