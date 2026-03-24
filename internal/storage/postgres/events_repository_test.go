package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"

	"github.com/stretchr/testify/require"
)

func TestEventRepositoryListFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

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
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool}

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	place := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	start := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulidValue := insertEvent(t, ctx, pool, "Jazz in the Park", "Live jazz", org, place, "music", "published", []string{"jazz"}, start)

	var eventID string
	err := pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidValue).Scan(&eventID)
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
	pool, _ := setupPostgres(t, ctx)

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

func TestFindSimilarPlacesReturnsAddressFields(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool}

	// Insert a place with all optional fields populated.
	placeULID := "01JPLCTEST00000000001"
	_, err := pool.Exec(ctx, `
		INSERT INTO places (ulid, name, address_locality, address_region, street_address, postal_code, url, telephone, email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, placeULID, "The Rex Jazz Bar", "Toronto", "ON",
		"194 Queen St W", "M5V 1Z1",
		"https://therex.com", "+1-416-598-2475", "info@therex.com",
	)
	require.NoError(t, err)

	candidates, err := repo.FindSimilarPlaces(ctx, "Rex Jazz Bar", "Toronto", "ON", 0.3)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	c := candidates[0]
	require.Equal(t, placeULID, c.ULID)
	require.Equal(t, "The Rex Jazz Bar", c.Name)

	require.NotNil(t, c.AddressStreet, "AddressStreet should be populated")
	require.Equal(t, "194 Queen St W", *c.AddressStreet)

	require.NotNil(t, c.AddressLocality, "AddressLocality should be populated")
	require.Equal(t, "Toronto", *c.AddressLocality)

	require.NotNil(t, c.AddressRegion, "AddressRegion should be populated")
	require.Equal(t, "ON", *c.AddressRegion)

	require.NotNil(t, c.PostalCode, "PostalCode should be populated")
	require.Equal(t, "M5V 1Z1", *c.PostalCode)

	require.NotNil(t, c.URL, "URL should be populated")
	require.Equal(t, "https://therex.com", *c.URL)

	require.NotNil(t, c.Telephone, "Telephone should be populated")
	require.Equal(t, "+1-416-598-2475", *c.Telephone)

	require.NotNil(t, c.Email, "Email should be populated")
	require.Equal(t, "info@therex.com", *c.Email)
}

func TestFindSimilarPlacesNullableFieldsAreNilWhenAbsent(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool}

	// Insert a place with only required fields.
	placeULID := "01JPLCTEST00000000002"
	_, err := pool.Exec(ctx, `
		INSERT INTO places (ulid, name, address_locality, address_region)
		VALUES ($1, $2, $3, $4)
	`, placeULID, "The Rex Jazz Bar", "Toronto", "ON")
	require.NoError(t, err)

	candidates, err := repo.FindSimilarPlaces(ctx, "Rex Jazz Bar", "Toronto", "ON", 0.3)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	c := candidates[0]
	require.Nil(t, c.AddressStreet)
	require.Nil(t, c.PostalCode)
	require.Nil(t, c.URL)
	require.Nil(t, c.Telephone)
	require.Nil(t, c.Email)
	// locality/region come back non-nil since they were set (used for filtering)
	require.NotNil(t, c.AddressLocality)
	require.NotNil(t, c.AddressRegion)
}

func TestFindSimilarOrganizationsReturnsAddressFields(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool}

	orgULID := "01JORGTEST00000000001"
	_, err := pool.Exec(ctx, `
		INSERT INTO organizations (ulid, name, address_locality, address_region, url, telephone, email)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, orgULID, "Toronto Jazz Society", "Toronto", "ON",
		"https://torjazz.org", "+1-416-555-0200", "info@torjazz.org",
	)
	require.NoError(t, err)

	candidates, err := repo.FindSimilarOrganizations(ctx, "Tor Jazz Society", "Toronto", "ON", 0.3)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	c := candidates[0]
	require.Equal(t, orgULID, c.ULID)

	require.NotNil(t, c.AddressLocality, "AddressLocality should be populated")
	require.Equal(t, "Toronto", *c.AddressLocality)

	require.NotNil(t, c.AddressRegion, "AddressRegion should be populated")
	require.Equal(t, "ON", *c.AddressRegion)

	require.NotNil(t, c.URL, "URL should be populated")
	require.Equal(t, "https://torjazz.org", *c.URL)

	require.NotNil(t, c.Telephone, "Telephone should be populated")
	require.Equal(t, "+1-416-555-0200", *c.Telephone)

	require.NotNil(t, c.Email, "Email should be populated")
	require.Equal(t, "info@torjazz.org", *c.Email)
}

