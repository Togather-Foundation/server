package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/places"

	"github.com/stretchr/testify/require"
)

func TestPlaceRepositoryListFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &PlaceRepository{pool: pool}

	placeA := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	setPlaceCreatedAt(t, ctx, pool, placeA.ID, time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))

	placeB := insertPlace(t, ctx, pool, "Riverside Gallery", "Toronto", "ON")
	setPlaceCreatedAt(t, ctx, pool, placeB.ID, time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC))

	insertPlace(t, ctx, pool, "Ottawa Arena", "Ottawa", "ON")

	filters := places.Filters{City: "Toronto"}
	page1, err := repo.List(ctx, filters, places.Pagination{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page1.Places, 1)
	require.Equal(t, "Centennial Park", page1.Places[0].Name)
	require.NotEmpty(t, page1.NextCursor)

	page2, err := repo.List(ctx, filters, places.Pagination{Limit: 1, After: page1.NextCursor})
	require.NoError(t, err)
	require.Len(t, page2.Places, 1)
	require.Equal(t, "Riverside Gallery", page2.Places[0].Name)
	require.Empty(t, page2.NextCursor)

	queryResult, err := repo.List(ctx, places.Filters{Query: "Gallery"}, places.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, queryResult.Places, 1)
	require.Equal(t, placeB.ULID, queryResult.Places[0].ULID)
}

func TestPlaceRepositoryGetByULID(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &PlaceRepository{pool: pool}

	place := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")

	result, err := repo.GetByULID(ctx, place.ULID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, place.ULID, result.ULID)
	require.Equal(t, "Centennial Park", result.Name)

	missing, err := repo.GetByULID(ctx, "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	require.ErrorIs(t, err, places.ErrNotFound)
	require.Nil(t, missing)
}
