package identity

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// seedIdentifierRow inserts an entity_identifiers row directly with an explicit
// is_primary flag, bypassing RecordObservation's automatic election. This is the
// only way to construct the drift states tidy exists to repair: a group with no
// primary, and (after the partial unique index is dropped) a group with more
// than one primary.
func seedIdentifierRow(t *testing.T, pool *pgxpool.Pool, entityType, entityID, authority, uri, method string, isPrimary bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO entity_identifiers
			(entity_type, entity_id, authority_code, identifier_uri, confidence, reconciliation_method, is_canonical, observed_at, is_primary)
		VALUES ($1, $2, $3, $4, 0.9, $5, false, now(), $6)`,
		entityType, entityID, authority, uri, method, isPrimary)
	require.NoError(t, err)
}

// dropPrimaryIndex drops the partial unique index that enforces at most one
// primary per group, and restores it when the test finishes. Dropping it is the
// only way a post-000051 schema can hold a multi-primary group, because the
// index rejects a second is_primary=true row outright.
func dropPrimaryIndex(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `DROP INDEX IF EXISTS idx_entity_identifiers_one_primary`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), `CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_identifiers_one_primary ON entity_identifiers(entity_type, entity_id, authority_code) WHERE is_primary`)
		if err != nil {
			t.Errorf("recreate primary index: %v", err)
		}
	})
}

// countDriftedGroups returns the number of groups violating the primary
// invariant: groups whose primary-row count is anything other than exactly one.
// After a successful --apply this must be zero.
func countDriftedGroups(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM (
			SELECT entity_type, entity_id, authority_code
			FROM entity_identifiers
			GROUP BY entity_type, entity_id, authority_code
			HAVING COUNT(*) FILTER (WHERE is_primary) <> 1
		) drift`).Scan(&n)
	require.NoError(t, err)
	return n
}

func countPrimaries(t *testing.T, pool *pgxpool.Pool, entityType, entityID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM entity_identifiers
		WHERE entity_type = $1 AND entity_id = $2 AND is_primary`, entityType, entityID).Scan(&n)
	require.NoError(t, err)
	return n
}

// TestTidy_RepairsDrift covers User Story 5: seed a "no primary" group and a
// legacy multi-primary group (after dropping the partial unique index), then
// verify dry-run purity, apply, invariant restoration, and idempotency.
func TestTidy_RepairsDrift(t *testing.T) {
	pool := setupIdentity(t)
	dropPrimaryIndex(t, pool)
	ctx := context.Background()

	// Group A: observations but no primary (fill case).
	seedIdentifierRow(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAA", "artsdata", "https://kg.artsdata.ca/resource/K11-100", "auto_high", false)

	// Group B: two primaries (legacy/pre-index state — demote case). auto_high
	// outranks auto_low, so the auto_low row must be demoted.
	seedIdentifierRow(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAB", "artsdata", "https://kg.artsdata.ca/resource/K11-200", "auto_high", true)
	seedIdentifierRow(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAB", "artsdata", "https://kg.artsdata.ca/resource/K11-201", "auto_low", true)

	// Group C: healthy (exactly one primary) — must be scanned but left alone.
	seedIdentifierRow(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAC", "artsdata", "https://kg.artsdata.ca/resource/K11-300", "auto_high", true)

	store := NewStore(pool)

	// Dry-run reports the drift but mutates nothing.
	stats, err := store.Tidy(ctx, EntityTypePlace, false)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.EntitiesScanned)
	require.Equal(t, int64(1), stats.PrimariesFilled)
	require.Equal(t, int64(1), stats.RowsDemoted)

	require.Equal(t, 2, countDriftedGroups(t, pool), "dry-run must not repair drift")
	require.Equal(t, 0, countPrimaries(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAA"), "group A still has no primary after dry-run")
	require.Equal(t, 2, countPrimaries(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAB"), "group B still has two primaries after dry-run")

	// Apply repairs the drift.
	stats, err = store.Tidy(ctx, EntityTypePlace, true)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.EntitiesScanned)
	require.Equal(t, int64(1), stats.PrimariesFilled)
	require.Equal(t, int64(1), stats.RowsDemoted)

	require.Equal(t, 0, countDriftedGroups(t, pool), "invariant must hold after apply")
	require.Equal(t, 1, countPrimaries(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAA"), "group A must have exactly one primary")
	require.Equal(t, 1, countPrimaries(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAB"), "group B must have exactly one primary")

	// The canonical winner (auto_high) must be the surviving primary in group B.
	states := loadPrimaryStates(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAB")
	for _, s := range states {
		if s.IsPrimary {
			require.Equal(t, "https://kg.artsdata.ca/resource/K11-200", s.URI, "auto_high must win over auto_low")
		} else {
			require.Equal(t, "https://kg.artsdata.ca/resource/K11-201", s.URI)
			require.NotNil(t, s.SupersededByID, "demoted row must record its superseding winner")
		}
	}

	// Idempotent: a second apply reports zero changes.
	stats, err = store.Tidy(ctx, EntityTypePlace, true)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.EntitiesScanned)
	require.Equal(t, int64(0), stats.PrimariesFilled)
	require.Equal(t, int64(0), stats.RowsDemoted)
}

// TestTidy_TypeFilter verifies --type scoping: only the selected entity type is
// repaired; the other type's drift is left untouched.
func TestTidy_TypeFilter(t *testing.T) {
	pool := setupIdentity(t)
	dropPrimaryIndex(t, pool)
	ctx := context.Background()

	// Drift in both place and organization.
	seedIdentifierRow(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAD", "artsdata", "https://kg.artsdata.ca/resource/K11-400", "auto_high", false)
	seedIdentifierRow(t, pool, "organization", "01ARZ3NDEKTSV4RRFFQ69G5FAE", "artsdata", "https://kg.artsdata.ca/resource/K11-500", "auto_high", false)

	store := NewStore(pool)

	// Tidy only places.
	stats, err := store.Tidy(ctx, EntityTypePlace, true)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.EntitiesScanned)
	require.Equal(t, int64(1), stats.PrimariesFilled)
	require.Equal(t, int64(0), stats.RowsDemoted)

	require.Equal(t, 1, countPrimaries(t, pool, "place", "01ARZ3NDEKTSV4RRFFQ69G5FAD"), "place primary must be filled")
	require.Equal(t, 0, countPrimaries(t, pool, "organization", "01ARZ3NDEKTSV4RRFFQ69G5FAE"), "organization drift must be untouched")

	// Tidy organizations next.
	stats, err = store.Tidy(ctx, EntityTypeOrganization, true)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.EntitiesScanned)
	require.Equal(t, int64(1), stats.PrimariesFilled)
	require.Equal(t, int64(0), stats.RowsDemoted)

	require.Equal(t, 1, countPrimaries(t, pool, "organization", "01ARZ3NDEKTSV4RRFFQ69G5FAE"), "organization primary must be filled")
}

// TestTidy_EmptySchema verifies tidy over an empty table reports zero counts and
// no error (both place and organization, and the zero-value "all types" scope).
func TestTidy_EmptySchema(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)

	stats, err := store.Tidy(context.Background(), "", true)
	require.NoError(t, err)
	require.Equal(t, int64(0), stats.EntitiesScanned)
	require.Equal(t, int64(0), stats.PrimariesFilled)
	require.Equal(t, int64(0), stats.RowsDemoted)
}

// TestTidy_InvalidType verifies an unsupported entity type is rejected.
func TestTidy_InvalidType(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)

	_, err := store.Tidy(context.Background(), EntityType("event"), true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidEntityType)
}
