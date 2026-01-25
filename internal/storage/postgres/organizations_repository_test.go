package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestOrganizationRepositoryListFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer container.Terminate(ctx)

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &OrganizationRepository{pool: pool}

	orgA := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	setOrganizationLegalName(t, ctx, pool, orgA.ID, "Toronto Arts Organization")
	setOrganizationCreatedAt(t, ctx, pool, orgA.ID, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	orgB := insertOrganization(t, ctx, pool, "City Gallery")
	setOrganizationCreatedAt(t, ctx, pool, orgB.ID, time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC))

	orgC := insertOrganization(t, ctx, pool, "Ottawa Tourism")
	setOrganizationCreatedAt(t, ctx, pool, orgC.ID, time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC))

	page1, err := repo.List(ctx, organizations.Filters{}, organizations.Pagination{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page1.Organizations, 1)
	require.Equal(t, "Toronto Arts Org", page1.Organizations[0].Name)
	require.NotEmpty(t, page1.NextCursor)

	page2, err := repo.List(ctx, organizations.Filters{}, organizations.Pagination{Limit: 1, After: page1.NextCursor})
	require.NoError(t, err)
	require.Len(t, page2.Organizations, 1)
	require.Equal(t, "City Gallery", page2.Organizations[0].Name)
	require.NotEmpty(t, page2.NextCursor)

	page3, err := repo.List(ctx, organizations.Filters{}, organizations.Pagination{Limit: 1, After: page2.NextCursor})
	require.NoError(t, err)
	require.Len(t, page3.Organizations, 1)
	require.Equal(t, "Ottawa Tourism", page3.Organizations[0].Name)
	require.Empty(t, page3.NextCursor)

	queryResult, err := repo.List(ctx, organizations.Filters{Query: "Gallery"}, organizations.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, queryResult.Organizations, 1)
	require.Equal(t, orgB.ULID, queryResult.Organizations[0].ULID)
}

func TestOrganizationRepositoryGetByULID(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer container.Terminate(ctx)

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &OrganizationRepository{pool: pool}

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")

	result, err := repo.GetByULID(ctx, org.ULID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, org.ULID, result.ULID)
	require.Equal(t, "Toronto Arts Org", result.Name)

	missing, err := repo.GetByULID(ctx, "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	require.ErrorIs(t, err, organizations.ErrNotFound)
	require.Nil(t, missing)
}