func TestFindSeriesCompanion(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool}

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	place := insertPlace(t, ctx, pool, "The Rex Jazz Bar", "Toronto", "ON")

	// Week 1: a published companion event at 19:00 UTC on March 31.
	week1Start := time.Date(2026, 3, 31, 19, 0, 0, 0, time.UTC)
	week1ULID := insertEvent(t, ctx, pool, "Jazz Jam Session", "Live jazz", org, place, "music", "published", nil, week1Start)

	// Week 2: the incoming event 7 days later at the same time.
	week2Start := time.Date(2026, 4, 7, 19, 0, 0, 0, time.UTC)
	week2ULID := insertEvent(t, ctx, pool, "Jazz Jam Session", "Live jazz", org, place, "music", "published", nil, week2Start)

	t.Run("finds companion from previous week", func(t *testing.T) {
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        place.ID,
			StartTime:      week2Start,
			ExcludeULID:    week2ULID,
		})
		require.NoError(t, err)
		require.NotNil(t, result, "expected companion from week 1 to be found")
		require.Equal(t, week1ULID, result.ULID)
		require.Equal(t, "Jazz Jam Session", result.Name)
		// StartDate must be formatted as "YYYY-MM-DD" — not an RFC3339 timestamp.
		require.Equal(t, "2026-03-31", result.StartDate)
		// StartTime must be formatted as "HH:MM:SS".
		require.Equal(t, "19:00:00", result.StartTime)
		require.Equal(t, "The Rex Jazz Bar", result.VenueName)
	})

	t.Run("returns nil when no companion exists (no matching event in window)", func(t *testing.T) {
		// Look for a companion 7–21 days before a date far in the future — nothing there.
		farFuture := time.Date(2027, 1, 1, 19, 0, 0, 0, time.UTC)
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        place.ID,
			StartTime:      farFuture,
			ExcludeULID:    "",
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when no companion exists in the window")
	})

	t.Run("returns nil when companion ULID is excluded", func(t *testing.T) {
		// Exclude the week-1 event — should return nil even though it matches.
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        place.ID,
			StartTime:      week2Start,
			ExcludeULID:    week1ULID,
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when the only match is excluded")
	})

	t.Run("does not match events at a different venue", func(t *testing.T) {
		otherPlace := insertPlace(t, ctx, pool, "Another Venue", "Toronto", "ON")
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        otherPlace.ID,
			StartTime:      week2Start,
			ExcludeULID:    week2ULID,
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when venue does not match")
	})

	t.Run("does not match events with dissimilar names", func(t *testing.T) {
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "totally different event",
			VenueID:        place.ID,
			StartTime:      week2Start,
			ExcludeULID:    week2ULID,
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when name similarity is too low")
	})

	t.Run("does not match soft-deleted events", func(t *testing.T) {
		// Soft-delete the week-1 event, then search — should return nil.
		_, err := pool.Exec(ctx, `UPDATE events SET deleted_at = NOW() WHERE ulid = $1`, week1ULID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `UPDATE events SET deleted_at = NULL WHERE ulid = $1`, week1ULID)
		})

		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        place.ID,
			StartTime:      week2Start,
			ExcludeULID:    week2ULID,
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when companion is soft-deleted")
	})

	t.Run("returns nil when companion is outside 7–21 day window (too close)", func(t *testing.T) {
		// 3 days before — inside the 7-day exclusion zone.
		tooClose := week1Start.AddDate(0, 0, 3)
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        place.ID,
			StartTime:      tooClose,
			ExcludeULID:    "",
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when candidate is too close (< 7 days)")
	})

	t.Run("returns nil when companion is outside 7–21 day window (too far)", func(t *testing.T) {
		// 30 days after week-2 — both fixtures are now more than 21 days in the past.
		tooFar := week2Start.AddDate(0, 0, 30)
		result, err := repo.FindSeriesCompanion(ctx, events.SeriesCompanionQuery{
			NormalizedName: "jazz jam session",
			VenueID:        place.ID,
			StartTime:      tooFar,
			ExcludeULID:    "",
		})
		require.NoError(t, err)
		require.Nil(t, result, "expected nil when both fixtures are more than 21 days in the past")
	})
}

func TestFindSimilarOrganizationsNullableFieldsAreNilWhenAbsent(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool}

	orgULID := "01JORGTEST00000000002"
	_, err := pool.Exec(ctx, `
		INSERT INTO organizations (ulid, name, address_locality, address_region)
		VALUES ($1, $2, $3, $4)
	`, orgULID, "Toronto Jazz Society", "Toronto", "ON")
	require.NoError(t, err)

	candidates, err := repo.FindSimilarOrganizations(ctx, "Tor Jazz Society", "Toronto", "ON", 0.3)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	c := candidates[0]
	require.Nil(t, c.URL)
	require.Nil(t, c.Telephone)
	require.Nil(t, c.Email)
	require.NotNil(t, c.AddressLocality)
	require.NotNil(t, c.AddressRegion)
}
