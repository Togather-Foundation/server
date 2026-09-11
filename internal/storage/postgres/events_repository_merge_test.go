package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

const artsdataAuthority = "artsdata"

// seedAuthority inserts the artsdata authority row. resetDatabase truncates
// knowledge_graph_authorities, and entity_identifiers.authority_code references it,
// so identifiers cannot be inserted until the authority exists.
func seedAuthority(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO knowledge_graph_authorities (authority_code, authority_name, base_uri_pattern, trust_level, priority_order)
VALUES ($1, $1, '^https?://kg\.artsdata\.ca/resource/K\d+-\d+$', 9, 10)
ON CONFLICT (authority_code) DO NOTHING
`, artsdataAuthority)
	require.NoError(t, err)
}

func insertIdentifier(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityType, entityID, authority, uri, method string, confidence float64, isPrimary bool) int32 {
	t.Helper()
	var id int32
	err := pool.QueryRow(ctx, `
INSERT INTO entity_identifiers (entity_type, entity_id, authority_code, identifier_uri, confidence, reconciliation_method, is_canonical, is_primary, observed_at, source)
VALUES ($1, $2, $3, $4, $5, $6, false, $7, now(), 'reconciliation')
RETURNING id
`, entityType, entityID, authority, uri, confidence, method, isPrimary).Scan(&id)
	require.NoError(t, err)
	return id
}

func countIdentifiers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityID string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM entity_identifiers WHERE entity_id = $1`, entityID).Scan(&n))
	return n
}

func countPrimaryIdentifiers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityType, entityID string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM entity_identifiers WHERE entity_type = $1 AND entity_id = $2 AND is_primary`, entityType, entityID).Scan(&n))
	return n
}

// setSupersededBy sets a row's superseded_by_id, simulating a prior demotion whose
// supersession provenance must survive (or be cleared only when the target is deleted).
func setSupersededBy(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rowID, supersededByID int32) {
	t.Helper()
	_, err := pool.Exec(ctx, `UPDATE entity_identifiers SET superseded_by_id = $2 WHERE id = $1`, rowID, supersededByID)
	require.NoError(t, err)
}

// TestMergePlaces_PreservesIdentity covers US2 scenario 1+2: two places with distinct
// identifiers; merging A into B leaves B holding both, exactly one primary, A
// soft-deleted with deletion_reason='merged', and zero identifiers referencing A.
func TestMergePlaces_PreservesIdentity(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)
	insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.98, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	result, err := repo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)
	require.Equal(t, primary.ID, result.CanonicalID)

	// Survivor holds both identifiers, exactly one primary.
	require.Equal(t, 2, countIdentifiers(t, ctx, pool, primary.ULID))
	require.Equal(t, 1, countPrimaryIdentifiers(t, ctx, pool, "place", primary.ULID))

	// The higher-confidence row (K11-24) wins the primary slot.
	var primaryURI string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT identifier_uri FROM entity_identifiers WHERE entity_type = 'place' AND entity_id = $1 AND is_primary`, primary.ULID).Scan(&primaryURI))
	require.Equal(t, "https://kg.artsdata.ca/resource/K11-24", primaryURI)

	// Duplicate is soft-deleted with merged_into_id and deletion_reason='merged'.
	var mergedInto, deletedAt, reason string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT merged_into_id::text, deleted_at::text, deletion_reason FROM places WHERE id = $1`, dup.ID).
		Scan(&mergedInto, &deletedAt, &reason))
	require.Equal(t, primary.ID, mergedInto)
	require.NotEmpty(t, deletedAt)
	require.Equal(t, "merged", reason)

	// No identifiers reference the duplicate.
	require.Equal(t, 0, countIdentifiers(t, ctx, pool, dup.ULID))
}

// TestMergeOrganizations_PreservesIdentity mirrors the place test for organizations.
func TestMergeOrganizations_PreservesIdentity(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertOrganization(t, ctx, pool, "Dup Org")
	primary := insertOrganization(t, ctx, pool, "Primary Org")

	insertIdentifier(t, ctx, pool, "organization", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)
	insertIdentifier(t, ctx, pool, "organization", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.98, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	result, err := repo.MergeOrganizations(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	require.Equal(t, 2, countIdentifiers(t, ctx, pool, primary.ULID))
	require.Equal(t, 1, countPrimaryIdentifiers(t, ctx, pool, "organization", primary.ULID))
	require.Equal(t, 0, countIdentifiers(t, ctx, pool, dup.ULID))

	var mergedInto, deletedAt, reason string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT merged_into_id::text, deleted_at::text, deletion_reason FROM organizations WHERE id = $1`, dup.ID).
		Scan(&mergedInto, &deletedAt, &reason))
	require.Equal(t, primary.ID, mergedInto)
	require.NotEmpty(t, deletedAt)
	require.Equal(t, "merged", reason)
}

