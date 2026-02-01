package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestEventRepositoryListFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &EventRepository{pool: pool}

	orgA := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	orgB := insertOrganization(t, ctx, pool, "City Gallery")
	placeA := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	placeB := insertPlace(t, ctx, pool, "Riverside Gallery", "Toronto", "ON")
	placeC := insertPlace(t, ctx, pool, "Ottawa Arena", "Ottawa", "ON")

	startA := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	startB := time.Date(2026, 7, 20, 18, 0, 0, 0, time.UTC)
	startC := time.Date(2026, 8, 1, 20, 0, 0, 0, time.UTC)

	ulidA := insertEvent(t, ctx, pool, "Jazz in the Park", "Live jazz", orgA, placeA, "music", "published", []string{"jazz", "summer"}, startA)
	_ = insertEvent(t, ctx, pool, "Summer Arts Expo", "Gallery showcase", orgB, placeB, "arts", "draft", []string{"gallery"}, startB)
	_ = insertEvent(t, ctx, pool, "Ottawa Winter Fest", "Snow fun", orgB, placeC, "culture", "published", []string{"winter"}, startC)

	filters := events.Filters{
		City:           "Toronto",
		StartDate:      timePtr(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
		EndDate:        timePtr(time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC)),
		LifecycleState: "published",
	}

	page1, err := repo.List(ctx, filters, events.Pagination{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page1.Events, 1)
	require.Equal(t, "Jazz in the Park", page1.Events[0].Name)
	require.Empty(t, page1.NextCursor)

	filters = events.Filters{Query: "Jazz"}
	queryResult, err := repo.List(ctx, filters, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, queryResult.Events, 1)
	require.Equal(t, ulidA, queryResult.Events[0].ULID)

	filters = events.Filters{Keywords: []string{"jazz"}}
	keywordResult, err := repo.List(ctx, filters, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, keywordResult.Events, 1)
	require.Equal(t, ulidA, keywordResult.Events[0].ULID)

	filters = events.Filters{VenueULID: placeA.ULID}
	venueResult, err := repo.List(ctx, filters, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, venueResult.Events, 1)
	require.Equal(t, ulidA, venueResult.Events[0].ULID)

	filters = events.Filters{OrganizerULID: orgA.ULID}
	orgResult, err := repo.List(ctx, filters, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, orgResult.Events, 1)
	require.Equal(t, ulidA, orgResult.Events[0].ULID)

	filters = events.Filters{Domain: "music"}
	domainResult, err := repo.List(ctx, filters, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, domainResult.Events, 1)
	require.Equal(t, ulidA, domainResult.Events[0].ULID)
}

func TestEventRepositoryListDedupesOccurrences(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &EventRepository{pool: pool}

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	place := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	start := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulidValue := insertEvent(t, ctx, pool, "Jazz in the Park", "Live jazz", org, place, "music", "published", []string{"jazz"}, start)

	var eventID string
	err = pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidValue).Scan(&eventID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
         VALUES ($1, $2, $3, $4)`,
		eventID, start.Add(24*time.Hour), start.Add(26*time.Hour), place.ID,
	)
	require.NoError(t, err)

	result, err := repo.List(ctx, events.Filters{}, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	require.Equal(t, ulidValue, result.Events[0].ULID)
	require.True(t, result.Events[0].Occurrences[0].StartTime.UTC().Equal(start))
}

func TestEventRepositoryGetByULID(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &EventRepository{pool: pool}

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	place := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	start := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulidValue := insertEvent(t, ctx, pool, "Jazz in the Park", "Live jazz", org, place, "music", "published", []string{"jazz"}, start)

	result, err := repo.GetByULID(ctx, ulidValue)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Jazz in the Park", result.Name)
	require.Len(t, result.Occurrences, 1)
	require.True(t, result.Occurrences[0].StartTime.UTC().Equal(start))

	missing, err := repo.GetByULID(ctx, "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	require.ErrorIs(t, err, events.ErrNotFound)
	require.Nil(t, missing)
}
