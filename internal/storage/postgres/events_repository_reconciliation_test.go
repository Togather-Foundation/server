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
}

// TestGetOrCreateSourceReconciliation tests source reconciliation by base_url,
// including handling of NULL/empty base_url values
func TestGetOrCreateSourceReconciliation(t *testing.T) {
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

	t.Run("same base_url deduplicates regardless of name", func(t *testing.T) {
		source1, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "BACKROOM COMEDY CLUB Events",
			SourceType:  "api",
			BaseURL:     "https://www.eventbrite.ca",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		source2, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "DROM Taberna Events",
			SourceType:  "api",
			BaseURL:     "https://www.eventbrite.ca",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		require.Equal(t, source1, source2, "Same base_url should return same source ID regardless of name")
	})

	t.Run("empty base_url deduplicates by name", func(t *testing.T) {
		// Events with no URL should reconcile by base_url (NULL)
		source1, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "Toronto Open Data Events",
			SourceType:  "api",
			BaseURL:     "", // Empty base_url
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		source2, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "Toronto Open Data Events",
			SourceType:  "api",
			BaseURL:     "", // Empty base_url
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		require.Equal(t, source1, source2, "Empty base_url should deduplicate correctly")
	})

	t.Run("different base_url creates different sources", func(t *testing.T) {
		source1, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "Toronto Events",
			SourceType:  "api",
			BaseURL:     "https://www.toronto.ca",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		source2, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "Toronto Events",
			SourceType:  "api",
			BaseURL:     "https://www.rom.on.ca",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		require.NotEqual(t, source1, source2, "Different base_url should create separate sources")
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