// TestMergePlaces_SharedAuthorityURIDedup covers US2 scenario 4: when both sides
// assert the same authority+URI, the union is deduplicated to one row and one primary,
// and the surviving row keeps the higher confidence.
func TestMergePlaces_SharedAuthorityURIDedup(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.90, true)
	insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.95, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	result, err := repo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	require.Equal(t, 1, countIdentifiers(t, ctx, pool, primary.ULID))
	require.Equal(t, 1, countPrimaryIdentifiers(t, ctx, pool, "place", primary.ULID))
	require.Equal(t, 0, countIdentifiers(t, ctx, pool, dup.ULID))

	// The single surviving row keeps the higher confidence (0.95, from the survivor).
	var conf float64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT confidence::float8 FROM entity_identifiers WHERE entity_type = 'place' AND entity_id = $1`, primary.ULID).Scan(&conf))
	require.InDelta(t, 0.95, conf, 1e-6)
}

// TestMergePlaces_RollbackOnFailure covers US2 scenario 3: if the transaction fails
// after identifier reassignment but before commit, the reassignment and soft-delete
// are rolled back together.
func TestMergePlaces_RollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)

	// Run the merge on an explicit transaction so we can inspect mid-tx state and
	// then roll it back to simulate a later failure.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	txRepo := &EventRepository{pool: pool, tx: tx, logger: zerolog.Nop()}

	result, err := txRepo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	// Mid-tx: the reassignment is visible on the transaction connection.
	var reassigned int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT count(*) FROM entity_identifiers WHERE entity_id = $1`, primary.ULID).Scan(&reassigned))
	require.Equal(t, 1, reassigned)

	// Roll back — simulating a failure after reassignment but before commit.
	require.NoError(t, tx.Rollback(ctx))

	// Identifiers still belong to the duplicate.
	require.Equal(t, 1, countIdentifiers(t, ctx, pool, dup.ULID))

	// The duplicate is not soft-deleted.
	var deletedAt *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT deleted_at::text FROM places WHERE id = $1`, dup.ID).Scan(&deletedAt))
	require.Nil(t, deletedAt)
}

// TestAdminService_MergePlaces_WritesTombstone covers US2 scenario 2: the duplicate
// receives a place_tombstones row with non-null URI, deleted_at, and payload.
func TestAdminService_MergePlaces_WritesTombstone(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)
	insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.98, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	svc := events.NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation", zerolog.Nop())

	result, err := svc.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	var uri string
	var payload []byte
	var deletedAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT place_uri, payload, deleted_at FROM place_tombstones WHERE place_id::text = $1`, dup.ID).
		Scan(&uri, &payload, &deletedAt))
	require.NotEmpty(t, uri)
	require.NotEmpty(t, payload)
	require.False(t, deletedAt.IsZero())
}

// TestMergePlaces_DedupePreservesStrongerMethod covers the provenance-preserving dedupe:
// when the duplicate carries a stronger reconciliation method for a shared authority+URI,
// the survivor's surviving row must adopt the duplicate's method (not the other way).
func TestMergePlaces_DedupePreservesStrongerMethod(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	// Same authority+URI; the duplicate carries the stronger (manual) method.
	insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "manual", 0.5, true)
	insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_low", 0.9, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	result, err := repo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	require.Equal(t, 1, countIdentifiers(t, ctx, pool, primary.ULID))

	var method string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT reconciliation_method FROM entity_identifiers WHERE entity_type = 'place' AND entity_id = $1`, primary.ULID).Scan(&method))
	require.Equal(t, "manual", method)
}

// TestMergePlaces_ReelectionSetsSupersededBy verifies that after a merge re-elects a
// primary, the demoted row records superseded_by_id pointing at the winner — mirroring
// DemotePrimaryAndSupersede/SetPrimary — rather than being wiped to NULL.
func TestMergePlaces_ReelectionSetsSupersededBy(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	dupRowID := insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)
	survRowID := insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.98, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	_, err := repo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)

	// The duplicate's K11-24 row (0.99) becomes primary.
	var primaryRowID int32
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM entity_identifiers WHERE entity_type = 'place' AND entity_id = $1 AND is_primary`, primary.ULID).Scan(&primaryRowID))
	require.Equal(t, dupRowID, primaryRowID)

	// The survivor's K11-25 row (0.98) is demoted with superseded_by_id → winner.
	var superseded pgtype.Int4
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT superseded_by_id FROM entity_identifiers WHERE id = $1`, survRowID).Scan(&superseded))
	require.True(t, superseded.Valid)
	require.Equal(t, dupRowID, superseded.Int32)
}

// TestMergePlaces_PreservesSurvivorSupersededByWhenNoDupIdentifiers verifies that a merge
// whose duplicate has no identifiers leaves the survivor's supersession provenance intact.
func TestMergePlaces_PreservesSurvivorSupersededByWhenNoDupIdentifiers(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	winnerID := insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.99, true)
	loserID := insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.98, false)
	setSupersededBy(t, ctx, pool, loserID, winnerID)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	_, err := repo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)

	var superseded pgtype.Int4
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT superseded_by_id FROM entity_identifiers WHERE id = $1`, loserID).Scan(&superseded))
	require.True(t, superseded.Valid)
	require.Equal(t, winnerID, superseded.Int32)
}

