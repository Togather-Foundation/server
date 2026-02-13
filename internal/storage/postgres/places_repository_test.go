package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
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

func TestPlaceRepositoryListProximitySearch(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &PlaceRepository{pool: pool}

	// Insert places with coordinates
	// Toronto City Hall: 43.6534, -79.3839
	cityHall := insertPlaceWithCoords(t, ctx, pool, "Toronto City Hall", "Toronto", "ON", 43.6534, -79.3839)

	// CN Tower: 43.6426, -79.3871 (about 1.2km from City Hall)
	cnTower := insertPlaceWithCoords(t, ctx, pool, "CN Tower", "Toronto", "ON", 43.6426, -79.3871)

	// Place far away (Hamilton, ON): 43.2557, -79.8711 (about 60km from City Hall)
	insertPlaceWithCoords(t, ctx, pool, "Hamilton Place", "Hamilton", "ON", 43.2557, -79.8711)

	// Place without coordinates (should be excluded from proximity results)
	insertPlace(t, ctx, pool, "Unknown Location", "Toronto", "ON")

	// Test 1: Search within 2km of City Hall (should find City Hall and CN Tower)
	lat := 43.6534
	lon := -79.3839
	radius := 2.0
	filters := places.Filters{
		Latitude:  &lat,
		Longitude: &lon,
		RadiusKm:  &radius,
	}

	result, err := repo.List(ctx, filters, places.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Places, 2)

	// Results should be sorted by distance (closest first)
	require.Equal(t, cityHall.ULID, result.Places[0].ULID)
	require.Equal(t, cnTower.ULID, result.Places[1].ULID)

	// Check distance_km is populated
	require.NotNil(t, result.Places[0].DistanceKm)
	require.NotNil(t, result.Places[1].DistanceKm)

	// City Hall should be very close (essentially 0km from search point)
	require.Less(t, *result.Places[0].DistanceKm, 0.1)

	// CN Tower should be about 1.2km away
	require.Greater(t, *result.Places[1].DistanceKm, 1.0)
	require.Less(t, *result.Places[1].DistanceKm, 1.5)

	// Test 2: Search within 1km (should find only City Hall)
	radius = 1.0
	filters.RadiusKm = &radius

	result, err = repo.List(ctx, filters, places.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Places, 1)
	require.Equal(t, cityHall.ULID, result.Places[0].ULID)

	// Test 3: Search within 100km (should find all 3 with coordinates)
	radius = 100.0
	filters.RadiusKm = &radius

	result, err = repo.List(ctx, filters, places.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Places, 3) // Excludes the place without coordinates
}

func TestPlaceRepositoryListProximityWithCityFilter(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &PlaceRepository{pool: pool}

	// Insert places in Toronto
	insertPlaceWithCoords(t, ctx, pool, "Toronto Place A", "Toronto", "ON", 43.6534, -79.3839)
	insertPlaceWithCoords(t, ctx, pool, "Toronto Place B", "Toronto", "ON", 43.6426, -79.3871)

	// Insert place in Hamilton nearby
	insertPlaceWithCoords(t, ctx, pool, "Hamilton Place", "Hamilton", "ON", 43.2557, -79.8711)

	// Search within 100km but filter by city
	lat := 43.6534
	lon := -79.3839
	radius := 100.0
	filters := places.Filters{
		City:      "Toronto",
		Latitude:  &lat,
		Longitude: &lon,
		RadiusKm:  &radius,
	}

	result, err := repo.List(ctx, filters, places.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Places, 2) // Only Toronto places
}

// Helper to insert a place with geo coordinates
func insertPlaceWithCoords(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, city string, region string, lat float64, lon float64) seededEntity {
	t.Helper()
	ulidValue := ulid.Make().String()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO places (ulid, name, address_locality, address_region, latitude, longitude)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		ulidValue, name, city, region, lat, lon,
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}
