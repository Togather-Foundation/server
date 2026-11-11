package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"

	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
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
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	t.Run("same name same city deduplicates", func(t *testing.T) {
		place1, err := repo.UpsertPlace(ctx, createPlaceParams("DROM Taberna", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		place2, err := repo.UpsertPlace(ctx, createPlaceParams("Drom Taberna", "Toronto", "ON", "CA"))
		require.NoError(t, err)

		require.Equal(t, place1.ID, place2.ID, "Same place name in same city should reconcile")
		require.Equal(t, place1.ULID, place2.ULID)
	})
}

// TestGetOrCreateSourceReconciliation tests source reconciliation by
// normalized feed URL, including names that change and URL-less legacy callers.
func TestGetOrCreateSourceReconciliation(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	t.Run("different labels sharing a feed reuse one source", func(t *testing.T) {
		// All TPL BiblioCommons feeds share gateway.bibliocommons.com; each feed
		// must get its own provenance row.
		source1, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "tpl-events",
			SourceType:  "api",
			BaseURL:     "https://gateway.bibliocommons.com",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		source2, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
			Name:        "tpl-career-job-search",
			SourceType:  "api",
			BaseURL:     "https://gateway.bibliocommons.com",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		})
		require.NoError(t, err)

		require.Equal(t, source1, source2, "Names label the same feed; they do not create identity")

		var count int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM sources WHERE base_url = $1`,
			"https://gateway.bibliocommons.com",
		).Scan(&count), "count rows for base_url")
		require.Equal(t, 1, count, "two names on one feed must produce exactly one row")
	})

	t.Run("same base_url and name deduplicates", func(t *testing.T) {
		params := events.SourceLookupParams{
			Name:        "tpl-events",
			SourceType:  "api",
			BaseURL:     "https://gateway.bibliocommons.com",
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  5,
		}

		source1, err := repo.GetOrCreateSource(ctx, params)
		require.NoError(t, err)

		source2, err := repo.GetOrCreateSource(ctx, params)
		require.NoError(t, err)

		require.Equal(t, source1, source2, "Same (base_url, name) should return the same source ID")
	})

	t.Run("existing pair keeps its stored metadata", func(t *testing.T) {
		const baseURL = "https://metadata.example.com"
		params := events.SourceLookupParams{
			Name:        "metadata-feed",
			SourceType:  "api",
			BaseURL:     baseURL,
			LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
			LicenseType: "CC0",
			TrustLevel:  7,
		}

		first, err := repo.GetOrCreateSource(ctx, params)
		require.NoError(t, err)

		// A later lookup with different descriptive fields must not rewrite the row.
		params.SourceType = "scraper"
		params.LicenseURL = "https://creativecommons.org/licenses/by/4.0/"
		params.LicenseType = "CC-BY"
		params.TrustLevel = 2

		second, err := repo.GetOrCreateSource(ctx, params)
		require.NoError(t, err)
		require.Equal(t, first, second)

		var (
			gotName, gotSourceType, gotLicenseURL, gotLicenseType string
			gotTrust                                              int
		)
		require.NoError(t, pool.QueryRow(ctx, `
SELECT name, source_type, license_url, license_type, trust_level
  FROM sources
 WHERE id = $1
`, first).Scan(&gotName, &gotSourceType, &gotLicenseURL, &gotLicenseType, &gotTrust))

		require.Equal(t, "metadata-feed", gotName)
		require.Equal(t, "api", gotSourceType, "source_type must not be rewritten by a repeat lookup")
		require.Equal(t, "https://creativecommons.org/publicdomain/zero/1.0/", gotLicenseURL)
		require.Equal(t, "CC0", gotLicenseType)
		require.Equal(t, 7, gotTrust)
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

	t.Run("unnamed URL-based source shares feed identity", func(t *testing.T) {
		const feed = "https://unnamed.example.org/events"
		named, err := repo.GetOrCreateSource(ctx, sourceParams("named", feed, feed))
		require.NoError(t, err)
		neutral, err := repo.GetOrCreateSource(ctx, sourceParams("", feed, feed))
		require.NoError(t, err)
		require.Equal(t, named, neutral)
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

// TestCreateSourceUpsertOnReingest verifies that recording the same
// (event, source, url) attribution again refreshes retrieval metadata instead
// of failing on the UNIQUE constraint, while append-only columns stay put.
func TestCreateSourceUpsertOnReingest(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	var eventID string
	require.NoError(t, pool.QueryRow(ctx, `
INSERT INTO events (ulid, name, lifecycle_state, virtual_url)
VALUES ($1, 'Provenance Event', 'published', 'https://example.com/provenance-event')
RETURNING id
`, ulid.Make().String()).Scan(&eventID))

	sourceID, err := repo.GetOrCreateSource(ctx, events.SourceLookupParams{
		Name:        "upsert-feed",
		SourceType:  "api",
		BaseURL:     "https://upsert.example.com",
		LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseType: "CC0",
		TrustLevel:  5,
	})
	require.NoError(t, err)

	const sourceURL = "https://upsert.example.com/events/1"
	confidence := 0.4
	first := events.EventSourceCreateParams{
		EventID:       eventID,
		SourceID:      sourceID,
		SourceURL:     sourceURL,
		SourceEventID: "ext-1",
		Payload:       []byte(`{"version":1}`),
		PayloadHash:   "hash-1",
		Confidence:    &confidence,
	}
	require.NoError(t, repo.CreateSource(ctx, first))

	var firstRetrieved time.Time
	require.NoError(t, pool.QueryRow(ctx, `
SELECT retrieved_at
  FROM event_sources
 WHERE event_id = $1 AND source_id = $2 AND source_url = $3
`, eventID, sourceID, sourceURL).Scan(&firstRetrieved))

	secondConfidence := 0.9
	second := first
	second.SourceEventID = "ext-2"
	second.Payload = []byte(`{"version":2}`)
	second.PayloadHash = "hash-2"
	second.Confidence = &secondConfidence
	require.NoError(t, repo.CreateSource(ctx, second), "repeat attribution must be conflict-tolerant")

	var (
		payload       []byte
		payloadHash   string
		sourceEventID *string
		gotConfidence *float64
		retrievedAt   time.Time
	)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT payload, payload_hash, source_event_id, confidence::float8, retrieved_at
  FROM event_sources
 WHERE event_id = $1 AND source_id = $2 AND source_url = $3
`, eventID, sourceID, sourceURL).Scan(&payload, &payloadHash, &sourceEventID, &gotConfidence, &retrievedAt))

	var decoded map[string]int
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, 2, decoded["version"], "payload must be refreshed on re-ingest")
	require.Equal(t, "hash-2", payloadHash, "payload_hash must be refreshed on re-ingest")
	require.NotNil(t, sourceEventID)
	require.Equal(t, "ext-1", *sourceEventID, "source_event_id must not be rewritten")
	require.NotNil(t, gotConfidence)
	require.InDelta(t, 0.4, *gotConfidence, 1e-9, "confidence must not be rewritten")
	require.False(t, retrievedAt.Before(firstRetrieved), "retrieved_at must be refreshed")

	var count int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*)
  FROM event_sources
 WHERE event_id = $1 AND source_id = $2 AND source_url = $3
`, eventID, sourceID, sourceURL).Scan(&count))
	require.Equal(t, 1, count, "re-ingest must update in place, not duplicate the row")
}

// TestUpsertOrganizationReconciliation tests organizations reconciliation
func TestUpsertOrganizationReconciliation(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

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