// TestMergePlaces_ClearsSupersededRefsBeforeDedupe verifies that superseded_by_id
// references into a row being deleted by the dedupe are cleared so the delete cannot
// fail on the FK (which has no ON DELETE action).
func TestMergePlaces_ClearsSupersededRefsBeforeDedupe(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	// The duplicate has two rows: d (K11-24, primary) and e (K11-25, superseded by d).
	dID := insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)
	eID := insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.98, false)
	setSupersededBy(t, ctx, pool, eID, dID)

	// The survivor collides with d (same K11-24).
	insertIdentifier(t, ctx, pool, "place", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	result, err := repo.MergePlaces(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	// d was deleted (collision); e was reassigned. Survivor holds K11-24 and K11-25.
	require.Equal(t, 2, countIdentifiers(t, ctx, pool, primary.ULID))
	require.Equal(t, 0, countIdentifiers(t, ctx, pool, dup.ULID))
}

// TestAdminService_MergeOrganizations_WritesTombstone verifies organization merges write
// an organization_tombstones row with non-null URI, deleted_at, payload, and a
// superseded_by_uri pointing at the survivor.
func TestAdminService_MergeOrganizations_WritesTombstone(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertOrganization(t, ctx, pool, "Dup Org")
	primary := insertOrganization(t, ctx, pool, "Primary Org")

	insertIdentifier(t, ctx, pool, "organization", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)
	insertIdentifier(t, ctx, pool, "organization", primary.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-25", "auto_high", 0.98, true)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	svc := events.NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation", zerolog.Nop())

	result, err := svc.MergeOrganizations(ctx, dup.ID, primary.ID)
	require.NoError(t, err)
	require.False(t, result.AlreadyMerged)

	var uri, supersededBy string
	var payload []byte
	var deletedAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT organization_uri, superseded_by_uri, payload, deleted_at FROM organization_tombstones WHERE organization_id::text = $1`, dup.ID).
		Scan(&uri, &supersededBy, &payload, &deletedAt))
	require.NotEmpty(t, uri)
	require.NotEmpty(t, supersededBy)
	require.NotEmpty(t, payload)
	require.False(t, deletedAt.IsZero())
}

// TestAdminService_MergePlaces_AtomicityOnTombstoneFailure injects a failure in the
// tombstone-write step (via a DB trigger) and asserts the whole merge rolls back: the
// duplicate is not soft-deleted and its identifiers are unchanged.
func TestAdminService_MergePlaces_AtomicityOnTombstoneFailure(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	seedAuthority(t, ctx, pool)

	dup := insertPlace(t, ctx, pool, "Dup Venue", "Toronto", "ON")
	primary := insertPlace(t, ctx, pool, "Primary Venue", "Toronto", "ON")

	insertIdentifier(t, ctx, pool, "place", dup.ULID, artsdataAuthority, "https://kg.artsdata.ca/resource/K11-24", "auto_high", 0.99, true)

	_, err := pool.Exec(ctx, `
CREATE OR REPLACE FUNCTION fail_place_tombstone() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'injected tombstone failure';
END;
$$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE TRIGGER fail_place_tombstone BEFORE INSERT ON place_tombstones FOR EACH ROW EXECUTE FUNCTION fail_place_tombstone()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_place_tombstone ON place_tombstones`)
		_, _ = pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS fail_place_tombstone()`)
	})

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	svc := events.NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation", zerolog.Nop())

	_, err = svc.MergePlaces(ctx, dup.ID, primary.ID)
	require.Error(t, err)

	// Identifiers still belong to the duplicate.
	require.Equal(t, 1, countIdentifiers(t, ctx, pool, dup.ULID))

	// The duplicate is not soft-deleted.
	var deletedAt *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT deleted_at::text FROM places WHERE id = $1`, dup.ID).Scan(&deletedAt))
	require.Nil(t, deletedAt)

	// No tombstone was written.
	var tombCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM place_tombstones WHERE place_id::text = $1`, dup.ID).Scan(&tombCount))
	require.Equal(t, 0, tombCount)
}

// TestMergeEvents_SetsDeletionReason verifies the event merge path sets
// deletion_reason='merged' on the merged (duplicate) event row.
func TestMergeEvents_SetsDeletionReason(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	dupULID := ulid.Make().String()
	primaryULID := ulid.Make().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO events (ulid, name, virtual_url) VALUES ($1, 'Dup Event', 'https://example.com/live'), ($2, 'Primary Event', 'https://example.com/live')`, dupULID, primaryULID)
	require.NoError(t, err)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	require.NoError(t, repo.MergeEvents(ctx, dupULID, primaryULID))

	var reason string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT deletion_reason FROM events WHERE ulid = $1`, dupULID).Scan(&reason))
	require.Equal(t, "merged", reason)
}
