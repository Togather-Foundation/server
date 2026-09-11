package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
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
// assert the same authority+URI, the union is deduplicated to one row and one primary.
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
