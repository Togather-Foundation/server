package postgres

import (
	"context"
	"regexp"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
)

// quotedLifecycleLiteral matches the quoted lifecycle values in the output of
// pg_get_constraintdef, e.g. "ARRAY['draft'::text, 'published'::text, ...]".
var quotedLifecycleLiteral = regexp.MustCompile(`'([a-z_]+)'`)

// TestPublicStatesMatchesLifecycleConstraint is a drift guard between the
// database's event lifecycle enum and the Go public-visibility allowlist.
//
// Public visibility is enforced in raw SQL, SQLc and Go. If a lifecycle state
// is added to (or removed from) the database CHECK constraint without updating
// events.PublicStates, the new state would fall through the visibility
// predicates: the list/detail SQL fail closed, but the change feed and any
// future query could silently misclassify it. This test makes that omission a
// CI failure instead.
func TestPublicStatesMatchesLifecycleConstraint(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	var definition string
	err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid)
		  FROM pg_constraint
		 WHERE conrelid = 'events'::regclass
		   AND conname = 'events_lifecycle_state_check'
	`).Scan(&definition)
	require.NoError(t, err, "events lifecycle CHECK constraint must exist")

	dbStates := map[string]struct{}{}
	for _, match := range quotedLifecycleLiteral.FindAllStringSubmatch(definition, -1) {
		dbStates[match[1]] = struct{}{}
	}
	require.NotEmpty(t, dbStates, "could not parse lifecycle states from constraint: %s", definition)

	publicStates := map[string]struct{}{}
	for _, state := range events.PublicStates {
		publicStates[state] = struct{}{}
	}

	// Everything the database accepts must be classified as either public or
	// system-only. Adding a state to the constraint therefore requires a
	// deliberate visibility decision here.
	hiddenStates := []string{"draft", "pending_review", "deleted"}
	expected := map[string]struct{}{}
	for state := range publicStates {
		expected[state] = struct{}{}
	}
	for _, state := range hiddenStates {
		expected[state] = struct{}{}
		require.NotContains(t, publicStates, state, "%q must not be publicly visible", state)
	}

	require.Equal(t, expected, dbStates,
		"events lifecycle CHECK constraint and events.PublicStates have drifted; update internal/domain/events/visibility.go and this test")

	// The first_published_at trigger classifies states with its own SQL copy of
	// the allowlist (event_is_public_state). If that copy drifts, an event could
	// be marked (or not marked) as ever-public without any Go code changing.
	var functionDef string
	err = pool.QueryRow(ctx, `
		SELECT pg_get_functiondef('event_is_public_state(text)'::regprocedure)
	`).Scan(&functionDef)
	require.NoError(t, err, "event_is_public_state(text) must exist (migration 000055)")

	functionStates := map[string]struct{}{}
	for _, match := range quotedLifecycleLiteral.FindAllStringSubmatch(functionDef, -1) {
		functionStates[match[1]] = struct{}{}
	}
	require.NotEmpty(t, functionStates, "could not parse lifecycle states from function: %s", functionDef)

	require.Equal(t, publicStates, functionStates,
		"event_is_public_state() and events.PublicStates have drifted; update migration 000055 so first_published_at uses the same allowlist")

	// Literal parsing cannot catch a logic-only mutation (a flipped operator, an
	// inverted predicate, etc.). Exercise the function for every classified state.
	for state := range publicStates {
		var visible bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT event_is_public_state($1)`, state).Scan(&visible))
		require.True(t, visible, "event_is_public_state(%q) must be true", state)
	}
	for _, state := range hiddenStates {
		var visible bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT event_is_public_state($1)`, state).Scan(&visible))
		require.False(t, visible, "event_is_public_state(%q) must be false", state)
	}
}
