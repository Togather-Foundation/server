package postgres

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func createPlaceParams(name, city, region, country string) events.PlaceCreateParams {
	return events.PlaceCreateParams{
		EntityCreateFields: events.EntityCreateFields{
			ULID:            ulid.Make().String(),
			Name:            name,
			AddressLocality: city,
			AddressRegion:   region,
			AddressCountry:  country,
		},
	}
}

func createOrgParams(name, city, region, country string) events.OrganizationCreateParams {
	return events.OrganizationCreateParams{
		EntityCreateFields: events.EntityCreateFields{
			ULID:            ulid.Make().String(),
			Name:            name,
			AddressLocality: city,
			AddressRegion:   region,
			AddressCountry:  country,
		},
	}
}

// TestUpsertPlaceReconciliation tests that places with the same normalized name
// in the same location are reconciled (deduplicated) correctly
func TestUpsertPlaceReconciliation(t *testing.T) {
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

	t.Run("same name same city deduplicates", func(t *testing.T) {
		place1, err := repo.UpsertPlace(ctx, createPlaceParams("DROM Taberna", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		place2, err := repo.UpsertPlace(ctx, createPlaceParams("Drom Taberna", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, place1.ID, place2.ID, "Same place name in same city should reconcile")
		require.Equal(t, place1.ULID, place2.ULID)
	})

	t.Run("ampersand vs and deduplicates", func(t *testing.T) {
		place1, err := repo.UpsertPlace(ctx, createPlaceParams("Studio & Gallery", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		place2, err := repo.UpsertPlace(ctx, createPlaceParams("Studio and Gallery", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, place1.ID, place2.ID, "& and 'and' should normalize to same entity")
		require.Equal(t, place1.ULID, place2.ULID)
	})

	t.Run("punctuation differences deduplicate", func(t *testing.T) {
		place1, err := repo.UpsertPlace(ctx, createPlaceParams("Café-Bar", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		place2, err := repo.UpsertPlace(ctx, createPlaceParams("Café Bar", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, place1.ID, place2.ID, "Punctuation differences should normalize")
		require.Equal(t, place1.ULID, place2.ULID)
	})

	t.Run("whitespace differences deduplicate", func(t *testing.T) {
		place1, err := repo.UpsertPlace(ctx, createPlaceParams("The   Art    Space", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		place2, err := repo.UpsertPlace(ctx, createPlaceParams("The Art Space", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, place1.ID, place2.ID, "Whitespace differences should normalize")
		require.Equal(t, place1.ULID, place2.ULID)
	})

	t.Run("same name different cities creates separate entities", func(t *testing.T) {
		place1, err := repo.UpsertPlace(ctx, createPlaceParams("City Theatre", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		place2, err := repo.UpsertPlace(ctx, createPlaceParams("City Theatre", "Montreal", "QC", "CA"))
		require.NoError(t, err)

		require.NotEqual(t, place1.ID, place2.ID, "Same name in different cities should create separate entities")
		require.NotEqual(t, place1.ULID, place2.ULID)
	})
}

// TestUpsertOrganizationReconciliation tests organizations reconciliation
func TestUpsertOrganizationReconciliation(t *testing.T) {
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

	t.Run("same name same city deduplicates", func(t *testing.T) {
		org1, err := repo.UpsertOrganization(ctx, createOrgParams("City of Toronto", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		org2, err := repo.UpsertOrganization(ctx, createOrgParams("CITY OF TORONTO", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, org1.ID, org2.ID, "Same org name in same city should reconcile")
		require.Equal(t, org1.ULID, org2.ULID)
	})

	t.Run("ampersand vs and deduplicates", func(t *testing.T) {
		org1, err := repo.UpsertOrganization(ctx, createOrgParams("Arts & Culture Co", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		org2, err := repo.UpsertOrganization(ctx, createOrgParams("Arts and Culture Co", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, org1.ID, org2.ID, "& and 'and' should normalize to same entity")
		require.Equal(t, org1.ULID, org2.ULID)
	})

	t.Run("same name different cities creates separate entities", func(t *testing.T) {
		org1, err := repo.UpsertOrganization(ctx, createOrgParams("Community Centre", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		org2, err := repo.UpsertOrganization(ctx, createOrgParams("Community Centre", "Vancouver", "BC", "CA"))
		require.NoError(t, err)

		require.NotEqual(t, org1.ID, org2.ID, "Same name in different cities should create separate entities")
		require.NotEqual(t, org1.ULID, org2.ULID)
	})

	t.Run("multiple variants reconcile to first entity", func(t *testing.T) {
		firstOrg, err := repo.UpsertOrganization(ctx, createOrgParams("City of Toronto", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		variants := []string{
			"CITY OF TORONTO",
			"City Of Toronto",
			"city of toronto",
			"City  of  Toronto",
		}

		for _, variant := range variants {
			org, err := repo.UpsertOrganization(ctx, createOrgParams(variant, "Toronto", "ON", "CA"))
			require.NoError(t, err)
			require.Equal(t, firstOrg.ID, org.ID, "Variant %q should reconcile to first entity", variant)
			require.Equal(t, firstOrg.ULID, org.ULID)
		}
	})
}
